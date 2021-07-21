package circuit

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/bunnier/circuit/breaker"
)

type CommandFunc func(context.Context, interface{}) (interface{}, error) // 功能函数签名。
type CommandFallbackFunc func(interface{}, error) (interface{}, error)   // 降级函数签名。

var ErrTimeout error = errors.New("command: timeout")      // 服务执行超时。
var ErrFallback error = errors.New("command: unavailable") // 服务不可用（熔断器开启后返回）。

// 在断路器中执行的命令对象。
type Command struct {
	cancel context.CancelFunc // 用于释放内部的goroutine。

	name string // 名称。

	run      CommandFunc         // 功能函数。
	fallback CommandFallbackFunc // 降级函数。

	timeout time.Duration // 超时时间。

	breaker breaker.Breaker // 熔断器。
}

func NewCommand(name string, run CommandFunc, options ...CommandOptionFunc) *Command {
	ctx, cancel := context.WithCancel(context.Background()) // 这个context主要用于处理内部的资源释放，而非执行功能函数。

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
	command.run = wrapExecuteWithTimeoutFucn(command, run)

	return command
}

// wrapExecuteWithTimeoutFucn 用于对功能函数包装超时处理。
func wrapExecuteWithTimeoutFucn(command *Command, run CommandFunc) CommandFunc {
	return func(ctx context.Context, param interface{}) (interface{}, error) {
		type resType struct {
			res interface{}
			err error
		}

		resCh := make(chan resType, 1)       // 设置一个1的缓冲，以免超时后goroutine泄漏。
		panicCh := make(chan interface{}, 1) // 由于放到独立的goroutine中，原本的panic保护会失效，这里做个panic转发，让其回归到原本的goroutine中。

		ctx, cancel := context.WithTimeout(ctx, command.timeout) // 为context加上统一的超时时间。
		defer cancel()

		go func() {
			defer func() {
				if err := recover(); err != nil {
					panicCh <- err
				}
			}()

			res, err := run(ctx, param)
			resCh <- resType{res, err}
		}()

		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.DeadlineExceeded) {
				command.breaker.Timeout()
				return nil, fmt.Errorf("%s: %w", command.name, ErrTimeout)
			}
			command.breaker.Failure()
			return nil, fmt.Errorf("%s: %w", command.name, ctx.Err())
		case err := <-panicCh:
			command.breaker.Failure()
			panic(err) // 接收goroutine转发过来的panic。
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
func (command *Command) executeFallback(param interface{}, err error) (interface{}, error) {
	if result, err := command.fallback(param, err); err != nil {
		command.breaker.FallbackFailure()
		return result, err
	} else {
		command.breaker.FallbackSuccess()
		return result, nil
	}
}

// Execute 用于直接执行目标函数。
func (command *Command) Execute(param interface{}) (interface{}, error) {
	return command.ContextExecute(context.Background(), param)
}

// Execute 用于直接执行目标函数。
func (command *Command) ContextExecute(ctx context.Context, param interface{}) (interface{}, error) {
	pass, statusMsg := command.breaker.Allow()

	// 已经熔断走降级逻辑。
	if !pass {
		openErr := fmt.Errorf("%s: %s: %w", command.name, statusMsg, ErrFallback)
		if command.fallback == nil { // 没有设置降级函数直接返回
			return nil, openErr
		}
		return command.executeFallback(param, openErr) // 降级函数。
	}

	if result, err := command.run(ctx, param); err != nil {
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
