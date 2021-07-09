package circuit

import (
	"sync/atomic"
	"time"
	"unsafe"
)

// BreakerStatus 是熔断器状态。
type BreakerStatus int8

const (
	Closed      BreakerStatus = 0    // 熔断关闭。
	Openning    BreakerStatus = iota // 熔断开启。
	HalfOpening BreakerStatus = iota // 半熔断状态。
)

// 由于常量无法取地址，这里设置几个和上面一致的变量。
var (
	closed      BreakerStatus = Closed      // 熔断关闭。
	openning    BreakerStatus = Openning    // 熔断开启。
	halfOpening BreakerStatus = HalfOpening // 半熔断状态。
)

// Breaker 是熔断器结构。
type Breaker struct {
	name   string  // 名称。
	metric *Metric // 执行情况统计数据。

	status BreakerStatus // 最后一次运行后的状态。

	minRequestThreshold      int64         // 熔断器生效必须满足的最小流量。
	errorThresholdPercentage float64       // 开启熔断的错误百分比阈值。
	sleepWindow              time.Duration // 熔断后重置熔断器的时间窗口。
	timeWindow               time.Duration // 滑动窗口的大小（单位秒1-60）。
}

// NewBreaker 用于新建一个熔断器。
func NewBreaker(name string, options ...BreakerOption) *Breaker {
	breaker := &Breaker{
		name:                     name,
		status:                   Closed, // 默认关闭。
		minRequestThreshold:      20,     // 默认20个请求起算。
		errorThresholdPercentage: 50,     // 默认50%。
		sleepWindow:              time.Second * 5,
		timeWindow:               5,
	}

	for _, option := range options {
		option(breaker)
	}

	// 初始化选项后，根据选项初始化Metric。
	breaker.metric = newMetric(WithMetricCounterSize(breaker.timeWindow))

	return breaker
}

// Metric 返回本Breaker所使用的Metric。
func (breaker *Breaker) Metric() *Metric {
	return breaker.metric
}

// IsOpen 判断当前熔断器是否打开。
func (breaker *Breaker) IsOpen() bool {
	healthSummary := breaker.metric.GetHealthSummary() // 当前健康统计。

	switch breaker.status {
	case Closed:
		// 没有满足最小流量要求 或 没有到达错误百分比阈值。
		if healthSummary.Total < breaker.minRequestThreshold ||
			healthSummary.ErrorPercentage < breaker.errorThresholdPercentage {
			return false
		}
		// 开启熔断器，Closed应该不会马上变化为其它状态，不过安全起见，还是通过CAS赋值把。
		status := &breaker.status
		atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&status)),
			unsafe.Pointer(&closed),
			unsafe.Pointer(&openning))
		return true

	case HalfOpening:
		return true // 半开状态，说明已经有一个请求正在尝试，拒绝所有其它请求。

	case Openning:
		// 判断是否已经达到熔断时间。
		if time.Since(healthSummary.lastExecuteTime) < breaker.sleepWindow {
			return true
		}
		// 过了休眠时间，设置为半开状态，并放一个请求试试。
		// 这里可能并发，用个CAS控制，换不到的还是开启，换到的就关闭一次。
		status := &breaker.status
		return !atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&status)),
			unsafe.Pointer(&openning),
			unsafe.Pointer(&halfOpening))

	default:
		panic("breaker: impossible status")
	}
}

// Success 用于记录成功信息。
func (breaker *Breaker) Success() {
	if breaker.status == HalfOpening {
		breaker.metric.Reset() // 注意：这里需要先Reset metric再改状态，否则会有并发问题。
		// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
		status := &breaker.status
		atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&status)),
			unsafe.Pointer(&halfOpening),
			unsafe.Pointer(&closed))
		return
	}
	breaker.metric.Success()
}

// Failure 用于记录失败信息。
func (breaker *Breaker) Failure() {
	if breaker.status == HalfOpening {
		// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
		status := &breaker.status
		atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&status)),
			unsafe.Pointer(&halfOpening),
			unsafe.Pointer(&openning))
		return
	}
	breaker.metric.Failure()
}

// Timeout 用于记录失败信息。
func (breaker *Breaker) Timeout() {
	if breaker.status == HalfOpening {
		// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
		status := &breaker.status
		atomic.CompareAndSwapPointer(
			(*unsafe.Pointer)(unsafe.Pointer(&status)),
			unsafe.Pointer(&halfOpening),
			unsafe.Pointer(&openning))
		return
	}
	breaker.metric.Timeout()
}

// FallbackSuccess 记录一次失败回调执行成功事件。
func (breaker *Breaker) FallbackSuccess() {
	breaker.metric.FallbackSuccess()
}

// FallbackFailure 记录一次失败回调执行失败事件。
func (breaker *Breaker) FallbackFailure() {
	breaker.metric.FallbackSuccess()
}

// BreakerOption 是Breaker的可选项。
type BreakerOption func(breaker *Breaker)

// WithBreakderMinRequestThreshold 设置熔断器生效必须满足的最小流量。
func WithBreakderMinRequestThreshold(minRequestThreshold int64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.minRequestThreshold = minRequestThreshold
	}
}

// WithBreakderMinRequestThreshold 设置熔断器生效必须满足的最小流量。
func WithBreakderErrorThresholdPercentage(errorThresholdPercentage float64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.errorThresholdPercentage = errorThresholdPercentage
	}
}

// WithBreakderMinRequestThreshold 设置熔断后重置熔断器的时间窗口。
func WithBreakderSleepWindow(sleepWindow time.Duration) BreakerOption {
	return func(breaker *Breaker) {
		breaker.sleepWindow = sleepWindow
	}
}

// WithBreakderMinRequestThreshold 设置滑动窗口的大小（要求1-60s）。
func WithBreakderCounterSize(timeWindow time.Duration) BreakerOption {
	if timeWindow < time.Second || timeWindow > time.Minute {
		panic("breaker: timeWindow invalid") // 窗口大小错误属于无法恢复的错误，直接panic把。
	}
	return func(breaker *Breaker) {
		breaker.timeWindow = timeWindow
	}
}
