package main

import (
	"errors"
	"fmt"
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
	command := circuit.NewCommand(
		"test", run,
		circuit.WithCommandFallback(fallback),
		circuit.WithCommandTimeout(time.Second*5))

	// 默认20次，50%失败率后熔断。
	for i := 0; i < 10; i++ {
		command.Execute([]interface{}{true})
		command.Execute([]interface{}{false})
	}
	res, _ := command.Execute([]interface{}{false})
	fmt.Printf("step1: %s\n", res) // fallback。

	// 熔断后不再执行功能函数，直接请求降级函数，返回fallback。
	res, _ = command.Execute([]interface{}{true})
	fmt.Printf("step2: %s\n", res) // fallback。

	// 默认熔断后休息5s进入半熔断。
	time.Sleep(5 * time.Second)

	// 半熔断如果失败，再次熔断。
	_, _ = command.Execute([]interface{}{false})
	res, _ = command.Execute([]interface{}{true})
	fmt.Printf("step3: %s\n", res) // fallback。

	// 再次休息5s，再次进入半熔断。
	time.Sleep(5 * time.Second)

	// 半熔断只要成功一次，就恢复。
	_, _ = command.Execute([]interface{}{true})
	res, _ = command.Execute([]interface{}{true})
	fmt.Printf("step4: %s\n", res) // ok
}
