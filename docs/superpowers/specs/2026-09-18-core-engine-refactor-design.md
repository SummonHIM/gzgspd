# gzgspd Core/Engine 重构设计

日期：2026-09-18

## 背景与目标

当前 gzgspd 是一个 800 行左右的单体 daemon：`main.go` 直接管进程生命周期与信号，`executor/worker.go` 用包级全局 map 维护状态，`portal/portal.go` 的协议函数有 18 个位置参数，`nnet` 反向依赖 `config`。这些结构使得「把 core 抽出来做成可嵌入库、后续另开 GUI」这件事无法进行。

本次目标：

1. 修复纵览中确认的 6 个 bug。
2. 把执行逻辑重构为可嵌入的 `engine` 包：无全局状态、context 驱动、事件流输出、日志可注入。
3. 为后续 `gzgspgui` 提供稳定的 `import` 边界。

明确不做（YAGNI）：配置热重载、GUI IPC 协议、密码加密存储、i18n。

## 约束

- **配置 JSON 字段名保持不变**（已确认）。`retry_time` 从「被忽略」变为「生效」是行为修正，不是格式变更。
- 现有部署（openwrt/systemd/nssm）的启动方式与 `--config/--version/--test` 参数不变。
- CI 的产物命名与路径不变。

## 目标包结构

```
main.go                     根目录最小引导（保留，使 `go build` 可用）
internal/version/           版本号与构建时间（ldflags 注入点）
internal/app/               进程外壳：flags、signal、os.Exit
config/                     纯数据 + 校验 + Load/Save（新增 Path 记忆）
nnet/                       纯网络工具（移除对 config 的依赖）
portal/                     纯协议实现（struct 参数 + context）
engine/                     ★ 引擎：Engine / Session / Event / State
```

依赖方向单向：`engine → portal → nnet`，`engine → config`。`internal/app → engine, config, internal/version`。`nnet` 不再 import 任何本项目包。

`engine` 为公开包（非 `internal`），GUI 直接 `import github.com/summonhim/gzgspd/engine`。

## engine 公共 API

```go
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

func (s State) String() string   // 可读名，用于日志/GUI

type Event struct {
    Key     string    // username@interface，唯一标识
    Time    time.Time
    State   State
    Message string
    Err     error     // 仅在失败事件中非 nil
}

type Session struct { /* 私有 */ }

func (s *Session) Key() string
func (s *Session) State() State          // 加锁读
func (s *Session) Start(ctx context.Context) error  // 阻塞至 ctx 取消并完成登出
func (s *Session) Pause()                // 请求登出并暂停，ctx 保持存活
func (s *Session) Resume()               // 从暂停恢复

type Options struct {
    Logger *slog.Logger   // nil 时使用 slog.Default()
}

type Engine struct { /* 私有 */ }

func New(cfg *config.Config, opts Options) (*Engine, error)
func (e *Engine) Sessions() []*Session
func (e *Engine) Events() <-chan Event     // 缓冲通道，GUI 订阅
func (e *Engine) Run(ctx context.Context) error   // 启动全部 Session，ctx 取消即全部登出
```

设计约束：

- Engine/Session 内部**没有包级可变状态**，不使用 `slog.SetDefault`。
- Engine 不注册信号、不调用 `os.Exit`、不读 `os.Args`。
- 所有阻塞（网络、sleep、重试等待）都接受 `ctx`，取消后应尽快返回。
- `Events()` 通道由 Engine 关闭于 `Run` 返回时；缓冲足够避免消费者慢导致阻塞（容量按 2×会话数 + 16）。

## Session 状态机与重试语义（修正现有 bug）

单轮循环：

1. `keep_alive_link` 探测（`PortalChecker`）→ 需要登录则执行登录流程。
2. 登录成功：`retry = 0`，状态 `StateLoggedIn`，等待 `KeepAlive` 秒。
3. 登录失败：`retry++`，状态 `StateNotLoggedIn`，等待 `RetryTime` 秒。
4. `RetryMax != 0 && retry >= RetryMax`：状态 `StatePaused`，等待固定的 `pauseDuration`（默认 10 分钟），**等待结束后 `retry` 归零**。`RetryMax == 0` 表示不限次。
5. 收到 ctx 取消或 Pause 请求：状态 `StateLoggingOut` → 执行登出 → `StateStopped`。

所有等待使用 `select { case <-ctx.Done(): case <-time.After(d): }`，保证可取消。

`Interface == ""` 时每轮重新探测默认网口的行为保留。

HTTP client 按 Session 创建一次并复用，Session 结束时 `CloseIdleConnections`。

## portal 包改动

协议函数由位置参数改为请求结构体，并接受 context：

```go
type Client struct { /* 复用 http.Client，绑定本地 IP */ }
func NewClient(localIP string, timeout time.Duration) (*Client, error)

type ActionRequest struct {
    Scheme, Host, UserAgent              string
    Wlanuserip, Wlanacname, MAC          string
    VLAN, Hostname, Rand                 string
}
func (c *Client) PortalJsonAction(ctx context.Context, r ActionRequest) (*ActionResponse, error)

type QuickAuthRequest struct {
    Scheme, Host, UserAgent                      string
    UserID, Password                             string
    Wlanuserip, Wlanacname, WlanacIP             string
    VLAN, MAC                                    string
    Version, PortalPageID                        int
    Timestamp                                    int64
    UUID, PortalType, Hostname, Rand             string
}
func (c *Client) QuickAuth(ctx context.Context, r QuickAuthRequest) (*QuickAuthResponse, error)

type DisconnRequest struct { /* 登出所需字段 */ }
func (c *Client) QuickAuthDisconn(ctx context.Context, r DisconnRequest) (*QuickAuthResponse, error)

// PortalChecker 返回三值：是否需要登录、登录链接、错误
func (c *Client) PortalChecker(ctx context.Context, kAliveLink string) (needLogin bool, loginURL string, err error)
```

`PortalChecker` 返回 error 是关键改动：网络失败不再被解释为「已在线」。调用方须区分「确定在线」与「探测失败」。

## Bug 修复清单

| # | 位置 | 修复 |
|---|------|------|
| 1 | `nnet/ifutils.go:92` | `GetKeyIfName` 判断写反，永远返回 "Auto"；修正为「空则 Auto，否则返回配置值」 |
| 2 | `executor/worker.go:316` | `RetryTime` 生效（此前误用 `KeepAlive`） |
| 3 | `executor/worker.go:320` | 达到上限暂停后 `retry` 未归零，导致死循环；改为暂停结束归零 |
| 4 | `main.go:44` | 全局 map 未加锁写；全局状态整体移除后不再存在 |
| 5 | `config/configs.go:24` | `LogPath` 被忽略；改为真正用于文件日志输出 |
| 6 | `config.deploy.json:28` | 字段名拼写 `max_retry` → `retry_max` |
| 7 | `portal/portal.go:327` | PortalChecker 吞掉所有错误；改为返回 error |
| 8 | `main.go:17` | 版本号硬绑 main 包；移至 `internal/version`，同步更新 `ci.yml:133` 的 ldflags |

## 日志与事件的分工

- **日志**：注入的 `*slog.Logger`，Session 日志带 `key` 字段。`LogPath` 非空时写入文件（`MultiWriter` 到 stdout + 文件，或在 configure 时选择目标）。
- **事件**：状态变更与失败原因，供 GUI 消费。二者不互相替代。

## 测试策略

因用户要求「尽量写完再统一测试」，本次不逐功能执行 red-green，而是：

1. 实现 + 测试一起写。
2. 全部完成后一次性运行 `go test ./...`。
3. 用覆盖表核对每个可测点确有对应用例（见下），弥补跳过「看红」带来的盲区。

计划用例：

- `config/configs_test.go`：Validate 各边界（空 instance/空 username/空 password/keep_alive<=0/retry_max<0/retry_time<=0）；Load/Save 往返；未知字段 `max_retry` 被忽略（不报错、不生效）。
- `nnet/ifutils_test.go`：`GetKeyIfName` 空返回 "Auto"、非空返回配置值；`GetIPMAC` 非法/非 IPv4 输入报错；`GetIfMAC` 不存在接口报错。
- `portal/portal_test.go`：以 `httptest.Server` 验证 QuickAuth 的 query 构造、Disconn 的表单构造与 Content-Type、302 + portal 关键字识别、200 页面内 `location.replace` 的 otto 解析、非 2xx 与超时的错误传播。
- `engine/session_test.go`：依赖注入 fake 网络行为，验证——登录成功状态序列、失败计数递增、达上限进入 Paused 且计数归零、`RetryTime` 被使用、ctx 取消后状态到 Stopped 且及时返回、Pause/Resume。

engine 的可测性与 GUI 可插拔性都依赖一个网络 seam：Session 依赖一个小接口（探测/取基本信息/登录/登出四个方法），生产实现包装 portal.Client，测试用 fake。这是为测试与 GUI 共同引入的唯一抽象。

## 工程化收尾

- `go mod tidy` 移除未使用的间接依赖（beeep、systray 等传递依赖）。
- CI（`.github/workflows/ci.yml`）增加 `go vet ./...` 与 `go test ./...` 步骤，置于 build 之前。
- `internal/version` 提供 `Version`、`BuildTime` 变量与 `String()`。

## 风险

- ldflags 包路径变更若不同步更新 CI，版本号会静默回落到 "dev"。缓解：CI 门禁加 `--version` 输出校验（可选）。
- 保留根 `main.go` 使 `internal/` 目录名与包名不完全对应（`internal/app` 的包名为 `app`），可接受。
