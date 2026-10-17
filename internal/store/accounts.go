package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/quota"
)

var (
	ErrNotFound      = errors.New("记录不存在")
	ErrConflict      = errors.New("记录冲突")
	ErrPortPoolUnset = errors.New("尚未配置用户端口分配范围")
	ErrPortExhausted = errors.New("没有足够的连续可用端口")
	ErrRevision      = errors.New("记录版本已改变")
)

type scanner interface{ Scan(...any) error }

const userColumns = "id,username,role,password_hash,plan_id,port_start,port_end,created_at"
const planColumns = "id,name,port_count,traffic_limit_bytes,accounting_mode,reset_mode,reset_day,created_at"

func scanUser(row scanner) (model.User, error) {
	var u model.User
	var created string
	err := row.Scan(&u.ID, &u.Username, &u.Role, &u.PasswordHash, &u.PlanID, &u.PortStart, &u.PortEnd, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return u, ErrNotFound
	}
	if err == nil {
		u.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	}
	return u, err
}

func scanPlan(row scanner) (model.Plan, error) {
	var p model.Plan
	var created string
	err := row.Scan(&p.ID, &p.Name, &p.PortCount, &p.TrafficLimitBytes, &p.AccountingMode, &p.ResetMode, &p.ResetDay, &created)
	if errors.Is(err, sql.ErrNoRows) {
		return p, ErrNotFound
	}
	if err == nil {
		p.CreatedAt, err = time.Parse(time.RFC3339Nano, created)
	}
	return p, err
}

func conflict(err error) error {
	if err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed") {
		return fmt.Errorf("%w：%v", ErrConflict, err)
	}
	return err
}

// EnsureAdmin 保持固定身份，环境变量改名绝不接管普通用户。
func (s *Store) EnsureAdmin(ctx context.Context, username string) (model.User, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return model.User{}, fmt.Errorf("管理员账号不能为空")
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return model.User{}, err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, "UPDATE users SET username=? WHERE id=? AND role='admin'", username, model.AdminID); err != nil {
		return model.User{}, conflict(err)
	}
	u, err := scanUser(tx.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id=?", model.AdminID))
	if err != nil {
		return u, err
	}
	return u, tx.Commit()
}

func (s *Store) GetUser(ctx context.Context, id string) (model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE id=?", id))
}
func (s *Store) UserByUsername(ctx context.Context, username string) (model.User, error) {
	return scanUser(s.db.QueryRowContext(ctx, "SELECT "+userColumns+" FROM users WHERE username=?", username))
}
func (s *Store) ListUsers(ctx context.Context) ([]model.User, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+userColumns+" FROM users ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.User, 0)
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}
func (s *Store) GetPlan(ctx context.Context, id string) (model.Plan, error) {
	return scanPlan(s.db.QueryRowContext(ctx, "SELECT "+planColumns+" FROM plans WHERE id=?", id))
}
func (s *Store) ListPlans(ctx context.Context) ([]model.Plan, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT "+planColumns+" FROM plans ORDER BY created_at,id")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Plan, 0)
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}
func (s *Store) CreatePlan(ctx context.Context, p model.Plan) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO plans("+planColumns+") VALUES(?,?,?,?,?,?,?,?)", p.ID, p.Name, p.PortCount, p.TrafficLimitBytes, p.AccountingMode, p.ResetMode, p.ResetDay, p.CreatedAt.UTC().Format(time.RFC3339Nano))
	return conflict(err)
}
func (s *Store) Settings(ctx context.Context) (model.Settings, error) {
	var value model.Settings
	err := s.db.QueryRowContext(ctx, "SELECT port_start,port_end,revision FROM system_settings WHERE id=1").Scan(&value.PortStart, &value.PortEnd, &value.Revision)
	return value, err
}
func (s *Store) SaveSettings(ctx context.Context, value model.Settings) error {
	result, err := s.db.ExecContext(ctx, "UPDATE system_settings SET port_start=?,port_end=?,revision=?,updated_at=? WHERE id=1 AND revision=?", value.PortStart, value.PortEnd, value.Revision, time.Now().UTC().Format(time.RFC3339Nano), value.Revision-1)
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrRevision
	}
	return nil
}

// CreateUser 在单事务内分配连续数字端口、创建账号及首个周期，失败不残留分配。
// 调用方与规则写操作共用服务操作锁，防止管理员新增规则穿透分配快照。
func (s *Store) CreateUser(ctx context.Context, u model.User, usage model.Usage, managementPort int) (model.User, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return u, err
	}
	defer tx.Rollback()
	if u.PlanID == nil {
		return u, fmt.Errorf("必须选择套餐")
	}
	p, err := scanPlan(tx.QueryRowContext(ctx, "SELECT "+planColumns+" FROM plans WHERE id=?", *u.PlanID))
	if err != nil {
		return u, err
	}
	var start, end int
	if err = tx.QueryRowContext(ctx, "SELECT port_start,port_end FROM system_settings WHERE id=1").Scan(&start, &end); err != nil {
		return u, err
	}
	if start == 0 {
		return u, ErrPortPoolUnset
	}
	rows, err := tx.QueryContext(ctx, "SELECT port FROM user_ports UNION SELECT listen_port FROM rules WHERE owner_id=?", model.AdminID)
	if err != nil {
		return u, err
	}
	occupied := make(map[int]bool)
	occupied[managementPort] = true
	for rows.Next() {
		var port int
		if err = rows.Scan(&port); err != nil {
			rows.Close()
			return u, err
		}
		occupied[port] = true
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return u, err
	}
	length := 0
	for port := start; port <= end; port++ {
		if occupied[port] {
			length = 0
		} else {
			length++
		}
		if length == p.PortCount {
			u.PortStart = port - length + 1
			u.PortEnd = port
			break
		}
	}
	if length != p.PortCount {
		return u, ErrPortExhausted
	}
	u.Role = model.RoleUser
	u.CreatedAt = u.CreatedAt.UTC()
	if _, err = tx.ExecContext(ctx, "INSERT INTO users("+userColumns+") VALUES(?,?,?,?,?,?,?,?)", u.ID, u.Username, u.Role, u.PasswordHash, u.PlanID, u.PortStart, u.PortEnd, u.CreatedAt.Format(time.RFC3339Nano)); err != nil {
		return u, conflict(err)
	}
	for port := u.PortStart; port <= u.PortEnd; port++ {
		if _, err = tx.ExecContext(ctx, "INSERT INTO user_ports(port,user_id) VALUES(?,?)", port, u.ID); err != nil {
			return u, err
		}
	}
	usage.UserID = u.ID
	usage.CycleStart, usage.CycleEnd = quota.Cycle(p, u, u.CreatedAt)
	if _, err = tx.ExecContext(ctx, "INSERT INTO user_usage(user_id,cycle_start,cycle_end,upload_bytes,download_bytes,version) VALUES(?,?,?,0,0,1)", u.ID, usageTime(usage.CycleStart), usageTime(usage.CycleEnd)); err != nil {
		return u, err
	}
	return u, tx.Commit()
}

func (s *Store) ListUsages(ctx context.Context) ([]model.Usage, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT u.user_id,u.cycle_start,u.cycle_end,u.upload_bytes,u.download_bytes,u.version,p.traffic_limit_bytes,p.accounting_mode FROM user_usage u JOIN users a ON a.id=u.user_id JOIN plans p ON p.id=a.plan_id ORDER BY u.user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]model.Usage, 0)
	for rows.Next() {
		var u model.Usage
		var start, end, mode string
		if err = rows.Scan(&u.UserID, &start, &end, &u.UploadBytes, &u.DownloadBytes, &u.Version, &u.LimitBytes, &mode); err != nil {
			return nil, err
		}
		if u.CycleStart, err = time.Parse(time.RFC3339Nano, start); err != nil {
			return nil, err
		}
		if u.CycleEnd, err = time.Parse(time.RFC3339Nano, end); err != nil {
			return nil, err
		}
		u.UsedBytes = quota.Used(mode, u.UploadBytes, u.DownloadBytes)
		u.Exceeded = u.UsedBytes >= u.LimitBytes
		out = append(out, u)
	}
	return out, rows.Err()
}

// SaveUsages 拒绝旧周期快照；同周期计数只增不减，失败保留整个批次。
func (s *Store) SaveUsages(ctx context.Context, values []model.Usage) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, u := range values {
		result, err := tx.ExecContext(ctx, `UPDATE user_usage SET upload_bytes=MAX(upload_bytes,?),download_bytes=MAX(download_bytes,?) WHERE user_id=? AND cycle_start=? AND version=?`, u.UploadBytes, u.DownloadBytes, u.UserID, usageTime(u.CycleStart), u.Version)
		if err != nil {
			return err
		}
		if n, err := result.RowsAffected(); err != nil {
			return err
		} else if n != 1 {
			return ErrRevision
		}
	}
	return tx.Commit()
}

// ResetUsage 用周期版本比较提交，防止重复清零和系统时钟回退发放旧额度。
func (s *Store) ResetUsage(ctx context.Context, u model.Usage) error {
	result, err := s.db.ExecContext(ctx, `UPDATE user_usage SET cycle_start=?,cycle_end=?,upload_bytes=?,download_bytes=?,version=? WHERE user_id=? AND version=? AND cycle_start<?`, usageTime(u.CycleStart), usageTime(u.CycleEnd), u.UploadBytes, u.DownloadBytes, u.Version, u.UserID, u.Version-1, usageTime(u.CycleStart))
	if err != nil {
		return err
	}
	if n, err := result.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrRevision
	}
	return nil
}

// usageTime 固定纳秒位数，使 SQLite 文本比较精确覆盖紧邻零点的首次周期。
func usageTime(value time.Time) string { return value.UTC().Format("2006-01-02T15:04:05.000000000Z") }
