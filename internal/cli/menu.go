package cli

import (
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/lwl0820/porter/internal/controlapi"
	"github.com/lwl0820/porter/internal/model"
)

var errCancel = errors.New("已取消")

func (r *runner) menu() error {
	view := *r
	view.out = r.tty.file
	view.err = r.tty.file
	for {
		choice, err := r.tty.read(r.text("\n规则管理\n1 创建\n2 列表\n3 详情\n4 修改\n5 删除\n6 切换语言\n0 退出\n> ", "\nRule management\n1 Create\n2 List\n3 Details\n4 Update\n5 Delete\n6 Change language\n0 Exit\n> "))
		if err != nil {
			return err
		}
		switch choice {
		case "0", "q":
			return nil
		case "6":
			var language string
			language, err = r.tty.language()
			if err == nil {
				r.lang = language
				r.lang = r.saveLanguage(language, true)
				view.lang = r.lang
			}
		case "1":
			err = view.createWizard()
		case "2":
			var items []model.Rule
			items, err = r.api.List(r.ctx, "")
			if err == nil {
				view.table(r.tty.file, items)
			}
		case "3", "4", "5":
			var selected model.Rule
			selected, err = view.selectRule()
			if err == nil {
				view.detail(r.tty.file, selected)
				switch choice {
				case "4":
					err = view.editWizard(selected)
				case "5":
					err = view.deleteWizard(selected)
				}
			}
		default:
			fmt.Fprintln(r.tty.file, r.text("序号无效，请重新选择。", "Invalid number; choose again."))
		}
		if errors.Is(err, io.EOF) {
			return err
		}
		if errors.Is(err, errCancel) {
			fmt.Fprintln(r.tty.file, r.text("已取消。", "Cancelled."))
			continue
		}
		if err != nil {
			view.fail(err, false)
		}
	}
}
func (r *runner) selectRule() (model.Rule, error) {
	items, err := r.api.List(r.ctx, "")
	if err != nil {
		return model.Rule{}, err
	}
	r.table(r.out, items)
	if len(items) == 0 {
		return model.Rule{}, errCancel
	}
	for {
		value, err := r.tty.read(r.text("选择序号（0 返回）：", "Choose a number (0 to return): "))
		if err != nil {
			return model.Rule{}, err
		}
		if value == "0" || value == "/cancel" {
			return model.Rule{}, errCancel
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > len(items) {
			fmt.Fprintln(r.out, r.text("序号无效。", "Invalid number."))
			continue
		}
		return r.api.Get(r.ctx, items[n-1].ID)
	}
}
func (r *runner) ask(key, current string, required bool) (string, error) {
	labels := map[string][2]string{
		"name": {"名称", "Name"}, "protocol": {"协议 tcp/udp/tcp+udp", "Protocol tcp/udp/tcp+udp"},
		"listen-host": {"监听 IP", "Listen IP"}, "listen-port": {"监听端口", "Listen port"},
		"target-host": {"目标 IP 或域名", "Target IP or hostname"}, "target-port": {"目标端口", "Target port"},
		"enabled": {"启用 true/false", "Enabled true/false"}, "traffic-enabled": {"展示统计 true/false", "Traffic display true/false"},
		"owner-id": {"归属用户 ID（留空为管理员）", "Owner ID (blank for administrator)"},
	}
	label := r.text(labels[key][0], labels[key][1])
	for {
		prompt := label
		if current != "" {
			prompt += fmt.Sprintf(" [%s]", strconv.Quote(current))
		}
		value, err := r.tty.read(prompt + ": ")
		if err != nil {
			return "", err
		}
		if value == "/cancel" {
			return "", errCancel
		}
		if value == "" {
			value = current
		}
		valid := true
		if required && value == "" {
			valid = false
		}
		switch key {
		case "listen-port", "target-port":
			n, err := strconv.Atoi(value)
			valid = err == nil && n >= 1 && n <= 65535
		case "enabled", "traffic-enabled":
			valid = value == "true" || value == "false"
		case "protocol":
			valid = len(model.Transports(value)) > 0
		}
		if !valid {
			fmt.Fprintln(r.out, r.text("输入无效，请重新输入；/cancel 返回。", "Invalid value; try again, or /cancel to return."))
			continue
		}
		return value, nil
	}
}
func valuesOf(v model.Input) map[string]string {
	enabled, traffic := true, false
	if v.Enabled != nil {
		enabled = *v.Enabled
	}
	if v.TrafficEnabled != nil {
		traffic = *v.TrafficEnabled
	}
	values := map[string]string{"name": v.Name, "protocol": v.Protocol, "listen-host": v.ListenHost, "target-host": v.TargetHost, "enabled": strconv.FormatBool(enabled), "traffic-enabled": strconv.FormatBool(traffic)}
	if v.ListenPort != 0 {
		values["listen-port"] = strconv.Itoa(v.ListenPort)
	}
	if v.TargetPort != 0 {
		values["target-port"] = strconv.Itoa(v.TargetPort)
	}
	return values
}
func (r *runner) collect(in model.Input) (model.Input, error) {
	values := valuesOf(in)
	fmt.Fprintln(r.out, r.text("留空保留显示值；随时输入 /cancel 返回。", "Leave blank to keep the displayed value; enter /cancel to return at any prompt."))
	for _, key := range editableFields {
		value, err := r.ask(key, values[key], true)
		if err != nil {
			return in, err
		}
		values[key] = value
	}
	return merge(in, values), nil
}
func (r *runner) confirm(zh, en string) (bool, error) {
	value, err := r.tty.read(r.text(zh, en) + r.text(" 输入 yes 确认，留空取消：", " Type yes to confirm; blank cancels: "))
	return value == "yes", err
}
func (r *runner) preview(in model.Input, owner string) {
	fmt.Fprintf(r.out, "%s: %q\n%s: %s\n%s: %s:%d → %s:%d\n%s: %s; %s: %s\n%s: %s\n",
		r.text("名称", "Name"), in.Name, r.text("协议", "Protocol"), in.Protocol, r.text("转发", "Forwarding"), in.ListenHost, in.ListenPort, in.TargetHost, in.TargetPort,
		r.text("启用", "Enabled"), r.yes(in.Enabled == nil || *in.Enabled), r.text("展示统计", "Traffic display"), r.yes(in.TrafficEnabled != nil && *in.TrafficEnabled), r.text("归属", "Owner"), owner)
}
func (r *runner) createWizard() error {
	in := model.Input{Protocol: "tcp"}
	var err error
	in, err = r.collect(in)
	if err != nil {
		return err
	}
	owner, err := r.ask("owner-id", "", false)
	if err != nil {
		return err
	}
	for {
		shownOwner := owner
		if shownOwner == "" {
			shownOwner = model.AdminID
		}
		r.preview(in, shownOwner)
		ok, err := r.confirm("提交创建？", "Create this rule?")
		if err != nil {
			return err
		}
		if !ok {
			return errCancel
		}
		value, err := r.api.Create(r.ctx, model.AdminRuleCreateInput{Input: in, OwnerID: owner})
		if err == nil {
			r.detail(r.out, value)
			return nil
		}
		var e *controlapi.Error
		if !errors.As(err, &e) || e.Code != "validation_error" {
			return err
		}
		r.fail(err, false)
		in, err = r.collect(in)
		if err != nil {
			return err
		}
	}
}
func (r *runner) editWizard(old model.Rule) error {
	before := inputOf(old)
	in, err := r.collect(before)
	if err != nil {
		return err
	}
	if sameInput(before, in) {
		return cliError("no_changes")
	}
	for {
		r.preview(in, old.OwnerID)
		ok, err := r.confirm("提交修改？", "Save these changes?")
		if err != nil {
			return err
		}
		if !ok {
			return errCancel
		}
		value, err := r.api.Update(r.ctx, old.ID, old.Revision, in)
		if err == nil {
			r.detail(r.out, value)
			return nil
		}
		var e *controlapi.Error
		if !errors.As(err, &e) || e.Code != "validation_error" {
			return err
		}
		r.fail(err, false)
		in, err = r.collect(in)
		if err != nil {
			return err
		}
	}
}
func sameInput(a, b model.Input) bool {
	av, bv := valuesOf(a), valuesOf(b)
	for _, key := range editableFields {
		if av[key] != bv[key] {
			return false
		}
	}
	return true
}
func (r *runner) deleteWizard(old model.Rule) error {
	ok, err := r.confirm("删除此规则？", "Delete this rule?")
	if err != nil {
		return err
	}
	if !ok {
		return errCancel
	}
	if err = r.api.Delete(r.ctx, old.ID, old.Revision); err != nil {
		return err
	}
	fmt.Fprintln(r.out, r.text("已删除规则：", "Deleted rule: "), strings.TrimSpace(old.ID))
	return nil
}
