// Package localapi 提供与 Web 分离、仅 root 对端可用的规则控制通道。
package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/lwl0820/porter/internal/model"
)

const DefaultSocket = "/var/lib/porter/porter-control.sock"
const resource = "/v1/rules"

type RuleList struct {
	Rules []model.Rule `json:"rules"`
}

type Error struct {
	Code    string            `json:"code"`
	Message string            `json:"message"`
	Fields  map[string]string `json:"fields,omitempty"`
	Status  int               `json:"-"`
}

func (e *Error) Error() string { return e.Code + ": " + e.Message }

type Client struct{ http *http.Client }

func NewClient(path string) *Client {
	transport := &http.Transport{DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
		return (&net.Dialer{Timeout: 3 * time.Second}).DialContext(ctx, "unix", path)
	}, DisableKeepAlives: true}
	return &Client{http: &http.Client{Transport: transport, Timeout: 15 * time.Second}}
}
func (c *Client) request(ctx context.Context, method, path string, revision int64, input, output any) error {
	var body io.Reader
	if input != nil {
		data, err := json.Marshal(input)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, "http://localhost"+path, body)
	if err != nil {
		return err
	}
	if input != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if revision > 0 {
		req.Header.Set("If-Match", fmt.Sprintf(`"%d"`, revision))
	}
	resp, err := c.http.Do(req)
	if err != nil {
		code := "service_unavailable"
		var n net.Error
		if errors.As(err, &n) && n.Timeout() {
			code = "request_timeout"
		}
		return &Error{Code: code}
	}
	defer resp.Body.Close()
	reader := io.LimitReader(resp.Body, 4<<20)
	if resp.StatusCode >= 300 {
		var envelope struct {
			Error Error `json:"error"`
		}
		if err := json.NewDecoder(reader).Decode(&envelope); err != nil || envelope.Error.Code == "" {
			return &Error{Code: "invalid_response"}
		}
		envelope.Error.Status = resp.StatusCode
		return &envelope.Error
	}
	if output != nil {
		if err := json.NewDecoder(reader).Decode(output); err != nil {
			return &Error{Code: "invalid_response"}
		}
	}
	return nil
}
func (c *Client) List(ctx context.Context, owner string) ([]model.Rule, error) {
	var out RuleList
	err := c.request(ctx, http.MethodGet, resource+"?owner_id="+url.QueryEscape(owner), 0, nil, &out)
	return out.Rules, err
}
func (c *Client) Get(ctx context.Context, id string) (model.Rule, error) {
	var out model.Rule
	err := c.request(ctx, http.MethodGet, resource+"/"+url.PathEscape(id), 0, nil, &out)
	return out, err
}
func (c *Client) Create(ctx context.Context, in model.AdminRuleCreateInput) (model.Rule, error) {
	var out model.Rule
	err := c.request(ctx, http.MethodPost, resource, 0, in, &out)
	return out, err
}
func (c *Client) Update(ctx context.Context, id string, revision int64, in model.Input) (model.Rule, error) {
	var out model.Rule
	err := c.request(ctx, http.MethodPut, resource+"/"+url.PathEscape(id), revision, in, &out)
	return out, err
}
func (c *Client) Delete(ctx context.Context, id string, revision int64) error {
	return c.request(ctx, http.MethodDelete, resource+"/"+url.PathEscape(id), revision, nil, nil)
}
func revision(r *http.Request) (int64, error) {
	value := r.Header.Get("If-Match")
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, &Error{Status: 400, Code: "revision_required"}
	}
	n, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || n < 1 {
		return 0, &Error{Status: 400, Code: "revision_invalid"}
	}
	return n, nil
}
