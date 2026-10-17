package forward

import (
	"context"
	"io"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

type trafficState struct {
	value   model.Traffic
	sampled model.Traffic
	at      time.Time
}

// Traffic 独立于监听器及会话生命周期，同一逻辑规则共享一个实例。
// 数据路径只取得此处的短锁，数据库操作不持有该锁。
type Traffic struct {
	enabled atomic.Bool
	mu      sync.Mutex
	state   trafficState
}

func NewTraffic(enabled bool, value model.Traffic, now time.Time) *Traffic {
	value.UploadBytesPerSecond, value.DownloadBytesPerSecond = 0, 0
	t := &Traffic{state: trafficState{value: value, sampled: value, at: now}}
	t.enabled.Store(enabled)
	return t
}

func saturated(a, b int64) int64 {
	if b > math.MaxInt64-a {
		return math.MaxInt64
	}
	return a + b
}

func (t *Traffic) Add(upload bool, n int) {
	if t == nil || n <= 0 || !t.enabled.Load() {
		return
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.enabled.Load() {
		return
	}
	if upload {
		t.state.value.UploadBytes = saturated(t.state.value.UploadBytes, int64(n))
	} else {
		t.state.value.DownloadBytes = saturated(t.state.value.DownloadBytes, int64(n))
	}
}

func (t *Traffic) SetEnabled(enabled bool, now time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.enabled.Load() == enabled {
		return
	}
	t.enabled.Store(enabled)
	t.state.value.UploadBytesPerSecond, t.state.value.DownloadBytesPerSecond = 0, 0
	t.state.sampled, t.state.at = t.state.value, now
}

func (t *Traffic) Snapshot() model.Traffic {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.state.value
}

func (t *Traffic) Sample(now time.Time, active bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	v := &t.state.value
	elapsed := now.Sub(t.state.at).Seconds()
	if elapsed <= 0 {
		return
	}
	v.UploadBytesPerSecond, v.DownloadBytesPerSecond = 0, 0
	if active && t.enabled.Load() {
		v.UploadBytesPerSecond = float64(v.UploadBytes-t.state.sampled.UploadBytes) / elapsed
		v.DownloadBytesPerSecond = float64(v.DownloadBytes-t.state.sampled.DownloadBytes) / elapsed
	}
	t.state.sampled, t.state.at = *v, now
}

// BeginReset 将后续计数切换到新一代，返回只供失败回滚的旧状态。
func (t *Traffic) BeginReset(now time.Time) trafficState {
	t.mu.Lock()
	defer t.mu.Unlock()
	old := t.state
	t.state = trafficState{at: now}
	return old
}

func (t *Traffic) RollbackReset(old trafficState) {
	t.mu.Lock()
	defer t.mu.Unlock()
	old.value.UploadBytes = saturated(old.value.UploadBytes, t.state.value.UploadBytes)
	old.value.DownloadBytes = saturated(old.value.DownloadBytes, t.state.value.DownloadBytes)
	t.state = old
}

type trafficWriter struct {
	writer  io.Writer
	ctx     context.Context
	meter   Meter
	traffic *Traffic
	upload  bool
}

func (w trafficWriter) Write(p []byte) (int, error) {
	if w.meter != nil {
		if err := w.meter.Wait(w.ctx); err != nil {
			return 0, err
		}
	}
	n, err := w.writer.Write(p)
	if w.meter != nil {
		w.meter.Add(w.upload, n)
	}
	w.traffic.Add(w.upload, n)
	return n, err
}
