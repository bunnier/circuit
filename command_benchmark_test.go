package circuit

import (
	"testing"
)

// run 原始的功能函数。
var run = func() string {
	return "ok"
}

// wrapRun 是包装后用于 command 用的功能函数。
var wrapRun = func(param interface{}) (interface{}, error) {
	return run(), nil
}

func BenchmarkDirectly(b *testing.B) {
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		run()
	}
	b.StopTimer()
}

func BenchmarkCommand(b *testing.B) {
	command := NewCommand("test", wrapRun)
	defer command.Close()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		command.Execute(nil)
	}
	b.StopTimer()
}

func BenchmarkParallelDirectly(b *testing.B) {
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			run()
		}
	})
}

func BenchmarkParallelCommand(b *testing.B) {
	command := NewCommand("test", wrapRun)
	defer command.Close()
	b.ResetTimer()
	b.RunParallel(func(p *testing.PB) {
		for p.Next() {
			command.Execute(nil)
		}
	})
	b.StopTimer()
}
