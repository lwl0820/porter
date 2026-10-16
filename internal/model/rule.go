package model

import "time"

// Rule 同时定义持久化配置与管理接口的规则契约，运行态字段不进入数据库。
type Rule struct {
	ID            string    `json:"id"`
	Name          string    `json:"name"`
	Protocol      string    `json:"protocol"`
	ListenHost    string    `json:"listen_host"`
	ListenPort    int       `json:"listen_port"`
	TargetHost    string    `json:"target_host"`
	TargetPort    int       `json:"target_port"`
	Enabled       bool      `json:"enabled"`
	Revision      int64     `json:"revision"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	Status        string    `json:"status,omitempty"`
	LastErrorCode string    `json:"last_error_code,omitempty"`
	LastError     string    `json:"last_error,omitempty"`
}

// Input 只允许客户端设置转发配置，不接受持久化元数据和运行态。
type Input struct {
	Name       string `json:"name"`
	Protocol   string `json:"protocol"`
	ListenHost string `json:"listen_host"`
	ListenPort int    `json:"listen_port"`
	TargetHost string `json:"target_host"`
	TargetPort int    `json:"target_port"`
	Enabled    *bool  `json:"enabled"`
}
