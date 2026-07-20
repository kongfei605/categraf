---
title: "Categraf 插件没有指标：从 test 模式到数据库权限的完整排查"
description: "本文给出 Categraf 进程正常但单个 input 没有指标时的完整排查路径，覆盖插件目录、instances、--inputs、test 模式、采集周期、网络、认证、TLS、运行用户、数据库权限和指标过滤。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Categraf插件没有指标", "test模式", "数据库监控", "权限排查", "metrics_pass", "监控采集"]
author: "快猫星云"
date: "2026-07-20T00:00:00+08:00"
tags: ["Categraf", "Troubleshooting", "Inputs"]
---

Categraf 服务已经是 running，CPU、内存等基础指标也正常，但新配置的 PostgreSQL、MySQL、Redis、Nginx 或其他插件就是没有指标。这是接入新监控对象时最常见、也最容易走弯路的一类问题。

它并不等于 Categraf 启动失败，也不一定是后端写入失败。故障可能发生在插件发现、配置解析、实例初始化、目标连接、认证授权、查询执行、指标过滤等任意一步。最有效的办法不是先改 Dashboard，而是用 `--test` 把“采集侧有没有生成指标”单独验证清楚。

本文基于 Categraf 当前仓库中的 Metrics Agent、local provider、input reader、writer、通用过滤配置，以及 MySQL、PostgreSQL、Redis 等插件实现，整理一套跨插件可复用的排查方法。若 Categraf 进程本身无法运行，请先参考：[Categraf 启动失败排查](/blog/categraf-startup-failure-troubleshooting/)。

## 核心要点

- `--inputs postgresql` 只过滤已发现的插件，不会凭空启用 PostgreSQL 插件；配置目录中仍然必须存在 `input.postgresql`。
- 默认插件模板中的 `[[instances]]` 不等于实例已经有效。数据库地址、URL 或目标列表仍是注释或空值时，实例会被跳过。
- 推荐先执行 `./categraf --debug --test --inputs <plugin>`。能打印指标，说明采集侧基本正常；没有指标时，继续查插件配置和目标端。
- `up = 0` 通常表示插件已经运行，但连接、认证或健康检查失败；`up = 1` 但部分指标缺失，通常要查数据库权限、功能开关和查询兼容性。
- `metrics_pass`、`metrics_drop` 和 relabel 可以把采集结果全部过滤掉；`metrics_name_prefix` 则会改变最终指标名。
- `--test` 中的普通 Metrics Agent 指标不会写入 writer，但主配置、writer 和其他已启用 Agent 仍会初始化，测试时仍要注意生产端点和共享目录。

## 1. 先确认问题属于哪一段

先用现象划定边界：

| 现象 | 所在环节 | 优先动作 |
| --- | --- | --- |
| Categraf 服务 stopped/failed | 主进程启动 | 查 journal、主配置和 systemd |
| 服务 active，日志没有目标插件启动记录 | 插件发现或初始化 | 查目录名、`--inputs`、配置和实例 |
| `--test` 完全没有目标插件指标 | 采集侧 | 查实例、网络、认证、权限和过滤 |
| `--test` 输出 `*_up = 0` | 目标连接或健康检查 | 查 DNS、端口、TLS、账号密码 |
| `--test` 输出 `*_up = 1`，但部分指标缺失 | 查询和授权 | 查数据库权限、插件开关和版本 |
| `--test` 有完整指标，后端查不到 | remote write | 查 writer、队列、HTTP 和后端 |
| 后端能查到，Dashboard 没数据 | 展示层 | 查数据源、变量、标签和 PromQL |

本文覆盖的是中间四行。第一行属于启动失败；最后两行将在后续 remote write 和 Dashboard 排障文章中处理。

## 2. 最短排查路径

可以把插件无指标压缩成下面这条路径：

```text
服务是否 active
    |
    v
配置目录中是否发现 input.<plugin>
    |
    v
--inputs 是否写对插件 key
    |
    v
配置能否解析，是否存在有效实例
    |
    v
--debug --test 是否打印指标
    |
    +--> 完全无输出：查初始化、周期和过滤
    |
    +--> up = 0：查网络、TLS、认证
    |
    +--> up = 1 但缺指标：查权限、开关、查询
    |
    +--> 指标完整：转查 remote write
```

推荐先执行：

```shell
cd /opt/categraf

sudo ./categraf --status
systemctl is-active categraf
journalctl -u categraf -b -n 200 --no-pager

find /opt/categraf/conf -maxdepth 2 -type f | sort
./categraf --version
```

如果 Categraf 使用 `[log].file_name` 写独立日志文件，还要同时查看该文件。插件运行阶段的错误不一定全部出现在 journal 中。

## 3. 正确使用 `--test` 隔离采集问题

以 PostgreSQL 为例：

```shell
cd /opt/categraf
./categraf --debug --test --inputs postgresql
```

`--test` 会把普通 Metrics Agent 生成的指标打印到标准输出，而不是放进 remote write 队列。输出类似：

```text
1784500000 10:00:00 postgresql_up agent_hostname=monitor-01 server=127.0.0.1:5432 1
```

重点观察三类内容：

1. 是否出现 `input: local.postgresql started`；
2. 是否有 `failed to load configuration`、`failed to init input` 或目标连接错误；
3. 是否打印目标插件的指标，以及 `up` 值是 `0` 还是 `1`。

测试进程会持续采集，确认结果后按 `Ctrl+C` 退出。如果全局周期较长，Linux 环境也可以给测试留出明确时间：

```shell
timeout 45s ./categraf --debug --test --inputs postgresql
```

不要在同一主机无计划地长期运行正式服务和测试进程。两者可能重复访问目标，也可能同时使用 Logs Agent、Prometheus Agent 的 WAL 或其他共享目录。生产环境建议在测试节点验证；必须原机排查时，可选择维护窗口暂时停止服务，或者复制一份最小配置并关闭无关的可选 Agent。

需要特别注意：`--test` 会阻止 Metrics Agent 的普通指标写入 writer，但不会跳过主配置和 writer 初始化，也不会自动关闭 heartbeat、Logs Agent、Prometheus Agent、Ibex Agent。

## 4. `--inputs` 是过滤器，不是启用开关

Categraf 的 `--inputs` 接受冒号分隔的插件 key：

```shell
./categraf --test --inputs postgresql
./categraf --test --inputs mysql:redis:postgresql
```

常见错误写法：

```shell
# 错误：不是逗号分隔
./categraf --test --inputs mysql,redis

# 错误：目录和插件 key 中没有这个连字符写法
./categraf --test --inputs postgre-sql
```

这个过滤器只允许匹配的插件继续加载。它不会创建配置，也不会把默认模板里的注释自动打开。因此，下面两项必须同时成立：

- 配置目录中存在 `input.postgresql`；
- `--inputs` 中使用插件注册的精确 key `postgresql`。

过滤匹配是精确的，建议使用小写插件名，不要在冒号两侧加入空格。忘记把目标插件放入正式服务的 `--inputs` 参数，也会造成“配置明明存在，服务却没有这个插件”的现象。

## 5. local provider 如何发现插件

默认 local provider 只扫描配置目录的直接子目录，并识别 `input.` 前缀：

```text
/opt/categraf/conf/
├── config.toml
├── logs.toml
├── input.cpu/
│   └── cpu.toml
└── input.postgresql/
    └── postgresql.toml
```

下面这些路径都不会按预期启用 PostgreSQL：

```text
/opt/categraf/conf/postgresql/postgresql.toml
/opt/categraf/conf/inputs/postgresql.toml
/opt/categraf/conf/input.postgres/postgresql.toml
/opt/categraf/conf/input.postgresql.toml
```

先确认 Categraf 实际读取的配置目录：

```shell
systemctl show categraf -p ExecStart -p WorkingDirectory
find /opt/categraf/conf -maxdepth 2 -type f | sort
```

Categraf 默认使用二进制旁边的 `conf`。如果显式传了其他目录，则 test 命令也必须使用同一个目录：

```shell
/opt/categraf/categraf \
  --configs /path/to/categraf/conf \
  --debug --test --inputs postgresql
```

插件目录内只读取 `.toml`、`.json`、`.yaml` 和 `.yml`。`postgresql.toml.bak` 不会生效；相反，旧配置如果仍以 `.toml` 结尾，就会和新配置一起加载。

## 6. `[[instances]]` 存在，为什么插件仍然没有启动

数据库和中间件插件通常使用一个或多个 `[[instances]]`。发行包里的模板为了展示字段，经常已经写了 table 头，但真正的目标地址仍被注释：

```toml
[[instances]]
# address = "host=127.0.0.1 port=5432 user=categraf password=<PASSWORD> dbname=postgres sslmode=disable"
```

这会得到一个结构上存在、业务上无效的空实例。PostgreSQL、MySQL、Redis 等插件在地址为空时会返回 `instances empty`，Metrics Agent 随后跳过该实例。只有开启 debug 时，空实例警告才更明显。

最小 PostgreSQL 配置应真正填写地址：

```toml
[[instances]]
address = "host=127.0.0.1 port=5432 user=categraf password=<PASSWORD> dbname=postgres sslmode=disable"
labels = { instance = "postgres-main" }
```

Redis 的最小实例：

```toml
[[instances]]
address = "127.0.0.1:6379"
password = "<PASSWORD>"
labels = { instance = "redis-main" }
```

有多个实例时，Categraf 会逐个初始化。一个实例无效不一定影响其他实例；但如果所有实例都为空或初始化失败，插件 reader 不会启动，也就不会产生采集指标。

不是所有插件都要求 `[[instances]]`。CPU、内存、系统负载等主机插件可以直接在插件级采集。排查时应以对应 `conf/input.<plugin>` 模板和 `inputs/<plugin>/README.md` 为准，不要机械地给所有插件增加 instances。

## 7. 插件配置解析失败时会发生什么

插件配置是在 Metrics Agent 启动后按插件分别加载的。单个 `input.*` 配置解析失败，常见日志是：

```text
E! failed to load configuration of plugin: local.postgresql error: ...
```

这种错误通常只跳过当前插件，Categraf 主进程和其他插件仍可运行。因此，单看 `systemctl status` 是 active 并不能证明每个插件都成功启动。

插件目录内的多个同格式文件会一起加载。常见问题包括：

- 新旧两个 `.toml` 都定义了冲突的 table；
- 字符串漏引号、数组少逗号或 table 括号不完整；
- duration 使用了不支持的格式；
- 字段类型不对，例如把布尔值写成普通字符串；
- 文件不可读、不是 UTF-8 或含有 NULL 字节；
- 从其他版本复制了不兼容的配置结构。

先列出实际参与加载的文件：

```shell
find /opt/categraf/conf/input.postgresql -maxdepth 1 -type f \
  \( -name '*.toml' -o -name '*.json' -o -name '*.yaml' -o -name '*.yml' \) \
  -print | sort
```

Python 3.11 及以上可以检查 TOML 文件及合并后的语法：

```shell
python3 - <<'PY'
from pathlib import Path
import tomllib

root = Path("/opt/categraf/conf/input.postgresql")
files = sorted(root.glob("*.toml"))

for path in files:
    data = path.read_bytes()
    if b"\x00" in data:
        raise SystemExit(f"NULL byte found: {path}")
    tomllib.loads(data.decode("utf-8"))
    print(f"OK: {path}")

merged = "\n".join(path.read_text(encoding="utf-8") for path in files)
tomllib.loads(merged)
print("OK: merged plugin TOML")
PY
```

这个脚本不能验证插件字段语义。最终仍要以前台 `--debug --test` 是否成功加载和采集为准。

## 8. 如何读懂 test 模式的几种结果

### 没有 `input ... started`，也没有指标

优先检查：

- `input.<plugin>` 目录是否位于正确配置目录；
- `--inputs` 是否拼写正确；
- 当前二进制是否支持该插件；
- 配置是否解析失败；
- 是否至少有一个有效实例。

### 有 `input ... started`，但第一轮没有指标

优先检查采集周期和实例周期。插件级 `interval` 可以覆盖全局周期；实例级 `interval_times` 会把实际周期放大：

```text
实例实际周期 = 插件或全局采集周期 × interval_times
```

例如全局周期为 15 秒，`interval_times = 4` 时，该实例约每 60 秒执行一次，而且第一次实例采集不一定发生在进程启动瞬间。短暂运行 test 后立即退出，可能只是还没等到实例轮次。

### 输出 `*_up = 0`

这通常是好线索：插件已经发现并运行，只是连接或健康检查失败。继续看同一时间附近的错误日志，例如 `connection refused`、`timeout`、认证失败或证书错误。

### 输出 `*_up = 1`，但业务指标很少

连接和 Ping 成功不代表所有查询都有权限。重点检查插件功能开关、目标版本、扩展是否启用，以及采集账号能否执行对应查询。

### 指标名称和预期不一样

检查 `metrics_name_prefix` 和 relabel。采集可能成功，只是最终名称已被改写。也要确认自己查的是当前插件真实输出的指标名，而不是其他 exporter 的命名方式。

## 9. 一定要用 Categraf 的运行用户做验证

终端里用登录用户测试成功，systemd 中仍可能失败。数据库 TCP 连接之外，很多插件还依赖本地资源：

- Unix socket；
- Docker socket；
- `/proc`、`/sys` 或设备文件；
- TLS CA、证书和私钥；
- `exec` 插件调用的脚本和命令；
- Kubernetes token、kubeconfig；
- SNMP 或其他插件读取的辅助文件。

先找出服务用户：

```shell
SERVICE_USER=$(systemctl show categraf -p User --value)
test -n "$SERVICE_USER" || SERVICE_USER=root
echo "$SERVICE_USER"
```

再用相同用户检查权限：

```shell
sudo -u "$SERVICE_USER" test -r /opt/categraf/conf/input.postgresql/postgresql.toml
sudo -u "$SERVICE_USER" test -r /path/to/ca.pem
sudo -u "$SERVICE_USER" test -r /path/to/client.crt
sudo -u "$SERVICE_USER" test -r /path/to/client.key
```

如果目标使用 Unix socket，还要检查每一级父目录的可遍历权限：

```shell
namei -l /var/run/postgresql/.s.PGSQL.5432
```

对于 Docker socket 等资源，不要直接 `chmod 777`。应把 Categraf 运行用户加入受控的权限组，或者通过 ACL 和最小权限明确授权，同时评估该资源本身带来的高权限风险。

## 10. 从同一网络位置检查 DNS 和端口

网络测试必须从 Categraf 实际运行的位置执行。宿主机能访问，不代表容器网络命名空间能访问；登录用户能解析，也不代表 systemd 使用的 DNS 和代理环境相同。

通用检查：

```shell
getent hosts db.example.com
nc -vz db.example.com 5432
curl -v --connect-timeout 3 http://target.example.com/metrics
```

如果没有 `nc`，可以使用目标协议自带客户端。数据库示例：

```shell
# PostgreSQL：按提示或环境变量提供测试密码
psql "host=db.example.com port=5432 user=categraf dbname=postgres sslmode=require" \
  -c 'select 1;'

# MySQL：-p 会交互式询问密码
mysql -h db.example.com -P 3306 -u categraf -p -e 'select 1;'

# Redis：REDISCLI_AUTH 避免把密码直接放在参数中
REDISCLI_AUTH='<PASSWORD>' redis-cli -h cache.example.com -p 6379 PING
```

常见错误与含义：

| 错误 | 常见原因 |
| --- | --- |
| `no such host` | DNS 名称错误或解析配置问题 |
| `connection refused` | 目标端口未监听、地址错误或容器内 `127.0.0.1` 指错位置 |
| `i/o timeout` / `context deadline exceeded` | 防火墙、路由、安全组、代理或目标无响应 |
| `network is unreachable` | 路由或网络命名空间问题 |
| 连接后立即断开 | 协议、TLS 模式、服务端策略或中间代理不匹配 |

容器内的 `127.0.0.1` 指向容器自身，不是宿主机，也不是另一个数据库容器。容器部署时应使用 compose 服务名、Kubernetes Service、宿主机可达地址或正确的共享网络。

## 11. 认证和 TLS 要与目标端配置一致

端口可达只证明 TCP 建连条件存在，不代表协议认证成功。继续区分：

- 用户不存在或密码错误；
- 账号来源地址不被允许；
- Redis ACL 用户缺少命令权限；
- PostgreSQL `pg_hba.conf` 不允许当前网段、用户或数据库；
- MySQL 用户只允许从其他 Host 登录；
- 服务端强制 TLS，但插件仍使用明文；
- 客户端启用 TLS，但 CA、Server Name 或证书链不匹配；
- 经过 PgBouncer、代理或负载均衡后，协议参数不兼容。

TLS 排查不要一开始就长期设置 `insecure_skip_verify = true`。先确认：

1. 是否确实需要 TLS；
2. CA 文件能否被服务用户读取；
3. 证书是否过期；
4. 连接使用的主机名是否包含在证书 SAN 中；
5. 客户端证书和私钥是否匹配；
6. TLS 最低版本和 cipher 是否被两端共同支持。

可以用下面的命令检查服务端证书链：

```shell
openssl s_client -starttls postgres \
  -connect db.example.com:5432 \
  -servername db.example.com \
  -showcerts
```

该命令只能辅助检查 TLS 握手和证书，不能替代数据库协议客户端的认证测试。

## 12. 数据库权限不足为什么不一定表现为 `up = 0`

数据库插件通常先做连接和 Ping，再执行多组统计查询。因此权限问题常有两种表现：

```text
连接或认证失败
  -> up = 0

连接成功，但统计查询被拒绝
  -> up = 1，同时部分指标缺失并记录 query 错误
```

不要因为 `mysql_up = 1` 或 `postgresql_up = 1` 就认定监控账号权限完整。应把日志中的具体失败查询与插件启用的功能对应起来。

常见权限边界：

| 插件 | 基础存活 | 额外指标常见要求 |
| --- | --- | --- |
| PostgreSQL | 能连接目标数据库并执行简单查询 | 统计视图读取权限；自定义 SQL 的对象权限；`pg_stat_statements` 扩展及统计读取权限 |
| MySQL | 能连接并 Ping | `PROCESS`、`REPLICATION CLIENT`、相关 `performance_schema` / `information_schema` 访问，以及自定义 SQL 的 SELECT 权限，具体取决于启用项 |
| Redis | 能认证并执行 PING | `INFO`、`SLOWLOG` 和自定义命令对应的 ACL 权限 |

在支持预定义角色的 PostgreSQL 版本中，可根据安全策略评估授予监控账号 `pg_monitor`：

```sql
GRANT pg_monitor TO categraf;
```

这不自动授予业务表的 SELECT，也不替代自定义 SQL 所需权限。启用 `pg_stat_statements` 指标时，还要确认扩展已加载、目标数据库已创建扩展，并限制语句标签可能带来的高基数和敏感文本风险。

MySQL 和 Redis 不建议直接照搬全权限账号。先从对应插件 README 和实际启用项列出所需命令或查询，再按最小权限授权。权限变更后，用同一个监控账号运行目标客户端命令和 test 模式复验。

## 13. 插件开关、目标范围和版本也会让指标变少

有些“缺指标”不是故障，而是配置明确关闭或限制了采集范围。例如：

- PostgreSQL 的 `databases`、`ignored_databases`；
- PostgreSQL 的 `disable_pg_stat_database`、`disable_pg_stat_bgwriter`；
- PostgreSQL 的 `enable_statement_metrics`；
- MySQL 的库表大小、Processlist、复制和 Binlog 采集开关；
- Redis 的慢日志采集开关；
- Prometheus input 的目标列表和服务发现结果；
- HTTP、DNS、网络探测插件中的目标数组。

排查时不要只看账号权限，还要把“期望指标”对应到具体开关和目标范围。某些视图、字段或命令也会随数据库版本变化，旧版 Categraf 配合新版数据库时可能出现部分查询失败；反向组合也一样。

记录两端版本：

```shell
/opt/categraf/categraf --version
psql --version
mysql --version
redis-cli --version
```

然后查当前 Categraf 版本中的 `conf/input.<plugin>`、`inputs/<plugin>/README.md` 和变更记录，不要只参考搜索引擎中其他版本的配置片段。

## 14. 指标过滤和 relabel 可能把结果全部丢掉

Categraf 的通用配置支持：

- `metrics_drop`：匹配到的指标丢弃；
- `metrics_pass`：只保留匹配到的指标；
- `metrics_name_prefix`：给指标名增加前缀；
- `relabel_configs`：改写标签、指标名，或执行 keep/drop。

`metrics_drop` 和 `metrics_pass` 支持 glob。例如在实例中配置：

```toml
[[instances]]
address = "host=127.0.0.1 port=5432 user=categraf password=<PASSWORD> dbname=postgres sslmode=disable"

# 仅用于演示：只保留存活指标
metrics_pass = ["postgresql_up"]
```

如果把 `metrics_pass` 写成一个永远匹配不到的名字，该实例采集成功后仍会得到空输出。类似地，过宽的 `metrics_drop = ["postgresql_*"]` 会丢掉全部 PostgreSQL 指标。

排障时建议临时移除这些处理配置：

```toml
# metrics_pass = [...]
# metrics_drop = [...]
# metrics_name_prefix = "..."
# [[instances.relabel_configs]]
# ...
```

然后重新运行 test。若指标恢复，再逐条加回过滤和 relabel。注意通用字段放在插件级还是 `[[instances]]` 内，作用对象会不同；数据库类实例指标通常应检查实例内部的通用配置。

## 15. 为什么 debug 日志很重要

普通日志会记录解析、初始化和采集错误，但空实例等提示在 debug 模式下更明显。建议同时观察 test 输出和服务日志：

```shell
cd /opt/categraf
./categraf --debug --test --inputs postgresql
```

常见日志线索：

| 日志或现象 | 最可能原因 | 优先检查 |
| --- | --- | --- |
| `no inputs` | 配置目录下没有任何可发现 input | `--configs`、目录层级 |
| 没有目标插件启动记录 | 被 `--inputs` 过滤或目录名不匹配 | 插件 key、ExecStart |
| `input: ... not supported` | 当前二进制没有注册该插件 | 插件名、版本、自定义构建 |
| `failed to get configuration of plugin` | 插件目录无法列出或文件不可读 | 目录和文件权限 |
| `failed to load configuration of plugin` | TOML/JSON/YAML 解析或合并失败 | 插件目录全部配置文件 |
| `no instances for input` | 没有有效实例，常见于地址仍被注释 | `[[instances]]` 关键字段 |
| `failed to init input` | TLS、正则、DSN 或插件初始化错误 | 错误上下文和配置字段 |
| `input: local.<plugin> started` | reader 已启动 | 继续看采集输出和目标错误 |
| `gather metrics panic` | 插件采集发生异常 | 完整堆栈、版本和输入数据 |
| `failed to ping ...` | 连接、认证、TLS 或目标状态异常 | 用相同账号和网络位置复验 |
| `failed to query ...` | 权限、SQL 兼容、扩展或功能开关 | 具体查询、数据库日志 |
| test 有指标，正式服务没有 | 服务配置、用户或环境不同 | ExecStart、User、环境变量 |

日志中可能包含目标地址、用户名或查询文本，分享日志前应脱敏，不要公开密码、Token、Cookie、连接串或业务 SQL。

## 16. 一套可以直接执行的修复流程

下面以 PostgreSQL 为例，其他插件只需替换插件 key 和连接测试命令：

```shell
cd /opt/categraf

# 1. 确认主进程和版本
sudo ./categraf --status
./categraf --version

# 2. 确认正式服务实际参数
systemctl show categraf -p ExecStart -p WorkingDirectory -p User

# 3. 确认插件目录和参与加载的文件
find /opt/categraf/conf/input.postgresql -maxdepth 1 -type f -print | sort

# 4. 检查关键字段是否仍然只是注释
sed -n '1,220p' /opt/categraf/conf/input.postgresql/postgresql.toml

# 5. 查看服务日志中的插件错误
journalctl -u categraf -b --since "30 minutes ago" --no-pager

# 6. 前台隔离测试采集
./categraf --debug --test --inputs postgresql
```

根据 test 结果分流：

```text
插件没 started
  -> 修目录、--inputs、配置解析、实例关键字段

插件 started，但没有输出
  -> 等待完整周期，移除过滤，检查实例初始化

postgresql_up = 0
  -> 修 DNS、端口、pg_hba、账号密码、TLS

postgresql_up = 1，但统计指标缺失
  -> 修 pg_monitor/对象权限、扩展、开关和版本兼容

test 指标完整
  -> 恢复正式服务，转查 remote write
```

配置修复后恢复服务并复查日志：

```shell
cd /opt/categraf
sudo ./categraf --stop
sudo ./categraf --start
sudo ./categraf --status
journalctl -u categraf -b -n 100 --no-pager
```

如果正式 unit 对 `--inputs` 做了限制，记得同步修改它的参数来源。使用 Categraf 内置 `--install` 创建的默认服务通常不会增加 input 过滤；自定义 unit 或启动脚本则需要单独核对。

## 17. 常见问题

**为什么目录和配置都正确，`--inputs postgresql` 还是没有指标？**

先加 `--debug`。最常见原因是模板中的 `address` 仍被注释，所有实例都被视为空；其次是配置解析失败、周期未到或指标被过滤。

**为什么不加 `--inputs` 时有指标，加了以后反而没有？**

通常是插件 key 写错、使用了逗号分隔或加入了空格。`--inputs` 是精确过滤，推荐直接使用 `input.` 目录后缀，例如 `input.postgresql` 对应 `postgresql`。

**为什么 test 模式还报 writer 配置错误？**

writer 在 Agent 运行前仍会初始化。test 模式只让 Metrics Agent 的样本打印到标准输出，不会绕过主配置和 writer 初始化。本地 TLS 文件路径等 writer 初始化错误仍需先修复。

**为什么 test 有数据，systemd 服务没有？**

比较 test 与 unit 的 `--configs`、`--inputs`、运行用户、环境变量和文件权限。不要默认 systemd 会继承当前 shell 的环境。

**为什么 `up = 1`，Dashboard 里的很多面板还是空的？**

先直接查看 test 是否有面板所需指标。如果 test 本身缺失，查权限和插件开关；如果 test 有、后端没有，查 remote write；后端也有时再查 Dashboard 变量和 PromQL。

**为什么修改配置后服务没有变化？**

local provider 当前不会自动监视本地文件变更。修改后需要重启服务，或者向进程发送 Categraf 支持的 reload 信号并确认日志。为了让变更过程更可控，常规运维建议使用 `--stop`、`--start` 并复查状态。

**可以直接用 root 或数据库管理员账号排除权限问题吗？**

测试环境可短暂用于定位，但不应作为最终配置。确认权限是原因后，应回到专用监控账号，按启用的查询和命令授予最小权限。

## 18. 如何减少插件无指标问题

生产环境可以把下面这些检查放进配置发布流程：

- 从当前 Categraf 版本的 `conf/input.<plugin>` 模板开始修改；
- 每个启用实例必须有非空目标地址，并设置稳定的 `instance` 等标签；
- 配置进入版本控制，凭据通过环境变量或密钥系统注入；
- 在测试节点先运行 `--debug --test --inputs <plugin>`；
- 记录 Categraf、目标服务和配置版本；
- 用与正式服务相同的用户、网络和证书做预检；
- 数据库监控账号按插件启用项授予最小权限；
- 对 `metrics_pass/drop` 和 relabel 做变更评审，避免全量丢弃；
- 配置修改后明确重启或 reload，并检查插件 started 日志；
- 同时监控 `*_up` 和关键业务指标，不能只监控 Categraf 进程存活。

对于数据库插件，推荐把验收拆成三层：

```text
第一层：连接成功，up = 1
第二层：基础统计指标完整
第三层：按需启用的复制、慢查询、自定义 SQL 等指标完整
```

这样可以准确区分网络认证问题、权限问题和高级采集项问题。

## 19. 小结

Categraf 插件没有指标时，最关键的动作是先运行：

```shell
./categraf --debug --test --inputs <plugin>
```

然后沿着下面的顺序逐段排除：

```text
配置目录
  -> 插件 key 与 --inputs
  -> 配置解析
  -> 有效 instances
  -> 采集周期
  -> 网络与 TLS
  -> 认证与数据库权限
  -> metrics_pass/drop 与 relabel
```

只要 test 还没有生成完整指标，就不要先去修改 writer 或 Dashboard。相反，一旦 test 能稳定打印完整指标，采集侧的任务基本完成，下一步应该转向：[Categraf 已采到指标但后端查不到：remote write 链路排查](/blog/categraf-remote-write-troubleshooting/)。

---

**内容更新时间**：2026-07-20

**证据边界**：插件发现、过滤、配置加载、实例初始化、采集周期、test 输出和通用指标处理来自当前 Categraf 仓库源码及默认配置；数据库权限示例用于说明常见边界，实际授权应结合数据库版本、插件启用项和组织安全策略验证。
