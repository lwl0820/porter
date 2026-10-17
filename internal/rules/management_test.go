package rules_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/lwl0820/porter/internal/forward"
	"github.com/lwl0820/porter/internal/rules"
	"github.com/lwl0820/porter/internal/store"
	"github.com/lwl0820/porter/internal/testutil"
)

func TestOptionalManagementPort(t *testing.T) {
	for _, address := range []string{"", "127.0.0.1:8080"} {
		t.Run(address, func(t *testing.T) {
			db, err := store.Open(filepath.Join(testutil.TempDir(t), "porter.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			s, err := rules.New(context.Background(), db, forward.New(forward.Limits{}), address)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			in := input(8080, 19080)
			enabled := false
			in.Enabled = &enabled
			_, err = s.Save(context.Background(), "", 0, in)
			if address == "" {
				if err != nil {
					t.Fatal("无Web仍保留默认管理端口", err)
				}
			} else {
				status(t, err, 409)
			}
		})
	}
}

func TestManagementAddressValidation(t *testing.T) {
	for _, address := range []string{"bad", "localhost:8080", "127.0.0.1:0", "127.0.0.1:65536"} {
		if _, err := rules.New(context.Background(), nil, forward.New(forward.Limits{}), address); err == nil {
			t.Fatal("非法管理地址未拒绝", address)
		}
	}
}
