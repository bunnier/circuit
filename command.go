package circuit

import (
	"fmt"
	"time"
)

type CommandFunc func([]interface{}) ([]interface{}, error)                // 功能函数签名。
type CommandFallbackFunc func([]interface{}, error) ([]interface{}, error) // 降级函数签名。

// 在断路器中执行的命令对象。
type Command struct {
	name string // 名称。

	run      CommandFunc         // 功能函数。
	fallback CommandFallbackFunc // 降级函数。

	timeout time.Duration // 超时时间。

	breaker *Breaker // 熔断器。
}

func NewCommand(name string, run CommandFunc, options ...CommandOptionFunc) *Command {
	command := &Command{
		name:    name,
		run:     run,
		timeout: time.Second * 10, // 默认超时10s。
	}

	for _, option := range options {
		option(command)
	}

	// breaker对象比较大，就不在前面设置默认值了。
	if command.breaker == nil {
		command.breaker = NewBreaker(name,
			WithBreakerCounterSize(5*time.Second),
			WithBreakerErrorThresholdPercentage(50),
			WithBreakerMinRequestThreshold(10),
			WithBreakerSleepWindow(5*time.Second))
	}

	return command
}

// Execute 用于直接执行目标函数。
func (command *Command) Execute(params []interface{}) ([]interface{}, error) {
	isOpen, statusMsg := command.breaker.IsOpen()

	// 已经熔断走降级逻辑。
	if isOpen {
		openErr := fmt.Errorf("breaker: %s", statusMsg)
		if command.fallback == nil { // 没有设置降级函数直接返回
			return nil, openErr
		}
		return command.executeFallback(params, openErr) // 降级函数。
	}

	// 执行目标函数。
	if result, err := command.run(params); err != nil {
		return command.executeFallback(result, err) // 降级函数。
	} else {
		command.breaker.Success()
		return result, nil
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

type CommandOptionFunc func(*Command)

// WithCommandBreaker 用于为Command设置熔断器。
func WithCommandBreaker(breaker *Breaker) CommandOptionFunc {
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
