package breaker

import (
	"sync"
	"testing"
	"time"

	"github.com/bunnier/circuit/breaker/internal"
)

// TestCutBreaker_allow 测试熔断器的状态判断逻辑。
func TestCutBreaker_allow(t *testing.T) {
	tests := []struct {
		name                  string
		healthSummary         *internal.MetricSummary
		breakerInternalStatus int32
		allow                 bool
		statusString          string
	}{
		{"case1", &internal.MetricSummary{
			Success:         100,
			Timeout:         30,
			Failure:         100,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           200,
			ErrorPercentage: 50,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, Closed, false, "open"},
		{"case2", &internal.MetricSummary{
			Success:         0,
			Timeout:         4,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           19,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, Closed, true, "closed"},
		{"case3", &internal.MetricSummary{
			Success:         0,
			Timeout:         4,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           19,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, HalfOpening, false, "half-open"},
		{"case4", &internal.MetricSummary{
			Success:         0,
			Timeout:         5,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           20,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now().Add(-time.Second * 10),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, Openning, true, "half-open"},
		{"case5", &internal.MetricSummary{
			Success:         0,
			Timeout:         5,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           20,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now().Add(-time.Second * 3),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, Openning, false, "open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breaker := NewCutBreaker(tt.name,
				WithCutBreakerTimeWindow(5*time.Second),
				WithCutBreakerErrorThresholdPercentage(50),
				WithCutBreakerMinRequestThreshold(20),
				WithCutBreakerSleepWindow(5*time.Second))
			breaker.internalStatus = tt.breakerInternalStatus

			got, got1 := breaker.allow(tt.healthSummary)
			if got != tt.allow {
				t.Errorf("CutBreaker.allow() got = %v, want %v", got, tt.allow)
			}
			if got1 != tt.statusString {
				t.Errorf("CutBreaker.allow() got1 = %v, want %v", got1, tt.statusString)
			}
		})
	}
}

// TestCutBreaker_workflow 测试熔断器的完整工作流程。
func TestCutBreaker_workflow(t *testing.T) {
	breaker := NewCutBreaker("test",
		WithCutBreakerTimeWindow(5*time.Second),
		WithCutBreakerErrorThresholdPercentage(50),
		WithCutBreakerMinRequestThreshold(20),
		WithCutBreakerSleepWindow(2*time.Second))

	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func() {
			breaker.Success()
			wg.Done()
		}()
	}
	for i := 0; i < 999; i++ {
		wg.Add(1)
		go func() {
			breaker.Failure()
			wg.Done()
		}()
	}
	wg.Wait()

	// 此时应还是关闭。
	if pass, _ := breaker.Allow(); !pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, true)
	}

	breaker.Timeout()
	// 此时应该开启了。
	if pass, _ := breaker.Allow(); pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, false)
	}

	time.Sleep(2 * time.Second)
	// 睡眠期结束，应该可以进入半熔断了。
	if pass, statusMsg := breaker.Allow(); !pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, true)
	} else if statusMsg != "half-open" {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", statusMsg, "half-open")
	}

	breaker.Failure() // 半熔断状态失败，再次进入熔断。
	if pass, _ := breaker.Allow(); pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, false)
	}

	time.Sleep(2 * time.Second)
	// 睡眠期结束，应该可以进入半熔断了。
	if pass, statusMsg := breaker.Allow(); !pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, true)
	} else if statusMsg != "half-open" {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", statusMsg, "half-open")
	}

	breaker.Success() // 半熔断状态成功，关闭熔断器。
	if pass, _ := breaker.Allow(); !pass {
		t.Errorf("CutBreaker.Allow() got = %v, want %v", pass, true)
	}
}
