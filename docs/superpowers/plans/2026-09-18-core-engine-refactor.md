# gzgspd Core/Engine 重构实现计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复已确认的 bug，并把执行逻辑重构为无全局状态、context 驱动、事件输出的可嵌入 `engine` 包，为后续 GUI 铺路。

**Architecture:** 依赖方向单向 `engine → portal → nnet`，`engine → config`。Engine 管一组 Session，Session 是单账号状态机，通过注入的 `slog.Logger` 记日志、通过事件通道输出状态。进程外壳（flags/signal/os.Exit）隔离在 `internal/app`。

**Tech Stack:** Go 1.25（模块声明）/ 本机 1.27，标准库 `log/slog`、`net/http`、`context`，`github.com/robertkrimen/otto`（JS 解析），`golang.org/x/sys/windows`。

## Global Constraints

- **配置 JSON 字段名保持不变**，不重命名 `retry_max`、`retry_time`、`keep_alive`、`keep_alive_link`、`log_level`、`log_path`、`instance`。
- 现有 CLI 契约不变：`--config`、`--version`、`--test`；环境变量 `GZGSPD_CONFIG_FILE`。
- CI 产物命名与路径不变（`-o gzgspd`）。
- `nnet` 不再 import 任何本项目包。
- Engine/Session 无包级可变状态；不使用 `slog.SetDefault`；不调用 `os.Exit`；不注册信号。
- 所有阻塞操作接受 `ctx`。
- **测试策略（用户指定）**：测试与实现一起写，全部完成后统一运行一次 `go test ./...`，不逐功能执行 red-green。
- 版本包路径变更必须同步更新 `.github/workflows/ci.yml` 的 ldflags。

## 测试执行方式（用户指定）

- 测试代码随各 Task 的实现一起写入仓库，但**不在每个 Task 内运行**。
- 全项目只在 **Task 7 Step 4** 统一运行一次 `go test ./... -count=1`。
- 各 Task 内只做 `go build ./...` 的编译验证。
- 为弥补跳过一次「看到红」，Task 7 额外核对本计划末尾的覆盖对照表。

---

## 文件结构

| 文件 | 职责 |
|------|------|
| `internal/version/version.go` | 版本号、构建时间（ldflags 注入点） |
| `internal/logx/logx.go` | 依据 `log_level`/`log_path` 构造 `*slog.Logger` |
| `internal/app/app.go` | flags 解析、配置加载、Engine 启动、信号处理、退出码 |
| `main.go` | 根目录最小引导，调用 `app.Main()` |
| `config/configs.go` | 数据 + 校验 + Load/Save + `FilePath()` + `PauseDuration` 默认值 |
| `config/configs_test.go` | 配置测试 |
| `nnet/ifutils.go` | 网络工具，移除 config 依赖 |
| `nnet/ifutils_test.go` | 接口工具测试 |
| `portal/portal.go` | 协议类型、Client、context 化方法 |
| `portal/portal_test.go` | 协议测试（httptest） |
| `engine/state.go` | `State` 枚举与 `String()` |
| `engine/event.go` | `Event` 类型 |
| `engine/session.go` | `Session` 状态机、网络 seam、重试语义 |
| `engine/engine.go` | `Engine`、`Options`、`New`、`Run`、`Stop` |
| `engine/session_test.go` | 状态机测试（fake seam） |
| `engine/engine_test.go` | Engine 生命周期与事件通道测试 |

---

## Task 1: internal/version 包与 ldflags 迁移

**Files:**
- Create: `internal/version/version.go`
- Modify: `main.go`
- Modify: `.github/workflows/ci.yml:133`

**Interfaces:**
- Produces: `version.Value string`、`version.BuildTime string`、`version.String() string`

- [ ] **Step 1: 创建版本包**

`internal/version/version.go`:

```go
// Package version 保存构建期注入的版本信息。
package version

import "fmt"

var (
	// Value 由构建期 -ldflags -X 注入。
	Value = "dev"
	// BuildTime 由构建期 -ldflags -X 注入。
	BuildTime = "0"
)

// String 返回 "Value BuildTime" 形式，供 CLI 与 GUI 展示。
func String() string {
	return fmt.Sprintf("%s %s", Value, BuildTime)
}
```

- [ ] **Step 2: 修改 main.go 引用版本包**

将 `main.go` 顶部的 version 变量声明删除，`--version` 输出改用 `version.String()`：

```go
package main

import (
	"fmt"
	"os"
	"runtime"

	"github.com/summonhim/gzgspd/internal/version"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Printf(
			"gzgspd %s %s %s with %s\n",
			version.Value,
			runtime.GOOS,
			runtime.GOARCH,
			runtime.Version(),
		)
		os.Exit(0)
	}
	fmt.Println("use internal/app")
	os.Exit(0)
}
```

（此步仅为让 ldflags 迁移可编译；Task 6 会用真正的 `app.Main()` 替换。）

- [ ] **Step 3: 更新 CI 的 ldflags 包路径**

`.github/workflows/ci.yml:133` 改为：

```yaml
          go build -v -trimpath -ldflags "${BUILDTAG} -X 'github.com/summonhim/gzgspd/internal/version.Value=${VERSION}' -X 'github.com/summonhim/gzgspd/internal/version.BuildTime=${BUILDTIME}' -w -s" -o gzgspd
```

- [ ] **Step 4: 构建验证**

Run: `go build ./...`
Expected: exit 0，无输出。

- [ ] **Step 5: 提交**

```bash
git add internal/version/version.go main.go .github/workflows/ci.yml
git commit -m "refact: 版本信息迁移到 internal/version"
```

---

## Task 2: config 的 LogPath 生效、Path 记忆、Save 与测试

**Files:**
- Modify: `config/configs.go`
- Test: `config/configs_test.go`
- Create: `internal/logx/logx.go`

**Interfaces:**
- Consumes: 无
- Produces:
  - `Config.FilePath() string`
  - `(*Config) SetFilePath(path string)`
  - `(*Config) Save() error`
  - `(*Config) PauseDuration() time.Duration`
  - `(*Config) Logger() (*slog.Logger, func() error)`（`logx` 包中实现为 `logx.New`）

- [ ] **Step 1: 写配置测试**

`config/configs_test.go`:

```go
package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestValidateRejects(t *testing.T) {
	base := func() *Config {
		return &Config{Instance: []ConfigInstance{{
			Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5,
		}}}
	}

	cases := []struct {
		name string
		mut  func(*Config)
	}{
		{"no instance", func(c *Config) { c.Instance = nil }},
		{"empty username", func(c *Config) { c.Instance[0].Username = "" }},
		{"empty password", func(c *Config) { c.Instance[0].Password = "" }},
		{"keep_alive zero", func(c *Config) { c.Instance[0].KeepAlive = 0 }},
		{"retry_max negative", func(c *Config) { c.Instance[0].RetryMax = -1 }},
		{"retry_time zero", func(c *Config) { c.Instance[0].RetryTime = 0 }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := base()
			tc.mut(c)
			if err := c.Validate(); err == nil {
				t.Fatalf("expected error for %s", tc.name)
			}
		})
	}
}

func TestValidateAccepts(t *testing.T) {
	c := &Config{Instance: []ConfigInstance{{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5}}}
	if err := c.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLoadIgnoresUnknownField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	data := `{"log_level":0,"instance":[{"username":"u","password":"p","keep_alive":5,"retry_time":5,"max_retry":3}]}`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Instance[0].RetryMax != 0 {
		t.Fatalf("expected misspelled max_retry to be ignored, got %d", c.Instance[0].RetryMax)
	}
	if c.FilePath() != path {
		t.Fatalf("expected FilePath %q, got %q", path, c.FilePath())
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	c := &Config{
		LogLevel: 0,
		LogPath:  "d.log",
		Instance: []ConfigInstance{{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 7, RetryMax: 3}},
	}
	c.SetFilePath(path)
	if err := c.Save(); err != nil {
		t.Fatalf("save: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if got.Instance[0].RetryTime != 7 || got.Instance[0].RetryMax != 3 || got.LogPath != "d.log" {
		t.Fatalf("round trip mismatch: %+v", got)
	}
}

func TestPauseDurationDefault(t *testing.T) {
	c := &Config{}
	if got := c.PauseDuration(); got != 10*time.Minute {
		t.Fatalf("expected 10m default, got %v", got)
	}
	c.PauseDurationSeconds = 1
	if got := c.PauseDuration(); got != time.Second {
		t.Fatalf("expected 1s, got %v", got)
	}
}
```

- [ ] **Step 2: 扩展 configs.go**

在 `config/configs.go` 中：

1. 结构体 `Config` 增加两个字段（第二个不参与 JSON）：

```go
type Config struct {
	LogLevel             int              `json:"log_level"`
	LogPath              string           `json:"log_path"`
	Instance             []ConfigInstance `json:"instance"`

	// 以下字段不参与 JSON 序列化
	PauseDurationSeconds int    `json:"-"`
	filePath             string `json:"-"`
}
```

2. 增加方法：

```go
// FilePath 返回该配置的加载来源路径，可能为空。
func (c *Config) FilePath() string { return c.filePath }

// SetFilePath 记录配置来源路径，供 Save 使用。
func (c *Config) SetFilePath(path string) { c.filePath = path }

// PauseDuration 返回连续失败达到上限后的暂停时长，默认 10 分钟。
func (c *Config) PauseDuration() time.Duration {
	if c.PauseDurationSeconds > 0 {
		return time.Duration(c.PauseDurationSeconds) * time.Second
	}
	return 10 * time.Minute
}

// Save 将配置写回其来源路径，使用缩进 JSON。
func (c *Config) Save() error {
	if c.filePath == "" {
		return fmt.Errorf("config file path is not set")
	}
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(c.filePath, data, 0o644)
}
```

3. `LoadConfig` 结尾记录路径：

```go
	cfg.filePath = path
	return &cfg, nil
```

4. import 增加 `"time"`。

- [ ] **Step 3: 创建 logx 包**

`internal/logx/logx.go`:

```go
// Package logx 依据配置构造 slog.Logger，负责日志落盘。
package logx

import (
	"fmt"
	"io"
	"log/slog"
	"os"
)

// New 返回一个 logger 与一个清理函数。
// path 为空时只写 stdout；否则同时写入文件（追加模式）。
func New(level int, path string) (*slog.Logger, func() error) {
	var w io.Writer = os.Stdout
	closer := func() error { return nil }

	if path != "" {
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
		if err == nil {
			w = io.MultiWriter(os.Stdout, f)
			closer = f.Close
		} else {
			fmt.Fprintf(os.Stderr, "failed to open log file %q: %v\n", path, err)
		}
	}

	logger := slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{
		Level: slog.Level(level),
	}))
	return logger, closer
}
```

- [ ] **Step 4: 构建验证**

Run: `go build ./...`
Expected: exit 0。

- [ ] **Step 5: 提交**

```bash
git add config/configs.go config/configs_test.go internal/logx/logx.go
git commit -m "fix: log_path 生效并补充 config 测试"
```

---

## Task 3: 修复 GetKeyIfName 与 nnet 测试

**Files:**
- Modify: `nnet/ifutils.go`
- Test: `nnet/ifutils_test.go`

**Interfaces:**
- Consumes: 无
- Produces: `nnet.GetKeyIfName(inst config.ConfigInstance) string`（行为修正）

- [ ] **Step 1: 写测试**

`nnet/ifutils_test.go`:

```go
package nnet

import (
	"testing"

	"github.com/summonhim/gzgspd/config"
)

func TestGetKeyIfName(t *testing.T) {
	if got := GetKeyIfName(config.ConfigInstance{}); got != "Auto" {
		t.Fatalf("empty interface should yield Auto, got %q", got)
	}
	if got := GetKeyIfName(config.ConfigInstance{Interface: "wanmac0"}); got != "wanmac0" {
		t.Fatalf("expected wanmac0, got %q", got)
	}
}

func TestGetIPMACInvalid(t *testing.T) {
	if _, err := GetIPMAC("not-an-ip"); err == nil {
		t.Fatal("expected error for invalid ip")
	}
	if _, err := GetIPMAC("::1"); err == nil {
		t.Fatal("expected error for ipv6 address")
	}
}

func TestGetIfMACMissing(t *testing.T) {
	if _, err := GetIfMAC("definitely-not-an-interface-xyz"); err == nil {
		t.Fatal("expected error for missing interface")
	}
}
```

- [ ] **Step 2: 修复 GetKeyIfName**

`nnet/ifutils.go:92-99` 整体替换为：

```go
// GetKeyIfName 从配置中获取接口字符串，为空则为 Auto
func GetKeyIfName(instance config.ConfigInstance) string {
	if instance.Interface == "" {
		return "Auto"
	}
	return instance.Interface
}
```

- [ ] **Step 3: 构建验证**

Run: `go build ./...`
Expected: exit 0。

- [ ] **Step 4: 提交**

```bash
git add nnet/ifutils.go nnet/ifutils_test.go
git commit -m "fix: GetKeyIfName 恒返回 Auto 的逻辑错误"
```

---

## Task 4: portal 改为 Client + struct 参数 + context

**Files:**
- Modify: `portal/portal.go`（整体重写客户端部分，保留两个响应结构体）
- Test: `portal/portal_test.go`

**Interfaces:**
- Consumes: `nnet.NewHttpClientBindIP`（保留现有签名）
- Produces:
  - `portal.NewClient(localIP string, timeout time.Duration) (*Client, error)`
  - `(*Client) Close()`
  - `(*Client) PortalChecker(ctx context.Context, kAliveLink string) (needLogin bool, loginURL string, err error)`
  - `(*Client) PortalJsonAction(ctx context.Context, r ActionRequest) (*ActionResponse, error)`
  - `(*Client) QuickAuth(ctx context.Context, r QuickAuthRequest) (*QuickAuthResponse, error)`
  - `(*Client) QuickAuthDisconn(ctx context.Context, r DisconnRequest) (*QuickAuthResponse, error)`
  - 类型 `ActionRequest`、`QuickAuthRequest`、`DisconnRequest`

- [ ] **Step 1: 写测试**

`portal/portal_test.go`:

```go
package portal

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func newTestClient(t *testing.T) *Client {
	t.Helper()
	c, err := NewClient("127.0.0.1", 3*time.Second)
	if err != nil {
		t.Fatalf("new client: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestQuickAuthBuildsQuery(t *testing.T) {
	var got url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.URL.Query()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"0","message":"ok","userId":"u1","groupId":19}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	resp, err := c.QuickAuth(context.Background(), QuickAuthRequest{
		Scheme: u.Scheme, Host: u.Host, UserAgent: "UA",
		UserID: "user", Password: "pass",
		Wlanuserip: "10.0.0.1", Wlanacname: "ac", WlanacIP: "10.0.0.2",
		VLAN: "1", MAC: "aa:bb:cc:dd:ee:ff",
		Version: 4, PortalPageID: 1, Timestamp: 123, UUID: "uuid",
		PortalType: "0", Hostname: "h", Rand: "r",
	})
	if err != nil {
		t.Fatalf("quickauth: %v", err)
	}
	if resp.Code != "0" || resp.UserID != "u1" {
		t.Fatalf("unexpected response: %+v", resp)
	}
	if got.Get("userid") != "user" || got.Get("passwd") != "pass" || got.Get("mac") != "aa:bb:cc:dd:ee:ff" {
		t.Fatalf("query mismatch: %v", got)
	}
}

func TestQuickAuthDisconnForm(t *testing.T) {
	var ctype, body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctype = r.Header.Get("Content-Type")
		buf := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(buf)
		body = string(buf)
		_, _ = w.Write([]byte(`{"code":"0"}`))
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	if _, err := c.QuickAuthDisconn(context.Background(), DisconnRequest{
		Scheme: u.Scheme, Host: u.Host, UserAgent: "UA",
		WlanacIP: "10.0.0.2", Wlanuserip: "10.0.0.1", Wlanacname: "ac",
		Version: 4, PortalType: "0", UserID: "u@SSGSXY",
		MAC: "aa:bb:cc:dd:ee:ff", GroupID: 19, ClearOperator: "0",
	}); err != nil {
		t.Fatalf("disconn: %v", err)
	}
	if !strings.HasPrefix(ctype, "application/x-www-form-urlencoded") {
		t.Fatalf("unexpected content type: %q", ctype)
	}
	if !strings.Contains(body, "wlanacip=10.0.0.2") || !strings.Contains(body, "groupId=19") {
		t.Fatalf("form mismatch: %q", body)
	}
}

func TestPortalCheckerRedirect(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://x/portal.do?wlanuserip=1", http.StatusFound)
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if !need || !strings.Contains(link, "portal.do") {
		t.Fatalf("expected login required, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerScriptReplace(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html><script>location.replace("http://x/portalScript.do?a=1");</script></html>`))
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if !need || !strings.Contains(link, "portalScript.do") {
		t.Fatalf("expected login required, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerOnline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<html>ok</html>`))
	}))
	defer srv.Close()

	c := newTestClient(t)
	need, link, err := c.PortalChecker(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("checker: %v", err)
	}
	if need || link != "" {
		t.Fatalf("expected online, got need=%v link=%q", need, link)
	}
}

func TestPortalCheckerNetworkError(t *testing.T) {
	c := newTestClient(t)
	if _, _, err := c.PortalChecker(context.Background(), "http://127.0.0.1:1/none"); err == nil {
		t.Fatal("expected error on connection failure, got nil")
	}
}

func TestQuickAuthHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	if _, err := c.QuickAuth(context.Background(), QuickAuthRequest{Scheme: u.Scheme, Host: u.Host, UserAgent: "UA"}); err == nil {
		t.Fatal("expected error on non-2xx response")
	}
}

func TestQuickAuthContextCanceled(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(2 * time.Second)
	}))
	defer srv.Close()

	u, _ := url.Parse(srv.URL)
	c := newTestClient(t)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.QuickAuth(ctx, QuickAuthRequest{Scheme: u.Scheme, Host: u.Host, UserAgent: "UA"}); err == nil {
		t.Fatal("expected context deadline error")
	}
}
```

- [ ] **Step 2: 重写 portal.go 的客户端部分**

保留文件顶部的 `ActionResponse`、`QuickAuthResponse` 两个结构体定义与 import 中的 `"encoding/json"`。将 import 调整为：

```go
import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/robertkrimen/otto"

	"github.com/summonhim/gzgspd/nnet"
)
```

将原四个函数整体替换为以下内容：

```go
// Client 是绑定到某个本地 IP 的 Portal 协议客户端，可复用于多次请求。
type Client struct {
	http *http.Client
}

// NewClient 构造绑定本地 IP 的客户端。
func NewClient(localIP string, timeout time.Duration) (*Client, error) {
	hc, err := nnet.NewHttpClientBindIP(localIP, timeout)
	if err != nil {
		return nil, err
	}
	return &Client{http: hc}, nil
}

// Close 释放空闲连接。
func (c *Client) Close() {
	if c.http != nil {
		if tr, ok := c.http.Transport.(*http.Transport); ok {
			tr.CloseIdleConnections()
		}
	}
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: unexpected status %d: %s", req.Method, req.URL, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s %s: decode response: %w", req.Method, req.URL, err)
	}
	return nil
}

func setCommonHeaders(req *http.Request, userAgent string) {
	req.Header.Set("User-Agent", userAgent)
	req.Header.Set("Accept", "application/json, text/javascript, */*; q=0.01")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en-US;q=0.8,en;q=0.7,ja;q=0.6")
}

// ActionRequest 是 PortalJsonAction 的请求参数。
type ActionRequest struct {
	Scheme     string
	Host       string
	UserAgent  string
	Wlanuserip string
	Wlanacname string
	MAC        string
	VLAN       string
	Hostname   string
	Rand       string
}

// PortalJsonAction 获取登录的基本信息。
func (c *Client) PortalJsonAction(ctx context.Context, r ActionRequest) (*ActionResponse, error) {
	params := url.Values{}
	params.Set("wlanuserip", r.Wlanuserip)
	params.Set("wlanacname", r.Wlanacname)
	params.Set("mac", r.MAC)
	params.Set("vlan", r.VLAN)
	params.Set("hostname", r.Hostname)
	params.Set("rand", r.Rand)
	params.Set("viewStatus", "1")

	fullURL := r.Scheme + "://" + r.Host + "/PortalJsonAction.do?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)

	var result ActionResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// QuickAuthRequest 是 QuickAuth 的请求参数。
type QuickAuthRequest struct {
	Scheme       string
	Host         string
	UserAgent    string
	UserID       string
	Password     string
	Wlanuserip   string
	Wlanacname   string
	WlanacIP     string
	VLAN         string
	MAC          string
	Version      int
	PortalPageID int
	Timestamp    int64
	UUID         string
	PortalType   string
	Hostname     string
	Rand         string
}

// QuickAuth 执行 portal 登录。
func (c *Client) QuickAuth(ctx context.Context, r QuickAuthRequest) (*QuickAuthResponse, error) {
	params := url.Values{}
	params.Set("userid", r.UserID)
	params.Set("passwd", r.Password)
	params.Set("wlanuserip", r.Wlanuserip)
	params.Set("wlanacname", r.Wlanacname)
	params.Set("wlanacIp", r.WlanacIP)
	params.Set("vlan", r.VLAN)
	params.Set("mac", r.MAC)
	params.Set("version", fmt.Sprintf("%d", r.Version))
	params.Set("portalpageid", fmt.Sprintf("%d", r.PortalPageID))
	params.Set("timestamp", fmt.Sprintf("%d", r.Timestamp))
	params.Set("uuid", r.UUID)
	params.Set("portaltype", r.PortalType)
	params.Set("hostname", r.Hostname)
	params.Set("rand", r.Rand)

	fullURL := r.Scheme + "://" + r.Host + "/quickauth.do?" + params.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fullURL, nil)
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)

	var result QuickAuthResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// DisconnRequest 是 QuickAuthDisconn 的请求参数。
type DisconnRequest struct {
	Scheme        string
	Host          string
	UserAgent     string
	WlanacIP      string
	Wlanuserip    string
	Wlanacname    string
	Version       int
	PortalType    string
	UserID        string
	MAC           string
	GroupID       int
	ClearOperator string
}

// QuickAuthDisconn 执行 portal 登出。
func (c *Client) QuickAuthDisconn(ctx context.Context, r DisconnRequest) (*QuickAuthResponse, error) {
	data := url.Values{}
	data.Set("wlanacip", r.WlanacIP)
	data.Set("wlanuserip", r.Wlanuserip)
	data.Set("wlanacname", r.Wlanacname)
	data.Set("version", fmt.Sprintf("%d", r.Version))
	data.Set("portaltype", r.PortalType)
	data.Set("userid", r.UserID)
	data.Set("mac", r.MAC)
	data.Set("groupId", fmt.Sprintf("%d", r.GroupID))
	data.Set("clearOperator", r.ClearOperator)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, r.Scheme+"://"+r.Host+"/quickauthdisconn.do", bytes.NewBufferString(data.Encode()))
	if err != nil {
		return nil, err
	}
	setCommonHeaders(req, r.UserAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var result QuickAuthResponse
	if err := c.doJSON(req, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// PortalChecker 探测当前网络是否需要登录。返回错误表示探测本身失败，而非"在线"。
func (c *Client) PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, kAliveLink, nil)
	if err != nil {
		return false, "", err
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return false, "", err
	}
	defer resp.Body.Close()

	portalKeywords := []string{"portalScript.do", "portal.do"}

	if resp.StatusCode >= 300 && resp.StatusCode < 400 {
		if loc, err := resp.Location(); err == nil {
			for _, kw := range portalKeywords {
				if strings.Contains(loc.String(), kw) {
					return true, loc.String(), nil
				}
			}
		}
	}

	if resp.StatusCode == 200 {
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return false, "", err
		}
		html := string(body)

		re := regexp.MustCompile(`<script[^>]*>([\s\S]*?)</script>`)
		scripts := re.FindAllStringSubmatch(html, -1)
		for _, s := range scripts {
			js := s[1]

			vm := otto.New()
			var finalURL string
			_ = vm.Set("location", map[string]interface{}{
				"replace": func(call otto.FunctionCall) otto.Value {
					v, _ := call.Argument(0).ToString()
					finalURL = v
					return otto.Value{}
				},
			})

			if _, err := vm.Run(js); err == nil && finalURL != "" {
				for _, kw := range portalKeywords {
					if strings.Contains(finalURL, kw) {
						return true, finalURL, nil
					}
				}
			}
		}
	}

	return false, "", nil
}
```

- [ ] **Step 3: 构建验证**

Run: `go build ./...`
Expected: exit 0。（此时 `executor` 仍在调用旧签名，会报编译错误——这是预期的，Task 5 会替换 `executor`。若需要独立构建，可临时执行 `go build ./config/... ./nnet/... ./portal/...`。）

- [ ] **Step 4: 提交**

```bash
git add portal/portal.go portal/portal_test.go
git commit -m "refact: portal 改为 Client/struct/context 接口"
```

---

## Task 5: engine 包

**说明**：Step 2 中的 `Session`/`sessionConfig`/`newSession` 已是最终版本，无需再做额外改动。

**Files:**
- Create: `engine/state.go`
- Create: `engine/event.go`
- Create: `engine/session.go`
- Create: `engine/engine.go`
- Delete: `executor/worker.go`（整个 `executor` 目录）
- Test: `engine/session_test.go`
- Test: `engine/engine_test.go`
- Modify: `main.go`（临时引用新 engine；Task 6 完成外壳）

**Interfaces:**
- Consumes: `portal.Client`、`config.Config`、`nnet`
- Produces:
  - `engine.State` 与常量 `StateStarting/StateNotLoggedIn/StateLoggingIn/StateLoggedIn/StatePaused/StateLoggingOut/StateStopped`
  - `engine.Event{Key string; Time time.Time; State State; Message string; Err error}`
  - `engine.Options{Logger *slog.Logger}`
  - `engine.New(cfg *config.Config, opts Options) (*Engine, error)`
  - `(*Engine) Sessions() []*Session`、`(*Engine) Events() <-chan Event`、`(*Engine) Run(ctx context.Context) error`、`(*Engine) Stop()`
  - `(*Session) Key() string`、`(*Session) State() State`、`(*Session) Start(ctx context.Context) error`、`(*Session) Pause()`、`(*Session) Resume()`
  - 内部 seam：`sessionDialer` 接口（供测试注入 fake）

- [ ] **Step 1: 写状态与事件**

`engine/state.go`:

```go
package engine

// State 表示单个会话的当前状态。
type State int

const (
	StateStarting State = iota
	StateNotLoggedIn
	StateLoggingIn
	StateLoggedIn
	StatePaused
	StateLoggingOut
	StateStopped
)

// String 返回可读状态名。
func (s State) String() string {
	switch s {
	case StateStarting:
		return "Starting"
	case StateNotLoggedIn:
		return "Not logged in"
	case StateLoggingIn:
		return "Logging in"
	case StateLoggedIn:
		return "Logged in"
	case StatePaused:
		return "Paused"
	case StateLoggingOut:
		return "Logging out"
	case StateStopped:
		return "Stopped"
	default:
		return "Unknown"
	}
}
```

`engine/event.go`:

```go
package engine

import "time"

// Event 是一次状态变更或失败通知，供 GUI 消费。
type Event struct {
	Key     string
	Time    time.Time
	State   State
	Message string
	Err     error
}
```

- [ ] **Step 2: 写 session.go**

`engine/session.go`:

```go
package engine

import (
	"context"
	"log/slog"
	"net"
	"sync"
	"time"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/nnet"
	"github.com/summonhim/gzgspd/portal"
)

const (
	defaultUserAgent    = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/140.0.0.0 Safari/537.36"
	defaultKAliveLink   = "http://3.3.3.3"
	defaultLoginScheme  = "https"
	defaultLoginHost    = "10.20.16.5"
	defaultWlanacIP     = "10.20.16.2"
	defaultWlanacname   = "NFV-BASE-01"
	defaultUserSuffix   = "@SSGSXY"
)

// sessionDialer 是一轮登录所需的网络动作。生产实现包装 portal.Client；
// 测试可注入 fake。这是为可测性与 GUI 可插拔性引入的唯一抽象。
type sessionDialer interface {
	PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error)
	PortalJsonAction(ctx context.Context, r portal.ActionRequest) (*portal.ActionResponse, error)
	QuickAuth(ctx context.Context, r portal.QuickAuthRequest) (*portal.QuickAuthResponse, error)
	QuickAuthDisconn(ctx context.Context, r portal.DisconnRequest) (*portal.QuickAuthResponse, error)
	Close()
}

// Session 是单个账号的登录状态机。
type Session struct {
	cfg      config.ConfigInstance
	keyStr   string
	logger   *slog.Logger
	pause    time.Duration
	events   chan<- Event

	// 可注入依赖（测试用）
	newDialer        func(localIP string) (sessionDialer, error)
	prepareInterface func() error

	mu     sync.RWMutex
	state  State
	dialer sessionDialer
	click  chan struct{} // Pause/Resume/唤醒 信号

	// 运行时状态
	loginIf    string
	loginIfIP  string
	scheme     string
	host       string
	wlanuserip string
	wlanacname string
	mac        string
	vlan       string
	hostname   string
	rand       string
	wlanacIP   string
	version    int
	portalPage int
	timestamp  int64
	uuid       string
	groupID    int
	logoutUID  string
}

type sessionConfig struct {
	inst      config.ConfigInstance
	key       string
	logger    *slog.Logger
	pause     time.Duration
	events    chan<- Event
	newDialer func(localIP string) (sessionDialer, error)
}

func newSession(sc sessionConfig) *Session {
	logger := sc.logger
	if logger == nil {
		logger = slog.Default()
	}
	s := &Session{
		cfg:       sc.inst,
		keyStr:    sc.key,
		logger:    logger.With("key", sc.key),
		pause:     sc.pause,
		events:    sc.events,
		click:     make(chan struct{}, 8),
		state:     StateStarting,
		newDialer: sc.newDialer,
	}
	s.prepareInterface = s.prepareInterfaceImpl
	return s
}

// Key 返回唯一标识，形如 username@interface。
func (s *Session) Key() string { return s.keyStr }

// State 返回当前状态（并发安全）。
func (s *Session) State() State {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Session) setState(st State, msg string, err error) {
	s.mu.Lock()
	s.state = st
	s.mu.Unlock()
	s.logger.Info("state changed", "state", st.String(), "message", msg)
	if s.events != nil {
		select {
		case s.events <- Event{Key: s.Key(), Time: time.Now(), State: st, Message: msg, Err: err}:
		default:
		}
	}
}

// Start 运行状态机，阻塞至 ctx 取消并完成登出。
func (s *Session) Start(ctx context.Context) error {
	s.setState(StateStarting, "starting", nil)

	newDialer := s.newDialer
	if newDialer == nil {
		newDialer = func(localIP string) (sessionDialer, error) {
			return portal.NewClient(localIP, 5*time.Second)
		}
	}

	if err := s.prepareInterface(); err != nil {
		s.setState(StateStopped, "interface error", err)
		return err
	}

	dialer, err := newDialer(s.loginIfIP)
	if err != nil {
		s.setState(StateStopped, "dialer error", err)
		return err
	}
	defer dialer.Close()

	s.mu.Lock()
	s.dialer = dialer
	s.mu.Unlock()

	// 默认值回填
	if s.cfg.UserAgent == "" {
		s.cfg.UserAgent = defaultUserAgent
	}
	if s.cfg.KAliveLink == "" {
		s.cfg.KAliveLink = defaultKAliveLink
	}

	s.setState(StateNotLoggedIn, "ready", nil)

	retry := 0
	for {
		if ctx.Err() != nil {
			s.setState(StateLoggingOut, "context canceled, logging out", nil)
			s.doLogout()
			s.setState(StateStopped, "stopped", nil)
			return ctx.Err()
		}

		if s.isPaused() {
			// 暂停中：等待唤醒或 ctx 取消，不执行登录
			t := time.NewTimer(200 * time.Millisecond)
			select {
			case <-ctx.Done():
			case <-s.click:
			case <-t.C:
			}
			t.Stop()
			continue
		}

		// 自动更新默认网口
		if s.cfg.Interface == "" {
			s.refreshDefaultInterface()
		}

		if err := s.attemptLogin(ctx); err != nil {
			if ctx.Err() != nil {
				continue
			}
			retry++
			s.setState(StateNotLoggedIn, "login failed", err)
			if s.cfg.RetryMax != 0 && retry >= s.cfg.RetryMax {
				s.setState(StatePaused, "reached max retries", nil)
				if !s.wait(ctx, s.pause) {
					continue
				}
				retry = 0
			} else if !s.wait(ctx, time.Duration(s.cfg.RetryTime)*time.Second) {
				continue
			}
		} else {
			retry = 0
			if !s.wait(ctx, time.Duration(s.cfg.KeepAlive)*time.Second) {
				continue
			}
		}
	}
}

// wait 等待 d 或 ctx 取消，返回 true 表示等待完成，false 表示应重新评估循环。
func (s *Session) wait(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-s.click:
		return false
	case <-t.C:
		return true
	}
}

func (s *Session) isPaused() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state == StatePaused
}

// Pause 请求登出并暂停。
func (s *Session) Pause() {
	s.setState(StatePaused, "paused by request", nil)
	select {
	case s.click <- struct{}{}:
	default:
	}
}

// Resume 从暂停恢复。
func (s *Session) Resume() {
	s.setState(StateNotLoggedIn, "resumed", nil)
	select {
	case s.click <- struct{}{}:
	default:
	}
}

func (s *Session) refreshDefaultInterface() {
	nowIf, nowIP, nowMAC, err := nnet.GetDefaultIfIP()
	if err != nil {
		s.logger.Error("failed to get default interface", "error", err)
		return
	}
	s.mu.Lock()
	if s.loginIf != nowIf || s.loginIfIP != nowIP || s.mac != nowMAC {
		s.logger.Info("interface updated", "name", nowIf, "ip", nowIP, "mac", nowMAC)
	}
	s.loginIf, s.loginIfIP, s.mac = nowIf, nowIP, nowMAC
	s.mu.Unlock()
}

func (s *Session) prepareInterfaceImpl() error {
	instIf := s.cfg.Interface
	if instIf == "" {
		ifname, ip, mac, err := nnet.GetDefaultIfIP()
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = ifname, ip, mac
	} else if net.ParseIP(instIf) == nil {
		ip, err := nnet.GetIfIP(instIf)
		if err != nil {
			return err
		}
		mac, err := nnet.GetIfMAC(instIf)
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = instIf, ip, mac
	} else {
		mac, err := nnet.GetIPMAC(instIf)
		if err != nil {
			return err
		}
		s.loginIf, s.loginIfIP, s.mac = instIf, instIf, mac
	}
	s.logger.Info("using interface", "name", s.loginIf, "ip", s.loginIfIP, "mac", s.mac)
	return nil
}

// attemptLogin 执行一轮探测+登录，返回 nil 表示已在线或登录成功。
func (s *Session) attemptLogin(ctx context.Context) error {
	dialer := s.dialer

	needLogin, needLoginURL, err := dialer.PortalChecker(ctx, s.cfg.KAliveLink)
	if err != nil {
		return fmt.Errorf("portal check: %w", err)
	}
	if !needLogin || needLoginURL == "" {
		return nil
	}

	s.setState(StateLoggingIn, "login required", nil)

	nlu, err := url.Parse(needLoginURL)
	if err != nil {
		return fmt.Errorf("parse redirect link: %w", err)
	}
	q := nlu.Query()
	s.scheme = nlu.Scheme
	s.host = nlu.Host
	s.wlanuserip = q.Get("wlanuserip")
	s.wlanacname = q.Get("wlanacname")
	s.mac = firstNonEmpty(q.Get("mac"), s.mac)
	s.vlan = q.Get("vlan")
	s.hostname = q.Get("hostname")
	s.rand = q.Get("rand")

	portalConfig, err := dialer.PortalJsonAction(ctx, portal.ActionRequest{
		Scheme: s.scheme, Host: s.host, UserAgent: s.cfg.UserAgent,
		Wlanuserip: s.wlanuserip, Wlanacname: s.wlanacname,
		MAC: s.mac, VLAN: s.vlan, Hostname: s.hostname, Rand: s.rand,
	})
	if err != nil {
		return fmt.Errorf("portal json action: %w", err)
	}

	s.wlanacIP = portalConfig.ServerForm.Serverip
	s.version = portalConfig.ServerForm.PortalVer
	s.portalPage = portalConfig.PortalConfig.ID
	s.timestamp = portalConfig.PortalConfig.Timestamp
	s.uuid = portalConfig.PortalConfig.UUID

	loginStat, err := dialer.QuickAuth(ctx, portal.QuickAuthRequest{
		Scheme: s.scheme, Host: s.host, UserAgent: s.cfg.UserAgent,
		UserID: s.cfg.Username, Password: s.cfg.Password,
		Wlanuserip: s.wlanuserip, Wlanacname: s.wlanacname, WlanacIP: s.wlanacIP,
		VLAN: s.vlan, MAC: s.mac, Version: s.version, PortalPageID: s.portalPage,
		Timestamp: s.timestamp, UUID: s.uuid, PortalType: "0",
		Hostname: s.hostname, Rand: s.rand,
	})
	if err != nil {
		return fmt.Errorf("quick auth: %w", err)
	}
	if loginStat.Code != "0" {
		return fmt.Errorf("login rejected: %s", loginStat.Message)
	}

	s.groupID = loginStat.GroupID
	s.logoutUID = loginStat.UserID
	s.setState(StateLoggedIn, "login successful", nil)
	return nil
}

func (s *Session) doLogout() {
	dialer := s.dialer
	if dialer == nil {
		return
	}
	if s.version == 0 {
		s.version = 4
	}
	if s.groupID == 0 {
		s.groupID = 19
	}

	stat, err := dialer.QuickAuthDisconn(context.Background(), portal.DisconnRequest{
		Scheme:     firstNonEmpty(s.scheme, defaultLoginScheme),
		Host:       firstNonEmpty(s.host, defaultLoginHost),
		UserAgent:  firstNonEmpty(s.cfg.UserAgent, defaultUserAgent),
		WlanacIP:   firstNonEmpty(s.wlanacIP, defaultWlanacIP),
		Wlanuserip: firstNonEmpty(s.wlanuserip, s.loginIfIP),
		Wlanacname: firstNonEmpty(s.wlanacname, defaultWlanacname),
		Version:    s.version,
		PortalType: "0",
		UserID:     firstNonEmpty(s.logoutUID, s.cfg.Username+defaultUserSuffix),
		MAC:        firstNonEmpty(s.mac, s.macByIP()),
		GroupID:    s.groupID,
		ClearOperator: "0",
	})
	if err != nil {
		s.logger.Error("logout failed", "error", err)
		return
	}
	if stat.Code != "0" {
		s.logger.Error("logout failed", "message", stat.Message)
		return
	}
	s.logger.Info("logged out")
}

func (s *Session) macByIP() string {
	mac, _ := nnet.GetIPMAC(s.loginIfIP)
	return mac
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
```

在 `sessionConfig` 中补上 `newDialer` 字段并在 `newSession` 中赋值：

```go
// （已在上方 Step 2 的最终代码中体现，此处无需重复）
```

- [ ] **Step 3: 写 engine.go**

`engine/engine.go`:

```go
// Package engine 提供可嵌入的 portal 登录引擎，无全局状态、context 驱动、事件输出。
package engine

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/summonhim/gzgspd/config"
)

// Options 配置 Engine 行为。
type Options struct {
	Logger *slog.Logger
	// BufferedEvents 指定事件通道缓冲，未设置时按会话数推算。
	BufferedEvents int
}

// Engine 管理一组 Session。
type Engine struct {
	cfg      *config.Config
	logger   *slog.Logger
	sessions []*Session
	events   chan Event
	keyIndex map[string]*Session

	mu     sync.Mutex
	cancel context.CancelFunc
}

// New 依据配置构造 Engine。
func New(cfg *config.Config, opts Options) (*Engine, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is nil")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	buf := opts.BufferedEvents
	if buf <= 0 {
		buf = 2*len(cfg.Instance) + 16
	}

	e := &Engine{
		cfg:      cfg,
		logger:   logger,
		events:   make(chan Event, buf),
		keyIndex: make(map[string]*Session),
	}

	for _, inst := range cfg.Instance {
		s := newSession(sessionConfig{
			inst:   inst,
			key:    inst.Username + "@" + keyIfName(inst),
			logger: logger,
			pause:  cfg.PauseDuration(),
			events: e.events,
		})
		if _, dup := e.keyIndex[s.Key()]; dup {
			return nil, fmt.Errorf("duplicate session key %q (same username and interface)", s.Key())
		}
		e.keyIndex[s.Key()] = s
		e.sessions = append(e.sessions, s)
	}

	return e, nil
}

// Sessions 返回全部会话。
func (e *Engine) Sessions() []*Session { return e.sessions }

// Session 按键查找会话。
func (e *Engine) Session(key string) (*Session, bool) {
	s, ok := e.keyIndex[key]
	return s, ok
}

// Events 返回事件通道，Engine.Run 返回后通道关闭。
func (e *Engine) Events() <-chan Event { return e.events }

// Run 启动全部会话，阻塞至 ctx 取消并完成全部登出。
func (e *Engine) Run(ctx context.Context) error {
	ctx, cancel := context.WithCancel(ctx)
	e.mu.Lock()
	e.cancel = cancel
	e.mu.Unlock()
	defer cancel()

	var wg sync.WaitGroup
	for _, s := range e.sessions {
		wg.Add(1)
		go func(s *Session) {
			defer wg.Done()
			if err := s.Start(ctx); err != nil && err != context.Canceled {
				e.logger.Error("session ended with error", "key", s.Key(), "error", err)
			}
		}(s)
	}
	wg.Wait()
	close(e.events)
	e.logger.Info("all sessions stopped")
	return nil
}

// Stop 取消运行中的 Engine。
func (e *Engine) Stop() {
	e.mu.Lock()
	cancel := e.cancel
	e.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func keyIfName(inst config.ConfigInstance) string {
	if inst.Interface == "" {
		return "Auto"
	}
	return inst.Interface
}
```

注意：`engine` 不 import `nnet` 只为 key（用本地 `keyIfName`），避免重复依赖；`session.go` 仍 import `nnet` 做接口解析。

- [ ] **Step 4: 删除 executor**

```bash
git rm executor/worker.go
```

- [ ] **Step 5: 写 engine 测试**

`engine/session_test.go`:

```go
package engine

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/portal"
)

type fakeDialer struct {
	needLogin   bool
	loginURL    string
	checkErr    error
	authCode    string
	authErr     error
	authCalls   int
	disconnCall int
}

func (f *fakeDialer) PortalChecker(ctx context.Context, kAliveLink string) (bool, string, error) {
	if f.checkErr != nil {
		return false, "", f.checkErr
	}
	return f.needLogin, f.loginURL, nil
}

func (f *fakeDialer) PortalJsonAction(ctx context.Context, r portal.ActionRequest) (*portal.ActionResponse, error) {
	var resp portal.ActionResponse
	resp.ServerForm.Serverip = "10.0.0.9"
	resp.ServerForm.PortalVer = 4
	resp.PortalConfig.ID = 1
	return &resp, nil
}

func (f *fakeDialer) QuickAuth(ctx context.Context, r portal.QuickAuthRequest) (*portal.QuickAuthResponse, error) {
	f.authCalls++
	if f.authErr != nil {
		return nil, f.authErr
	}
	code := f.authCode
	if code == "" {
		code = "0"
	}
	return &portal.QuickAuthResponse{Code: code, Message: "msg", GroupID: 19, UserID: "u1"}, nil
}

func (f *fakeDialer) QuickAuthDisconn(ctx context.Context, r portal.DisconnRequest) (*portal.QuickAuthResponse, error) {
	f.disconnCall++
	return &portal.QuickAuthResponse{Code: "0"}, nil
}

func (f *fakeDialer) Close() {}

func newTestSession(t *testing.T, inst config.ConfigInstance, pause time.Duration, d sessionDialer) *Session {
	t.Helper()
	events := make(chan Event, 64)
	s := newSession(sessionConfig{
		inst:   inst,
		key:    inst.Username + "@Test",
		pause:  pause,
		events: events,
		newDialer: func(localIP string) (sessionDialer, error) {
			return d, nil
		},
	})
	// 跳过真实网卡探测，并给出稳定的出口地址供日志与登出使用
	s.prepareInterface = func() error {
		s.loginIf, s.loginIfIP, s.mac = "lo", "127.0.0.1", "00:00:00:00:00:00"
		return nil
	}
	s.loginIf, s.loginIfIP, s.mac = "lo", "127.0.0.1", "00:00:00:00:00:00"
	return s
}

func TestSessionLoginSuccess(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?wlanuserip=1&wlanacname=ac&mac=aa&vlan=1&hostname=h&rand=r"}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 5, RetryTime: 5}
	s := newTestSession(t, inst, time.Minute, d)
	s.prepareInterface = func() error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()
	_ = s.Start(ctx)

	if d.authCalls == 0 {
		t.Fatal("expected at least one auth attempt")
	}
	if d.disconnCall == 0 {
		t.Fatal("expected logout on shutdown")
	}
	if s.State() != StateStopped {
		t.Fatalf("expected Stopped, got %v", s.State())
	}
}

func TestSessionPauseOnMaxRetryResetsCounter(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?a=1", authErr: errors.New("boom")}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 2}
	s := newTestSession(t, inst, 50*time.Millisecond, d)
	s.prepareInterface = func() error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = s.Start(ctx) }()

	deadline := time.After(3 * time.Second)
	for {
		if s.State() == StateStopped {
			t.Fatal("session stopped unexpectedly")
		}
		select {
		case <-deadline:
			t.Fatal("session did not exceed max retries in time")
		default:
		}
		if d.authCalls >= 3 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func TestSessionContextCancelIsFast(t *testing.T) {
	d := &fakeDialer{needLogin: true, loginURL: "http://p/portal.do?a=1"}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 3600, RetryTime: 3600}
	s := newTestSession(t, inst, time.Minute, d)
	s.prepareInterface = func() error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(200 * time.Millisecond)

	start := time.Now()
	cancel()
	deadline := time.Now().Add(2 * time.Second)
	for s.State() != StateStopped {
		if time.Now().After(deadline) {
			t.Fatal("session did not stop after cancel")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if time.Since(start) > 2*time.Second {
		t.Fatal("shutdown too slow")
	}
}

func TestSessionCheckerErrorDoesNotLogin(t *testing.T) {
	d := &fakeDialer{checkErr: errors.New("network down")}
	inst := config.ConfigInstance{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1, RetryMax: 1}
	s := newTestSession(t, inst, 50*time.Millisecond, d)
	s.prepareInterface = func() error { return nil }

	ctx, cancel := context.WithCancel(context.Background())
	go func() { _ = s.Start(ctx) }()
	time.Sleep(300 * time.Millisecond)
	cancel()
	time.Sleep(100 * time.Millisecond)

	if d.authCalls != 0 {
		t.Fatalf("expected no auth attempts when checker fails, got %d", d.authCalls)
	}
}
```

**注意**：测试将 `s.prepareInterface` 替换为 no-op 以跳过真实网卡探测。为让该赋值可编译，`Session` 需将接口准备抽为可替换字段：

```go
type Session struct {
	// ...
	prepareInterface func() error
}
```

并在 `Start` 中调用 `s.prepareInterface`（在 `newSession` 中默认指向 `s.prepareInterfaceImpl`，后者即上文 `prepareInterface` 方法体，重命名为 `prepareInterfaceImpl`）。

`engine/engine_test.go`:

```go
package engine

import (
	"context"
	"testing"
	"time"

	"github.com/summonhim/gzgspd/config"
)

func TestEngineDuplicateKey(t *testing.T) {
	cfg := &config.Config{Instance: []config.ConfigInstance{
		{Username: "u", Password: "p", KeepAlive: 5, RetryTime: 5},
		{Username: "u", Password: "p2", KeepAlive: 5, RetryTime: 5},
	}}
	if _, err := New(cfg, Options{}); err == nil {
		t.Fatal("expected duplicate key error")
	}
}

func TestEngineRunClosesEvents(t *testing.T) {
	cfg := &config.Config{Instance: []config.ConfigInstance{
		{Username: "u", Password: "p", Interface: "lo", KeepAlive: 1, RetryTime: 1},
	}}
	e, err := New(cfg, Options{})
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	// 替换为 fake，避免真实网络
	for _, s := range e.sessions {
		s.newDialer = func(localIP string) (sessionDialer, error) {
			return &fakeDialer{}, nil
		}
		s.prepareInterface = func() error { return nil }
	}

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	done := make(chan struct{})
	go func() { _ = e.Run(ctx); close(done) }()

	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("engine did not stop")
	}

	// 通道应已关闭
	select {
	case _, ok := <-e.Events():
		if ok {
			// 可能仍有缓冲事件，继续读到关闭
			for range e.Events() {
			}
		}
	case <-time.After(time.Second):
		t.Fatal("events channel not closed")
	}
}

func TestEngineInvalidConfig(t *testing.T) {
	if _, err := New(&config.Config{}, Options{}); err == nil {
		t.Fatal("expected validation error for empty instance")
	}
}
```

- [ ] **Step 6: 构建验证**

Run: `go build ./...`
Expected: exit 0。

- [ ] **Step 7: 提交**

```bash
git add engine/ 
git rm executor/worker.go
git add main.go
git commit -m "feat: 引入 engine 包替换 executor，无全局状态且 context 驱动"
```

---

## Task 6: internal/app 外壳与根 main.go

**Files:**
- Create: `internal/app/app.go`
- Modify: `main.go`
- Delete: `config/flags.go`（flags 移入 app）

**Interfaces:**
- Consumes: `engine.Engine`、`config.LoadConfig`、`logx.New`、`version.Value`
- Produces: `app.Main() int`

- [ ] **Step 1: 写 app.go**

`internal/app/app.go`:

```go
// Package app 是进程外壳：解析参数、加载配置、驱动 Engine、处理信号与退出码。
package app

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/summonhim/gzgspd/config"
	"github.com/summonhim/gzgspd/engine"
	"github.com/summonhim/gzgspd/internal/logx"
	"github.com/summonhim/gzgspd/internal/version"
)

// Main 是进程入口，返回退出码。
func Main() int {
	fs := flag.NewFlagSet("gzgspd", flag.ContinueOnError)
	defaultConfig := "config.json"
	if v := firstEnv("GZGSPD_CONFIG_FILE", "GSGZPD_CONFIG_FILE"); v != "" {
		defaultConfig = v
	}
	configFile := fs.String("config", defaultConfig, "Specify the configuration file path.")
	showVersion := fs.Bool("version", false, "Display current version of gzgspd.")
	testConfig := fs.Bool("test", false, "Test configuration and exit.")

	if err := fs.Parse(os.Args[1:]); err != nil {
		return 2
	}

	if *showVersion {
		fmt.Printf("gzgspd %s %s %s with %s %s\n",
			version.Value, runtime.GOOS, runtime.GOARCH, runtime.Version(), version.BuildTime)
		return 0
	}

	cfg, err := config.LoadConfig(*configFile)
	if err != nil {
		fmt.Printf("Failed to load configuration file: %v\n", err)
		return 10
	}

	if *testConfig {
		fmt.Println("The configuration file test passed.")
		return 0
	}

	logger, closeLog := logx.New(cfg.LogLevel, cfg.LogPath)
	defer closeLog()

	eng, err := engine.New(cfg, engine.Options{Logger: logger})
	if err != nil {
		fmt.Printf("Failed to initialize engine: %v\n", err)
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("starting gzgspd", "version", version.Value, "sessions", len(eng.Sessions()))
	if err := eng.Run(ctx); err != nil {
		fmt.Printf("%v\n", err)
		return 1
	}
	return 0
}

func firstEnv(keys ...string) string {
	for _, k := range keys {
		if v, ok := os.LookupEnv(k); ok && v != "" {
			return v
		}
	}
	return ""
}
```

- [ ] **Step 2: 替换根 main.go**

`main.go`:

```go
package main

import (
	"os"

	"github.com/summonhim/gzgspd/internal/app"
)

func main() {
	os.Exit(app.Main())
}
```

- [ ] **Step 3: 删除 config/flags.go**

`config/flags.go` 的 `Flags`/`ParseFlags` 不再被使用。

- [ ] **Step 4: 构建验证**

Run: `go build ./...`
Expected: exit 0。

- [ ] **Step 5: 提交**

```bash
git rm config/flags.go
git add internal/app/app.go main.go
git commit -m "refact: 抽出 internal/app 进程外壳"
```

---

## Task 7: 统一构建、测试、vet、tidy

**Files:** 无新增；可能修改 `go.mod`/`go.sum`

- [ ] **Step 1: 格式化**

Run: `gofmt -l .`
Expected: 仅列出尚无文件的目录或空输出。若有文件列出，执行 `gofmt -w <file>`。

- [ ] **Step 2: 构建**

Run: `go build ./...`
Expected: exit 0。

- [ ] **Step 3: vet**

Run: `go vet ./...`
Expected: exit 0，无输出。

- [ ] **Step 4: 统一测试**

Run: `go test ./... -count=1`
Expected: 全部 `ok`，0 FAIL。

- [ ] **Step 5: 整理依赖**

Run: `go mod tidy`
Expected: 移除不再使用的间接依赖（beeep/systray 相关）。执行后再次 `go build ./...` 与 `go test ./...` 确认仍通过。

- [ ] **Step 6: 提交**

```bash
git add go.mod go.sum
git commit -m "chore: go mod tidy 移除未使用依赖"
```

---

## Task 8: 更新 CI 与示例配置

**Files:**
- Modify: `.github/workflows/ci.yml`
- Modify: `config.deploy.json:28`

- [ ] **Step 1: 修正示例配置字段名**

`config.deploy.json` 中第 28 行的 `"max_retry": 3` 改为 `"retry_max": 3`。

- [ ] **Step 2: CI 增加 vet 与 test 门禁**

在 `.github/workflows/ci.yml` 的 "Build core" step 之前插入：

```yaml
      - name: Vet
        run: go vet ./...

      - name: Test
        run: go test ./... -count=1
```

- [ ] **Step 3: 验证 YAML 语法**

Run: `git diff .github/workflows/ci.yml`
Expected: 仅新增上述两个 step，其余不变。

- [ ] **Step 4: 提交**

```bash
git add .github/workflows/ci.yml config.deploy.json
git commit -m "ci: 增加 vet/test 门禁并修正示例配置字段名"
```

---

## 覆盖对照（spec → task）

| Spec 要求 | 对应 Task |
|-----------|-----------|
| version 迁移 + ldflags | Task 1 |
| LogPath 生效 | Task 2 |
| Config.Path/Save/PauseDuration | Task 2 |
| GetKeyIfName 修复 | Task 3 |
| portal Client/struct/context | Task 4 |
| PortalChecker 返回 error | Task 4 |
| engine State/Event/Session/Engine | Task 5 |
| 重试语义修正（RetryTime/归零/Pause） | Task 5 |
| HTTP client 复用 | Task 4 + Task 5 |
| internal/app 外壳 | Task 6 |
| go mod tidy | Task 7 |
| CI vet/test | Task 8 |
| config.deploy.json 修正 | Task 8 |
| 测试（用户指定的统一模式） | Task 2/3/4/5 写测试，Task 7 Step 4 统一运行 |
