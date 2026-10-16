package model

import "time"

const (
	ProtocolTCP  = "tcp"
	ProtocolUDP  = "udp"
	ProtocolBoth = "tcp+udp"
)

// Transports 将逻辑协议展开为底层传输，未知协议不产生任何监听。
func Transports(protocol string) []string {
	switch protocol {
	case ProtocolTCP:
		return []string{ProtocolTCP}
	case ProtocolUDP:
		return []string{ProtocolUDP}
	case ProtocolBoth:
		return []string{ProtocolTCP, ProtocolUDP}
	default:
		return nil
	}
}

func HasTransport(protocol, transport string) bool {
	for _, p := range Transports(protocol) {
		if p == transport {
			return true
		}
	}
	return false
}

// CommonTransport 返回首个交集传输，用于给出具体的端口冲突原因。
func CommonTransport(a, b string) string {
	for _, p := range Transports(a) {
		if HasTransport(b, p) {
			return p
		}
	}
	return ""
}

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
