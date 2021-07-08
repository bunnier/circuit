package circuit

import "time"

// BreakerStatus 是熔断器状态。
type BreakerStatus int8

const (
	Closed      BreakerStatus = 0 // 熔断关闭。
	Openning    BreakerStatus = 1 // 熔断开启。
	HalfOpening BreakerStatus = 2 // 半熔断状态。
)

// 熔断器结构。
type Breaker struct {
	name   string  // 名称。
	metric *Metric // 执行情况统计数据。

	minRequestThreshold      int64         // 断路器生效必须满足的最小流量。
	errorThresholdPercentage float64       // 开启熔断的错误百分比阈值。
	sleepWindow              time.Duration // 熔断后重置断路器的时间窗口。
	timeWindow               time.Duration // 滑动窗口的大小（单位秒1-60）。
}

// NewBreaker 用于新建一个熔断器。
func NewBreaker(name string, options ...BreakerOption) *Breaker {
	breaker := &Breaker{
		name: name,

		minRequestThreshold:      20,
		errorThresholdPercentage: 0.5,
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

// BreakerOption 是Breaker的可选项。
type BreakerOption func(breaker *Breaker)

// WithBreakderMinRequestThreshold 设置断路器生效必须满足的最小流量。
func WithBreakderMinRequestThreshold(minRequestThreshold int64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.minRequestThreshold = minRequestThreshold
	}
}

// WithBreakderMinRequestThreshold 设置断路器生效必须满足的最小流量。
func WithBreakderErrorThresholdPercentage(errorThresholdPercentage float64) BreakerOption {
	return func(breaker *Breaker) {
		breaker.errorThresholdPercentage = errorThresholdPercentage
	}
}

// WithBreakderMinRequestThreshold 设置熔断后重置断路器的时间窗口。
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
