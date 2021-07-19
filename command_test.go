package circuit

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestCommand_workflow(t *testing.T) {
	// 功能函数。
	run := func(i []interface{}) ([]interface{}, error) {
		param := i[0].(int)
		param++
		if param > 5000 {
			return nil, errors.New("more then 5000")
		}
		return []interface{}{param}, nil
	}

	// 降级函数。
	fallback := func(i []interface{}, e error) ([]interface{}, error) {
		return nil, fmt.Errorf("fallback: %w", e)
	}

	// 初始化Command。
	command := NewCommand("test", run, WithCommandFallback(fallback))
	defer command.Close()

	for i := 0; i < 10000; i++ {
		r, err := command.Execute([]interface{}{i})

		// 前5000正常。
		if i < 5000 {
			if err != nil {
				t.Errorf("Command.Execute() got = %v, wantErr %v", err, nil)
			}
			if r[0] != i+1 {
				t.Errorf("Command.Execute() got = %v, want %v", r[0], i+1)
			}
			continue
		}

		// 后5000错误。
		if err == nil || err.Error() != "fallback: more then 5000" {
			t.Errorf("Command.Execute() got = %v, wantErr %v", err, "fallback: more then 5000")
		}
	}

	// 再一个熔断。
	if _, err := command.Execute([]interface{}{10001}); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	// 熔断中，正常的也熔断。
	if _, err := command.Execute([]interface{}{1}); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个错误的。
	if _, err := command.Execute([]interface{}{10001}); err == nil || err.Error() != "fallback: more then 5000" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: more then 5000")
	}
	// 由于刚放了个错误的进行半熔断测试，又恢复熔断了。
	if _, err := command.Execute([]interface{}{1}); err == nil || err.Error() != "fallback: test: open: command: unavailable" {
		t.Errorf("Command.Execute() got = %v, want %v", err, "fallback: test: open: command: unavailable")
	}

	time.Sleep(5 * time.Second)
	// 进入半熔断了，放入一个正常的。
	if _, err := command.Execute([]interface{}{1}); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
	// 恢复了。
	if _, err := command.Execute([]interface{}{2}); err != nil {
		t.Errorf("Command.Execute() got = %v, want %v", err, nil)
	}
}
