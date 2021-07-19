package breaker

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"time"

	"github.com/bunnier/circuit/breaker/internal"
)

var _ Breaker = (*SreBreaker)(nil)

// SreBreaker 是 Breaker 的一种实现。
type SreBreaker struct {
	ctx context.Context // 用于释放资源的context。

	name   string           // 名称。
	metric *internal.Metric // 执行情况统计数据。

	k        float64     // 算法的调节系数。
	rand     *rand.Rand  // 随机数生成器。
	randLock *sync.Mutex // 用于控制随机数生成时候的并发。

	timeWindow time.Duration // 滑动窗口的大小（单位秒1-60）。
}

// NewSreBreaker 用于新建一个 SreBreaker 熔断器。
// SreBreaker 提供基Google SRE提出的Handling Overload算法。
// 算法参考：https://sre.google/sre-book/handling-overload/#eq2101
func NewSreBreaker(name string, options ...SreBreakerOption) *SreBreaker {
	b := &SreBreaker{
		ctx:  context.Background(),
		name: name,

		k:        1.5, // 算法的调节系数，越高算法越懒惰，反之越主动。
		rand:     &rand.Rand{},
		randLock: &sync.Mutex{},

		timeWindow: 5,
	}

	for _, option := range options {
		option(b)
	}

	// 初始化选项后，根据选项初始化Metric。
	b.metric = internal.NewMetric(internal.WithMetricCounterSize(b.timeWindow))

	return b
}

// Allow 用于判断断路器是否允许通过请求。
// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
func (b *SreBreaker) Allow() (bool, string) {
	b.randLock.Lock()
	rf := b.rand.Float64() // 计算本次概率。
	b.randLock.Unlock()

	summary := b.metric.Summary()
	prob := b.getRejectionProbability(summary) // 当前熔断概率。

	return rf < prob, fmt.Sprintf("rejection probability = %3.3f, this time = %3.3f", prob, rf)
}

// getRejectionProbability 用于计算当前请求的熔断概率。
func (b *SreBreaker) getRejectionProbability(summary *internal.MetricSummary) float64 {
	// 算法参考：https://sre.google/sre-book/handling-overload/#eq2101
	return (float64(summary.Total) - b.k*float64(summary.Success)) / float64(summary.Total+1)
}

// Success 用于记录成功事件。
func (b *SreBreaker) Success() {
	b.metric.Success()
}

// Failure 用于记录失败事件。
func (b *SreBreaker) Failure() {
	b.metric.Failure()
}

// Timeout 用于记录失败事件。
func (b *SreBreaker) Timeout() {
	b.metric.Timeout()
}

// FallbackSuccess 记录一次降级函数执行成功事件。
func (b *SreBreaker) FallbackSuccess() {
	b.metric.FallbackSuccess()
}

// FallbackFailure 记录一次降级函数执行失败事件。
func (b *SreBreaker) FallbackFailure() {
	b.metric.FallbackSuccess()
}

// Summary 返回当前健康状态。
func (b *SreBreaker) Summary() *BreakerSummary {
	summary := b.metric.Summary() // 当前健康统计。
	return &BreakerSummary{
		Status:          fmt.Sprintf("current rejection probability: %3.3f", b.getRejectionProbability(summary)), // 直接显示概率
		Success:         summary.Success,
		Timeout:         summary.Timeout,
		Failure:         summary.Failure,
		FallbackSuccess: summary.FallbackSuccess,
		FallbackFailure: summary.FallbackFailure,
		Total:           summary.Total,
		ErrorPercentage: summary.ErrorPercentage,
		LastExecuteTime: summary.LastExecuteTime,
		LastSuccessTime: summary.LastSuccessTime,
		LastTimeoutTime: summary.LastTimeoutTime,
		LastFailureTime: summary.LastFailureTime,
	}
}

// SreBreakerOption 是 SreBreaker 的可选项。
type SreBreakerOption func(b *SreBreaker)

// WithSreBreakerTimeWindow 设置滑动窗口的大小（要求1-60s）。
func WithSreBreakerTimeWindow(timeWindow time.Duration) SreBreakerOption {
	return func(b *SreBreaker) {
		b.timeWindow = timeWindow
	}
}

// WithSreBreakerContext 设置用于释放资源的context。
func WithSreBreakerContext(ctx context.Context) SreBreakerOption {
	return func(b *SreBreaker) {
		b.ctx = ctx
	}
}

// WithSreBreakerK 设置Google SRE Handling Overload算法熔断算法公式中的调节系数。
func WithSreBreakerK(k float64) SreBreakerOption {
	return func(b *SreBreaker) {
		b.k = k
	}
}
