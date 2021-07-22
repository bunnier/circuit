package circuit

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestCommand_workflow(t *testing.T) {
	// 功能函数。
	run := func(ctx context.Context, i interface{}) (interface{}, error) {
		param := i.(int)
		param++
		if param > 5000 {
			return nil, errors.New("more then 5000")
		}
		return param, nil
	}
	// 降级函数。
	fallback := func(ctx context.Context, i interface{}, e error) (interface{}, error) {
		return nil, fmt.Errorf("fallback: %w", e)
	}
	// 初始化Command。
	command := NewCommand("test", run, WithCommandFallback(fallback))
	defer command.Close()

	for i := 0; i < 10000; i++ {
		r, err := command.Execute(i)

		// 前5000正常。
		if i < 5000 {
			if err != nil {
				t.Errorf("Command.Execute() got = %v, wantErr %v", err, nil)
			}
			if r.(int) != i+1 {
				t.Errorf("Command.Execute() got = %v, want %v", r, i+1)
			}
			continue
		}

		// 后5000错误。
		if err == nil || err.Error() != "fallback: more then 5000" {
			t.Errorf("Command.Execute() got = %v, wantErr %v", err, "fallback: more then 5000")
		}
	}

	// 再一个熔断。
	if _, err := command.Execute(10001); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	// 熔断中，正常的也熔断。
	if _, err := command.Execute(1); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个错误的。
	if _, err := command.Execute(10001); err == nil || err.Error() != "fallback: more then 5000" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: more then 5000")
	}
	// 由于刚放了个错误的进行半熔断测试，又恢复熔断了。
	if _, err := command.Execute(1); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个正常的。
	if _, err := command.Execute(1); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
	// 恢复了。
	if _, err := command.Execute(2); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
}

// TestCommand_withtimeout_workflow 由于加入超时机制后，将放入独立的goroutine中运行，执行流程与原本有区别，故独立测试一份。
func TestCommand_withtimeout_workflow(t *testing.T) {
	// 功能函数。
	run := func(ctx context.Context, i interface{}) (interface{}, error) {
		param := i.(int)
		param++
		if param > 5000 {
			return nil, errors.New("more then 5000")
		}
		return param, nil
	}
	// 降级函数。
	fallback := func(ctx context.Context, i interface{}, e error) (interface{}, error) {
		return nil, fmt.Errorf("fallback: %w", e)
	}
	// 初始化Command。
	command := NewCommand("test", run, WithCommandFallback(fallback), WithCommandTimeout(time.Second*2))
	defer command.Close()

	for i := 0; i < 10000; i++ {
		r, err := command.Execute(i)

		// 前5000正常。
		if i < 5000 {
			if err != nil {
				t.Errorf("Command.Execute() got = %v, wantErr %v", err, nil)
			}
			if r.(int) != i+1 {
				t.Errorf("Command.Execute() got = %v, want %v", r, i+1)
			}
			continue
		}

		// 后5000错误。
		if err == nil || err.Error() != "fallback: more then 5000" {
			t.Errorf("Command.Execute() got = %v, wantErr %v", err, "fallback: more then 5000")
		}
	}

	// 再一个熔断。
	if _, err := command.Execute(10001); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	// 熔断中，正常的也熔断。
	if _, err := command.Execute(1); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个错误的。
	if _, err := command.Execute(10001); err == nil || err.Error() != "fallback: more then 5000" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: more then 5000")
	}
	// 由于刚放了个错误的进行半熔断测试，又恢复熔断了。
	if _, err := command.Execute(1); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个正常的。
	if _, err := command.Execute(1); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
	// 恢复了。
	if _, err := command.Execute(2); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
}

func TestCommand_timeout(t *testing.T) {
	// 功能函数。
	run := func(ctx context.Context, i interface{}) (interface{}, error) {
		time.Sleep(time.Second * time.Duration(i.(int)))
		return i, nil
	}

	// 初始化Command。
	command := NewCommand("test", run,
		WithCommandTimeout(time.Second*2))
	defer command.Close()

	// 还没超时。
	if _, err := command.Execute(1); err != nil {
		t.Errorf("Command.Execute() got = %v, want nil", err)
	}

	// 超过默认超时。
	if _, err := command.Execute(3); !errors.Is(err, ErrTimeout) {
		t.Errorf("Command.Execute() got = %v, want nil", err)
	}

	// 测试下传入的超时。
	startTime := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := command.ContextExecute(ctx, 2); !errors.Is(err, ErrTimeout) {
		t.Errorf("Command.ContextExecute() got = %v, want %v", err, ErrTimeout)
	}
	// 此时应该时间过去1秒左右，允许一点时差。
	if time.Since(startTime) > time.Second+time.Millisecond*100 {
		t.Errorf("Command.ContextExecute() got = %v, want less than %v", time.Since(startTime), time.Second+time.Millisecond*100)
	}
}

func TestCommand_fallback_timeout(t *testing.T) {
	// 功能函数。
	run := func(ctx context.Context, i interface{}) (interface{}, error) {
		return i, errors.New("must err")
	}
	// 降级函数。
	fallback := func(ctx context.Context, i interface{}, e error) (interface{}, error) {
		time.Sleep(time.Second * time.Duration(i.(int)))
		return i, nil
	}
	// 初始化Command。
	command := NewCommand("test", run,
		WithCommandFallback(fallback),
		WithCommandTimeout(time.Second*2))
	defer command.Close()

	// 还没超时。
	if _, err := command.Execute(1); err != nil {
		t.Errorf("Command.Execute() got = %v, want nil", err)
	}

	// 超过默认超时。
	if _, err := command.Execute(3); !errors.Is(err, ErrTimeout) {
		t.Errorf("Command.Execute() got = %v, want nil", err)
	}
}
