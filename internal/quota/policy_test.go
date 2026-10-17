package quota

import (
	"math"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

func instant(t *testing.T, value string) time.Time {
	t.Helper()
	v, err := time.Parse(time.RFC3339Nano, value)
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func TestCycleOriginalDayUTCAndFirstCycle(t *testing.T) {
	day := 31
	for _, test := range []struct{ name, mode, created, now, start, end string }{
		{"首次开户", model.ResetCalendarMonth, "2025-01-17T15:04:05Z", "2025-01-17T15:04:05Z", "2025-01-17T15:04:05Z", "2025-02-01T00:00:00Z"},
		{"整点开户", model.ResetCalendarMonth, "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", "2025-01-01T00:00:00Z", "2025-02-01T00:00:00Z"},
		{"平年二月", model.ResetAnniversary, "2025-01-31T15:04:05Z", "2025-02-28T00:00:00Z", "2025-02-28T00:00:00Z", "2025-03-31T00:00:00Z"},
		{"三月回锚", model.ResetAnniversary, "2025-01-31T15:04:05Z", "2025-03-30T23:59:59Z", "2025-02-28T00:00:00Z", "2025-03-31T00:00:00Z"},
		{"四月截断", model.ResetAnniversary, "2025-01-31T15:04:05Z", "2025-04-30T00:00:00Z", "2025-04-30T00:00:00Z", "2025-05-31T00:00:00Z"},
		{"闰年", model.ResetFixedDay, "2023-01-01T00:00:00Z", "2024-02-29T00:00:00Z", "2024-02-29T00:00:00Z", "2024-03-31T00:00:00Z"},
		{"跨年停机", model.ResetFixedDay, "2023-01-01T00:00:00Z", "2026-01-20T00:00:00Z", "2025-12-31T00:00:00Z", "2026-01-31T00:00:00Z"},
		{"开户按UTC", model.ResetAnniversary, "2025-02-01T01:00:00+08:00", "2025-03-01T01:00:00+08:00", "2025-02-28T00:00:00Z", "2025-03-31T00:00:00Z"},
		{"本地时间无影响", model.ResetCalendarMonth, "2025-01-01T00:00:00Z", "2025-03-01T01:00:00+08:00", "2025-02-01T00:00:00Z", "2025-03-01T00:00:00Z"},
		{"时钟早于开户", model.ResetCalendarMonth, "2025-01-17T12:00:00Z", "2024-01-01T00:00:00Z", "2025-01-17T12:00:00Z", "2025-02-01T00:00:00Z"},
	} {
		t.Run(test.name, func(t *testing.T) {
			start, end := Cycle(model.Plan{ResetMode: test.mode, ResetDay: &day}, model.User{CreatedAt: instant(t, test.created)}, instant(t, test.now))
			if !start.Equal(instant(t, test.start)) || !end.Equal(instant(t, test.end)) || start.Location() != time.UTC || end.Location() != time.UTC {
				t.Fatalf("周期错误：%v — %v", start, end)
			}
		})
	}
}

func TestEveryMonthlyAnchorAndExactBoundary(t *testing.T) {
	for day := 1; day <= 31; day++ {
		for _, year := range []int{2024, 2025} {
			for month := time.January; month <= time.December; month++ {
				created := time.Date(2020, time.January, day, 8, 0, 0, 0, time.UTC)
				boundary := monthDay(year, month, day)
				plan := model.Plan{ResetMode: model.ResetAnniversary}
				beforeStart, beforeEnd := Cycle(plan, model.User{CreatedAt: created}, boundary.Add(-time.Nanosecond))
				afterStart, afterEnd := Cycle(plan, model.User{CreatedAt: created}, boundary)
				if !beforeEnd.Equal(boundary) || !afterStart.Equal(boundary) || !beforeStart.Before(boundary) || !afterEnd.After(boundary) {
					t.Fatalf("第%d日 %d-%d 边界不连贯", day, year, month)
				}
			}
		}
	}
}

func TestAccountingAggregatesAndSaturates(t *testing.T) {
	// 两条规则方向相反，单向先对用户总量聚合再取较大值。
	for mode, want := range map[string]int64{model.AccountingBoth: 220, model.AccountingMax: 110, model.AccountingUpload: 110, model.AccountingDownload: 110} {
		if got := Used(mode, 100+10, 10+100); got != want {
			t.Fatalf("%s 结果 %d，期望%d", mode, got, want)
		}
	}
	if Used(model.AccountingMax, 80, 10) != 80 || Used(model.AccountingDownload, 80, 10) != 10 || Used(model.AccountingBoth, math.MaxInt64-1, 5) != math.MaxInt64 || SaturatingAdd(math.MaxInt64, 1) != math.MaxInt64 {
		t.Fatal("计费或溢出保护错误")
	}
}
