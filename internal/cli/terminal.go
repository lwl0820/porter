package cli

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
)

type terminal struct {
	file   *os.File
	reader *bufio.Reader
	ctx    context.Context
	once   sync.Once
}

func openTerminal(ctx context.Context) (*terminal, error) {
	f, err := os.OpenFile("/dev/tty", os.O_RDWR, 0)
	if err != nil {
		return nil, err
	}
	return &terminal{file: f, reader: bufio.NewReader(f), ctx: ctx}, nil
}
func (t *terminal) Close() { t.once.Do(func() { _ = t.file.Close() }) }
func (t *terminal) read(prompt string) (string, error) {
	if t.ctx.Err() != nil {
		return "", io.EOF
	}
	fmt.Fprint(t.file, prompt)
	type result struct {
		line string
		err  error
	}
	ready := make(chan result, 1)
	go func() { line, err := t.reader.ReadString('\n'); ready <- result{line, err} }()
	select {
	case <-t.ctx.Done():
		return "", io.EOF
	case got := <-ready:
		// 输入结束时丢弃尚未确认的半行内容，不能把它当作一次提交。
		if got.err != nil {
			return "", io.EOF
		}
		return strings.TrimSpace(got.line), nil
	}
}
func (t *terminal) language() (string, error) {
	for {
		value, err := t.read("语言 / Language\n1 中文\n2 English\n> ")
		if err != nil {
			return "", err
		}
		switch value {
		case "1":
			return "zh-CN", nil
		case "2":
			return "en", nil
		case "q", "cancel":
			return "", io.EOF
		}
		fmt.Fprintln(t.file, "请选择 1 或 2；q 取消。 / Choose 1 or 2; q to cancel.")
	}
}
