package model

import "time"

const (
	AdminID            = "system-admin"
	RoleAdmin          = "admin"
	RoleUser           = "user"
	AccountingBoth     = "both"
	AccountingMax      = "max"
	AccountingUpload   = "upload"
	AccountingDownload = "download"
	ResetCalendarMonth = "calendar_month"
	ResetAnniversary   = "anniversary"
	ResetFixedDay      = "fixed_day"
)

// User 不向接口暴露密码派生结果。
type User struct {
	ID           string    `json:"id"`
	Username     string    `json:"username"`
	Role         string    `json:"role"`
	PlanID       *string   `json:"plan_id"`
	PortStart    int       `json:"port_start"`
	PortEnd      int       `json:"port_end"`
	CreatedAt    time.Time `json:"created_at"`
	PasswordHash string    `json:"-"`
}

type Owner struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Role     string `json:"role"`
}

func (u User) Owner() Owner { return Owner{ID: u.ID, Username: u.Username, Role: u.Role} }

type Plan struct {
	ID                string    `json:"id"`
	Name              string    `json:"name"`
	PortCount         int       `json:"port_count"`
	TrafficLimitBytes int64     `json:"traffic_limit_bytes,string"`
	AccountingMode    string    `json:"accounting_mode"`
	ResetMode         string    `json:"reset_mode"`
	ResetDay          *int      `json:"reset_day"`
	CreatedAt         time.Time `json:"created_at"`
}

type PlanInput struct {
	Name              string `json:"name"`
	PortCount         int    `json:"port_count"`
	TrafficLimitBytes int64  `json:"traffic_limit_bytes,string"`
	AccountingMode    string `json:"accounting_mode"`
	ResetMode         string `json:"reset_mode"`
	ResetDay          *int   `json:"reset_day"`
}

type UserInput struct {
	Username string `json:"username"`
	Password string `json:"password"`
	PlanID   string `json:"plan_id"`
}

type Settings struct {
	PortStart int   `json:"port_start"`
	PortEnd   int   `json:"port_end"`
	Revision  int64 `json:"revision"`
}

type SettingsInput struct {
	PortStart int `json:"port_start"`
	PortEnd   int `json:"port_end"`
}

// Usage 的周期版本只供持久化比较，字节始终使用精确十进制字符串输出。
type Usage struct {
	UserID        string    `json:"-"`
	CycleStart    time.Time `json:"cycle_start"`
	CycleEnd      time.Time `json:"cycle_end"`
	UploadBytes   int64     `json:"upload_bytes,string"`
	DownloadBytes int64     `json:"download_bytes,string"`
	UsedBytes     int64     `json:"used_bytes,string"`
	LimitBytes    int64     `json:"limit_bytes,string"`
	Exceeded      bool      `json:"exceeded"`
	Version       int64     `json:"-"`
}

type Account struct {
	User  User   `json:"user"`
	Plan  *Plan  `json:"plan"`
	Usage *Usage `json:"usage"`
}

type AdminRuleCreateInput struct {
	Input
	OwnerID string `json:"owner_id"`
}
