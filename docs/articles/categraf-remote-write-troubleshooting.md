---
title: "Categraf 已采到指标但后端查不到：remote write 链路排查"
description: "本文介绍 Categraf 使用 test 模式确认采集、再用 debug 或正式模式验证写入，但夜莺、VictoriaMetrics 或 Prometheus 兼容后端仍查不到数据时的完整排查方法，覆盖运行模式、writer、标签转换、内存队列、HTTP、认证、TLS、时间偏差、租户和查询端点。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "remote write", "夜莺", "VictoriaMetrics", "Prometheus", "writer", "监控排障"]
author: "快猫星云"
date: "2026-07-20T00:00:00+08:00"
tags: ["Categraf", "Troubleshooting", "Remote Write"]
---

`./categraf --test --inputs postgresql` 已经可以打印指标，`postgresql_up` 也是 `1`，但夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端怎么都查不到。首先要明确：这是 test 模式的预期行为，因为 `--test` 只打印采集结果，不会把样本写入后端。

只有退出 test，改用正式运行模式，或者使用 `--debug --inputs postgresql` 前台运行，样本才会继续转换、进入 writer 队列并发送。因此，真正的 remote write 故障应定义为：**test 已确认采集正常，同时 Categraf 已在正式模式或仅带 `--debug` 的模式下运行，但后端仍查不到。**

普通 input 生成样本后，会经过指标过滤和标签处理，转换成 Prometheus remote write 时序，进入 Metrics Agent 的内存队列，再由一个或多个 `[[writers]]` 通过 HTTP 发送。任意一段配置错误，都可能出现“test 有数据，后端没数据”。

本文基于 Categraf 当前仓库中的 `writer`、`types.Sample`、`input.self_metrics` 和默认 `config.toml`，给出一条可直接执行的排查路径。若 test 本身还没有目标指标，请先参考：[Categraf 插件没有指标排查](/blog/categraf-plugin-no-metrics-troubleshooting/)。

## 核心要点

- `--test` 会打印 Metrics Agent 样本并直接返回，不会把这些样本放进 writer 队列；它只能证明采集侧，不能验证 remote write。
- `--debug` 会打印指标，同时继续转换、入队和写入后端；但 `--debug --test` 仍以 test 逻辑为准，只打印、不写入。
- 前台脚本或 `--debug` 正常、systemd 服务异常时，要比较两者的实际环境。登录 shell 中的 `PATH`、代理、Token 和自定义变量不会自动进入 systemd 服务。
- Metrics Agent 的 `[[writers]]` 使用进程内存队列，没有磁盘 WAL。当前实现取出一个批次后只发送一次，HTTP 失败的批次不会重新放回队列。
- `categraf_metrics_enqueue_failed_*` 只统计队列已满导致的入队失败，不统计 HTTP 4xx、5xx、超时或 TLS 失败；远端发送错误要看 Categraf 本地日志。
- heartbeat、Metrics writer、Prometheus Agent 的 `remote_write`、Logs Agent 的 `send_to` 是不同链路。心跳正常不能证明指标写入正常。
- Categraf 使用 `agent_hostname` 标识自身。经过夜莺接入时，只有指标不存在非空 `ident`，夜莺才会把 `agent_hostname` 重命名为 `ident`；已有非空 `ident` 时，显式 `ident` 优先且 `agent_hostname` 保留。直写其他 TSDB 时通常也保留 `agent_hostname`。
- HTTP 返回 2xx 但查不到时，优先检查写入租户与查询租户、查询端点、时间偏差、标签和指标名，而不是只检查网络。
- 一个 batch 会并发发送给所有配置的 writer，并等待全部 writer 返回。任意 writer 长时间阻塞，都可能拖慢后续批次。

## 1. 先确认问题真的在 remote write

把常见现象分成四层：

| 现象 | 故障层 | 下一步 |
| --- | --- | --- |
| Categraf 无法启动 | 主进程 | 查主配置、systemd、权限 |
| `--test` 没有目标指标 | input 采集 | 查插件目录、实例、连接和权限 |
| `--test` 有指标，正式运行后端查不到 | writer 与后端 | 本文范围 |
| 后端裸指标有数据，Dashboard 空白 | Dashboard | 查数据源、变量、标签和 PromQL |

先把几个容易混淆的运行方式分开：

| 命令模式 | 标准输出打印指标 | 进入 writer 队列 | 写入后端 TSDB |
| --- | --- | --- | --- |
| 普通正式模式 | 否 | 是 | 是 |
| `--debug` | 是 | 是 | 是 |
| `--test` | 是 | 否 | 否 |
| `--debug --test` | 是 | 否 | 否 |

源码执行顺序是先判断 test：命中后打印并直接返回；只有不是 test 时，debug 才会在打印之后继续写入。因此，**需要前台验证完整链路时应使用 `--debug`，不要同时带 `--test`。**

第一步，用 test 只验证采集并记录指标名称和关键标签：

```shell
cd /opt/categraf
./categraf --test --inputs postgresql
```

例如记录：

```text
metric: postgresql_up
server: db.example.com:5432
agent_hostname: monitor-01
value: 1
```

这里记录的是 **Categraf 采集端输出的标签**，不一定等于接收端最终保存的标签：

| 写入路径 | 后端通常看到的主机标识标签 | 查询示例 |
| --- | --- | --- |
| Categraf -> 夜莺 -> TSDB，原指标没有非空 `ident` | `agent_hostname` 被重命名为 `ident` | `postgresql_up{ident="monitor-01"}` |
| Categraf -> 夜莺 -> TSDB，原指标已有非空 `ident` | 显式 `ident` 优先，同时保留 `agent_hostname` | `postgresql_up{ident="<EXPLICIT_IDENT>",agent_hostname="monitor-01"}` |
| Categraf -> VictoriaMetrics、Prometheus 兼容后端等 TSDB | 没有夜莺转换，通常保留 `agent_hostname` | `postgresql_up{agent_hostname="monitor-01"}` |

因此，`--test` 中只出现 `agent_hostname="monitor-01"` 且没有非空 `ident` 时，经夜莺写入后应查 `ident="monitor-01"`。如果 test 已经出现非空 `ident`，经夜莺写入后应使用该显式值查询，同时还可以看到原 `agent_hostname`。直写其他 TSDB 时通常仍查 `agent_hostname="monitor-01"`。如果链路上还有 relabel 或网关转换，最终仍以后端实际返回的标签为准。

第二步，退出 test 后，再选择一种真正会写入的方式。

方式一：恢复正式服务，并在日志与后端查询中验证：

```shell
sudo ./categraf --start
sudo ./categraf --status
systemctl show categraf -p ExecStart -p User -p EnvironmentFiles
```

方式二：在测试节点或维护窗口停止正式服务，使用 debug 前台验证“打印 + 入队 + remote write”：

```shell
cd /opt/categraf
sudo ./categraf --stop
./categraf --debug --inputs postgresql
```

此时终端打印出的指标也会继续写入 `[[writers]]`。在另一个终端查询后端并观察 remote write 日志；验证完成后按 `Ctrl+C` 退出，再恢复服务：

```shell
cd /opt/categraf
sudo ./categraf --start
sudo ./categraf --status
```

不要运行 `--debug --test` 来验证写入，它仍然不会入队。也不要在同一主机长期同时运行正式服务和前台 debug 进程，以免重复采集和重复写入。

## 2. Categraf Metrics writer 的真实链路

普通 input 的数据路径是：

```text
input Gather
  -> metrics_pass/drop、前缀、labels、relabel
  -> Sample 转 Prometheus TimeSeries
  -> writer 内存队列
  -> 按 batch 取出
  -> Snappy + protobuf remote write
  -> 所有 [[writers]]
  -> 后端接收、存储、查询
```

Categraf 当前发送时会设置这些 remote write 请求头：

```text
Content-Encoding: snappy
Content-Type: application/x-protobuf
User-Agent: categraf
X-Prometheus-Remote-Write-Version: 0.1.0
```

因此，普通浏览器访问 writer URL 返回 404、405 或空白，不能直接证明 remote write 可用。它是一个接收压缩 protobuf POST 的接口，不是给人浏览的页面。

还有三个容易混淆的边界：

| 数据来源 | 发送配置 | 缓冲方式 |
| --- | --- | --- |
| 普通 `input.*`，包括 `input.prometheus` | `config.toml` 的 `[[writers]]` | Metrics writer 内存队列 |
| Prometheus Agent | `prometheus.yaml` 的 `remote_write` | Prometheus Agent WAL |
| Logs Agent | `logs.toml` 的 `[logs].send_to` | Logs Agent 自己的状态和发送链路 |

本文主要排查第一行。不要用 `prometheus.yaml` 的 remote write 成功来证明 `[[writers]]` 正常，也不要反过来推断。

## 3. 最短排查路径

建议按下面顺序执行：

```text
--test 是否有目标指标（只验证采集）
    |
    v
正式模式或 --debug（不带 --test）是否正在写入
    |
    v
正式服务是否使用预期 config.toml
    |
    v
[[writers]].url、认证、TLS、headers
    |
    v
Categraf 日志中的 DNS/TCP/TLS/HTTP 错误
    |
    v
self_metrics 队列是否堆积或入队失败
    |
    v
后端接收日志、容量、限流和租户
    |
    v
同一查询端点按准确时间与标签查裸指标
```

对应命令：

```shell
cd /opt/categraf

sudo ./categraf --status
systemctl show categraf -p ExecStart -p User -p Environment
journalctl -u categraf -b --since "30 minutes ago" --no-pager

sed -n '1,180p' /opt/categraf/conf/config.toml
timedatectl status
```

排障时要同时保留三类证据：Categraf 本地发送日志、后端接收日志、后端查询结果。只看其中一类，很容易把写入、存储和查询混为一谈。

## 4. 首先确认正式服务读取的是哪份 writer 配置

推荐部署布局是：

```text
/opt/categraf/
├── categraf
└── conf/
    ├── config.toml
    └── input.*/
```

默认情况下，`/opt/categraf/categraf` 读取旁边的 `/opt/categraf/conf`。如果 unit、启动脚本或环境变量指定了其他目录，你修改默认文件不会影响正式服务。

检查实际启动参数：

```shell
systemctl show categraf \
  -p ExecStart \
  -p WorkingDirectory \
  -p User \
  -p Environment \
  -p EnvironmentFiles
```

如果 `ExecStart` 带有 `--configs /path/to/categraf/conf`，就检查那一份 `config.toml`：

```shell
sed -n '1,180p' /path/to/categraf/conf/config.toml
```

修改后要重启或明确 reload，并从新日志确认生效：

```shell
cd /opt/categraf
sudo ./categraf --stop
sudo ./categraf --start
sudo ./categraf --status
```

不要只凭文件修改时间判断。最可靠的证据是正式进程参数、重启时间和重启后的发送日志。

## 5. `[[writers]].url` 必须是接收接口，不是首页或查询接口

默认配置结构：

```toml
[writer_opt]
batch = 1000
chan_size = 1000000

[[writers]]
url = "http://monitor.example.com/prometheus/v1/write"
basic_auth_user = ""
basic_auth_pass = ""
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100
```

不同后端、部署模式和反向代理的路径可能不同。常见错误包括：

- 把夜莺登录页面地址当成 remote write 地址；
- 把 Prometheus 兼容查询地址 `/api/v1/query` 当成写入地址；
- 漏掉反向代理前缀或多写一层路径；
- 后端迁移后仍写旧域名、旧端口；
- 单机和集群版后端使用了不同入口；
- writer 写入 A 集群，Dashboard 却查询 B 集群。

应从当前后端部署说明或服务配置中确认接收 URL，不要根据页面 URL 猜路径。写入地址和查询地址通常属于同一系统，但不一定是同一个端口、域名或网关。

如果配置多个 writer：

```toml
[[writers]]
url = "https://metrics-a.example.com/prometheus/v1/write"

[[writers]]
url = "https://metrics-b.example.com/api/v1/write"
```

Categraf 会把每个 batch 发给所有不同 URL。相同 URL 重复配置不会产生两条独立 writer；生产配置应避免重复。

## 6. 从 Categraf 所在环境检查 DNS、路由和代理

先检查 writer 主机是否可解析、端口是否可达：

```shell
getent hosts metrics.example.com
nc -vz metrics.example.com 443
curl -sv --connect-timeout 3 https://metrics.example.com/health
```

健康检查路径只是示例，应替换为后端真实提供的健康接口。不要用一个 200 的首页替代 remote write 接收验证。

Metrics writer 使用系统代理环境：

```text
HTTP_PROXY
HTTPS_PROXY
NO_PROXY
```

手工 shell 和 systemd 的环境可能不同。检查正式服务：

```shell
systemctl show categraf -p Environment -p EnvironmentFiles
```

常见情况：

- shell 中配置了代理，systemd 没有，因此手工 curl 成功、服务失败；
- systemd 配置了全局代理，但内网后端没有加入 `NO_PROXY`；
- 容器内的 DNS、路由和 CA 与宿主机不同；
- 反向代理要求特殊 `Host`，直接访问后端 IP 被路由到错误虚拟主机。

Categraf 的自定义 `headers` 对 `Host` 做了专门处理，可在确有反向代理需求时设置。但优先使用正确域名，不要把 Host 覆盖当成常规配置。

### 前台脚本或 debug 正常，systemd 为什么没有数据

这是一个很典型的环境差异场景：

```text
登录 shell 运行脚本或 ./categraf --debug
  -> 继承当前用户的 profile、PATH、代理和临时 export
  -> 能采集，也能写入

systemd 启动 Categraf
  -> 不会自动读取用户的 .bashrc、.profile
  -> 缺少变量后，脚本无输出、认证失败或 writer 走错网络
```

常见依赖包括：

| 环境变量 | 缺失后的常见表现 |
| --- | --- |
| `PATH` | `exec` 插件在前台能找到命令，systemd 中提示 executable not found 或没有输出 |
| `HTTP_PROXY` / `HTTPS_PROXY` | 前台能经代理访问目标或 TSDB，服务连接超时 |
| `NO_PROXY` | systemd 把内网 remote write 请求错误地发给代理 |
| 脚本 Token、API Key、Region | 脚本返回认证错误、空结果或访问错误区域 |
| `HOME`、`KUBECONFIG`、云凭据路径 | CLI 或 SDK 找不到用户目录下的配置与凭据 |
| `LD_LIBRARY_PATH` 等运行库变量 | 外部命令或动态库只在登录 shell 中可用 |

Categraf `exec` 插件通过子进程运行命令，子进程会继承 Categraf 服务进程的环境。Metrics writer 的 HTTP Transport 也会读取代理环境。因此，同一个配置文件在前台和 systemd 中可能表现不同。

先确认 unit 的运行用户和环境文件：

```shell
systemctl show categraf \
  -p User \
  -p ExecStart \
  -p EnvironmentFiles

systemctl cat categraf
```

Categraf 当前 Linux 内置安装模板包含：

```ini
EnvironmentFile=-/etc/sysconfig/categraf
```

前面的 `-` 表示文件不存在时不阻止服务启动。所以 unit 中出现这行，并不代表环境文件已经创建或变量已经配置。

下面的做法针对通过 `sudo ./categraf --install` 安装的系统级服务。先创建目录和受控的环境文件：

```shell
sudo install -d -m 755 /etc/sysconfig
sudo touch /etc/sysconfig/categraf
sudo chown root:root /etc/sysconfig/categraf
sudo chmod 600 /etc/sysconfig/categraf
sudoedit /etc/sysconfig/categraf
```

示例内容：

```text
PATH=/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin
HTTPS_PROXY=http://proxy.example.com:3128
NO_PROXY=127.0.0.1,localhost,metrics.internal.example.com
SCRIPT_API_TOKEN=<TOKEN>
REGION=cn-north-1
```

`EnvironmentFile` 使用 `KEY=value`，不要写 `export KEY=value`，也不要依赖 shell 命令替换。真实 Token 不应写入文章、工单或截图；环境文件应限制读取权限，并纳入组织的密钥轮换流程。

只修改 `/etc/sysconfig/categraf` 的内容不需要 `daemon-reload`，重启服务即可让新进程读取：

```shell
cd /opt/categraf
sudo ./categraf --stop
sudo ./categraf --start
sudo ./categraf --status
```

如果使用自定义 unit，且没有 `EnvironmentFile`，才需要在 `[Service]` 中补充该项。手工修改 unit 后执行 `systemctl daemon-reload`，再重启服务。使用 `--user` 安装用户级服务时，则要按用户 unit 的权限和环境管理方式配置，不能直接套用仅 root 可读的 `0600 /etc/sysconfig/categraf`。

为了避免打印密钥，可以只比较环境变量名称：

```shell
# 当前登录 shell 的变量名
env | cut -d= -f1 | sort

# 正在运行的 Categraf 服务进程变量名
PID=$(systemctl show categraf -p MainPID --value)
sudo cat "/proc/$PID/environ" \
  | tr '\0' '\n' \
  | cut -d= -f1 \
  | sort
```

重点确认 `PATH`、代理、`NO_PROXY` 和脚本所需变量是否存在。必须检查值时，只查询指定的非敏感变量；不要把完整 `/proc/<PID>/environ` 输出贴到聊天、工单或文章中。

最后要用服务模式复验，而不是再次用登录 shell：

```shell
journalctl -u categraf -b --since "10 minutes ago" --no-pager
```

如果 `--debug` 前台能写入、systemd 仍然没有数据，优先比较：运行用户、环境变量名、PATH 中的命令、证书/凭据文件权限，以及请求是否经过同一个代理。

## 7. 认证、租户 Header 和自定义 Header 怎么查

Categraf 支持 Basic Auth：

```toml
[[writers]]
url = "https://metrics.example.com/prometheus/v1/write"
basic_auth_user = "<USERNAME>"
basic_auth_pass = "<PASSWORD>"
```

也支持按键值对排列的自定义 Header：

```toml
headers = [
  "Authorization", "Bearer <TOKEN>",
  "X-Scope-OrgID", "<TENANT_ID>",
]
```

注意：

- `headers` 必须是成对的 key、value；
- Token、租户 ID 和 Header 名称都要与写入网关约定一致；
- Basic Auth 和自定义 Authorization 同时存在时，要避免互相覆盖或造成网关歧义；
- systemd 中的环境变量和密钥文件必须对正式服务用户可用；
- 分享配置或日志前必须脱敏。

多租户系统尤其要检查“写入租户”和“查询租户”是否相同。即使写入返回 2xx，只要 Grafana、夜莺数据源或查询 API 使用了另一个租户 Header，查询仍然会是空结果。

## 8. TLS 错误如何定位

HTTPS writer 会自动启用 TLS，也可以配置客户端证书：

```toml
[[writers]]
url = "https://metrics.example.com/prometheus/v1/write"
tls_min_version = "1.2"
tls_ca = "/path/to/ca.pem"
tls_cert = "/path/to/client.crt"
tls_key = "/path/to/client.key"
insecure_skip_verify = false
```

先用正式服务用户检查文件：

```shell
SERVICE_USER=$(systemctl show categraf -p User --value)
test -n "$SERVICE_USER" || SERVICE_USER=root

sudo -u "$SERVICE_USER" test -r /path/to/ca.pem
sudo -u "$SERVICE_USER" test -r /path/to/client.crt
sudo -u "$SERVICE_USER" test -r /path/to/client.key
```

再检查证书链和主机名：

```shell
openssl s_client \
  -connect metrics.example.com:443 \
  -servername metrics.example.com \
  -showcerts
```

常见日志包括：

| 错误 | 常见原因 |
| --- | --- |
| `x509: certificate signed by unknown authority` | 缺少正确 CA 或中间证书 |
| `certificate is valid for ..., not ...` | URL 主机名与证书 SAN 不符 |
| `certificate has expired` | 服务端或客户端证书过期 |
| `tls: bad certificate` | mTLS 客户端证书不受信任或不匹配 |
| `permission denied` | Categraf 服务用户无法读取证书文件 |

`insecure_skip_verify = true` 只能作为受控测试手段，不能替代修复证书链和主机名。

## 9. 如何读懂 remote write HTTP 状态码

Categraf 对 400 及以上状态记录 warning，并打印响应 body。常见状态可按下面理解：

| 状态 | 常见原因 | 优先检查 |
| --- | --- | --- |
| 400 | 标签、时间戳、协议、租户参数或后端校验失败 | response body、后端接收日志 |
| 401 | 凭据缺失或错误 | Basic Auth、Authorization |
| 403 | 凭据有效但无写权限，或租户策略拒绝 | 权限、租户、网关策略 |
| 404 | URL 路径、端口或反向代理路由错误 | writer URL、网关 route |
| 405 | 请求落到不接收 POST 的页面或接口 | 写入路径是否正确 |
| 413 | batch 超过代理或后端请求体限制 | `batch`、代理 body limit |
| 429 | 限流或租户配额耗尽 | 写入速率、配额、扩容 |
| 5xx | 后端、网关或存储异常 | 服务健康、容量、后端日志 |

典型日志形态：

```text
W! post to https://metrics.example.com/... got error: ...
W! example timeseries: ...
```

response body 往往比状态码更有价值。比如同样是 400，可能分别表示样本时间过旧、标签不合法、租户缺失或单请求序列数超限。

不要只截取最后一行。保留错误之前和之后的时间、URL、状态码、response body，以及同一时间的后端日志。

## 10. 当前 writer 的队列和失败语义

默认配置：

```toml
[writer_opt]
batch = 1000
chan_size = 1000000
```

含义：

- `batch`：writer 每次最多从内存队列取出的时序数量；
- `chan_size`：内存队列最多容纳的时序数量。

当前处理逻辑是：

```text
新样本尝试入队
  -> 队列有空间：整批入队
  -> 队列空间不足：整批入队失败并记录错误

writer 从队列取出 batch
  -> 并发发送给所有 writers
  -> 等待所有 writers 返回
  -> 无论远端成功或失败，该批次都不会重新入队
```

因此必须理解两个结论：

1. 队列是吞吐缓冲，不是可靠持久化队列；Categraf 重启会丢失尚未发送的队列内容。
2. 当前 Metrics writer 没有对 HTTP 失败批次做本地 WAL 或自动重试；后端故障期间的失败样本需要靠日志及时发现，不能假设恢复后会自动补传。

扩大 `chan_size` 只能缓解短时堆积，不能修复错误 URL、401、持续限流或后端长时间故障，还会增加内存占用。

## 11. 用 `input.self_metrics` 看队列，但不要误读

默认发行配置包含：

```text
/opt/categraf/conf/input.self_metrics/metrics.toml
```

启用后会生成：

| 指标 | 含义 |
| --- | --- |
| `categraf_metrics_enqueue_sum` | 成功转换后尝试进入 writer 队列的累计时序数 |
| `categraf_metrics_enqueue_failed_sum` | 因队列容量不足而入队失败的累计时序数 |
| `categraf_metrics_enqueue_failed_count` | 发生整批入队失败的累计次数 |
| `categraf_current_queue_size` | 最近记录的内存队列长度 |
| `categraf_info` | Categraf 版本信息 |

常用 PromQL：

```promql
categraf_current_queue_size
```

```promql
rate(categraf_metrics_enqueue_sum[5m])
```

```promql
increase(categraf_metrics_enqueue_failed_sum[10m])
```

正确解读：

- queue size 持续升高，说明生产速度长期大于 writer 消费速度，或某个 writer 阻塞；
- enqueue failed 增长，说明队列已无足够空间，整批样本开始丢弃；
- enqueue failed 不增长，不代表远端发送成功；HTTP 失败发生在出队之后，不计入这些指标。

还有一个观测盲区：self metrics 自己也通过同一 writer 发送。后端完全不可用时，后端自然看不到最新队列指标，所以必须同时保留 Categraf 本地日志和进程监控。

`--test` 会绕过入队，因此不要用 test 模式下的 self metrics 判断正式 writer 吞吐。

## 12. batch、超时和多 writer 应该怎么调

默认项：

```toml
[[writers]]
timeout = 5000
dial_timeout = 2500
max_idle_conns_per_host = 100
```

当前实现中：

- `dial_timeout` 用于建立 TCP 连接；
- `timeout` 用于等待响应头；
- `max_idle_conns_per_host` 控制每个目标主机的空闲连接上限。

调优原则：

- 413 时可以先适当减小 `batch`，同时核对反向代理和后端限制；
- queue 持续堆积时，先查后端延迟和所有 writer，再考虑 batch、网络和容量；
- timeout 过小会放大慢请求失败，过大则会让故障 writer 更久地阻塞后续批次；
- 多 writer 中一个很慢，也会拖慢整个消费循环；
- 不要把超大 `chan_size` 当成容灾方案。

调整后观察至少几个完整采集周期，并同时看 queue、内存、remote write 日志和后端写入速率。

## 13. test 能打印的样本为什么仍可能没进入队列

test 模式在样本转换成 Prometheus 时序之前打印数据。正式写入时，Categraf 还会把样本 value 转成浮点数；无法转换成数值的样本会被丢弃。

因此，极少数自定义插件或自定义 SQL 可能出现：

```text
--test 能看到某条字符串值
  -> 正式转换为 TimeSeries 失败
  -> 不进入 writer 队列
```

排查自定义采集时应确保 metric field 是数值，把维度字符串放进 labels，而不是 value。标准插件已经尽量在采集侧处理类型，但自定义 SQL、exec 输出和新开发插件仍要特别检查。

另外，`metrics_drop`、`metrics_pass`、`metrics_name_prefix` 和 relabel 在 test 输出之前已经生效。test 中看到的名称和标签，才是 writer 准备处理的名称和标签。

## 14. 主机时间偏差会让“已经写入”看起来像“查不到”

Categraf 会给没有时间戳的普通样本补当前时间，并按 `[global].precision` 处理精度。主机时间明显错误时会出现：

- 样本写到了查询时间范围之外；
- 后端拒绝过旧或过新的样本；
- Dashboard 最近 15 分钟无数据，但扩大范围后出现在异常时间；
- 多节点曲线彼此错位。

检查：

```shell
date -Ins
timedatectl status
chronyc tracking
```

如果系统没有 chrony，使用本环境实际的 NTP 客户端检查。还要比较：

- Categraf 主机时间；
- 后端接收节点时间；
- 查询页面时区和时间范围；
- test 输出中的 Unix 时间戳。

不要通过扩大 Dashboard 到最近 30 天来掩盖时钟错误。修复时间同步后，重新产生新样本并查询。

## 15. HTTP 2xx 但后端仍查不到怎么排查

2xx 只表示接收端对请求返回成功，不等于你正在正确位置查询。按下面顺序检查：

### 写入和查询是不是同一套后端

记录 writer URL 的域名、端口、路径和网关，再记录夜莺/Grafana 数据源实际查询 URL。确认没有写 A 查 B、写旧集群查新集群。

### 写入和查询是不是同一个租户

比较 writer 的 tenant Header、Basic Auth 用户，与查询数据源使用的 tenant Header。多租户后端中，同名指标可以存在于不同租户，彼此不可见。

### 查询的是不是 test 中的最终指标名

先查裸指标：

```promql
postgresql_up
```

然后先查看后端最终保存的标签，不要假定 `--test` 中的标签名会原样保留：

```promql
count by (agent_hostname, ident, instance, server) (postgresql_up)
```

如果 Categraf 经过夜莺接入，且原指标没有非空 `ident`，夜莺会把 `agent_hostname` 重命名为 `ident`，应查询：

```promql
postgresql_up{ident="monitor-01"}
```

如果原指标已经带有非空 `ident`，夜莺保留显式 `ident` 和 `agent_hostname`，可以查询：

```promql
postgresql_up{ident="<EXPLICIT_IDENT>",agent_hostname="monitor-01"}
```

如果 Categraf 直接写入 VictoriaMetrics 或其他 Prometheus 兼容 TSDB，没有经过夜莺的标签转换，则通常查询：

```promql
postgresql_up{agent_hostname="monitor-01"}
```

不要一开始复制 Dashboard 的复杂 PromQL。`metrics_name_prefix` 或 relabel 可能已经改变指标名，夜莺等接入层还可能改变标签名。

### 查询时间是否覆盖样本

先用 instant query 查当前值，再用最近 1 小时 range query。检查返回结果中的时间戳，不要只看图表是否画线。

### 后端是否异步落盘、复制或限流

检查接收日志、拒绝计数、租户配额、写入队列、存储容量和集群健康。网关返回 2xx 但下游异步处理失败时，需要沿后端内部链路继续查。

## 16. 直接用查询 API 验证裸指标

如果后端提供 Prometheus 兼容查询 API，可以绕过 Dashboard：

```shell
curl -sS -G 'https://query.example.com/api/v1/query' \
  --data-urlencode 'query=postgresql_up'
```

带 Basic Auth：

```shell
curl -sS -u '<USERNAME>:<PASSWORD>' \
  -G 'https://query.example.com/api/v1/query' \
  --data-urlencode 'query=postgresql_up'
```

带租户 Header：

```shell
curl -sS \
  -H 'X-Scope-OrgID: <TENANT_ID>' \
  -G 'https://query.example.com/api/v1/query' \
  --data-urlencode 'query=postgresql_up'
```

注意查询 URL 不一定能从 writer URL机械推导。请使用夜莺、VictoriaMetrics、Prometheus 兼容后端或查询网关实际公布的 API 地址。

查看结果时区分：

- HTTP/API 报错：查询端认证、语法或服务问题；
- `status=success` 但 result 为空：当前时间和筛选条件下没有序列；
- 有结果但 Dashboard 空白：进入数据源、变量和 PromQL 排查；
- 返回的标签与预期不同：以后端真实标签为准调整筛选。

## 17. 常见错误速查

| 日志或现象 | 最可能原因 | 优先动作 |
| --- | --- | --- |
| test 有数据，日志完全没有写入错误，后端空 | 配置不是正式进程使用的那份，或查错后端/租户 | 查 ExecStart、writer URL、数据源 |
| 前台脚本或 `--debug` 正常，systemd 无数据 | 服务缺少 PATH、代理、Token 或其他环境变量 | User、EnvironmentFiles、进程环境 |
| `no such host` | writer 域名解析失败 | DNS、systemd 网络环境 |
| `connection refused` | 端口错误或服务未监听 | writer 端口、网关 |
| `i/o timeout` | 路由、防火墙、代理或后端无响应 | 网络路径、NO_PROXY |
| x509 错误 | CA、主机名、证书有效期或 mTLS | TLS 文件和证书链 |
| 401/403 | 凭据、权限或租户错误 | Basic Auth、headers |
| 404/405 | remote write 路径错误 | 接收接口和反向代理 route |
| 413 | batch 或代理请求体限制 | 减小 batch、调后端限制 |
| 429 | 限流或配额 | 写入速率、租户配额 |
| 5xx | 后端或网关异常 | 服务健康和后端日志 |
| queue size 持续升高 | writer 太慢、阻塞或吞吐不足 | 所有 writers、延迟、容量 |
| enqueue failed 增长 | 内存队列满，整批丢弃 | 先修写入，再评估队列 |
| enqueue failed 不变但日志有 5xx | HTTP 失败不计入入队失败 | 以发送日志为准 |
| 2xx 但查询为空 | 查错集群、租户、时间或指标名 | 查询端点、tenant、时钟 |
| 重启后历史缺口 | 内存队列无持久化 | 修复故障并接受未发送样本缺口 |

## 18. 一套可以直接执行的修复流程

```shell
cd /opt/categraf

# 1. 记录版本、服务参数和时间
./categraf --version
sudo ./categraf --status
systemctl show categraf -p ExecStart -p User -p Environment -p EnvironmentFiles
date -Ins

# 2. 只验证采集侧；该命令不会写入后端
./categraf --test --inputs postgresql

# 3. 退出 test，检查正式 writer 配置
sed -n '1,180p' /opt/categraf/conf/config.toml

# 同时确认 systemd 的运行用户和环境文件
systemctl show categraf -p User -p EnvironmentFiles
systemctl cat categraf

# 4A. 恢复正式服务并观察发送日志
sudo ./categraf --start
journalctl -u categraf -f
```

如果需要在前台同时看指标与发送错误，使用 debug 但不要加 test：

```shell
cd /opt/categraf
sudo ./categraf --stop

# 会打印指标，也会入队并写入 [[writers]]
./categraf --debug --inputs postgresql
```

验证完成后按 `Ctrl+C`，再执行 `sudo ./categraf --start` 恢复服务。

在另一个终端：

```shell
# 5. 检查 writer 域名和端口
getent hosts metrics.example.com
nc -vz metrics.example.com 443

# 6. 直接查询后端裸指标
curl -sS -G 'https://query.example.com/api/v1/query' \
  --data-urlencode 'query=postgresql_up'
```

根据结果分流：

```text
Categraf 报网络/TLS/HTTP 错误
  -> 修 writer、认证、代理或后端

无 HTTP 错误，但 queue 堆积
  -> 查慢 writer、超时、批量和容量

写入 2xx，裸指标为空
  -> 查租户、查询端点、时间和后端接收日志

裸指标有数据
  -> remote write 已打通，转查 Dashboard
```

## 19. 常见问题

**heartbeat 正常，为什么指标仍然查不到？**

heartbeat 使用 `[heartbeat].url`，普通指标使用 `[[writers]].url`，两者是独立请求。心跳成功只能证明 heartbeat 链路。

**为什么 `--test` 有数据，等很久后端也不会出现？**

test 模式会打印 Metrics Agent 样本并直接返回，不会进入 writer 队列。退出 test，启动正式模式再验证。

**`--debug` 和 `--test` 对写入有什么区别？**

`--debug` 会打印样本并继续写入；`--test` 会打印后直接返回，不写入。两个参数同时使用时，test 逻辑优先，所以 `--debug --test` 仍然不写入后端。

**为什么脚本或 `--debug` 前台有数据，systemd 服务却没有？**

前台进程继承登录 shell 的 PATH、代理、Token 和其他 export，systemd 默认不会读取用户的 `.bashrc` 或 `.profile`。检查 unit 的 `User`、`EnvironmentFiles` 和服务进程实际环境；使用内置安装模板时，可在 `/etc/sysconfig/categraf` 中维护变量并重启服务。

**Categraf 会自动重试失败的 remote write 吗？**

当前 Metrics writer 没有为 HTTP 失败批次提供 WAL 或重新入队。失败会写日志，批次不会自动补传。Prometheus Agent 是另一条带 WAL 的链路，不能混为一谈。

**`categraf_metrics_enqueue_failed_sum = 0` 是否表示全部写入成功？**

不是。它只表示没有因为内存队列容量不足而入队失败，不统计出队后的网络、TLS 和 HTTP 失败。

**是否应该把 `chan_size` 调得越大越好？**

不是。过大会占用更多内存，而且无法修复持续写入失败。先消除错误 URL、认证、限流和后端容量问题，再根据短时峰值评估队列。

**多个 writer 中一个失败，另一个还会收到吗？**

会。每个 batch 会并发发送给所有配置的不同 URL；某个 writer 失败不阻止其他 writer 的这次请求，但发送循环会等待所有 writer 返回，慢 writer 会影响后续消费速度。

**为什么日志里有 example timeseries？**

发送失败时，Categraf 会打印本批次的一条示例时序帮助定位。分享日志前要检查其中的标签、地址和业务信息并脱敏。

## 20. 如何降低写入链路风险

- 把 writer URL、认证 Header、租户和查询数据源纳入同一份环境清单；
- 把 systemd 所需的 PATH、代理和脚本变量显式维护在受控的 EnvironmentFile 中，不依赖登录 shell profile；
- 在测试节点完成“正式模式写入 + 裸指标查询”，不能只跑 test；
- 启用 `input.self_metrics`，监控 queue size 和 enqueue failed；
- 对 Categraf 日志中的 remote write warning 建立采集与告警；
- 监控后端接收错误、限流、磁盘、写入延迟和租户配额；
- 保证 Categraf 与后端时间同步；
- 多 writer 场景分别监控每个接收端的延迟和错误；
- 调整 batch、queue 或 timeout 前先确认根因；
- 明确认知 Metrics writer 是内存缓冲，不把它当成带重试的持久队列；
- 变更后用准确指标名、租户和查询 API 做闭环验收。

最小验收标准：

```text
--test 有指标（采集验证）
  + 正式模式或仅 --debug 模式正在写入
  + Categraf 无 remote write 错误
  + self_metrics 无持续堆积/入队失败
  + 后端裸指标查询有最新时间戳
```

## 21. 小结

“test 有数据、后端没数据”不能用一条 curl 就下结论。正确顺序是：

```text
确认正式配置
  -> 检查 writer URL 与认证
  -> 看本地发送日志
  -> 看内存队列
  -> 看后端接收状态
  -> 用同租户查询 API 查裸指标
  -> 检查时间戳和标签
```

其中最容易误判的四点是：test 不发送、debug 会发送、heartbeat 不等于 writer、enqueue failed 不等于 HTTP failed。尤其要记住：`--debug --test` 仍然是 test，不会写入后端。把这些边界说清楚，remote write 问题通常就能快速落到具体一段。

当后端裸指标已经能查到，采集和写入闭环就完成了。下一篇继续处理展示层问题：[Categraf Dashboard 没有数据：数据源、变量、标签和 PromQL 排查](/blog/categraf-dashboard-no-data-troubleshooting/)。

---

**内容更新时间**：2026-07-20

**证据边界**：Metrics writer 的队列、批量、HTTP 请求、失败处理和 self metrics 语义来自当前 Categraf 仓库源码与默认配置；夜莺、VictoriaMetrics、Prometheus 兼容后端的具体接收路径、认证和租户机制应以实际部署版本为准。
