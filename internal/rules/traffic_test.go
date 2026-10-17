package rules_test

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

func waitTraffic(t *testing.T, s *rules.Service, up, down int64) model.Rule {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		r := s.List()[0]
		if r.Traffic.UploadBytes == up && r.Traffic.DownloadBytes == down {
			return r
		}
		if time.Now().After(deadline) {
			t.Fatalf("累计量应为 %d/%d，实际 %+v", up, down, r.Traffic)
		}
		time.Sleep(time.Millisecond)
	}
}

func TestTrafficToggleResetAndRestartWithLiveConnections(t *testing.T) {
	for _, protocol := range []string{model.ProtocolTCP, model.ProtocolUDP, model.ProtocolBoth} {
		t.Run(protocol, func(t *testing.T) {
			ctx := context.Background()
			s, repo := setup(t)
			target := pairedTarget(t)
			in := bothInput(combinedPort(t), target.port)
			in.Protocol = protocol
			r, err := s.Save(ctx, "", 0, in)
			if err != nil {
				t.Fatal(err)
			}
			if r.TrafficEnabled {
				t.Fatal("新规则统计默认开启")
			}
			tcp := model.HasTransport(protocol, model.ProtocolTCP)
			udp := model.HasTransport(protocol, model.ProtocolUDP)
			var tcpSend func(string)
			if tcp {
				c := ruleTCP(t, in.ListenPort)
				tcpSend = func(v string) { exchange(t, c, v) }
			}
			var udpSend func(string)
			upstream := 0
			if udp {
				c := ruleUDP(t, in.ListenPort)
				udpSend = func(v string) {
					port := combinedUDPExchange(t, c, target, v)
					if upstream != 0 && port != upstream {
						t.Fatal("开关或重置重建了UDP会话")
					}
					upstream = port
				}
			}
			send := func(v string) {
				if tcp {
					tcpSend(v)
				}
				if udp {
					udpSend(v)
				}
			}
			multiplier := int64(len(model.Transports(protocol)))
			send("off")
			waitTraffic(t, s, 0, 0)
			enabled := true
			in.TrafficEnabled = &enabled
			r, err = s.Save(ctx, r.ID, r.Revision, in)
			if err != nil {
				t.Fatal(err)
			}
			send("count")
			r = waitTraffic(t, s, 5*multiplier, 5*multiplier)
			in.TrafficEnabled = nil
			r, err = s.Save(ctx, r.ID, r.Revision, in)
			if err != nil || !r.TrafficEnabled {
				t.Fatalf("省略字段未保留开关：%+v %v", r, err)
			}
			enabled = false
			in.TrafficEnabled = &enabled
			r, err = s.Save(ctx, r.ID, r.Revision, in)
			if err != nil {
				t.Fatal(err)
			}
			send("paused")
			r = waitTraffic(t, s, 5*multiplier, 5*multiplier)
			if r.Traffic.UploadBytesPerSecond != 0 {
				t.Fatal("关闭统计速度非零")
			}
			enabled = true
			r, err = s.Save(ctx, r.ID, r.Revision, in)
			if err != nil {
				t.Fatal(err)
			}
			send("more")
			r = waitTraffic(t, s, 9*multiplier, 9*multiplier)
			before := r.Revision
			r, err = s.ResetTraffic(ctx, r.ID, r.Revision)
			if err != nil || r.Revision != before+1 || !r.TrafficEnabled {
				t.Fatalf("重置结果错误：%+v %v", r, err)
			}
			waitTraffic(t, s, 0, 0)
			send("new")
			waitTraffic(t, s, 3*multiplier, 3*multiplier)
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			rows, err := repo.List(ctx)
			if err != nil || len(rows) != 1 || rows[0].Traffic.UploadBytes != 3*multiplier {
				t.Fatalf("正常退出未保存累计量：%+v %v", rows, err)
			}
			restored, err := rules.New(ctx, repo, forward.New(forward.Limits{}), "127.0.0.1:8080")
			if err != nil {
				t.Fatal(err)
			}
			defer restored.Close()
			r = waitTraffic(t, restored, 3*multiplier, 3*multiplier)
			if !r.TrafficEnabled || r.Traffic.UploadBytesPerSecond != 0 {
				t.Fatalf("重启未恢复开关或沿用旧速度：%+v", r)
			}
			if tcp {
				exchange(t, ruleTCP(t, in.ListenPort), "r")
			}
			if udp {
				combinedUDPExchange(t, ruleUDP(t, in.ListenPort), target, "r")
			}
			waitTraffic(t, restored, 4*multiplier, 4*multiplier)
		})
	}
}

type trafficFaultRepo struct {
	*store.Store
	failReset    atomic.Bool
	failFlush    atomic.Bool
	resetEntered chan struct{}
	resetRelease chan struct{}
	flushes      atomic.Int64
}

func (r *trafficFaultRepo) ResetTraffic(ctx context.Context, v model.Rule) error {
	if r.resetEntered != nil {
		close(r.resetEntered)
		<-r.resetRelease
	}
	if r.failReset.Load() {
		return errors.New("注入重置失败")
	}
	return r.Store.ResetTraffic(ctx, v)
}
func (r *trafficFaultRepo) SaveTraffic(ctx context.Context, v map[string]model.Traffic) error {
	r.flushes.Add(1)
	if r.failFlush.Load() {
		return errors.New("注入周期保存失败")
	}
	return r.Store.SaveTraffic(ctx, v)
}
func faultTrafficService(t *testing.T) (*rules.Service, *trafficFaultRepo) {
	t.Helper()
	db, err := store.Open(filepath.Join(testutil.TempDir(t), "traffic.db"))
	if err != nil {
		t.Fatal(err)
	}
	repo := &trafficFaultRepo{Store: db}
	s, err := rules.New(context.Background(), repo, forward.New(forward.Limits{}), "127.0.0.1:8080")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close(); _ = db.Close() })
	return s, repo
}

func TestTrafficResetFailureDoesNotBlockDataOrLoseBytes(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{true: "失败回滚", false: "成功隔离"}[fail], func(t *testing.T) {
			s, repo := faultTrafficService(t)
			in := input(freePort(t), echo(t))
			enabled := true
			in.TrafficEnabled = &enabled
			r, err := s.Save(context.Background(), "", 0, in)
			if err != nil {
				t.Fatal(err)
			}
			c := ruleTCP(t, in.ListenPort)
			exchange(t, c, "before")
			r = waitTraffic(t, s, 6, 6)
			repo.resetEntered = make(chan struct{})
			repo.resetRelease = make(chan struct{})
			repo.failReset.Store(fail)
			result := make(chan error, 1)
			go func() { _, e := s.ResetTraffic(context.Background(), r.ID, r.Revision); result <- e }()
			<-repo.resetEntered
			exchange(t, c, "during")
			close(repo.resetRelease)
			err = <-result
			if fail {
				status(t, err, 500)
				r = waitTraffic(t, s, 12, 12)
				if r.Revision != 1 {
					t.Fatal("失败重置增加版本")
				}
			} else {
				if err != nil {
					t.Fatal(err)
				}
				r = waitTraffic(t, s, 6, 6)
				if r.Revision != 2 {
					t.Fatal("重置未增加版本")
				}
			}
			exchange(t, c, "after")
			want := int64(11)
			if fail {
				want = 17
			}
			r = waitTraffic(t, s, want, want)
			if err = s.Close(); err != nil {
				t.Fatal(err)
			}
			rows, err := repo.List(context.Background())
			if err != nil || rows[0].Traffic.UploadBytes != want {
				t.Fatalf("重置后的最终保存错误：%+v %v", rows, err)
			}
		})
	}
}

func TestTrafficPeriodicFlushRetriesAndResetCannotBeOverwritten(t *testing.T) {
	s, repo := faultTrafficService(t)
	in := input(freePort(t), echo(t))
	enabled := true
	in.TrafficEnabled = &enabled
	r, err := s.Save(context.Background(), "", 0, in)
	if err != nil {
		t.Fatal(err)
	}
	c := ruleTCP(t, in.ListenPort)
	exchange(t, c, "pending")
	waitTraffic(t, s, 7, 7)
	repo.failFlush.Store(true)
	deadline := time.Now().Add(7 * time.Second)
	for repo.flushes.Load() == 0 {
		if time.Now().After(deadline) {
			t.Fatal("未执行定期保存")
		}
		time.Sleep(10 * time.Millisecond)
	}
	repo.failFlush.Store(false)
	deadline = time.Now().Add(7 * time.Second)
	for {
		rows, e := repo.List(context.Background())
		if e != nil {
			t.Fatal(e)
		}
		if rows[0].Traffic.UploadBytes == 7 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("定期保存失败后未重试")
		}
		time.Sleep(10 * time.Millisecond)
	}
	r, err = s.ResetTraffic(context.Background(), r.ID, r.Revision)
	if err != nil {
		t.Fatal(err)
	}
	if err = s.Close(); err != nil {
		t.Fatal(err)
	}
	rows, err := repo.List(context.Background())
	if err != nil || rows[0].Traffic.UploadBytes != 0 {
		t.Fatalf("旧快照覆盖清零：%+v %v", rows, err)
	}
}

func TestTrafficConcurrentResetRevisionAndFinalFlushError(t *testing.T) {
	s, repo := faultTrafficService(t)
	in := input(freePort(t), echo(t))
	enabled := true
	in.TrafficEnabled = &enabled
	r, err := s.Save(context.Background(), "", 0, in)
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for range 2 {
		wg.Add(1)
		go func() { defer wg.Done(); _, e := s.ResetTraffic(context.Background(), r.ID, r.Revision); results <- e }()
	}
	wg.Wait()
	close(results)
	success := 0
	for e := range results {
		if e == nil {
			success++
		} else {
			status(t, e, 412)
		}
	}
	if success != 1 {
		t.Fatal("相同版本重置未互斥")
	}
	exchange(t, ruleTCP(t, in.ListenPort), "unsaved")
	waitTraffic(t, s, 7, 7)
	repo.failFlush.Store(true)
	if err = s.Close(); err == nil {
		t.Fatal("最终保存失败未报告")
	}
	if again := s.Close(); again != err {
		t.Fatal("重复关闭未保留最终错误")
	}
}
