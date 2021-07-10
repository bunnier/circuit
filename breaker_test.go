package circuit

import (
	"testing"
	"time"

	"github.com/bunnier/circuit/internal"
)

// TestBreaker_isOpen 测试熔断器的状态判断逻辑。
func TestBreaker_isOpen(t *testing.T) {
	tests := []struct {
		name                  string
		healthSummary         *internal.MetricSummary
		breakerInternalStatus int32
		isOpen                bool
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
		}, Closed, true, "open"},
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
		}, Closed, false, "closed"},
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
		}, HalfOpening, true, "half-open"},
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
		}, Openning, false, "half-open"},
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
		}, Openning, true, "open"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			breaker := NewBreaker(tt.name,
				WithBreakerCounterSize(5*time.Second),
				WithBreakerErrorThresholdPercentage(50),
				WithBreakerMinRequestThreshold(20),
				WithBreakerSleepWindow(5*time.Second))
			breaker.internalStatus = tt.breakerInternalStatus

			got, got1 := breaker.isOpen(tt.healthSummary)
			if got != tt.isOpen {
				t.Errorf("Breaker.isOpen() got = %v, want %v", got, tt.isOpen)
			}
			if got1 != tt.statusString {
				t.Errorf("Breaker.isOpen() got1 = %v, want %v", got1, tt.statusString)
			}
		})
	}
}

// TestBreaker_workflow 测试熔断器的完整工作流程。
func TestBreaker_workflow(t *testing.T) {
}
