# Categraf 配置文件 Reload 设计方案

## 背景

当前 `kill -HUP $(pidof categraf)` 触发的 reload 主要调用 `Agent.Reload()`，也就是对同一个 agent 对象执行 `Stop()` 再 `Start()`。这个方式对部分模块不安全，尤其是 `LogsAgent`：

- `LogsAgent` 的任务来自全局 `config.Config.Logs.Items`，如果不重新解析磁盘配置，`conf/logs.toml` 的变更不会生效。
- `LogsAgent` 内部的 `LogSources`、scanner、pipeline 等对象不适合 stop 后复用。
- 如果 reload 失败后已经污染了全局 `config.Config`，旧 agent 仍运行但读取到半新半旧配置，会产生不一致行为。

`http_provider.go` 的主要思路值得参考：provider 维护配置缓存，reload 时先解析新配置、计算差异，再对受影响模块做局部重建。社区版本次先实现“本地配置文件 reload”的稳定子集，范围收窄到 logs 配置。

## 目标

本阶段目标：

- `SIGHUP` 后重新加载本地配置文件。
- 支持 `conf/logs.toml` 或合并配置中的 `[logs]` 热更新。
- `LogsAgent` 使用新配置全量重建，不复用旧对象。
- reload 失败时旧 agent 和旧 `config.Config` 不受影响。
- 对暂不支持热更新的配置变更给出明确日志提示。

非目标：

- 本阶段不热更新 writer、HTTP API、heartbeat、log output、ibex 等全局服务。
- 本阶段不重启整个 `Agent`。
- 本阶段不做文件 watch，只处理显式 `SIGHUP`。
- 本阶段不要求 metrics input 的已有 provider reload 行为重构。

## 总体原则

1. 配置加载与提交分离。

   解析磁盘配置时不得修改全局 `config.Config`。只有所有预检都成功后，才提交新配置。

2. 模块局部重建。

   `SIGHUP` 不再调用旧的 `Agent.Reload()`。需要 reload 的模块由协调器单独处理。

3. `LogsAgent` 全量重建。

   不对旧 `LogsAgent` 执行 `Stop()` 后再 `Start()`。正确流程是：先创建新 `LogsAgent`，成功后 stop 旧 `LogsAgent`，替换引用，再 start 新对象。

4. 失败不影响现状。

   新配置解析失败、新模块创建失败、校验失败时，旧配置和旧 agent 继续运行。

5. 明确热更新边界。

   对不支持热更新的配置项，如果发生变化，记录 warning，提示需要重启进程。

## 推荐实现步骤

### 1. 拆分配置加载函数

在 `config/config.go` 中增加无副作用加载函数：

```go
func LoadConfig(configDir string, debugLevel int, debugMode, testMode bool, interval int64, inputFilters string) (*ConfigType, error)
```

职责：

- 检查 `config.toml` 是否存在。
- 创建新的 `ConfigType`。
- 调用 `cfg.LoadConfigByDir(configDir, newConfig)`。
- 应用 CLI 参数覆盖和默认值，例如 interval、precision、writer_opt、test mode log output。
- 不修改 `config.Config`。
- 不启动或更新 `HostInfo`。

保留 `InitConfig(...) error` 作为启动路径：

```go
func InitConfig(...) error {
    newConfig, err := LoadConfig(...)
    if err != nil {
        return err
    }
    newHostInfo, err := LoadHostInfo(newConfig)
    if err != nil {
        return err
    }
    Config = newConfig
    CommitHostInfo(newHostInfo)
    print config if needed
    return nil
}
```

### 2. 拆分 HostInfo 加载与提交

当前 `InitHostInfo()` 会读取全局 `Config`。需要拆成两步：

```go
func LoadHostInfo(cfg *ConfigType) (*HostInfoCache, error)
func CommitHostInfo(newHostInfo *HostInfoCache)
func InitHostInfo(cfg *ConfigType) error
```

要求：

- `LoadHostInfo` 使用传入的 `cfg`，不要读取全局 `Config`。
- `GetOutboundIP` 改为接收 `*ConfigType`：

```go
func GetOutboundIP(cfg *ConfigType) (net.IP, error)
```

- `CommitHostInfo` 只在首次启动时启动一个 `HostInfo.update()` goroutine。
- 后续 reload 只更新已有 `HostInfo` 的字段，不重复启动 goroutine。
- `logs/util/docker/containers.go` 中的 `GetDockerHostIPs()` 也需要同步改为 `GetOutboundIP(config.Config)` 或等价形式。

注意：

- `HostInfo.update()` 可以继续读取当前全局 `Config`，因为它代表运行中已提交配置。
- `LoadHostInfo(newConfig)` 必须在提交 `config.Config = newConfig` 前完成。

### 3. Agent 支持按模块替换

建议给 `AgentModule` 增加模块名：

```go
type AgentModule interface {
    Start() error
    Stop() error
    Name() string
}
```

为各模块定义稳定名称：

```go
const (
    MetricsAgentName    = "metrics-agent"
    LogsAgentName       = "logs-agent"
    PrometheusAgentName = "prometheus-agent"
    IbexAgentName       = "ibex-agent"
)
```

`Agent` 增加按模块访问和替换方法：

```go
type Agent struct {
    agents []AgentModule
}

func (a *Agent) GetAgent(name string) AgentModule
func (a *Agent) SetAgent(name string, ag AgentModule)
func (a *Agent) StopAgent(name string) error
```

第一阶段可以先不加 `sync.RWMutex`。当前 reload 由 signal handler goroutine 串行触发，没有其他 goroutine 并发修改 `agents` 切片。可以在方法注释中说明这些方法必须在 signal handler 或同等串行上下文中调用。后续如果增加 HTTP API 触发 reload，再引入锁。

模块替换职责建议拆开，不要做一个过大的 `ReplaceAgent`：

- `GetAgent(name)`：获取旧模块。
- `StopAgent(name)`：停止旧模块。
- `SetAgent(name, newAg)`：替换模块引用。

这样 `main.go` 可以明确控制关键顺序：先构造新模块，成功后 stop 旧模块，再提交全局配置，最后 set/start 新模块。

### 4. 移除或禁用旧 `Agent.Reload()`

旧方法语义是复用同一批 agent 对象：

```go
a.Stop()
a.Start()
```

这个语义对 `LogsAgent` 不安全。建议：

- 删除 `Agent.Reload()`。
- 或保留但不在 `SIGHUP` 中使用，并在注释中标明不要用于配置 reload。

`main.go` 的 `SIGHUP` 必须改走新的配置 reload 协调器。

### 5. LogsAgent 显式接收配置

`NewLogsAgent` 改为：

```go
func NewLogsAgent(cfg *config.ConfigType) AgentModule
```

`LogsAgent` 保存配置指针：

```go
type LogsAgent struct {
    ...
    cfg *config.ConfigType
}
```

构造和启动过程中，logs 相关配置应从 `cfg` 或 `la.cfg` 读取，不要读取全局 `config.Config`。

需要重点避免在 `LogsAgent` 构造和启动路径中读取旧全局配置。以下配置必须来自传入的 `cfg` 或 `la.cfg`：

- `GetLogRunPath`
- `NumberOfPipelines`
- `OpenLogsLimit`
- `MaxTraverseLimit`
- `MaxDepthLimit`
- `FileScanPeriod`
- `LogFrameSize`
- `EnableCollectContainer`
- `GetContainerCollectAll`
- `BatchConcurrence`
- `BatchMaxSize`
- `BatchMaxContentSize`
- `GlobalProcessingRules`
- `BuildEndpoints`

不要修改 `config/logs.go` 中现有公开 helper 的签名。这些函数被 `logs/` 子包和部分 `inputs/` 包调用，直接改签名会造成大量级联修改，超出本阶段范围。

推荐做法是在 `agent` 包内部抽私有小函数，直接从传入的 `cfg.Logs` 计算默认值：

```go
func logsRunPath(cfg *config.ConfigType) string
func logsPipelineCount(cfg *config.ConfigType) int
func logsOpenFilesLimit(cfg *config.ConfigType) int
func logsBatchMaxSize(cfg *config.ConfigType) int
```

注意：

- 这些函数只返回带默认值的结果，不要修改传入的 `cfg`。
- `config/logs.go` 的旧 helper 保持原样，继续给既有 `logs/` 子包运行时路径使用。
- `BuildEndpoints`、`GlobalProcessingRules` 等 agent 内部函数可以改为接收 `*config.ConfigType`，因为调用点集中在 `agent` 包内。
- 如果遇到必须被 `logs/` 子包调用的默认值函数，第一阶段优先在 agent 内联或新增私有 helper，不扩散公共 API 改动。

### 6. LogsAgent Stop 清理 sources

`LogsAgent.Stop()` 中清理 `LogSources`，防止未来误复用时重复追加：

```go
func (a *LogsAgent) Stop() error {
    if a.sources != nil {
        a.sources.Clear()
    }
    ...
}
```

在 `config/logs/sources.go` 增加：

```go
func (s *LogSources) Clear() {
    s.mu.Lock()
    defer s.mu.Unlock()
    s.sources = nil
}
```

更理想的长期方案是让 `LogsAgent` 明确不可复用，但本次增加 `Clear` 可以降低误用风险。

### 7. Reload 协调器

建议新增预检函数，例如放在 `agent/reload.go`：

```go
type ReloadPlan struct {
    NewLogsAgent AgentModule
    LogsReloaded bool
    RestartRequired []string
}

func (a *Agent) PrepareReload(newCfg *config.ConfigType) (*ReloadPlan, error)
```

职责：

1. 比较旧配置 `config.Config` 与 `newCfg`。
2. 判断哪些模块需要热更新。
3. 对 logs 变更创建新 `LogsAgent`，但不 stop 旧模块、不提交全局配置、不 start 新模块。
4. 返回 reload plan，交给 `main.go` 按固定顺序执行。

第一阶段 diff 规则：

- 为降低复杂度，`SIGHUP` 成功加载配置后可以直接重建 `LogsAgent`，不必先对 `Logs` 做精细 diff。
- 如果想跳过无变化 reload，可以先只比较简单字段；不要用会引入复杂风险的深层比较作为必要条件。
- 如果 writer、HTTP、Heartbeat、Ibex、Log、Prometheus 等不同，加入 `RestartRequired`，但不热更新。
- metrics input 配置仍由现有 provider 处理。不要在这里重建 `MetricsAgent`。

说明：

- `config.Logs` 内嵌 `*sarama.Config` 等复杂字段，第一阶段不建议依赖 `reflect.DeepEqual` 作为 logs 变更判断。
- 即使 logs 未变化，重建一次 logs agent 的行为也比错误跳过 reload 更容易理解。后续可以增加 checksum 优化。

### 8. main.go 的 SIGHUP 流程

推荐流程：

```go
case syscall.SIGHUP:
    log.Println("I! received signal:", sig.String())
    log.Println("I! loading configuration...")

    newCfg, err := config.LoadConfig(...)
    if err != nil {
        log.Println("E! failed to load config:", err)
        continue
    }

    newHostInfo, err := config.LoadHostInfo(newCfg)
    if err != nil {
        log.Println("E! failed to load host info:", err)
        continue
    }

    plan, err := ag.PrepareReload(newCfg)
    if err != nil {
        log.Println("E! failed to reload config:", err)
        continue
    }

    if plan.LogsReloaded {
        if err := ag.StopAgent(agent.LogsAgentName); err != nil {
            log.Println("E! failed to stop old logs agent:", err)
            continue
        }
    }

    config.Config = newCfg
    config.CommitHostInfo(newHostInfo)

    if plan.LogsReloaded {
        ag.SetAgent(agent.LogsAgentName, plan.NewLogsAgent)
        if plan.NewLogsAgent != nil {
            if err := plan.NewLogsAgent.Start(); err != nil {
                log.Println("E! failed to start new logs agent:", err)
            }
        }
    }

    log reload result
```

重要顺序：

- `LoadConfig` 和 `LoadHostInfo` 失败时不能修改全局状态。
- `PrepareReload` 内部构造新模块失败时不能 stop 旧模块。
- 旧 `LogsAgent` stop 期间，全局 `config.Config` 应仍然是旧配置。
- `config.Config = newCfg` 必须在 stop 旧 `LogsAgent` 之后、start 新 `LogsAgent` 之前执行。
- 新 `LogsAgent` 构造阶段必须通过参数读取 `newCfg`，不要依赖全局 `config.Config` 已经切换。

### 9. build tags 同步

修改构造函数签名时，必须同步对应 no-op 文件。

本阶段如果只修改 `NewLogsAgent(cfg)`，只需要同步：

- `agent/logs_agent_none.go`

如果同时修改 `NewPrometheusAgent` 或 `NewIbexAgent` 签名，再同步：

- `agent/promethues_agent_none.go`
- `agent/ibex_agent_none.go`

例如：

```go
import coreconfig "flashcat.cloud/categraf/config"

func NewLogsAgent(cfg *coreconfig.ConfigType) AgentModule {
    return nil
}
```

需要验证：

```bash
go test -tags no_logs ./agent
go test -tags no_prometheus ./agent
go test -tags no_ibex ./agent
```

### 10. 测试与验收

最低测试集：

```bash
go test . ./agent ./config ./config/logs ./logs/...
go test -tags no_logs ./agent
go test -tags no_prometheus ./agent
go test -tags no_ibex ./agent
git diff --check
```

建议新增单元测试：

1. `config.LoadConfig` 无副作用

   - 设置旧 `config.Config`。
   - 调用 `LoadConfig` 读取新配置。
   - 断言全局 `config.Config` 未变化。

2. reload 坏配置不影响旧配置

   - 构造旧配置和旧 agent。
   - 新配置解析失败。
   - 断言旧 agent 未 stop，`config.Config` 未替换。

3. logs 变更触发重建

   - old logs 与 new logs 不同。
   - `PrepareReload` 只构造新 logs agent，不 stop 旧 logs agent。
   - main reload 执行阶段按 stop old -> commit config -> set/start new 的顺序运行。
   - 不调用旧 logs agent 的 stop/start 复用流程。

4. 非热更新配置变更提示 restart

   - writer 或 http 变化。
   - `ReloadPlan.RestartRequired` 包含对应模块。
   - 不重建整个 agent。

5. no_logs build tag

   - 确保 no-op 构造函数签名与正常构建一致。

## 风险与注意点

- `LogsAgent` 构造路径里任何全局 `config.Config` 读取都会破坏两阶段 reload，需要逐个清理。
- `util.Debug()` 等辅助函数如果读全局配置，只用于日志输出可以接受；如果影响构造行为，应改为显式配置。
- `config.Config` 仍是全局变量，长期更好方案是用配置快照传递到各模块。本阶段只要求 logs reload 路径做到不依赖未提交的全局配置。
- 如果新 logs agent start 失败，旧 logs agent 已经被 stop。第一阶段可先记录错误；更强的实现可以在 stop 前做更多校验，或创建另一个旧配置快照 agent 用于回滚，但不要复用已经 stop 的旧 `LogsAgent`。
- writer、api、heartbeat 等全局服务如果未来要支持热更新，应分别设计生命周期，不要搭整 agent reload 的便车。
- 不要修改 `config/logs.go` 中被 `logs/` 子包广泛调用的公开 helper 签名。优先在 `agent` 包内部新增私有 helper。

## 建议提交粒度

1. `config`: 增加 `LoadConfig`、`LoadHostInfo`、`CommitHostInfo`。
2. `agent`: 增加模块命名和按模块替换能力，移除 SIGHUP 对 `Agent.Reload()` 的依赖。
3. `agent/logs`: `NewLogsAgent(cfg)`，并在 agent 内部新增 logs 默认值私有 helper；不要改 `config/logs.go` 公开 helper 签名。
4. `main`: SIGHUP 改为两阶段 reload。
5. `tests`: 补 reload 单测和 build-tag 验证。
