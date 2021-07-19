# circuit

[![Go](https://github.com/bunnier/circuit/actions/workflows/go.yml/badge.svg)](https://github.com/bunnier/circuit/actions/workflows/go.yml)

## 简介

提供能简单处理服务熔断逻辑的工具包。

本工具包主要通过`Command`对象交互，可通过`circuit.NewCommand` 函数获取一个 `Command`对象，通过该对象的`Execute`方法即可在熔断器上执行设置的功能函数。

初始化 `Command` 时可设置熔断器算法，熔断器的主接口为`Breaker`，包中目前内置了两个熔断器供选择：

- `CutBreaker`（默认）：提供了常规的断路器模式的熔断器实现。即维护 `Open`、`half-Open`、`Closed` 3个状态，错误率达到阈值后`Open`，`Open`后休眠指定时间转变为`half-Open`，之后允许一个请求探测，如恢复正常，则`Closed`，反之重新进入`Open`；
- `SreBreaker`：提供了Google SRE提出的Handling Overload算法实现的弹性熔断器。算法介绍参考：<https://sre.google/sre-book/handling-overload/#eq2101>；

## 文档

<https://pkg.go.dev/github.com/bunnier/circuit>

## DEMO

```go
package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/bunnier/circuit"
)

/**
* 功能函数签名：func([]interface{}) ([]interface{}, error)
* 降级函数（可无）签名：func([]interface{}, error) ([]interface{}, error)
* 参数/返回值中的[]interface{}，为功能函数预定的参数和返回。
* 通过返回值的error判断成功/失败。
 */
func main() {
	// 功能函数，简单的通过参数true/false来控制成功失败。
	run := func(i []interface{}) ([]interface{}, error) {
		if success := i[0].(bool); !success {
			return nil, errors.New("error")
		}
		return []interface{}{"ok"}, nil
	}

	// 降级函数，固定返回fallback。
	fallback := func(i []interface{}, e error) ([]interface{}, error) {
		return []interface{}{"fallback"}, nil
	}

	// 初始化Command。
	// 默认参数5s内20次以上，50%失败率后开启熔断器。
	command := circuit.NewCommand(
		"test", run,
		circuit.WithCommandFallback(fallback),
		circuit.WithCommandTimeout(time.Second*5))

	defer command.Close() // 主要用于释放command中开启的统计goroutine。

	var wg sync.WaitGroup

	// 模拟20次请求，10个成功，10个失败，让其刚好到临界。
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func(res bool) {
			command.Execute([]interface{}{res})
			wg.Done()
		}(i%2 == 0)
	}
	wg.Wait()

	// 窗口期内再来一个错误请求，开启熔断器。
	res, _ := command.Execute([]interface{}{false})
	fmt.Printf("step1: %s\n", res) // fallback。

	// 开启熔断器后再模拟10并发个请求，都会直接走降级函数。
	wg.Add(10)
	for i := 0; i < 10; i++ {
		go func() {
			res, _ = command.Execute([]interface{}{true})
			fmt.Printf("step2: %s\n", res) // fallback。
			wg.Done()
		}()
	}
	wg.Wait()

	// 熔断器开启5s后进入半开状态。
	time.Sleep(5 * time.Second)

	// 默认使用“一刀切”的恢复算法（另外提供了Google SRE中提到的概率熔断算法可以在初始化时候切换），半开状态下，只能有一个请求进入尝试，通过就重置统计，不通过重新完全开启熔断器。
	// 这里模拟一个不通过的请求，将重新开启熔断器。
	_, _ = command.Execute([]interface{}{false})
	res, _ = command.Execute([]interface{}{true})
	fmt.Printf("step3: %s\n", res) // fallback。

	// 再次休息5s，再次进入半开状态。
	time.Sleep(5 * time.Second)

	// 半开状态时候请求成功，将重置统计。
	_, _ = command.Execute([]interface{}{true})

	// 重置后模拟10个并发成功请求，都被会执行～
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			res, _ = command.Execute([]interface{}{true})
			fmt.Printf("step4: %s\n", res) // ok
			wg.Done()
		}()
	}
	wg.Wait()
}
```

## 下一步

- 提供一个通过反射包装普通函数为 Command 需要的功能函数的工具函数;
- 支持限流功能;
- 提供订阅状态变化的hook;
- 提供状态观察接口（接入hystrix-dashboard？）;