package internal

import (
	"sync"
	"testing"
	"time"
)

// TestMetric_Collect 测试数据收集逻辑。
func TestMetric_Collect(t *testing.T) {
	metric := NewMetric(WithMetricCounterSize(time.Second * 3)) // 3s的窗口

	// 下面有直接/2，所以这里的数字需要都是偶数。
	const successCount = 4000
	const failureCount = 900
	const timeoutCount = 100
	const fallbackFailureCount = 20
	const fallbackSuccessCount = 40
	const totalCount = successCount + timeoutCount + failureCount
	const errorPercentage = float64(failureCount+timeoutCount) / totalCount * 100

	// 让下面代码的开始尽量靠近整数秒的开始时间，以便控制滑块。
	time.Sleep(time.Second - time.Duration(time.Now().Nanosecond()))

	// 分2批写入数据，让数据分散在不同滑块。
	// 每次写入应该都是几毫秒。
	doMetricCollect(metric, successCount/2, failureCount/2, timeoutCount/2, fallbackFailureCount/2, fallbackSuccessCount/2)
	time.Sleep(time.Second)
	doMetricCollect(metric, successCount/2, failureCount/2, timeoutCount/2, fallbackFailureCount/2, fallbackSuccessCount/2)

	// 此时时间窗口肯定还没到，验证数据，应该满血。
	validateMetricCollect(t, "case1", metric,
		successCount, failureCount, timeoutCount, fallbackFailureCount, fallbackSuccessCount,
		totalCount, errorPercentage)

	time.Sleep(time.Second * 2) // 这个时间后已经最早的滑块应该刚好清0。

	validateMetricCollect(t, "case2", metric,
		successCount/2, failureCount/2, timeoutCount/2, fallbackFailureCount/2, fallbackSuccessCount/2,
		totalCount/2, errorPercentage)

	time.Sleep(time.Second * 1) // 这个时间后一定已经清0。

	// 验证数据
	validateMetricCollect(t, "case3", metric, 0, 0, 0, 0, 0, 0, 0)

	// 再写一次数据，来验证Reset。
	doMetricCollect(metric, successCount, failureCount, timeoutCount, fallbackFailureCount, fallbackSuccessCount)
	time.Sleep(time.Second) // 确保数据写完了。
	metric.Reset()
	time.Sleep(time.Second) // 确保数据写完了。
	validateMetricCollect(t, "case4", metric, 0, 0, 0, 0, 0, 0, 0)
}

func doMetricCollect(metric *Metric,
	successCount, failureCount, timeoutCount, fallbackFailureCount, fallbackSuccessCount int) {
	var wg sync.WaitGroup
	for i := 0; i < successCount; i++ {
		wg.Add(1)
		go func() {
			metric.Success()
			wg.Done()
		}()
	}
	for i := 0; i < failureCount; i++ {
		wg.Add(1)
		go func() {
			metric.Failure()
			wg.Done()
		}()
	}
	for i := 0; i < timeoutCount; i++ {
		wg.Add(1)
		go func() {
			metric.Timeout()
			wg.Done()
		}()
	}
	for i := 0; i < fallbackFailureCount; i++ {
		wg.Add(1)
		go func() {
			metric.FallbackFailure()
			wg.Done()
		}()
	}
	for i := 0; i < fallbackSuccessCount; i++ {
		wg.Add(1)
		go func() {
			metric.FallbackSuccess()
			wg.Done()
		}()
	}
	wg.Wait()
}

func validateMetricCollect(t *testing.T, name string, metric *Metric,
	successCount, failureCount, timeoutCount, fallbackFailureCount, fallbackSuccessCount int,
	totalCount int64, errorPercentage float64) {
	summary := metric.Summary()
	if summary.Success != int64(successCount) {
		t.Errorf("%s: summary.Success is wrong, want %d, but %d", name, successCount, summary.Success)
	}
	if summary.Failure != int64(failureCount+timeoutCount) { // 这里超时记录时候也算入Failure。
		t.Errorf("%s: summary.Failure is wrong, want %d, but %d", name, failureCount, summary.Failure)
	}
	if summary.Timeout != int64(timeoutCount) {
		t.Errorf("%s: summary.Timeout is wrong, want %d, but %d", name, timeoutCount, summary.Timeout)
	}
	if summary.Total != int64(totalCount) {
		t.Errorf("%s: summary.Total is wrong, want %d, but %d", name, totalCount, summary.Total)
	}
	if summary.ErrorPercentage != float64(errorPercentage) {
		t.Errorf("%s: summary.ErrorPercentage is wrong, want %f, but %f", name, errorPercentage, summary.ErrorPercentage)
	}
}
