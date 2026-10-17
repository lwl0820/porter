// Package cli 实现共用本机规则服务的交互及参数入口。
package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/lwl0820/porter/internal/localapi"
	"github.com/lwl0820/porter/internal/model"
	"github.com/lwl0820/porter/internal/native"
)

type ruleAPI interface {
	List(context.Context, string) ([]model.Rule, error)
	Get(context.Context, string) (model.Rule, error)
	Create(context.Context, model.AdminRuleCreateInput) (model.Rule, error)
	Update(context.Context, string, int64, model.Input) (model.Rule, error)
	Delete(context.Context, string, int64) error
}
type options struct {
	action, id string
	values     map[string]string
	json, help bool
	revision   int64
}
type runner struct {
	lang     string
	out, err io.Writer
	api      ruleAPI
	tty      *terminal
	ctx      context.Context
}

// Run 必须在服务配置、日志和数据库初始化前调用。
func Run(ctx context.Context, args []string, out, stderr io.Writer) int {
	r := &runner{ctx: ctx, out: out, err: stderr}
	validEntry := len(args) > 0 && (args[0] == "rules" || args[0] == "--help" || args[0] == "-h")
	if len(args) > 0 && args[0] == "rules" {
		args = args[1:]
	}
	lang, jsonMode, langErr := languageOption(args)
	r.lang = lang
	var err error
	if langErr != nil {
		return r.fail(langErr, jsonMode)
	}
	if r.lang == "" {
		r.tty, err = openTerminal(ctx)
		if err != nil {
			return r.fail(cliError("language_required"), jsonMode)
		}
		defer r.tty.Close()
		r.lang, err = r.tty.language()
		if err != nil {
			return 0
		}
	}
	if !validEntry {
		detail := ""
		if len(args) > 0 {
			detail = args[0]
		}
		return r.fail(usage(detail), jsonMode)
	}
	opts, err := parse(args)
	if err != nil {
		return r.fail(err, jsonMode)
	}
	if opts.help {
		r.help()
		return 0
	}
	if opts.action == "" && opts.json {
		return r.fail(cliError("menu_json"), true)
	}
	if !native.Available() {
		return r.fail(cliError("native_only"), opts.json)
	}
	if os.Geteuid() != 0 {
		return r.fail(cliError("root_required"), opts.json)
	}
	path := opts.values["socket"]
	if path == "" {
		path = os.Getenv("PORTER_CONTROL_SOCKET")
	}
	if path == "" {
		path = localapi.DefaultSocket
	}
	r.api = localapi.NewClient(path)
	if opts.action == "" {
		if r.tty == nil {
			r.tty, err = openTerminal(ctx)
			if err != nil {
				return r.fail(cliError("terminal_required"), false)
			}
			defer r.tty.Close()
		}
		if err = r.menu(); err != nil && !errors.Is(err, io.EOF) && ctx.Err() == nil {
			return r.fail(err, false)
		}
		return 0
	}
	value, err := r.execute(opts)
	if err != nil {
		return r.fail(err, opts.json)
	}
	if opts.json {
		if err = json.NewEncoder(out).Encode(r.localizeResult(value)); err != nil {
			return 1
		}
		return 0
	}
	r.render(out, value)
	return 0
}

var valueFlags = map[string]bool{"lang": true, "socket": true, "name": true, "protocol": true, "listen-host": true, "listen-port": true, "target-host": true, "target-port": true, "owner-id": true, "revision": true}
var boolFlags = map[string]bool{"enabled": true, "traffic-enabled": true}

func splitOption(arg string) (string, string, bool) {
	key, value, eq := strings.Cut(strings.TrimPrefix(arg, "--"), "=")
	return key, value, eq
}

// 预扫描遵循已知参数的消费规则，字段值中的 --lang 不参与语言选择。
func languageOption(args []string) (lang string, jsonMode bool, err error) {
	for i := 0; i < len(args); i++ {
		if !strings.HasPrefix(args[i], "--") {
			continue
		}
		key, value, eq := splitOption(args[i])
		if key == "json" && (!eq || value == "true") {
			jsonMode = true
		}
		if valueFlags[key] && !eq {
			if i+1 < len(args) {
				i++
				value = args[i]
			} else if key == "lang" {
				err = cliError("language_invalid")
			}
		}
		if key == "lang" {
			if value != "zh-CN" && value != "en" {
				err = cliError("language_invalid")
			} else if lang != "" {
				err = cliError("language_invalid")
			} else {
				lang = value
			}
		}
	}
	return
}
func parse(args []string) (options, error) {
	o := options{values: map[string]string{}}
	positions := []string{}
	seen := map[string]bool{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "-h" {
			o.help = true
			continue
		}
		if !strings.HasPrefix(arg, "--") {
			if strings.HasPrefix(arg, "-") {
				return o, usage(arg)
			}
			positions = append(positions, arg)
			continue
		}
		key, value, eq := splitOption(arg)
		if seen[key] {
			return o, usage("--" + key)
		}
		seen[key] = true
		switch {
		case key == "json" || key == "help":
			if eq {
				return o, usage("--" + key)
			}
			if key == "json" {
				o.json = true
			} else {
				o.help = true
			}
		case valueFlags[key]:
			if !eq {
				if i+1 >= len(args) {
					return o, usage("--" + key)
				}
				i++
				value = args[i]
			}
			o.values[key] = value
		case boolFlags[key]:
			if !eq || (value != "true" && value != "false") {
				return o, usage("--" + key + "=true|false")
			}
			o.values[key] = value
		default:
			return o, usage("--" + key)
		}
	}
	if len(positions) > 0 {
		o.action = positions[0]
	}
	if o.help {
		return o, nil
	}
	switch o.action {
	case "":
		if len(positions) > 0 {
			return o, usage("")
		}
	case "add", "list":
		if len(positions) != 1 {
			return o, usage(o.action)
		}
	case "get", "update", "delete":
		if len(positions) != 2 || positions[1] == "" || strings.ContainsAny(positions[1], "/\\?#") {
			return o, usage("ID")
		}
		o.id = positions[1]
	default:
		return o, usage(o.action)
	}
	allowed := map[string]bool{"lang": true, "socket": true}
	switch o.action {
	case "add", "update":
		for key := range valueFlags {
			if key != "owner-id" && key != "revision" {
				allowed[key] = true
			}
		}
		for key := range boolFlags {
			allowed[key] = true
		}
	}
	if o.action == "add" || o.action == "list" {
		allowed["owner-id"] = true
	}
	if o.action == "update" || o.action == "delete" {
		allowed["revision"] = true
	}
	for key := range o.values {
		if !allowed[key] {
			return o, usage("--" + key)
		}
	}
	if value, ok := o.values["revision"]; ok {
		var e error
		o.revision, e = strconv.ParseInt(value, 10, 64)
		if e != nil || o.revision < 1 {
			return o, usage("--revision")
		}
	}
	if o.action == "add" {
		for _, key := range configFields {
			if strings.TrimSpace(o.values[key]) == "" {
				return o, usage("--" + key)
			}
		}
	}
	if o.action == "update" {
		changed := false
		for _, key := range editableFields {
			if _, ok := o.values[key]; ok {
				changed = true
			}
		}
		if !changed {
			return o, cliError("no_changes")
		}
	}
	for _, key := range []string{"listen-port", "target-port"} {
		if value, ok := o.values[key]; ok {
			n, e := strconv.Atoi(value)
			if e != nil || n < 1 || n > 65535 {
				return o, usage("--" + key + " (1–65535)")
			}
		}
	}
	return o, nil
}

var configFields = []string{"name", "protocol", "listen-host", "listen-port", "target-host", "target-port"}
var editableFields = []string{"name", "protocol", "listen-host", "listen-port", "target-host", "target-port", "enabled", "traffic-enabled"}

func merge(in model.Input, values map[string]string) model.Input {
	for key, value := range values {
		switch key {
		case "name":
			in.Name = value
		case "protocol":
			in.Protocol = value
		case "listen-host":
			in.ListenHost = value
		case "target-host":
			in.TargetHost = value
		case "listen-port":
			in.ListenPort, _ = strconv.Atoi(value)
		case "target-port":
			in.TargetPort, _ = strconv.Atoi(value)
		case "enabled":
			b := value == "true"
			in.Enabled = &b
		case "traffic-enabled":
			b := value == "true"
			in.TrafficEnabled = &b
		}
	}
	return in
}
func inputOf(v model.Rule) model.Input {
	return model.Input{Name: v.Name, Protocol: v.Protocol, ListenHost: v.ListenHost, ListenPort: v.ListenPort, TargetHost: v.TargetHost, TargetPort: v.TargetPort, Enabled: &v.Enabled, TrafficEnabled: &v.TrafficEnabled}
}
func (r *runner) execute(o options) (any, error) {
	switch o.action {
	case "list":
		items, err := r.api.List(r.ctx, o.values["owner-id"])
		return localapi.RuleList{Rules: items}, err
	case "get":
		return r.api.Get(r.ctx, o.id)
	case "add":
		return r.api.Create(r.ctx, model.AdminRuleCreateInput{Input: merge(model.Input{}, o.values), OwnerID: o.values["owner-id"]})
	case "update", "delete":
		old, err := r.api.Get(r.ctx, o.id)
		if err != nil {
			return nil, err
		}
		rev := old.Revision
		if o.revision > 0 {
			rev = o.revision
		}
		if o.action == "update" {
			return r.api.Update(r.ctx, o.id, rev, merge(inputOf(old), o.values))
		}
		err = r.api.Delete(r.ctx, o.id, rev)
		return map[string]any{"deleted": o.id}, err
	}
	return nil, usage(o.action)
}
func cliError(code string) *localapi.Error { return &localapi.Error{Code: code} }
func usage(detail string) *localapi.Error {
	return &localapi.Error{Code: "usage_error", Message: detail}
}
func (r *runner) fail(err error, jsonMode bool) int {
	var problem *localapi.Error
	if !errors.As(err, &problem) {
		problem = cliError("internal_error")
	}
	message := r.errorMessage(problem)
	fields := map[string]string{}
	for key := range problem.Fields {
		fields[key] = r.fieldError(key)
	}
	if jsonMode {
		_ = json.NewEncoder(r.err).Encode(map[string]any{"error": struct {
			Code    string            `json:"code"`
			Message string            `json:"message"`
			Fields  map[string]string `json:"fields,omitempty"`
		}{problem.Code, message, fields}})
	} else {
		fmt.Fprintf(r.err, "%s [%s]\n", message, problem.Code)
		for _, key := range []string{"name", "protocol", "listen_host", "listen_port", "target_host", "target_port", "enabled", "traffic_enabled"} {
			if value, ok := fields[key]; ok {
				fmt.Fprintf(r.err, "  %s: %s\n", key, value)
			}
		}
	}
	switch problem.Code {
	case "usage_error", "no_changes", "language_required", "language_invalid", "menu_json", "terminal_required":
		return 2
	}
	return 1
}
func (r *runner) text(zh, en string) string {
	if r.lang == "en" {
		return en
	}
	return zh
}

func (r *runner) localizeResult(value any) any {
	localize := func(v model.Rule) model.Rule {
		if v.LastErrorCode != "" {
			v.LastError = r.errorMessage(&localapi.Error{Code: v.LastErrorCode})
		}
		return v
	}
	switch v := value.(type) {
	case model.Rule:
		return localize(v)
	case localapi.RuleList:
		for i := range v.Rules {
			v.Rules[i] = localize(v.Rules[i])
		}
		return v
	default:
		return value
	}
}
