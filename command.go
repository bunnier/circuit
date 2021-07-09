package circuit

import "time"

type CommandFunc func([]interface{}) ([]interface{}, error)                // Command功能函数签名。
type CommandFallbackFunc func([]interface{}, error) ([]interface{}, error) // Command Fallback函数签名。

// 在断路器中执行的命令对象。
type Command struct {
	name string // 名称。

	run      CommandFunc         // 需要运行的功能函数。
	fallback CommandFallbackFunc // 失败后的善后/提供默认值函数。

	timeout time.Duration // 超时时间。

	breaker *Breaker // 熔断器。
}

func NewCommand(name string, run CommandFunc, options ...CommandOptionFunc) *Command {
	command := &Command{
		name:    name,
		timeout: time.Second * 10, // 默认超时。

		run: run,
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

func (command *Command) Execute(params []interface{}) error {
	// TODO
	return nil
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

// WithCommandBreaker 用于为Command设置失败后的善后/提供默认值函数。
func WithCommandFallback(fallback CommandFallbackFunc) CommandOptionFunc {
	return func(c *Command) {
		c.fallback = fallback
	}
}
