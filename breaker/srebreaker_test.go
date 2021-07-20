package breaker

import (
	"fmt"
	"math"
	"testing"
	"time"

	"github.com/bunnier/circuit/breaker/internal"
)

func TestSreBreaker_allow(t *testing.T) {
	tests := []struct {
		name          string
		healthSummary *internal.MetricSummary
		prob          float64
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
		}, 0.249},
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
		}, 0.950},
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
		}, 0.950},
		{"case4", &internal.MetricSummary{
			Success:         0,
			Timeout:         5,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           20,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, 0.952},
		{"case5", &internal.MetricSummary{
			Success:         0,
			Timeout:         5,
			Failure:         15,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           20,
			ErrorPercentage: 100,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, 0.952},
		{"case6", &internal.MetricSummary{
			Success:         20,
			Timeout:         0,
			Failure:         0,
			FallbackSuccess: 0,
			FallbackFailure: 0,
			Total:           20,
			ErrorPercentage: 0,
			LastExecuteTime: time.Now(),
			LastSuccessTime: time.Now(),
			LastTimeoutTime: time.Now(),
			LastFailureTime: time.Now(),
		}, 0.000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			braeker := NewSreBreaker(tt.name,
				WithSreBreakerK(1.5))

			// 先判断概率是否符合公式计算结果。
			got := fmt.Sprintf("%3.3f", braeker.getRejectionProbability(tt.healthSummary))
			probStr := fmt.Sprintf("%3.3f", tt.prob)
			if got != probStr {
				t.Errorf("SreBreaker.getRejectionProbability() got = %v, want %v", got, probStr)
			}

			// 创建一大批请求进行测试，如果实现无误，最后统计出来的通过率应该不会与公式概率相差太多。
			const difference float64 = 0.01 // 允许的误差。
			const testCount int = 10000     // 测试次数。

			rejectCount := 0
			for i := 0; i < testCount; i++ {
				if allow, _ := braeker.allow(tt.healthSummary); !allow {
					rejectCount++
				}
			}
			actProb := float64(rejectCount) / float64(testCount)
			if math.Abs(actProb-tt.prob) > difference {
				t.Errorf("SreBreaker.allow() got = %v, want %3.3f, allow difference %3.3f", got, actProb, difference)
			}
		})
	}
}
