package internal

import (
	"context"
	"time"
)

// Metric 用于保存Command的运行情况统计数据。
// 内部使用滑动窗口方式存储统计数据。
type Metric struct {
	ctx context.Context // 用于释放资源的context。

	timeWindow time.Duration  // 滑动窗口的大小（单位秒1-60）。
	counters   []*UnitCounter // 滑动窗口的所有统计数据，按timeWindow的秒数，多少秒就多少长度。

	successCh         chan time.Time // 用于记录一次成功数量统计。
	timeoutCh         chan time.Time // 用于记录一次超时数量统计
	failureCh         chan time.Time // 用于记录一次失败数量统计。
	fallbackSuccessCh chan time.Time // 用于记录一次降级函数执行成功统计。
	fallbackFailureCh chan time.Time // 用于记录一次降级函数执行失败统计。

	resetCh chan time.Time // 用于重置所有统计数据。

	makeSummaryCh chan struct{}       // 用于计算统计数据。
	getSummaryCh  chan *MetricSummary // 用于获取统计数据。

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
	FallbackSuccess int64 // 降级函数执行成功数量。
	FallbackFailure int64 // 降级函数执行失败数量。

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

// MetricSummary 返回统计数据摘要。
type MetricSummary struct {
	Success         int64 // 成功数量。
	Timeout         int64 // 超时数量。
	Failure         int64 // 失败数量。
	FallbackSuccess int64 // 降级函数执行成功数量。
	FallbackFailure int64 // 降级函数执行失败数量。

	Total           int64   // 本次统计窗口所执行的所有次数。
	ErrorPercentage float64 // 错误数量百分比。

	LastExecuteTime time.Time // 最后一次执行时间。
	LastSuccessTime time.Time // 最后一次成功执行时间。
	LastTimeoutTime time.Time // 最后一次超时时间。
	LastFailureTime time.Time // 最后一次失败时间。
}

// NewMetric 用于获取一个Metric对象。
func NewMetric(options ...MerticOption) *Metric {
	const channelBufferSize int8 = 10 // 用于发送统计数据的channel大小。
	m := &Metric{
		ctx:               context.Background(),
		timeWindow:        time.Second * 5, // 默认统计窗口5s。
		successCh:         make(chan time.Time, channelBufferSize),
		timeoutCh:         make(chan time.Time, channelBufferSize),
		failureCh:         make(chan time.Time, channelBufferSize),
		fallbackSuccessCh: make(chan time.Time, channelBufferSize),
		fallbackFailureCh: make(chan time.Time, channelBufferSize),
		resetCh:           make(chan time.Time, channelBufferSize),
		makeSummaryCh:     make(chan struct{}, channelBufferSize),
		getSummaryCh:      make(chan *MetricSummary, channelBufferSize),
	}

	for _, option := range options {
		option(m)
	}

	// 根据窗口大小初始化统计切片。
	m.counters = make([]*UnitCounter, m.timeWindow/time.Second)

	// 开始接收统计。
	m.run()
	return m
}

func (m *Metric) makeSummary() {
	summary := MetricSummary{}

	for _, counter := range m.counters {
		if counter == nil {
			continue
		}

		// 如果调用不连续，统计块可能有一些不属于本次窗口，所以需要一一判断时间。
		if time.Since(counter.LastRecordTime) > m.timeWindow {
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

	summary.LastExecuteTime = m.lastExecuteTime
	summary.LastSuccessTime = m.lastSuccessTime
	summary.LastTimeoutTime = m.lastTimeoutTime
	summary.LastFailureTime = m.lastFailureTime

	m.getSummaryCh <- &summary
}

// Summary 根据当前统计信息给出健康摘要。
func (m *Metric) Summary() *MetricSummary {
	m.makeSummaryCh <- struct{}{}
	return <-m.getSummaryCh
}

// Success 记录一次成功事件。
func (m *Metric) Success() {
	m.successCh <- time.Now()
}

// Timeout 记录一次超时事件。
func (m *Metric) Timeout() {
	m.timeoutCh <- time.Now()
}

// Failure 记录一次失败事件。
func (m *Metric) Failure() {
	m.failureCh <- time.Now()
}

// FallbackSuccess 记录一次降级函数执行成功事件。
func (m *Metric) FallbackSuccess() {
	m.fallbackSuccessCh <- time.Now()
}

// FallbackFailure 记录一次降级函数执行失败事件。
func (m *Metric) FallbackFailure() {
	m.fallbackFailureCh <- time.Now()
}

// Reset 用于重置所有统计数据。
func (m *Metric) Reset() {
	m.resetCh <- time.Now()
}

// run 用于开始统计数据处理。
func (m *Metric) run() {
	go func() {
		for {
			select {
			case <-m.ctx.Done():
				return // 结束。
			case now := <-m.successCh:
				m.doSuccess(now)
			case now := <-m.timeoutCh:
				m.doTimeout(now)
			case now := <-m.failureCh:
				m.doFailure(now)
			case now := <-m.fallbackSuccessCh:
				m.doFallbackSuccess(now)
			case now := <-m.fallbackFailureCh:
				m.doFallbackFailure(now)
			case now := <-m.resetCh:
				m.doReset(now)
			case <-m.makeSummaryCh: // 获取Summary采用收到信号后计算并返回的方式。
				m.makeSummary()
			}
		}
	}()
}
func (m *Metric) doSuccess(now time.Time) {
	m.lastExecuteTime = now
	m.lastSuccessTime = now
	m.getCurrentCounter(now).Success++
}

func (m *Metric) doTimeout(now time.Time) {
	m.lastExecuteTime = now
	m.lastTimeoutTime = now
	m.getCurrentCounter(now).Timeout++
	m.getCurrentCounter(now).Failure++ // 超时也算失败的一种，这里也将失败加1。
}

func (m *Metric) doFailure(now time.Time) {
	m.lastExecuteTime = now
	m.lastFailureTime = now
	m.getCurrentCounter(now).Failure++
}

func (m *Metric) doFallbackSuccess(now time.Time) {
	m.lastExecuteTime = now
	m.getCurrentCounter(now).FallbackSuccess++
}

func (m *Metric) doFallbackFailure(now time.Time) {
	m.lastExecuteTime = now
	m.getCurrentCounter(now).FallbackFailure++
}

func (m *Metric) doReset(now time.Time) {
	m.lastResetTime = now
	m.counters = make([]*UnitCounter, m.timeWindow/time.Second) // 直接新建一个统计量。
}

// getCurrentCounter 获取当前的统计块。
func (m *Metric) getCurrentCounter(now time.Time) *UnitCounter {
	// 直接把秒取模做数组索引作为当前统计块。
	index := now.Second() % len(m.counters)
	currentCounter := m.counters[index]

	if currentCounter == nil {
		currentCounter = &UnitCounter{}
		m.counters[index] = currentCounter
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
type MerticOption func(m *Metric)

// WithMetricCounterSize 设置滑动窗口的大小（单位秒）。
func WithMetricCounterSize(timeWindow time.Duration) MerticOption {
	if timeWindow < time.Second || timeWindow > time.Minute {
		panic("metric: timeWindow invalid") // 窗口大小错误属于无法恢复的错误，直接panic把。
	}
	return func(m *Metric) {
		m.timeWindow = timeWindow
	}
}

// WithMetricContext 用于设置一个context，以便优雅退出内部消耗统计信息的gorotine。
func WithMetricContext(ctx context.Context) MerticOption {
	return func(m *Metric) {
		m.ctx = ctx
	}
}
