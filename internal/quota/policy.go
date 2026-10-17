// Package quota 提供与运行时及存储无关的 UTC 周期和流量计费策略。
package quota

import (
	"math"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

// SaturatingAdd 将非负字节累计限制在 int64 上限，避免溢出返还额度。
func SaturatingAdd(a, b int64) int64 {
	if a < 0 {
		a = 0
	}
	if b <= 0 {
		return a
	}
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

func Used(mode string, upload, download int64) int64 {
	switch mode {
	case model.AccountingMax:
		if upload > download {
			return upload
		}
		return download
	case model.AccountingUpload:
		return upload
	case model.AccountingDownload:
		return download
	default:
		return SaturatingAdd(upload, download)
	}
}

// Cycle 始终从原始开户日或套餐固定日计算，短月截断不改变后续月份锚点。
// 首个周期从开户时刻开始，周期边界统一为 UTC 零点，区间左闭右开。
func Cycle(plan model.Plan, user model.User, now time.Time) (start, end time.Time) {
	now = now.UTC()
	created := user.CreatedAt.UTC()
	if now.Before(created) {
		now = created
	}
	day := 1
	switch plan.ResetMode {
	case model.ResetAnniversary:
		day = created.Day()
	case model.ResetFixedDay:
		if plan.ResetDay != nil && *plan.ResetDay >= 1 && *plan.ResetDay <= 31 {
			day = *plan.ResetDay
		}
	}
	boundary := monthDay(now.Year(), now.Month(), day)
	if now.Before(boundary) {
		start = monthDay(now.Year(), now.Month()-1, day)
		end = boundary
	} else {
		start = boundary
		end = monthDay(now.Year(), now.Month()+1, day)
	}
	if start.Before(created) {
		start = created
	}
	return start, end
}

func monthDay(year int, month time.Month, day int) time.Time {
	first := time.Date(year, month, 1, 0, 0, 0, 0, time.UTC)
	last := first.AddDate(0, 1, -1).Day()
	if day > last {
		day = last
	}
	return first.AddDate(0, 0, day-1)
}
