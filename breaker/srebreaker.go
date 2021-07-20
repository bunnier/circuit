package breaker

import (
	"context"
	"fmt"
	"math"
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

	timeWindow time.Duration // 滑动窗口的大小。
}

// NewSreBreaker 用于新建一个 SreBreaker 熔断器。
// SreBreaker 提供基Google SRE提出的 adaptive throttling 算法。
// 算法参考：https://sre.google/sre-book/handling-overload/#eq2101
func NewSreBreaker(name string, options ...SreBreakerOption) *SreBreaker {
	b := &SreBreaker{
		ctx:  context.Background(),
		name: name,

		k:        1.5, // 算法的调节系数，越高算法越懒惰，反之越主动。
		rand:     rand.New(rand.NewSource(time.Now().Unix())),
		randLock: &sync.Mutex{},

		timeWindow: time.Minute * 2,
	}

	for _, option := range options {
		option(b)
	}

	// 初始化选项后，根据选项初始化Metric。
	b.metric = internal.NewMetric(
		internal.WithMetricTimeWindow(b.timeWindow),
		internal.WithMetricMetricInterval(time.Second*30),
	)

	return b
}

// Allow 用于判断断路器是否允许通过请求。
// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
func (b *SreBreaker) Allow() (bool, string) {
	summary := b.metric.Summary()
	return b.allow(summary)
}

// Allow 用于判断断路器是否允许通过请求。
// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
func (b *SreBreaker) allow(summary *internal.MetricSummary) (bool, string) {
	b.randLock.Lock()
	currentProb := b.rand.Float64() // 计算本次概率。
	b.randLock.Unlock()

	rejectProb := b.getRejectionProbability(summary) // 当前熔断概率。

	return currentProb > rejectProb, fmt.Sprintf("rejection probability = %3.3f, this time = %3.3f", rejectProb, currentProb)
}

// getRejectionProbability 用于计算当前请求的熔断概率。
func (b *SreBreaker) getRejectionProbability(summary *internal.MetricSummary) float64 {
	// 算法参考：https://sre.google/sre-book/handling-overload/#eq2101
	prob := (float64(summary.Total) - b.k*float64(summary.Success)) / float64(summary.Total+1)
	return math.Max(0, prob)
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
		Status:               fmt.Sprintf("current rejection probability: %3.3f", b.getRejectionProbability(summary)), // 直接显示概率
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

// SreBreakerOption 是 SreBreaker 的可选项。
type SreBreakerOption func(b *SreBreaker)

// WithSreBreakerTimeWindow 设置滑动窗口的大小（默认2分钟）。
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

// WithSreBreakerK 设置Google SRE adaptive throttling 算法公式中的调节系数K。
func WithSreBreakerK(k float64) SreBreakerOption {
	return func(b *SreBreaker) {
		b.k = k
	}
}
