---
title: "Categraf 配置文件结构详解：global、writer、heartbeat、inputs 与四类 Agent"
description: "本文从源码和实际目录结构出发，讲清楚 Categraf 的配置目录、global、writer、heartbeat、input 插件，以及 Metrics、Prometheus、Logs、Ibex 四类 Agent 的配置与数据链路。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Categraf配置", "remote write", "heartbeat", "input插件", "Prometheus Agent", "Logs Agent", "Ibex Agent", "logs.toml", "prometheus.yaml"]
author: "快猫星云"
date: "2026-07-17T00:00:00+08:00"
tags: ["Categraf", "Configuration", "Monitoring"]
---

第一次打开 Categraf 的 `conf` 目录，很多人会有类似疑问：为什么既有 `config.toml`、`logs.toml`，又有几十个 `input.*` 目录？`writer` 和 `heartbeat` 都指向夜莺，它们是不是一回事？`prometheus.yaml` 又走哪条链路？全局 `interval`、插件 `interval` 和实例 `interval_times` 到底谁生效？

这些问题如果没有先弄清楚，后面很容易出现“服务能启动但插件没运行”“测试模式有指标但后端没有数据”“改了 writer 却只发送了 HUP，配置没有生效”等现象。

本文从 Categraf 当前源码和默认配置出发，完整解释配置加载路径、Metrics Agent 与三个可选 Agent 模块，以及各个配置层级的协作方式。读完后，你应该能够独立判断一个配置应该写在哪里，数据最终走哪条发送链路，以及修改后该用测试、重载还是重启来验证。

本文重点核对的源码与样例包括 `agent/agent.go`、三个可选 Agent 的启动代码、`config/config.go`、`config/logs.go`、`prometheus/prometheus.go`、`conf/config.toml`、`conf/logs.toml` 和 `k8s/in_cluster_scrape.yaml`。

## 核心要点

- Categraf 先寻找配置目录中的 `config.toml`；默认目录是二进制旁的 `conf`，也可用 `--configs` 或 `CATEGRAF_CONFIGS` 指定。本地插件则放在 `input.<插件名>` 目录。
- 同一个 Categraf 进程包含 Metrics、Logs、Prometheus、Ibex 四类 Agent 模块；后三者按配置启用，不是三个独立的系统进程。
- Metrics Agent 的 `input.*` 指标走 `[[writers]]`；heartbeat 只上报 Agent 与主机元数据。两条链路相互独立。
- Logs Agent 读取 `logs.toml` 并发送到 `[logs].send_to`；Prometheus Agent 读取 YAML 中的 `scrape_configs` 和 `remote_write`，两者都不复用 Metrics Agent 的 writer。
- Ibex Agent 通过 RPC 领取并执行任务，需要最小权限与可信服务端；`--inputs` 不能关闭这三个可选 Agent，修改 `config.toml` 后通常应重启进程。

## 1. 先看完整配置结构

一个典型的 Categraf 发布目录如下：

```text
categraf/
├── categraf
└── conf/
    ├── config.toml
    ├── logs.toml
    ├── input.cpu/
    │   └── cpu.toml
    ├── input.mem/
    │   └── mem.toml
    ├── input.system/
    │   └── system.toml
    ├── input.mysql/
    │   └── mysql.toml
    ├── input.redis/
    │   └── redis.toml
    └── input.postgresql/
        └── postgresql.toml
```

这些文件可以分成三层：

```text
启动参数与环境变量
        |
        v
配置目录：config.toml、logs.toml
        |
        +---- global / writer / heartbeat / log / ibex / prometheus
        +---- logs：日志采集源与发送端
        |
        v
input.<plugin> 目录
        |
        +---- 插件级配置
        +---- 一个或多个 instances
        +---- 实例标签、过滤与处理规则
```

`config.toml` 是入口文件，Categraf 会先确认它存在，然后合并加载配置目录顶层的 TOML、JSON 和 YAML 配置。默认发布目录把主配置拆成 `config.toml` 和 `logs.toml`；它们最后会进入同一个主配置对象。指标插件由 input provider 管理；使用默认的本地 provider 时，它会另外扫描配置目录下名称以 `input.` 开头的目录。

例如：

```text
input.mysql       -> mysql 插件
input.postgresql  -> postgresql 插件
input.http_response -> http_response 插件
```

目录名不是任意名称。`input.` 后面的部分必须与 Categraf 注册的插件名一致，否则日志会出现 input 不受支持或插件没有启动。

## 2. Categraf 进程内有哪四类 Agent

从启动代码看，Categraf 会组装四类 Agent 模块：

```text
Categraf 进程
├── Metrics Agent
│   └── input.* -> 标签与过滤 -> [[writers]]
├── Logs Agent（可选）
│   └── logs.toml -> 文件/journald/TCP/UDP -> HTTP/TCP/Kafka
├── Prometheus Agent（可选）
│   └── prometheus.yaml -> scrape_configs -> WAL -> remote_write
└── Ibex Agent（可选）
    └── [ibex] -> RPC 领取任务 -> 本地执行与状态上报
```

Metrics Agent 是大家最熟悉的指标插件框架，负责加载 `input.*` 并把普通指标交给 `[[writers]]`。另外三个常被称为“外挂 Agent”，但这里的“外挂”是功能上的说法：它们与 Metrics Agent 运行在同一个 Categraf 进程里，并不是三个需要单独安装、单独管理的守护进程。

| Agent 模块 | 启用入口 | 主要职责 | 发送链路 |
| --- | --- | --- | --- |
| Metrics Agent | `[global].providers` 与 `input.*` | 运行 Categraf 指标插件 | `[[writers]]` |
| Logs Agent | `logs.toml` 的 `[logs]` | 采集、处理并发送日志 | `[logs].send_to` |
| Prometheus Agent | `config.toml` 的 `[prometheus]` | 使用 Prometheus 抓取配置和服务发现采集指标 | YAML 中的 `remote_write` |
| Ibex Agent | `config.toml` 的 `[ibex]` | 从 Ibex Server 领取并执行任务 | Ibex RPC |

这个划分是排障时最重要的地图：四个模块虽然共享进程和部分主机信息，但配置入口、发送协议和故障现象并不相同。看到“Categraf 进程正常”只能证明进程存活，不能证明四条链路都正常。

以上描述针对包含这些能力的常规构建。Categraf 源码还提供 `no_logs`、`no_prometheus`、`no_ibex` 构建标签；如果使用自行裁剪的二进制，相应模块即使写了配置也不会被编译进程序。

## 3. 配置目录是怎么确定的

Categraf 的默认启动方式是：

```shell
cd /opt/categraf
./categraf
```

默认配置目录为：

```text
conf
```

需要注意，Categraf 启动时会主动把当前工作目录切换到二进制所在目录。因此这里的相对路径是相对于 `categraf` 二进制，而不是执行命令前所在的 shell 目录。按照推荐布局把二进制放在 `/opt/categraf/categraf` 时，默认配置目录就是：

```text
/opt/categraf/conf
```

这种布局可以直接运行，也适合使用 `./categraf --install` 安装系统服务，通常不需要手工传入 `--configs`。

只有在配置不放在二进制旁的 `conf` 目录时，才需要显式指定自定义路径：

```shell
/opt/categraf/categraf --configs /path/to/categraf/conf
```

也可以通过环境变量设置：

```shell
export CATEGRAF_CONFIGS=/path/to/categraf/conf
/opt/categraf/categraf
```

命令行 `--configs` 的默认值来自 `CATEGRAF_CONFIGS`；没有设置环境变量时才使用 `conf`。

无论使用哪种方式，目标目录中必须存在：

```text
config.toml
```

否则 Categraf 会直接报错：

```text
configuration file(.../config.toml) not found
```

## 4. `[global]` 管什么

默认主配置的核心部分如下：

```toml
[global]
print_configs = false
hostname = ""
omit_hostname = false
interval = 15
providers = ["local"]
concurrency = -1

[global.labels]
region = "shanghai"
environment = "production"
```

常用字段可以这样理解：

| 配置项 | 作用 | 默认或建议 |
| --- | --- | --- |
| `interval` | 全局指标采集周期 | 未设置或小于等于 0 时为 15 秒 |
| `hostname` | `agent_hostname` 的来源 | 空字符串表示自动获取系统主机名 |
| `omit_hostname` | 是否不自动添加 `agent_hostname` | 一般保持 `false` |
| `labels` | 给所有指标补充全局标签 | 适合 region、environment、cluster |
| `providers` | input 配置来源 | 默认使用 `local` |
| `concurrency` | 每轮实例型 input 的并发上限 | 小于等于 0 时为 `CPU 核数 × 10` |
| `print_configs` | 启动时打印解析后的主配置 | 仅临时排查时使用 |

`hostname` 支持几个内置变量：

```toml
[global]
hostname = "$hostname-$ip"
```

支持的内置值包括：

- `$hostname`：操作系统主机名；
- `$ip`：Categraf 探测到的本机 IP；
- `$sn`：BIOS 序列号；
- 环境变量：按 `$NAME` 或 `${NAME}` 展开。

全局标签也支持相同的展开方式：

```toml
[global.labels]
region = "${REGION}"
host_ip = "$ip"
serial_number = "$sn"
```

不要在 `labels` 中放请求 ID、PID、完整 URL 参数、SQL 文本等高变化数据，否则会制造大量时序。

`print_configs = true` 会把解析后的主配置输出到终端，其中可能包含认证信息。排查结束后应立即关闭，并避免把输出粘贴到工单或公开聊天中。

## 5. 全局标签、实例标签和 `agent_hostname` 的优先级

Categraf 对一条指标处理标签时，大致按以下顺序进行：

```text
插件原始标签
    |
    v
实例或插件 labels：同名时覆盖
    |
    v
global.labels：只补充尚不存在的标签
    |
    v
agent_hostname：只在缺失且 omit_hostname=false 时补充
    |
    v
relabel_configs
```

因此，实例标签的优先级高于全局标签：

```toml
[global.labels]
environment = "production"
region = "shanghai"

[[instances]]
labels = { environment = "staging", service = "order" }
```

该实例最终使用：

```text
environment=staging
region=shanghai
service=order
```

实例标签值设为 `"-"` 时，会删除已有同名标签：

```toml
labels = { region = "-" }
```

这个能力适合少数特殊实例，但不建议大量使用，否则同一插件不同实例的标签结构会很难维护。

## 6. `interval` 和 `interval_times` 怎么配合

Categraf 有三个容易混淆的采集周期入口。

**全局周期**

```toml
[global]
interval = 15
```

未被插件覆盖时，input 每 15 秒执行一轮。

**插件级周期**

```toml
interval = "30s"
```

插件配置中的 `interval` 大于 0 时，会覆盖全局周期。数值可以写成秒数，也可以使用 Go duration 格式，例如 `"500ms"`、`"30s"`、`"2m"`。

**实例轮次倍数**

```toml
[[instances]]
interval_times = 4
```

`interval_times` 不是独立 duration，而是“每隔多少个 input 轮次执行一次”。例如：

```toml
[global]
interval = 15
```

插件没有设置自己的 `interval`，实例设置：

```toml
interval_times = 4
```

该实例大约每 60 秒采集一次。

如果插件把周期覆盖为 30 秒：

```toml
interval = "30s"

[[instances]]
interval_times = 4
```

该实例则大约每 120 秒执行一次。

当一次采集耗时超过计划周期时，下一轮不会并行无限堆积，而是尽快开始下一轮。对于目标很多的 `ping`、`http_response`、`net_response` 等插件，还要结合 `[global].concurrency` 控制实例并发。

命令行参数也可以临时覆盖全局周期：

```shell
./categraf --interval 30
```

这个参数单位是秒，且只在值大于 0 时覆盖 `[global].interval`。

## 7. `[[writers]]` 如何发送指标

writer 负责把采集后的样本转换成 Prometheus remote write 数据并发送到后端：

```toml
[writer_opt]
batch = 1000
chan_size = 1000000

[[writers]]
url = "http://127.0.0.1:17000/prometheus/v1/write"
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100
```

数据路径如下：

```text
input Gather
    |
    v
标签、过滤、重命名和 relabel
    |
    v
writer queue
    |
    v
按 batch 取出
    |
    v
Prometheus remote write
```

`writer_opt` 控制所有 writer 共用的发送队列：

- `batch`：每次从队列取出的最大时序数量，默认 1000；
- `chan_size`：队列容量，默认 1000000。

如果队列装不下新样本，日志会提示增加 queue size。不要只把 `chan_size` 无限制调大，还应检查后端响应速度、网络和 writer 错误。

可以配置多个 `[[writers]]`：

```toml
[[writers]]
url = "https://metrics-a.example.com/api/v1/write"

[[writers]]
url = "https://metrics-b.example.com/api/v1/write"
```

它们不是主备关系。Categraf 会把同一批时序并行写给所有 writer。增加 writer 会增加网络流量和后端存储量。

writer 支持 Basic Auth、自定义 Header 和 TLS：

```toml
[[writers]]
url = "https://metrics.example.com/api/v1/write"
basic_auth_user = "categraf"
basic_auth_pass = "<PASSWORD>"
headers = ["X-Tenant", "production"]
tls_ca = "/opt/categraf/conf/ca.pem"
tls_min_version = "1.2"
```

`headers` 按“名称、值”成对填写。密码、Token 和租户密钥不应提交到 Git；应由配置管理或密钥系统在部署时注入，并限制配置文件权限。

## 8. `[heartbeat]` 为什么不是另一个 writer

默认配置中还有一段：

```toml
[heartbeat]
enable = true
url = "http://127.0.0.1:17000/v1/n9e/heartbeat"
interval = 10
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100
```

heartbeat 上报的是 Agent 和主机元数据，例如：

- Agent 版本；
- 操作系统和 CPU 架构；
- 主机名和主机 IP；
- CPU 数量、CPU 利用率和内存利用率；
- 全局标签；
- 扩展系统信息。

它与 writer 的职责不同：

| 链路 | 数据 | 常见目标地址 |
| --- | --- | --- |
| `[[writers]]` | 普通时序指标 | `/prometheus/v1/write` |
| `[heartbeat]` | Agent 与主机元数据 | `/v1/n9e/heartbeat` |

因此会出现几种独立状态：

- writer 正常、heartbeat 失败：指标可能仍能查询，但夜莺中的主机元数据不更新；
- heartbeat 正常、writer 失败：夜莺能看到 Agent 心跳，但查询不到新指标；
- 两者都失败：需要检查地址、网络、认证和 TLS。

如果后端不需要 heartbeat，可以关闭：

```toml
[heartbeat]
enable = false
```

关闭 heartbeat 不会自动关闭 remote write。

## 9. `input.<plugin>` 目录如何启用插件

默认 provider 为：

```toml
[global]
providers = ["local"]
```

local provider 会扫描配置目录下所有以 `input.` 开头的目录，并把后缀当作插件名。

插件大致分为两类。

**主机型插件**

例如 `cpu`、`mem`、`system`。它们通常不需要目标地址，目录存在且初始化成功后即可运行：

```text
conf/input.cpu/cpu.toml
conf/input.mem/mem.toml
conf/input.system/system.toml
```

**实例型插件**

例如 MySQL、Redis、PostgreSQL、HTTP 探测。它们通常要求至少一个有效 `[[instances]]`：

```toml
[[instances]]
address = "127.0.0.1:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { service = "order", environment = "production" }
```

仅仅保留一个全部注释的模板文件，不代表实例已经启用。插件初始化时如果没有有效实例，会跳过运行；开启 debug 后通常可以看到 `no instances for input` 一类提示。

插件目录中可以放 TOML、JSON、YAML 文件。local provider 会读取该插件目录中的这些配置文件，再加载到对应插件结构中。生产环境建议一个插件目录保持清晰的文件职责，不要放多份互相冲突的实例配置。

## 10. input 的通用处理能力

除了插件自己的地址、账号和采集开关，input 与 instance 还共享一些通用能力。

**补充标签**

```toml
labels = { region = "shanghai", service = "order" }
```

**只保留或丢弃指标**

```toml
metrics_pass = ["mysql_up", "mysql_global_status_*"]
metrics_drop = ["mysql_info_schema_table_size*"]
```

`metrics_pass` 和 `metrics_drop` 支持 glob 规则。使用前应先在测试模式观察真实指标名，避免把插件所有指标都过滤掉。

**增加指标名前缀**

```toml
metrics_name_prefix = "production_"
```

修改指标名会影响现有 Dashboard、告警和查询，通常不建议只为区分环境而增加前缀；更适合使用低基数标签。

**标签重写**

```toml
[[relabel_configs]]
source_labels = ["instance"]
regex = "(.*):5432"
target_label = "instance"
replacement = "$1"
action = "replace"
```

relabel 在实例标签、全局标签和 `agent_hostname` 补充之后执行，既可以修改标签，也可能丢弃整条指标。复杂规则上线前一定要在测试环境验证。

## 11. local provider 和 HTTP provider 有什么区别

绝大多数用户使用 local provider：

```toml
[global]
providers = ["local"]
```

它从本机的 `input.*` 目录读取配置，优点是简单、直观、容易纳入配置管理。

Categraf 还支持 HTTP provider：

```toml
[global]
providers = ["http"]

[http_provider]
remote_url = "https://config.example.com/categraf/configs"
timeout = 5
reload_interval = 120
```

HTTP provider 会定期请求远端配置，并根据远端返回的版本和校验值增加、更新或删除 input。它适合大规模集中配置场景，但服务端必须按 Categraf 约定返回插件名、配置内容和格式。

也可以同时配置：

```toml
providers = ["local", "http"]
```

此时本地和远端 input 都会参与运行。设计配置中心时要明确每个插件由哪个 provider 管理，避免对同一个目标重复采集。

## 12. Logs Agent 与 `logs.toml` 怎么配置

默认配置目录中同时存在两类日志配置。

`[log]` 控制 Categraf 自身运行日志：

```toml
[log]
file_name = "stdout"
max_size = 100
max_age = 1
max_backups = 1
local_time = true
compress = false
```

当 `file_name` 是 `stdout` 或 `stderr` 时，后面的文件轮转配置不生效。systemd 部署通常使用 `stdout`，再通过 journal 查看：

```shell
journalctl -u categraf -f
```

`[logs]` 则控制 Logs Agent 的采集、处理和发送管道，不是 Categraf 自身日志级别。默认发行包把它放在独立的 `logs.toml` 中，但启动时仍会与 `config.toml` 一起合并加载。

一份最小的文件日志采集配置可以写成：

```toml
[logs]
enable = true
api_key = "<API_KEY>"
send_to = "127.0.0.1:17878"
send_type = "http"
batch_wait = 5
run_path = "/opt/categraf/run"
open_files_limit = 100
scan_period = 10
pipeline = 4

[[logs.items]]
type = "file"
path = "/var/log/myapp/*.log"
source = "myapp"
service = "order-service"
```

几个关键字段的职责如下：

| 配置项 | 作用 |
| --- | --- |
| `enable` | 是否启用 Logs Agent |
| `send_to` | 日志接收端地址，通常为 `host:port` |
| `send_type` | 发送协议，支持 `http`、`tcp`、`kafka` |
| `api_key` | 发送日志时携带的认证或租户标识，按后端要求填写 |
| `run_path` | 保存日志读取状态的运行目录，应可写且持久化 |
| `scan_period` | 文件采集配置的扫描周期，单位为秒 |
| `pipeline` | 日志处理管道数量 |
| `[[logs.items]]` | 一个日志采集源，可配置多个 |

`[[logs.items]]` 常见 `type` 包括 `file`、`journald`、`tcp` 和 `udp`。其中 `file` 必须配置 `path`，`tcp` 和 `udp` 必须配置 `port`。文件路径可以使用通配符，但应同时关注 `open_files_limit`、目录遍历规模和日志轮转方式。

Logs Agent 的启动条件不只是 `enable = true`：还必须至少存在一个 `[[logs.items]]`，或者启用容器日志采集。否则模块不会创建。这是“配置已开启但日志采集器没有运行”时首先要检查的地方。

Logs Agent 的数据不会进入 `[[writers]]`，而是发送到 `[logs].send_to`。因此日志无数据时应检查 `logs.toml`、采集源状态、`run_path` 权限和日志接收端，不能用指标 remote write 正常来证明日志链路正常。

如果只使用 Categraf 采集指标，保持 `[logs].enable = false` 即可。示例中的 API Key 仅使用占位符；真实密钥不应写入文章、代码仓库或终端截图。

## 13. Prometheus Agent 与 `prometheus.yaml` 怎么协作

Prometheus Agent 适合直接复用 Prometheus 的抓取、服务发现、relabel 和 remote write 配置。它不是 `input.prometheus` 插件：后者是 Metrics Agent 下的一个普通 input，前者则在 Categraf 进程中启动了一套独立的 Prometheus Agent 模式链路。

先在 `config.toml` 中启用模块并指向抓取配置：

```toml
[prometheus]
enable = true
scrape_config_file = "/opt/categraf/conf/prometheus.yaml"
log_level = "info"
wal_storage_path = "/opt/categraf/data-agent"
```

然后在 `prometheus.yaml` 中配置抓取任务与远端写入：

```yaml
global:
  scrape_interval: 15s

scrape_configs:
  - job_name: "node-app"
    static_configs:
      - targets: ["10.0.0.21:9100", "10.0.0.22:9100"]

remote_write:
  - url: "http://n9e.example.com/prometheus/v1/write"
```

这里的文件名不是硬编码要求，仓库中的 Kubernetes 示例使用的是 `in_cluster_scrape.yaml`；只要 `scrape_config_file` 指向真实、可读的文件即可。把它命名为 `prometheus.yaml` 的好处是职责直观。

这条链路有三个必须同时满足的条件：

1. `[prometheus].enable = true`；
2. `scrape_config_file` 存在，且 YAML 能通过 Prometheus 配置解析；
3. YAML 中配置正确的 `scrape_configs` 和 `remote_write`。

尤其要注意，Prometheus Agent **不会复用** `config.toml` 中的 `[[writers]]`。两者虽然都可能发送 Prometheus remote write，但属于两套独立配置：

| 指标来源 | 抓取配置 | 发送目标 |
| --- | --- | --- |
| `input.*` 插件 | 各插件 TOML | `config.toml` 的 `[[writers]]` |
| Prometheus Agent | `prometheus.yaml` 的 `scrape_configs` | 同一 YAML 的 `remote_write` |

Prometheus Agent 使用 WAL 缓冲抓取到的样本。`wal_storage_path` 未设置时，源码默认使用 `./data-agent`；由于相对路径容易随安装方式产生理解偏差，生产环境建议设置绝对路径，并确保目录可写、磁盘空间可监控。这个 WAL 也不等同于 Metrics Agent 的 `writer_opt.chan_size` 内存队列。

排障时，可以先检查 Categraf 日志中是否出现 Prometheus scraping 启动信息，再检查目标发现、抓取错误、WAL 目录和 `remote_write` 错误。若 YAML 里使用 Kubernetes 服务发现，还要同时确认 ServiceAccount、Token、CA 文件和 RBAC 权限。

## 14. Ibex Agent 负责什么

Ibex Agent 用于从 Ibex Server 领取任务、在本机执行并上报状态。它不是指标采集插件，也不通过 `[[writers]]` 上报任务结果。配置位于 `config.toml`：

```toml
[ibex]
enable = true
interval = "1000ms"
servers = ["127.0.0.1:20090"]
meta_dir = "/opt/categraf/meta"
```

| 配置项 | 作用 |
| --- | --- |
| `enable` | 是否启用 Ibex Agent |
| `interval` | 向服务端轮询和上报任务状态的周期 |
| `servers` | Ibex Server 的 RPC 地址列表 |
| `meta_dir` | 保存任务脚本、状态和临时数据的目录 |

源码中的默认配置会连接 Ibex RPC 端口，并由 `Server.Report` 交互领取分配任务。因为该模块具备本地任务执行能力，生产启用前应明确三个安全边界：只连接可信的 Ibex Server；限制 Categraf 运行用户的系统权限；让 `meta_dir` 使用专用、可写但不对普通用户开放的目录。

如果没有远程任务执行需求，应保持 `[ibex].enable = false`。关闭 Ibex Agent 不影响指标、日志和 heartbeat。

## 15. 命令行参数如何覆盖配置

几个最常用的启动参数如下：

```shell
# 推荐布局：默认读取 /opt/categraf/conf
cd /opt/categraf

# 只启用 cpu、mem、system
./categraf --inputs cpu:mem:system

# 临时把全局周期改为 30 秒
./categraf --interval 30

# 打印指标但不写后端
./categraf --test --inputs cpu:mem:system

# 打开 debug，打印指标且仍写后端
./categraf --debug --inputs cpu:mem:system

# 配置不在默认 conf 目录时，显式指定路径
./categraf --configs /path/to/categraf/conf --test --inputs cpu
```

`--inputs` 使用冒号分隔插件名。它是允许列表：设置后，未列出的指标插件不会启动。

Categraf 还提供 `--install`、`--start`、`--stop`、`--status` 和 `--remove` 管理系统服务。这些参数用于服务生命周期管理，不会进入正常的指标采集流程，具体用法见第 17 节。

这个参数主要过滤 Metrics Agent 下的 `input.*`，不会自动关闭 Logs Agent、Prometheus Agent 或 Ibex Agent。做完全隔离的插件测试时，应复制一份测试配置，并在其中把 `[logs]`、`[prometheus]`、`[ibex]` 和不需要的 heartbeat 显式关闭，避免测试命令仍然连接生产端点。

最容易混淆的是 `--test` 和 `--debug`：

| 模式 | 打印指标 | 写入 writer | 适用场景 |
| --- | --- | --- | --- |
| `--test` | 是 | 不通过 writer 写普通指标 | 验证采集和标签 |
| `--debug` | 是 | 是 | 联调完整写入链路 |

测试模式不会把普通时序指标送入 writer，但如果 `[heartbeat].enable = true`，heartbeat 协程仍会按配置尝试上报主机元数据。完全离线测试时，可以在测试配置副本中临时关闭 heartbeat。在生产主机执行 `--debug` 可能输出大量指标，不建议长时间开启。

## 16. 修改配置后是重载还是重启

Categraf 收到 HUP 信号时，会停止并重新启动 Agent 模块：

```shell
kill -HUP "$(pidof categraf)"
```

systemd 安装模板也会把 reload 映射为 HUP。local provider 在 Agent 重新启动时会再次扫描 `input.*` 目录并读取插件配置，因此修改插件配置后可以使用 HUP 重载。

但是，HUP 不会重新执行整个 `InitConfig`，writer 也不会重新初始化。以下修改建议直接重启 Categraf：

- `[global]`；
- `[writer_opt]`；
- `[[writers]]`；
- `[heartbeat]`；
- `[log]`、`[logs]`；
- `[prometheus]`、`[ibex]`；
- provider 类型和 `[http_provider]`。

例如：

```shell
cd /opt/categraf
sudo ./categraf --stop
sudo ./categraf --start
sudo ./categraf --status
```

Categraf 没有单独的 `--restart` 参数；需要重启时依次执行 `--stop` 和 `--start`。使用 `systemctl restart categraf` 仍然可以，但推荐优先使用 Categraf 自带的服务管理参数，部署脚本也更容易保持一致。

修改单个 `input.<plugin>` 后，可以选择 reload；如果无法确认版本行为或本次还改了主配置，直接做受控重启更稳妥。

如果只修改 `scrape_config_file` 指向的 Prometheus YAML，Prometheus Agent 自身注册了 HUP 配置重载处理，可以发送 HUP 后通过日志确认新 YAML 是否加载成功；如果同时修改了 `[prometheus]` 中的开关、路径或 WAL 参数，仍应重启整个 Categraf 进程。

HTTP provider 不依赖手工 HUP，它会按照 `reload_interval` 定期检查远端版本并动态更新 input。

## 17. 一套推荐的生产配置组织方式

推荐把 Categraf 二进制、配置和可选 Agent 的运行数据统一放在 `/opt/categraf` 下，再按子目录区分职责：

```text
/opt/categraf/
├── categraf
├── conf/
│   ├── config.toml
│   ├── logs.toml
│   ├── prometheus.yaml       # 启用 Prometheus Agent 时使用
│   ├── input.cpu/
│   ├── input.mem/
│   ├── input.system/
│   ├── input.mysql/
│   ├── input.redis/
│   └── ...
├── run/                      # Logs Agent 读取状态
├── data-agent/               # Prometheus Agent WAL
└── meta/                     # Ibex Agent 任务目录
```

文件就位后，推荐使用 Categraf 自带的安装命令创建并启用 systemd 服务：

```shell
cd /opt/categraf
sudo ./categraf --install
sudo ./categraf --start
sudo ./categraf --status
```

必须先把二进制和 `conf` 放到最终目录，再执行 `--install`。安装程序会根据当前二进制的绝对路径生成 unit；在上述布局下，生成的关键配置相当于：

```ini
[Service]
WorkingDirectory=/opt/categraf
ExecStart=/opt/categraf/categraf -configs /opt/categraf/conf
Restart=on-failure
```

`--install` 会写入 unit、启用服务并执行 `daemon-reload`，但不会立即启动，所以还需要执行 `./categraf --start`。日常服务管理都可以使用 Categraf 自带参数：

| 命令 | 作用 |
| --- | --- |
| `sudo ./categraf --install` | 安装并启用服务，同时刷新 systemd 配置 |
| `sudo ./categraf --start` | 启动服务 |
| `sudo ./categraf --stop` | 停止服务 |
| `sudo ./categraf --status` | 查看服务状态 |
| `sudo ./categraf --remove` | 停止并卸载服务，同时刷新 systemd 配置 |

因此，常规安装和升级不需要手工编写 unit，也不需要另外执行 `systemctl daemon-reload`。安装后仍可使用 `systemctl cat categraf` 查看生成结果，使用 `journalctl -u categraf` 查看运行日志。

只有在需要自定义 `User`、资源限制、安全加固或其他 systemd 选项时，才建议手工维护 unit。此时可以利用默认 `conf` 目录省略 `--configs`：

```ini
[Service]
WorkingDirectory=/opt/categraf
ExecStart=/opt/categraf/categraf
Restart=on-failure
```

这里能够默认读取 `/opt/categraf/conf`，根本原因是二进制本身位于 `/opt/categraf`，并且 Categraf 启动后会主动切换到二进制目录。`WorkingDirectory` 可以让 unit 的目录约定更清晰，但不能把其他目录中的二进制自动指向 `/opt/categraf/conf`；如果二进制和配置分离，仍应显式使用 `--configs /opt/categraf/conf`。手工新增或修改 unit 后需要自行执行 `systemctl daemon-reload`。

配置管理建议：

- `config.toml`、`logs.toml`、`prometheus.yaml` 和 input 模板纳入版本控制；
- 密码、Token、AK/SK 不进入 Git；
- 配置文件只允许 Categraf 运行用户读取；
- `logs.toml` 中的 API Key 和 `prometheus.yaml` 中的 remote write 认证信息按密钥管理规范注入；
- `run_path`、`wal_storage_path` 和 `meta_dir` 使用不同子目录，并分别监控权限和磁盘空间；
- 如果服务使用非 root 用户运行，提前确保 `/opt/categraf/run`、`/opt/categraf/data-agent` 和 `/opt/categraf/meta` 对该用户可写；
- 修改前保留备份和回滚版本；
- 先在测试模式验证单个 input，再重启正式服务；
- 重启后同时检查服务日志和后端指标。

## 18. 配置验证的推荐顺序

每次新增或修改插件时，可以按下面的顺序验证。

**确认路径和文件**

```shell
test -x /opt/categraf/categraf
test -f /opt/categraf/conf/config.toml
find /opt/categraf/conf -maxdepth 2 -type f | sort
```

如果启用了可选 Agent，还要确认对应运行目录存在且运行用户可写：

```shell
ls -ld /opt/categraf/run /opt/categraf/data-agent /opt/categraf/meta
```

**确认 systemd 安装结果**

```shell
sudo /opt/categraf/categraf --status
systemctl cat categraf
systemctl show categraf -p ExecStart -p WorkingDirectory
systemctl is-enabled categraf
```

使用 `--install` 生成 unit 时，`ExecStart` 应指向 `/opt/categraf/categraf`，配置参数应指向 `/opt/categraf/conf`，`WorkingDirectory` 应为 `/opt/categraf`。如果使用上一节的手写 unit，省略配置参数也是正常的，但二进制必须位于 `/opt/categraf`，才能默认读取旁边的 `conf`。

**确认二进制和命令行参数**

```shell
/opt/categraf/categraf --version
/opt/categraf/categraf --help
```

**只测试目标插件**

```shell
cd /opt/categraf
./categraf --test --inputs postgresql
```

测试模式有指标，说明“配置加载 -> 插件初始化 -> Gather -> 标签处理”基本正常，但不能证明 writer 已经成功。

建议在正式服务首次启动前执行这一步。如果服务已经运行，应先停止正式进程，或者使用一份关闭 Logs Agent、Prometheus Agent、Ibex Agent 和 heartbeat 的隔离测试配置，避免两个进程同时访问 WAL、日志读取状态或重复上报数据。

**启动正式服务并看日志**

```shell
cd /opt/categraf

# 首次启动
sudo ./categraf --start
sudo ./categraf --status

# 修改配置后重启
sudo ./categraf --stop
sudo ./categraf --start
sudo ./categraf --status

journalctl -u categraf -n 100 --no-pager
```

**在后端查询原始指标**

```promql
postgresql_up
```

最后再导入 Dashboard。不要在原始指标还查不到时就反复修改大盘。

如果启用了三个可选 Agent，还应分别增加验证：

```text
Logs Agent：采集源可读 -> run_path 可写 -> send_to 可达 -> 后端出现日志
Prometheus Agent：YAML 可解析 -> target 可抓取 -> WAL 可写 -> remote_write 成功
Ibex Agent：RPC 地址可达 -> meta_dir 可写 -> 服务端能看到状态 -> 任务权限符合预期
```

不要用其中一条链路的成功推断另一条链路正常。例如，`[[writers]]` 写入成功不能证明 `prometheus.yaml` 中的 `remote_write` 正常，日志接收正常也不能证明 input 指标采集正常。

## 19. 常见问题

**配置目录里有插件，为什么没有启动？**

目录只表示 local provider 会发现这个插件。还要确认插件名受支持、配置能解析、实例不为空、初始化成功，并且没有被 `--inputs` 排除。

**为什么 `--test` 有指标，夜莺里却没有？**

因为测试模式会在打印普通指标后直接返回，不进入 writer。这只能证明采集正常，下一步应以正式模式检查 writer 和后端。注意，已启用的 heartbeat 仍可能发起独立请求。

**为什么 heartbeat 正常但没有指标？**

heartbeat 和 remote write 是独立 HTTP 请求。检查 `[[writers]].url`，不要只看主机心跳状态。

**为什么改了 writer，发送 HUP 后还是写旧地址？**

HUP 不会重新初始化 writer。修改 `config.toml` 中的 writer 后需要重启进程。

**为什么给实例设置了标签，却没有使用全局同名值？**

实例标签会覆盖同名原始标签；全局标签只补充不存在的字段，这是预期行为。

**为什么设置 `interval_times = 4` 后不是 4 秒采一次？**

它表示每 4 个 input 轮次执行一次。实际周期还要乘以全局或插件级 `interval`。

**`[log]` 和 `[logs]` 有什么区别？**

`[log]` 是 Categraf 自身运行日志；`[logs]` 是日志采集和发送功能。两者不是同一个模块。

**为什么 `[logs].enable = true`，Logs Agent 还是没有运行？**

还要确认至少配置了一个有效的 `[[logs.items]]`，或者启用了容器日志采集；只有开关、没有采集源时不会创建 Logs Agent。

**`input.prometheus` 和 Prometheus Agent 是一回事吗？**

不是。`input.prometheus` 属于 Metrics Agent，按 Categraf input 配置运行并走 `[[writers]]`；Prometheus Agent 由 `[prometheus]` 启用，读取 `scrape_config_file` 指向的 Prometheus YAML，并走 YAML 自己的 `remote_write`。

**为什么普通 input 指标能写入，Prometheus Agent 抓取的指标却没有？**

普通 input 和 Prometheus Agent 使用不同发送配置。应检查 `prometheus.yaml` 中是否配置了正确的 `remote_write`，以及 target、WAL、认证和远端响应，而不是只检查 `config.toml` 的 `[[writers]]`。

**为什么开启 Ibex Agent 前要额外关注权限？**

Ibex Agent 会领取并在本机执行任务。Categraf 进程拥有什么系统权限，远端任务就可能在相应权限边界内运行，因此需要可信服务端、最小权限运行用户和受控的 `meta_dir`。

## 20. 小结

Categraf 的配置可以归纳成七个核心问题：

```text
global：多久采、主机叫什么、补哪些全局标签、从哪里获取 input 配置
writer：指标写到哪里、如何批量和认证
heartbeat：Agent 与主机元数据报到哪里
inputs：具体采什么、目标是谁、实例标签和过滤规则是什么
logs-agent：采哪些日志、读取状态放哪里、日志发到哪里
prometheus-agent：按哪些 scrape_configs 抓取、WAL 放哪里、remote_write 到哪里
ibex-agent：向哪个服务端领取任务、任务文件放哪里、以什么权限执行
```

配置排障时也应该保持同样的层次：先确认配置目录和 `config.toml`，再确认目标 Agent 是否真正启动，接着沿该 Agent 自己的“采集源 -> 缓冲或状态目录 -> 发送端”逐段验证。把 Metrics writer、Prometheus remote write、Logs send_to、Ibex RPC、heartbeat 和 Dashboard 分开判断，绝大多数“Categraf 没数据”或“功能没生效”问题都会清晰很多。

下一篇：[Categraf 启动失败排查：TOML、配置路径、权限和 systemd 常见错误](/blog/categraf-startup-failure-troubleshooting/)。

---

**内容更新时间**：2026-07-17

**证据边界**：配置字段、模块启动条件和数据链路来自本文所列当前仓库源码与默认样例；示例 IP、域名、路径和认证值仅用于说明，实际部署应以所使用版本和环境为准。
