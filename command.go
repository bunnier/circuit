package circuit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bunnier/circuit/breaker"
)

type CommandFunc func([]interface{}) ([]interface{}, error)                // 功能函数签名。
type CommandFallbackFunc func([]interface{}, error) ([]interface{}, error) // 降级函数签名。

// 在断路器中执行的命令对象。
type Command struct {
	cancel context.CancelFunc // 用于释放内部的goroutine。
	name   string             // 名称。

	run      CommandFunc         // 功能函数。
	fallback CommandFallbackFunc // 降级函数。

	timeout time.Duration // 超时时间。

	breaker breaker.Breaker // 熔断器。
}

func NewCommand(name string, run CommandFunc, options ...CommandOptionFunc) *Command {
	ctx, cancel := context.WithCancel(context.Background())

	command := &Command{
		cancel:  cancel,
		name:    name,
		timeout: time.Second * 10, // 默认超时10s。
	}

	for _, option := range options {
		option(command)
	}

	// breaker对象比较大，就不在前面设置默认值了。
	if command.breaker == nil {
		command.breaker = breaker.NewCutBreaker(name,
			breaker.WithCutBreakerContext(ctx),
			breaker.WithCutBreakerTimeWindow(5*time.Second),
			breaker.WithCutBreakerErrorThresholdPercentage(50),
			breaker.WithCutBreakerMinRequestThreshold(10),
			breaker.WithCutBreakerSleepWindow(5*time.Second))
	}

	// 对run方法包装一层超时处理，由于需要用到参数，在其它参数处理后调用。
	command.run = executeWithTimeout(command, run)

	return command
}

// executeWithTimeout 用于对功能函数包装超时处理。
func executeWithTimeout(command *Command, run CommandFunc) CommandFunc {
	return func(param []interface{}) ([]interface{}, error) {
		type resType struct {
			res []interface{}
			err error
		}

		panicCh := make(chan interface{}, 1) // 由于放到独立的goroutine中，原本的panic保护会失效，这里做个panic转发，让其回归到原本的goroutine中。
		resCh := make(chan resType, 1)       // 设置一个1的缓冲，以免超时后goroutine泄漏。
		go func() {
			defer func() {
				if err := recover(); err != nil {
					panicCh <- err
				}
			}()

			res, err := run(param)
			resCh <- resType{res, err}
		}()

		select {
		case <-time.After(command.timeout):
			command.breaker.Timeout()
			return nil, errors.New("command: timeout")
		case err := <-panicCh:
			panic(err) // 转发panic。
		case res := <-resCh:
			if res.err != nil {
				command.breaker.Failure()
			} else {
				command.breaker.Success()
			}
			return res.res, res.err
		}
	}
}

// executeFallback 用于执行降级函数。
func (command *Command) executeFallback(params []interface{}, err error) ([]interface{}, error) {
	if result, err := command.fallback(params, err); err != nil {
		command.breaker.FallbackFailure()
		return result, err
	} else {
		command.breaker.FallbackSuccess()
		return result, nil
	}
}

// Execute 用于直接执行目标函数。
func (command *Command) Execute(params []interface{}) ([]interface{}, error) {
	pass, statusMsg := command.breaker.Allow()

	// 已经熔断走降级逻辑。
	if !pass {
		openErr := fmt.Errorf("breaker: %s", statusMsg)
		if command.fallback == nil { // 没有设置降级函数直接返回
			return nil, openErr
		}
		return command.executeFallback(params, openErr) // 降级函数。
	}

	if result, err := command.run(params); err != nil {
		if command.fallback == nil { // 没有设置降级函数直接返回
			return nil, err
		}
		return command.executeFallback(result, err) // 降级函数。
	} else {
		return result, nil
	}
}

// Close 用于释放整个Command对象内部资源（）。
func (command *Command) Close() {
	command.cancel()
}

type CommandOptionFunc func(*Command)

// WithCommandBreaker 用于为Command设置熔断器。
func WithCommandBreaker(breaker breaker.Breaker) CommandOptionFunc {
	return func(c *Command) {
		c.breaker = breaker
	}
}

// WithCommandBreaker 用于为Command设置默认超时。
func WithCommandTimeout(timeout time.Duration) CommandOptionFunc {
	return func(c *Command) {
		c.timeout = timeout
	}
}

// WithCommandBreaker 用于为Command设置降级函数。
func WithCommandFallback(fallback CommandFallbackFunc) CommandOptionFunc {
	return func(c *Command) {
		c.fallback = fallback
	}
}
