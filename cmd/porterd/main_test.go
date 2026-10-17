package main

import (
	"io"
	"testing"
)

func TestServerArgumentsRejectClientCommands(t *testing.T) {
	for _, args := range [][]string{{"rules"}, {"rules", "--lang", "en"}, {"--unknown"}, {"--version", "rules"}} {
		if _, err := parseArguments(args, io.Discard); err == nil {
			t.Fatalf("服务接收客户端参数：%v", args)
		}
	}
	for _, arg := range []string{"--version", "-version"} {
		if version, err := parseArguments([]string{arg}, io.Discard); !version || err != nil {
			t.Fatal(version, err)
		}
	}
}
