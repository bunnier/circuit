package circuit

import (
	"context"
	"sync"
	"time"
)

// Metric 用于保存Command的运行情况统计数据。
// 内部使用滑动窗口方式存储统计数据。
type Metric struct {
	ctx    context.Context    // 执行上下文，主要用于关闭Run中起的goroutine。
	cancel context.CancelFunc // 保存ctx的cancel函数。

	rwLock sync.RWMutex // 用于同步统计信息的读写操作。

	timeWindow time.Duration  // 滑动窗口的大小（单位秒1-60）。
	counters   []*UnitCounter // 滑动窗口的所有统计数据，按timeWindow的秒数，多少秒就多少长度。

	successCh         chan time.Time // 用于记录一次成功数量统计。
	timeoutCh         chan time.Time // 用于记录一次超时数量统计
	failureCh         chan time.Time // 用于记录一次失败数量统计。
	fallbackSuccessCh chan time.Time // 用于记录一次失败回调执行成功统计。
	fallbackFailureCh chan time.Time // 用于记录一次失败回调执行失败统计。
	resetCh           chan time.Time // 用于重置所有统计数据。

	lastExecuteTime time.Time // 最后一次执行时间。
	lastSuccessTime time.Time // 最后一次成功执行时间。
	lastTimeoutTime time.Time // 最后一次超时时间。
	lastFailureTime time.Time // 最后一次失败时间。
	lastResetTime   time.Time // 最后一次重置统计时间。
}

// UnitCounter 用于记录滑动窗口中一个单元（1s）的统计数据。
type UnitCounter struct {
	Success         int64 // 成功数量。
	Timeout         int64 // 超时数量。
	Failure         int64 // 失败数量。
	FallbackSuccess int64 // 失败回调执行成功数量。
	FallbackFailure int64 // 失败毁掉执行失败数量。

	LastRecordTime time.Time // 记录最后一次写入的时间。
}

// Reset 用于重置统计量。
func (counter *UnitCounter) Reset() {
	counter.Success = 0
	counter.Timeout = 0
	counter.Failure = 0
	counter.FallbackSuccess = 0
	counter.FallbackFailure = 0
	counter.LastRecordTime = time.Time{}
}

// HealthSummary 是根据统计信息计算出来的当前健康状态。
type HealthSummary struct {
	Success         int64 // 成功数量。
	Timeout         int64 // 超时数量。
	Failure         int64 // 失败数量。
	FallbackSuccess int64 // 失败回调执行成功数量。
	FallbackFailure int64 // 失败毁掉执行失败数量。

	Total           int64   // 本次统计窗口所执行的所有次数。
	ErrorPercentage float64 // 错误数量百分比。

	lastExecuteTime time.Time // 最后一次执行时间。
	lastSuccessTime time.Time // 最后一次成功执行时间。
	lastTimeoutTime time.Time // 最后一次超时时间。
	lastFailureTime time.Time // 最后一次失败时间。
}

// newMetric 用于获取一个Metric对象。
func newMetric(options ...MerticOption) *Metric {
	const channelBufferSize int8 = 10 // 用于发送统计数据的channel大小。
	ctx, cancel := context.WithCancel(context.Background())
	metric := &Metric{
		ctx:               ctx,
		cancel:            cancel,
		rwLock:            sync.RWMutex{},
		timeWindow:        time.Second * 5, // 默认统计窗口5s。
		successCh:         make(chan time.Time, channelBufferSize),
		timeoutCh:         make(chan time.Time, channelBufferSize),
		failureCh:         make(chan time.Time, channelBufferSize),
		fallbackSuccessCh: make(chan time.Time, channelBufferSize),
		fallbackFailureCh: make(chan time.Time, channelBufferSize),
		resetCh:           make(chan time.Time, channelBufferSize),
	}

	for _, option := range options {
		option(metric)
	}

	// 根据窗口大小初始化统计切片。
	metric.counters = make([]*UnitCounter, metric.timeWindow/time.Second)
	return metric
}

// GetHealthSummary 根据当前统计信息给出健康摘要。
func (metric *Metric) GetHealthSummary() HealthSummary {
	summary := HealthSummary{}

	metric.rwLock.RLock()
	defer metric.rwLock.Unlock()

	for _, counter := range metric.counters {
		if counter != nil {
			continue
		}

		// 如果调用不连续，统计块可能有一些不属于本次窗口，所以需要一一判断时间。
		if time.Since(counter.LastRecordTime) > metric.timeWindow {
			continue
		}

		summary.Success += counter.Success
		summary.Timeout += counter.Timeout
		summary.Failure += counter.Failure
		summary.FallbackSuccess += counter.FallbackSuccess
		summary.FallbackFailure += counter.FallbackFailure
	}

	// 计算错误率。
	summary.Total = summary.Success + summary.Failure
	if summary.Total == 0 {
		summary.ErrorPercentage = 0
	} else {
		summary.ErrorPercentage = float64(summary.Failure) / float64(summary.Total) * 100
	}

	summary.lastExecuteTime = metric.lastExecuteTime
	summary.lastSuccessTime = metric.lastSuccessTime
	summary.lastTimeoutTime = metric.lastTimeoutTime
	summary.lastFailureTime = metric.lastFailureTime

	return summary
}

// Success 记录一次成功事件。
func (metric *Metric) Success() {
	metric.successCh <- time.Now()
}

// Timeout 记录一次超时事件。
func (metric *Metric) Timeout() {
	metric.timeoutCh <- time.Now()
}

// Failure 记录一次失败事件。
func (metric *Metric) Failure() {
	metric.failureCh <- time.Now()
}

// FallbackSuccess 记录一次失败回调执行成功事件。
func (metric *Metric) FallbackSuccess() {
	metric.fallbackSuccessCh <- time.Now()
}

// FallbackFailure 记录一次失败回调执行失败事件。
func (metric *Metric) FallbackFailure() {
	metric.fallbackFailureCh <- time.Now()
}

// Reset 用于重置所有统计数据。
func (metric *Metric) Reset() {
	metric.resetCh <- time.Now()
}

// Run 用于开始统计数据处理。
func (metric *Metric) Run(ctx context.Context) {
	go func() {
		for {
			select {
			case <-metric.ctx.Done():
				return // 结束。
			case now := <-metric.successCh:
				metric.doSuccess(now)
			case now := <-metric.timeoutCh:
				metric.doTimeout(now)
			case now := <-metric.failureCh:
				metric.doFailure(now)
			case now := <-metric.fallbackSuccessCh:
				metric.doFallbackSuccess(now)
			case now := <-metric.fallbackFailureCh:
				metric.doFallbackFailure(now)
			case now := <-metric.resetCh:
				metric.doReset(now)
			}
		}
	}()
}

// doWithStatusLock 用于占用metric并执行写操作。
func (metric *Metric) doWithStatusLock(doFunc func()) {
	metric.rwLock.Lock()
	defer metric.rwLock.Unlock()
	doFunc()
}

func (metric *Metric) doSuccess(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastExecuteTime = now
		metric.lastSuccessTime = now
		metric.getCurrentCounter(now).Success++
	})
}

func (metric *Metric) doTimeout(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastExecuteTime = now
		metric.lastTimeoutTime = now
		metric.getCurrentCounter(now).Timeout++
		metric.getCurrentCounter(now).Failure++ // 超时也算失败的一种，这里也将失败加1。
	})
}

func (metric *Metric) doFailure(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastExecuteTime = now
		metric.lastFailureTime = now
		metric.getCurrentCounter(now).Failure++
	})
}

func (metric *Metric) doFallbackSuccess(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastExecuteTime = now
		metric.getCurrentCounter(now).FallbackSuccess++
	})
}

func (metric *Metric) doFallbackFailure(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastExecuteTime = now
		metric.getCurrentCounter(now).FallbackFailure++
	})
}

func (metric *Metric) doReset(now time.Time) {
	metric.doWithStatusLock(func() {
		metric.lastResetTime = now
		metric.counters = make([]*UnitCounter, metric.timeWindow/time.Second) // 直接新建一个统计量。
	})
}

// getCurrentCounter 获取当前的统计块。
func (metric *Metric) getCurrentCounter(now time.Time) *UnitCounter {
	// 直接把秒取模做数组索引作为当前统计块。
	index := now.Second() % len(metric.counters)
	currentCounter := metric.counters[index]

	if currentCounter == nil {
		currentCounter = &UnitCounter{}
	} else {
		// unix时间戳到秒，只要时间戳不同，说明已经不再同一秒，只是取模后结果相同而已，需要重置。
		if now.Unix() != currentCounter.LastRecordTime.Unix() {
			currentCounter.Reset()
		}
	}

	currentCounter.LastRecordTime = now // 每次获取都更新记录时间。
	return currentCounter
}

// MerticOption 是Mertic的可选项。
type MerticOption func(metric *Metric)

// WithMetricCounterSize 设置滑动窗口的大小（单位秒）。
func WithMetricCounterSize(timeWindow time.Duration) MerticOption {
	if timeWindow < time.Second || timeWindow > time.Minute {
		panic("metric: timeWindow invalid") // 窗口大小错误属于无法恢复的错误，直接panic把。
	}
	return func(metric *Metric) {
		metric.timeWindow = timeWindow
	}
}

// WithMetricContext 用于设置一个context，以便优雅退出内部消耗统计信息的gorotine。
func WithMetricContext(ctx context.Context) MerticOption {
	return func(metric *Metric) {
		metric.ctx, metric.cancel = context.WithCancel(ctx)
	}
}
