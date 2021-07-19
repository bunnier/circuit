package breaker

import (
	"fmt"
	"testing"
	"time"

	"github.com/bunnier/circuit/breaker/internal"
)

func TestSreBreaker_getRejectionProbability(t *testing.T) {
	tests := []struct {
		name          string
		healthSummary *internal.MetricSummary
		prob          string
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
		}, "0.249"},
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
		}, "0.950"},
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
		}, "0.950"},
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
		}, "0.952"},
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
		}, "0.952"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			braeker := NewSreBreaker(tt.name,
				WithSreBreakerTimeWindow(2*time.Second),
				WithSreBreakerK(1.5))

			got := fmt.Sprintf("%3.3f", braeker.getRejectionProbability(tt.healthSummary))
			if got != tt.prob {
				t.Errorf("SreBreaker.getRejectionProbability() got = %v, want %v", got, tt.prob)
			}
		})
	}
}
