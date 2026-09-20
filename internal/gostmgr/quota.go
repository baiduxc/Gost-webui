package gostmgr

import (
	"time"
)

// QuotaWindow 根据周期返回配额的窗口 [startsAt, expiresAt)（RFC3339，本地时区）。
// total 周期返回空字符串，表示不重置。
func QuotaWindow(period string) (startsAt, expiresAt string) {
	now := time.Now()
	switch period {
	case "daily":
		start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 0, 1)
		return start.Format(time.RFC3339), end.Format(time.RFC3339)
	case "monthly":
		start := time.Date(now.Year(), now.Month(), 1, 0, 0, 0, 0, now.Location())
		end := start.AddDate(0, 1, 0)
		return start.Format(time.RFC3339), end.Format(time.RFC3339)
	default: // total
		return "", ""
	}
}

// WindowMatches 判断配额窗口是否与目标周期一致。
func WindowMatches(q *Quota, period string) bool {
	sa, ea := QuotaWindow(period)
	return q != nil && q.StartsAt == sa && q.ExpiresAt == ea
}
