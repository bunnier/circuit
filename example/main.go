package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bunnier/circuit"
)

/**
* 功能函数签名：CommandFunc，该类型注释有详细说明。
* 降级函数（可选）签名：CommandFallbackFunc，该类型注释有详细说明。
* 更多用法也可参考command_test.go中的测试用例。
 */
func main() {
	// 功能函数，简单的通过参数true/false来控制成功失败。
	run := func(ctx context.Context, param interface{}) (interface{}, error) {
		if success := param.(bool); !success {
			return nil, errors.New("error")
		}
		return "ok", nil
	}

	// 降级函数（可选）。
	fallback := func(ctx context.Context, param interface{}, e error) (interface{}, error) {
		return "fallback", nil
	}

	// 初始化Command。
	// 默认参数5s内20次以上，50%失败率后开启熔断器。
	command := circuit.NewCommand(
		"test", run,
		// 默认为CutBreaker，这里可通过选项函数切换为SreBreaker，后面的例子使用CutBreaker，所以下面这行注释掉。
		// circuit.WithCommandBreaker(breaker.NewSreBreaker("test")),
		circuit.WithCommandFallback(fallback),
		circuit.WithCommandTimeout(time.Second*5))

	defer command.Close() // 主要用于释放command中开启的统计goroutine。

	var wg sync.WaitGroup

	// 模拟20次请求，10个成功，10个失败，让其刚好到临界。
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func(res bool) {
			command.Execute(res)
			wg.Done()
		}(i%2 == 0)
	}
	wg.Wait()

	// 窗口期内再来一个错误请求，开启熔断器。
	res, _ := command.Execute(false)
	fmt.Printf("step1: %s\n", res) // fallback。

	// 开启熔断器后再模拟10并发个请求，都会直接走降级函数。
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			res, _ = command.Execute(true)
			fmt.Printf("step2: %s\n", res) // fallback。
			wg.Done()
		}()
	}
	wg.Wait()

	// 熔断器开启5s后进入半开状态。
	time.Sleep(5 * time.Second)

	// 默认使用“一刀切”的恢复算法，半开状态下，只能有一个请求进入尝试，通过就重置统计，不通过重新完全开启熔断器。
	// 这里模拟一个不通过的请求，将重新开启熔断器。
	_, _ = command.Execute(false)
	res, _ = command.Execute(true)
	fmt.Printf("step3: %s\n", res) // fallback。

	// 再次休息5s，再次进入半开状态。
	time.Sleep(5 * time.Second)

	// 半开状态时候请求成功，将重置统计。
	_, _ = command.Execute(true)

	// 重置后模拟10个并发成功请求，都被会执行～
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			res, _ = command.Execute(true)
			fmt.Printf("step4: %s\n", res) // ok
			wg.Done()
		}()
	}
	wg.Wait()
}
