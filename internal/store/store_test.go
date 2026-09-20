package store

import (
	"testing"
	"time"

	"gost-webui/internal/model"
)

func TestTrafficRangeWithQuotaSince(t *testing.T) {
	dir := t.TempDir()
	st, err := Open(dir + "/t.db")
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	now := time.Now().Unix()
	hour := LocalHourStart(now)
	if err := st.AddTraffic("n1", hour, 123, 456); err != nil {
		t.Fatal(err)
	}

	// 模拟 QuotaSince 晚于小时起点
	since := now - 60
	pts, err := st.RangeTraffic("n1", since, time.Now().Unix()+1)
	if err != nil {
		t.Fatal(err)
	}
	if len(pts) != 1 || pts[0].In != 123 || pts[0].Out != 456 {
		t.Fatalf("RangeTraffic 结果异常: %+v", pts)
	}

	// 30 天窗口聚合不应越界
	pts2, _ := st.RangeTraffic("n1", now-30*86400, now+1)
	if len(pts2) != 1 {
		t.Fatalf("30 天窗口结果异常: %+v", pts2)
	}

	// model 兼容性
	_ = model.Point{}
}
