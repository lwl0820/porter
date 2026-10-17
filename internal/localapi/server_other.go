//go:build !linux

package localapi

import (
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/rules"
)

type Server struct{}

func Start(string, *rules.Service, model.User) (*Server, error) { return nil, nil }
func (*Server) Close() error                                    { return nil }
