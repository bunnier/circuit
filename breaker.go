package circuit

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bunnier/circuit/internal"
)

// 这里本不需要用int32，为了放到CAS方法中使用，使用int32。
const (
	Closed      int32 = 0 // 熔断关闭。
	Openning    int32 = 1 // 熔断开启。
	HalfOpening int32 = 2 // 半熔断状态。
)

// Breaker 是熔断器结构。
type Breaker struct {
	ctx context.Context // 用于释放资源的context。

	name   string           // 名称。
	metric *internal.Metric // 执行情况统计数据。

	internalStatus int32 // 熔断器的内部状态，内部维护3个状态。

	minRequestThreshold      int64         // 熔断器生效必须满足的最小流量。
	errorThresholdPercentage float64       // 开启熔断的错误百分比阈值。
	sleepWindow              time.Duration // 熔断后重置熔断器的时间窗口。
	timeWindow               time.Duration // 滑动窗口的大小（单位秒1-60）。
}

// NewBreaker 用于新建一个熔断器。
func NewBreaker(name string, options ...BreakerOption) *Breaker {
	breaker := &Breaker{
		ctx:                      context.Background(),
		name:                     name,
		internalStatus:           Closed, // 默认关闭。
		minRequestThreshold:      20,     // 默认20个请求起算。
		errorThresholdPercentage: 50,     // 默认50%。
		sleepWindow:              time.Second * 5,
		timeWindow:               5,
	}

	for _, option := range options {
		option(breaker)
	}

	// 初始化选项后，根据选项初始化Metric。
	breaker.metric = internal.NewMetric(internal.WithMetricCounterSize(breaker.timeWindow))

	return breaker
}

// HealthSummary 返回当前健康状态。
func (breaker *Breaker) HealthSummary() *internal.HealthSummary {
	return breaker.metric.GetHealthSummary()
}

// IsOpen 判断当前熔断器是否打开。
func (breaker *Breaker) IsOpen() (bool, string) {
	healthSummary := breaker.metric.GetHealthSummary() // 当前健康统计。

	switch breaker.internalStatus {
	case Closed:
		// 没有满足最小流量要求 或 没有到达错误百分比阈值。
		if healthSummary.Total < breaker.minRequestThreshold ||
			healthSummary.ErrorPercentage < breaker.errorThresholdPercentage {
			return false, "closed"
		}
		// 开启熔断器，Closed应该不会马上变化为除Open外的其它状态，不过安全起见，还是通过CAS赋值把。
		atomic.CompareAndSwapInt32(&breaker.internalStatus, Closed, Openning)
		return true, "open" // 无论上面结果如何，都开启。

	case HalfOpening:
		return true, "half-open" // 半开状态，说明已经有一个请求正在尝试，拒绝所有其它请求。

	case Openning:
		// 判断是否已经达到熔断时间。
		if time.Since(healthSummary.LastExecuteTime) < breaker.sleepWindow {
			return true, "open"
		}
		// 过了休眠时间，设置为半开状态，并放一个请求试试。
		// 这里可能并发，用个CAS控制，换不到的还是开启，换到的就关闭一次。
		return !atomic.CompareAndSwapInt32(&breaker.internalStatus, Openning, HalfOpening), "half-open"

	default:
		panic("breaker: impossible status")
	}
}

// Success 用于记录成功信息。
func (breaker *Breaker) Success() {
	if breaker.internalStatus == HalfOpening {
		breaker.metric.Reset() // 注意：这里需要先Reset metric再改状态，否则会有并发问题。
		// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
		atomic.CompareAndSwapInt32(&breaker.internalStatus, HalfOpening, Closed)
		return
	}
	breaker.metric.Success()
}

// Failure 用于记录失败信息。
func (breaker *Breaker) Failure() {
	// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
	if atomic.CompareAndSwapInt32(&breaker.internalStatus, HalfOpening, Openning) {
		return
	}
	breaker.metric.Failure()
}

// Timeout 用于记录失败信息。
func (breaker *Breaker) Timeout() {
	// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
	if atomic.CompareAndSwapInt32(&breaker.internalStatus, HalfOpening, Openning) {
		return
	}
	breaker.metric.Timeout()
}

// FallbackSuccess 记录一次降级函数执行成功事件。
func (breaker *Breaker) FallbackSuccess() {
	breaker.metric.FallbackSuccess()
}

// FallbackFailure 记录一次降级函数执行失败事件。
func (breaker *Breaker) FallbackFailure() {
	breaker.metric.FallbackSuccess()
}

// BreakerOption 是Breaker的可选项。
type BreakerOption func(breaker *Breaker)

// WithBreakerMinRequestThreshold 设置熔断器生效必须满足的最小流量。
func WithBreakerMinRequestThreshold(minRequestThreshold int64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.minRequestThreshold = minRequestThreshold
	}
}

// WithBreakderMinRequestThreshold 设置熔断器生效必须满足的最小流量。
func WithBreakerErrorThresholdPercentage(errorThresholdPercentage float64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.errorThresholdPercentage = errorThresholdPercentage
	}
}

// WithBreakderMinRequestThreshold 设置熔断后重置熔断器的时间窗口。
func WithBreakerSleepWindow(sleepWindow time.Duration) BreakerOption {
	return func(breaker *Breaker) {
		breaker.sleepWindow = sleepWindow
	}
}

// WithBreakderMinRequestThreshold 设置滑动窗口的大小（要求1-60s）。
func WithBreakerCounterSize(timeWindow time.Duration) BreakerOption {
	if timeWindow < time.Second || timeWindow > time.Minute {
		panic("breaker: timeWindow invalid") // 窗口大小错误属于无法恢复的错误，直接panic把。
	}
	return func(breaker *Breaker) {
		breaker.timeWindow = timeWindow
	}
}

// WithBreakerContext 设置用于释放资源的context。
func WithBreakerContext(ctx context.Context) BreakerOption {
	return func(breaker *Breaker) {
		breaker.ctx = ctx
	}
}
