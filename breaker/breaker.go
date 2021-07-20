package breaker

import (
	"time"
)

// Breaker 是熔断器接口。
type Breaker interface {

	// Allow 用于判断断路器是否允许通过请求。
	// 第一返回值：true能通过/false不能；第二返回值：当前Breaker状态的文字描述。
	Allow() (bool, string)

	// Success 用于记录成功事件。
	Success()

	// Failure 用于记录失败事件。
	Failure()

	// Timeout 用于记录失败事件。
	Timeout()

	// FallbackSuccess 记录一次降级函数执行成功事件。
	FallbackSuccess()

	// FallbackFailure 记录一次降级函数执行失败事件。
	FallbackFailure()

	// Summary 返回当前熔断器状态信息。
	Summary() *BreakerSummary
}

// BreakerSummary 返回统计数据摘要。
type BreakerSummary struct {
	Status string // 熔断器当前状态的文字描述。

	TimeWindowSecond     int64 // 滑动窗口的大小。
	MetricIntervalSecond int64 // 窗口中每个统计量的间隔区间。

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

// 定义熔断器的通用状态数字表示常量。
// 这里本不需要用int32，为了放到CAS方法中使用，使用int32。
const (
	Closed      int32 = 0 // 熔断关闭。
	Openning    int32 = 1 // 熔断开启。
	HalfOpening int32 = 2 // 半熔断状态。
)
