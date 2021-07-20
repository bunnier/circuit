package breaker

import (
	"context"
	"sync/atomic"
	"time"

	"github.com/bunnier/circuit/breaker/internal"
)

var _ Breaker = (*CutBreaker)(nil)

// CutBreaker 是 Breaker 的一种实现。
type CutBreaker struct {
	ctx context.Context // 用于释放资源的context。

	name   string           // 名称。
	metric *internal.Metric // 执行情况统计数据。

	internalStatus int32 // 熔断器的内部状态，内部维护3个状态。

	minRequestThreshold      int64         // 熔断器生效必须满足的最小流量。
	errorThresholdPercentage float64       // 开启熔断的错误百分比阈值。
	sleepWindow              time.Duration // 熔断后重置熔断器的时间窗口。
	timeWindow               time.Duration // 滑动窗口的大小（单位秒1-60）。
}

// NewCutBreaker 用于新建一个 CutBreaker 熔断器。
// CutBreaker 提供一个“一刀切”的恢复算法。
// 算法特点：内部维护开启、关闭、半开 三个状态，半开状态时只能有一个请求进入尝试，通过就重置统计，不通过重新完全开启熔断器。
func NewCutBreaker(name string, options ...CutBreakerOption) *CutBreaker {
	b := &CutBreaker{
		ctx:                      context.Background(),
		name:                     name,
		internalStatus:           Closed, // 默认关闭。
		minRequestThreshold:      20,     // 默认20个请求起算。
		errorThresholdPercentage: 50,     // 默认50%。
		sleepWindow:              time.Second * 5,
		timeWindow:               5,
	}

	for _, option := range options {
		option(b)
	}

	// 初始化选项后，根据选项初始化Metric。
	b.metric = internal.NewMetric(
		internal.WithMetricTimeWindow(b.timeWindow),
		internal.WithMetricContext(b.ctx),
	)

	return b
}

// Allow 用于判断断路器是否允许通过请求。
// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
func (b *CutBreaker) Allow() (bool, string) {
	summary := b.metric.Summary() // 当前健康统计。
	return b.allow(summary)
}

// allow 用于判断断路器是否允许通过请求。
// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
func (b *CutBreaker) allow(summary *internal.MetricSummary) (bool, string) {
	switch b.internalStatus {
	case Closed:
		// 没有满足最小流量要求 或 没有到达错误百分比阈值。
		if summary.Total < b.minRequestThreshold ||
			summary.ErrorPercentage < b.errorThresholdPercentage {
			return true, "closed"
		}
		// 开启熔断器，Closed应该不会马上变化为除Open外的其它状态，不过安全起见，还是通过CAS赋值把。
		atomic.CompareAndSwapInt32(&b.internalStatus, Closed, Openning)
		return false, "open" // 无论上面结果如何，都开启。

	case HalfOpening:
		return false, "half-open" // 半开状态，说明已经有一个请求正在尝试，拒绝所有其它请求。

	case Openning:
		// 判断是否已经达到熔断时间。
		if time.Since(summary.LastExecuteTime) < b.sleepWindow {
			return false, "open"
		}
		// 过了休眠时间，设置为半开状态，并放一个请求试试。
		// 这里可能并发，用个CAS控制，换不到的还是开启，换到的就关闭一次。
		return atomic.CompareAndSwapInt32(&b.internalStatus, Openning, HalfOpening), "half-open"

	default:
		panic("breaker: impossible status")
	}
}

// Success 用于记录成功事件。
func (b *CutBreaker) Success() {
	if b.internalStatus == HalfOpening {
		b.metric.Reset() // 注意：这里需要先Reset metric再改状态，否则会有并发问题。
		// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
		atomic.CompareAndSwapInt32(&b.internalStatus, HalfOpening, Closed)
	}
	b.metric.Success()
}

// Failure 用于记录失败事件。
func (b *CutBreaker) Failure() {
	// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
	atomic.CompareAndSwapInt32(&b.internalStatus, HalfOpening, Openning)
	b.metric.Failure()
}

// Timeout 用于记录失败事件。
func (b *CutBreaker) Timeout() {
	// HalfOpening状态目前的实现不会有并发，但还是顺手用CAS吧。
	atomic.CompareAndSwapInt32(&b.internalStatus, HalfOpening, Openning)
	b.metric.Timeout()
}

// FallbackSuccess 记录一次降级函数执行成功事件。
func (b *CutBreaker) FallbackSuccess() {
	b.metric.FallbackSuccess()
}

// FallbackFailure 记录一次降级函数执行失败事件。
func (b *CutBreaker) FallbackFailure() {
	b.metric.FallbackSuccess()
}

// Summary 返回当前健康状态。
func (b *CutBreaker) Summary() *BreakerSummary {
	summary := b.metric.Summary() // 当前健康统计。
	_, statusStr := b.allow(summary)
	return &BreakerSummary{
		Status:               statusStr,
		TimeWindowSecond:     summary.TimeWindowSecond,
		MetricIntervalSecond: summary.MetricIntervalSecond,
		Success:              summary.Success,
		Timeout:              summary.Timeout,
		Failure:              summary.Failure,
		FallbackSuccess:      summary.FallbackSuccess,
		FallbackFailure:      summary.FallbackFailure,
		Total:                summary.Total,
		ErrorPercentage:      summary.ErrorPercentage,
		LastExecuteTime:      summary.LastExecuteTime,
		LastSuccessTime:      summary.LastSuccessTime,
		LastTimeoutTime:      summary.LastTimeoutTime,
		LastFailureTime:      summary.LastFailureTime,
	}
}

// CutBreakerOption 是 CutBreaker 的可选项。
type CutBreakerOption func(b *CutBreaker)

// WithCutBreakerMinRequestThreshold 设置熔断器生效必须满足的最小流量。
func WithCutBreakerMinRequestThreshold(minRequestThreshold int64) CutBreakerOption {
	return func(b *CutBreaker) {
		b.minRequestThreshold = minRequestThreshold
	}
}

// WithCutBreakerErrorThresholdPercentage 设置熔断器生效必须满足的错误百分比。
func WithCutBreakerErrorThresholdPercentage(errorThresholdPercentage float64) CutBreakerOption {
	return func(b *CutBreaker) {
		b.errorThresholdPercentage = errorThresholdPercentage
	}
}

// WithCutBreakerSleepWindow 设置熔断后重置熔断器的时间窗口。
func WithCutBreakerSleepWindow(sleepWindow time.Duration) CutBreakerOption {
	return func(b *CutBreaker) {
		b.sleepWindow = sleepWindow
	}
}

// WithCutBreakerTimeWindow 设置滑动窗口的大小（要求1-60s）。
func WithCutBreakerTimeWindow(timeWindow time.Duration) CutBreakerOption {
	return func(b *CutBreaker) {
		b.timeWindow = timeWindow
	}
}

// WithCutBreakerContext 设置用于释放资源的context。
func WithCutBreakerContext(ctx context.Context) CutBreakerOption {
	return func(b *CutBreaker) {
		b.ctx = ctx
	}
}
