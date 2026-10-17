// Package localapi 提供与 Web 分离、仅 root 对端可用的规则控制服务。
package localapi

import "github.com/lwl0820/porter/internal/controlapi"

type Error = controlapi.Error
type RuleList = controlapi.RuleList

const resource = controlapi.Resource

var revision = controlapi.Revision
