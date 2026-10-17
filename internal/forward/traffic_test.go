package forward

import (
	"errors"
	"math"
	"sync"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

type partialWriter struct{}

func (partialWriter) Write([]byte) (int, error) { return 3, errors.New("注入部分写入") }

func TestTrafficSamplesResetAndSuccessfulBytes(t *testing.T) {
	now := time.Now()
	tr := NewTraffic(false, model.Traffic{}, now)
	tr.Add(true, 100)
	if tr.Snapshot().UploadBytes != 0 {
		t.Fatal("默认关闭仍累计")
	}
	tr.SetEnabled(true, now)
	w := trafficWriter{writer: partialWriter{}, traffic: tr, upload: true}
	if n, err := w.Write([]byte("123456")); n != 3 || err == nil {
		t.Fatal("未保留部分写入结果")
	}
	tr.Add(false, 6)
	tr.Sample(now.Add(2*time.Second), true)
	v := tr.Snapshot()
	if v.UploadBytes != 3 || v.DownloadBytes != 6 || v.UploadBytesPerSecond != 1.5 || v.DownloadBytesPerSecond != 3 {
		t.Fatalf("统计不符：%+v", v)
	}
	if tr.Snapshot() != v || tr.Snapshot() != v {
		t.Fatal("查询改变了采样")
	}
	tr.Sample(now.Add(3*time.Second), true)
	if tr.Snapshot().UploadBytesPerSecond != 0 {
		t.Fatal("空闲速度未归零")
	}
	old := tr.BeginReset(now.Add(4 * time.Second))
	tr.Add(true, 10)
	tr.RollbackReset(old)
	if tr.Snapshot().UploadBytes != 13 {
		t.Fatal("失败重置丢失增量")
	}
	tr.Sample(now.Add(5*time.Second), true)
	if tr.Snapshot().UploadBytesPerSecond != 5 {
		t.Fatalf("失败重置改变采样基线：%+v", tr.Snapshot())
	}
	tr.BeginReset(now.Add(6 * time.Second))
	tr.Add(true, 8)
	tr.Sample(now.Add(8*time.Second), true)
	if v := tr.Snapshot(); v.UploadBytes != 8 || v.DownloadBytes != 0 || v.UploadBytesPerSecond != 4 {
		t.Fatalf("重置后采样串代：%+v", v)
	}
	tr.SetEnabled(false, now.Add(9*time.Second))
	tr.Add(true, 100)
	if v := tr.Snapshot(); v.UploadBytes != 8 || v.UploadBytesPerSecond != 0 {
		t.Fatalf("关闭统计仍累计：%+v", v)
	}
	tr.SetEnabled(true, now.Add(10*time.Second))
	tr.Add(true, 2)
	tr.Sample(now.Add(11*time.Second), false)
	if v := tr.Snapshot(); v.UploadBytes != 10 || v.UploadBytesPerSecond != 0 {
		t.Fatalf("停用规则速度未归零：%+v", v)
	}
}

func TestTrafficConcurrentCountingAndSaturation(t *testing.T) {
	tr := NewTraffic(true, model.Traffic{}, time.Now())
	var wg sync.WaitGroup
	for range 16 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for range 1000 {
				tr.Add(true, 3)
				tr.Add(false, 2)
				_ = tr.Snapshot()
			}
		}()
	}
	wg.Wait()
	if v := tr.Snapshot(); v.UploadBytes != 48000 || v.DownloadBytes != 32000 {
		t.Fatalf("并发计数丢失：%+v", v)
	}
	tr = NewTraffic(true, model.Traffic{UploadBytes: math.MaxInt64 - 1}, time.Now())
	tr.Add(true, 20)
	if tr.Snapshot().UploadBytes != math.MaxInt64 {
		t.Fatal("累计量溢出为负数")
	}
}
