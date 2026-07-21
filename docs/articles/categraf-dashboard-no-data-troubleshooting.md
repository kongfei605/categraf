---
title: "Categraf Dashboard 没有数据：数据源、变量、标签和 PromQL 排查"
description: "本文介绍 Categraf 指标已写入后端、但夜莺或 Grafana Dashboard 没有数据时的完整排查方法，覆盖 Dashboard 文件选择、数据源、时间范围、变量、实际标签、PromQL、采集周期、版本兼容和面板变换。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Dashboard没有数据", "夜莺", "Grafana", "PromQL", "Dashboard变量", "监控排障"]
author: "快猫星云"
date: "2026-07-20T00:00:00+08:00"
tags: ["Categraf", "Troubleshooting", "Dashboard"]
---

Categraf 已在正式模式或仅带 `--debug` 的模式下运行，后端查询 API 能看到 `mysql_up`、`redis_up` 或 `postgresql_up`，但导入夜莺或 Grafana Dashboard 后，变量下拉框是空的，或者所有面板都显示 No data。

这里不能只用 `--test` 作为前提：test 模式只打印采集结果，不会写入后端。需要前台同时观察指标并验证写入时，应使用 `--debug --inputs <plugin>`，不要再加 `--test`；或者直接让 Categraf 以正式服务模式运行。

这类问题已经不在采集和写入层。Dashboard 的每个面板都要经过“数据源 → 时间范围 → 变量 → PromQL → 面板计算与展示”几层处理。裸指标有数据，只能证明第一段闭环完成；任何一个变量名、标签筛选或查询窗口不匹配，都可能把已有数据筛成空结果。

本文基于 Categraf 仓库现有的夜莺 `dashboard.json`、Grafana `*_grafana.json`，以及 PostgreSQL、MySQL、Redis 等大盘中的真实变量和 PromQL 写法，给出一套跨 Dashboard 可复用的排查流程。若后端裸指标仍然查不到，请先参考：[Categraf remote write 链路排查](/blog/categraf-remote-write-troubleshooting/)。

## 核心要点

- 先在 Dashboard 实际使用的数据源中查询裸指标；裸指标为空时，不要先改变量或面板。
- `--test` 不会写入后端；只有正式模式或不带 `--test` 的 `--debug` 模式产生的新样本，才可能被 Dashboard 查询到。
- 夜莺与 Grafana Dashboard 都是 JSON，但格式不能直接混用：夜莺通常使用 `dashboard.json`，Grafana 通常使用 `dashboard_grafana.json` 或 `*_grafana.json`。
- `label_values(metric, label)` 是 Dashboard 变量查询语法，不是标准 PromQL；变量为空时，要分别验证裸指标和标签是否存在。
- Dashboard 依赖的是后端真实标签，不是配置中“以为会有”的标签。Categraf 使用 `agent_hostname` 标识自身；经过夜莺接入时，只有指标不存在非空 `ident`，夜莺才会把它重命名为 `ident`。已有非空 `ident` 时显式值优先，`agent_hostname` 保留。
- 变量可以级联。PostgreSQL 大盘中 `db` 依赖 `$server`，上游变量为空或选错后，下游变量和所有面板都会空。
- `rate()`、`increase()` 需要范围内有足够样本。裸 Gauge 有值，不代表过短窗口内的区间查询一定有结果。
- 指标前缀、relabel、插件开关、Categraf 版本和 Dashboard 版本不一致，都可能让面板查询旧名称或不存在的指标。

## 1. 先判断是全局空白还是局部空白

不同范围的空白通常指向不同原因：

| 现象 | 最可能范围 | 优先检查 |
| --- | --- | --- |
| 所有 Dashboard 都空 | 数据源、租户、查询后端或时间 | 数据源连接、裸指标 |
| 同一 Dashboard 所有面板空，变量也空 | 变量基准指标或标签不存在 | 变量 definition、实际标签 |
| 变量有值，但所有面板空 | 变量值、PromQL 匹配方式或时间 | 展开后的查询 |
| 只有一个 row/一组面板空 | 对应插件开关、权限或指标族 | 面板指标名、采集项 |
| Gauge 面板有值，rate/increase 面板空 | 范围窗口或样本太稀疏 | 采集周期、`$__rate_interval` |
| 面板 Query 有结果，但图上不显示 | transform、字段选择、计算或单位 | Query inspector、面板设置 |
| 一台实例有数据，另一台没有 | 标签和值不一致 | `instance`、`server`、`ident` |

先不要批量修改 Dashboard。选一个最基础、最确定的指标和一个面板做纵向排查，比同时查看几十个面板更快。

## 2. 最短排查路径

推荐按下面顺序：

```text
确认 Dashboard 文件类型
    |
    v
确认 Dashboard 实际数据源与租户
    |
    v
在该数据源查询裸指标
    |
    v
检查时间范围和样本时间戳
    |
    v
列出裸指标的真实标签和值
    |
    v
逐个验证变量查询和级联变量
    |
    v
把面板 PromQL 替换成真实变量值执行
    |
    v
检查 rate 窗口、版本、transform 和计算
```

以 PostgreSQL 为例，最小验证序列：

```promql
postgresql_up
```

```promql
postgresql_datid
```

```promql
count by (server, db, agent_hostname) (postgresql_datid)
```

```promql
postgresql_numbackends{server="<ACTUAL_SERVER>",db="<ACTUAL_DB>"}
```

前一条没有通过，不要直接跳到后一条。

## 3. 先选对夜莺和 Grafana 的 Dashboard 文件

Categraf 把 Dashboard 与插件放在一起：

```text
inputs/postgresql/dashboard.json
inputs/postgresql/dashboard_grafana.json

inputs/mysql/dashboard-by-ident.json
inputs/mysql/dashboard-by-ident_grafana.json
inputs/mysql/dashboard-by-instance.json
inputs/mysql/dashboard-by-instance_grafana.json

inputs/redis/dashboard.json
inputs/redis/dashboard_grafana.json
```

一般约定：

| 平台 | 常见文件名 |
| --- | --- |
| 夜莺 | `dashboard.json`、`dashboard-by-*.json` |
| Grafana | `dashboard_grafana.json`、`*_grafana.json` |

两者不能因为扩展名都是 JSON 就直接混用。导错文件可能表现为导入失败、面板缺失、变量定义不兼容，或者导入后无法正确选择数据源。

同一个插件也可能有多个大盘版本。例如 MySQL 同时提供按 `ident`、按 `instance` 和 AWS RDS 的大盘。导入成功不代表选对版本：

- 指标主要带 `instance`，就优先使用 by-instance；
- 环境依赖夜莺主机标识 `ident`，再使用 by-ident；
- AWS CloudWatch 指标不能使用普通 MySQL input 大盘替代。

导入前先查看文件中的变量和第一批指标：

```shell
jq '.configs.var // .templating.list' inputs/postgresql/dashboard.json
jq '.templating.list' inputs/postgresql/dashboard_grafana.json
rg -n '"expr"|"definition"' inputs/postgresql/dashboard*.json | head -n 60
```

## 4. 必须在 Dashboard 实际使用的数据源中查裸指标

常见链路：

```text
Categraf
  -> remote write 接收入口
  -> 时序存储
  -> Prometheus 兼容查询入口
  -> 夜莺 / Grafana 数据源
  -> Dashboard
```

写入 URL 和查询 URL 不一定相同。更关键的是，Dashboard 可能选中了另一个集群或租户。

在夜莺或 Grafana 的查询页面中执行插件最基础指标：

```promql
mysql_up
```

```promql
redis_up
```

```promql
postgresql_up
```

如果页面不方便，也可以使用该数据源对应的查询 API：

```shell
curl -sS -G 'https://query.example.com/api/v1/query' \
  --data-urlencode 'query=postgresql_up'
```

这里必须使用 Dashboard 实际数据源的查询入口、认证和租户 Header。用另一个后端查到同名指标，不能证明当前 Dashboard 数据源里有数据。

裸指标为空时，先确认不是只运行了 `--test`。如果前台脚本或 `--debug` 能写入，而 systemd 服务模式没有新样本，还要比较 unit 的运行用户、`EnvironmentFiles`、PATH、代理和脚本所需变量。随后回到 writer、租户和后端接收排查；裸指标有数据后，再继续看变量。

## 5. 数据源“测试成功”为什么仍可能查不到业务指标

Grafana 的 Save & test 或夜莺的数据源连通检查，通常只能证明：

- URL 可达；
- 基本认证通过；
- 查询 API 能响应。

它不能证明：

- 选中了正确集群；
- 查询与写入使用同一租户；
- 该租户中存在 Categraf 指标；
- Dashboard 当前变量能匹配这些指标。

重点核对：

| 项目 | writer | Dashboard 数据源 |
| --- | --- | --- |
| 域名/集群 | 写入哪个后端 | 查询哪个后端 |
| 租户 | tenant Header 或账号 | 查询 tenant Header 或账号 |
| 网络位置 | Categraf 所在环境 | 夜莺/Grafana 服务端所在环境 |
| TLS/认证 | `config.toml` writer | 数据源自己的 TLS/认证 |

Grafana 数据源 URL 是从 Grafana 服务端访问查询后端的地址，不一定等于浏览器能访问的地址。容器中的 `127.0.0.1` 指向 Grafana 容器自身，不是宿主机上的 VictoriaMetrics。

如果 Dashboard 有 `datasource` 变量，导入后还必须选中真实的 Prometheus 类型数据源。Grafana 大盘常见写法：

```json
{
  "name": "datasource",
  "type": "datasource",
  "query": "prometheus"
}
```

后续变量和面板再通过 `${datasource}` 使用它。数据源变量没有选中时，其他变量可能全部为空。

## 6. 时间范围、时区和样本时间戳怎么检查

先把 Dashboard 切到最近 1 小时，并确认自动刷新开启。若采集周期较长，可以临时看最近 6 小时。

需要同时检查：

- Categraf 主机时间；
- 后端样本时间戳；
- 浏览器时区；
- Dashboard 的 timezone 设置；
- 页面右上角时间范围；
- 面板是否设置了独立 time shift 或 relative time。

主机检查：

```shell
date -Ins
timedatectl status
```

PromQL 查看最新样本距当前多久：

```promql
time() - timestamp(postgresql_up)
```

这是 instant query，其中的 `postgresql_up` 是 instant vector selector，受查询后端 lookback delta 限制。Prometheus 默认 lookback 为 5 分钟，但可以配置，Prometheus 兼容后端也可能采用不同值。只有最近样本仍在 lookback 范围内时，上式才能返回陈旧秒数；样本一旦超出 lookback，selector 会返回空向量，整个表达式也为空，不会显示一个很大的陈旧时长。

如果结果远大于采集周期，说明在当前 lookback 范围内已经出现明显延迟；如果是很大的负数，则可能存在未来时间戳。如果表达式为空，应扩大查询时间范围，使用 range query 查看 `postgresql_up` 的历史样本及其时间戳，例如依次检查最近 1 小时、6 小时和 1 天，不能把空结果解释为“从未写入”。

时间范围扩大后有数据，不代表问题已经解决。应继续确认为什么最新样本没有持续写入，以及时钟是否同步。

## 7. 后端真实标签才是 Dashboard 的依据

Categraf 常见标签来源包括：

- `agent_hostname`：Categraf 默认附加的 Agent 主机标识，可被 `omit_hostname` 关闭；
- `[global.labels]`：全局标签；
- `[[instances]].labels`：实例自定义标签，例如 `instance`、`cluster`、`env`；
- 插件自动标签：例如 PostgreSQL 的 `server`、`db`；
- relabel：可能新增、删除或改写标签；
- 后端接入层：指标不存在非空 `ident` 时，夜莺会把 Categraf 的 `agent_hostname` 重命名为 `ident`；已有非空 `ident` 时显式值优先，并保留 `agent_hostname`。其他平台也可能补充或转换标签。

这意味着同一份 Categraf 配置，仅仅因为写入路径不同，Dashboard 可用的主机标签就可能不同：

| 写入路径 | 后端主机标识标签 | Dashboard 变量应优先使用 |
| --- | --- | --- |
| Categraf -> 夜莺 -> TSDB，原指标没有非空 `ident` | `agent_hostname` 被重命名为 `ident` | `ident` |
| Categraf -> 夜莺 -> TSDB，原指标已有非空 `ident` | 显式 `ident` 与 `agent_hostname` 都保留 | `ident`，必要时同时筛选 `agent_hostname` |
| Categraf -> 其他 TSDB | 通常为 `agent_hostname` | `agent_hostname`，或显式补充 Dashboard 需要的标签 |

不要根据 `--test` 输出直接断定后端也存在 `agent_hostname`。`--test` 只出现 `agent_hostname`、没有非空 `ident` 时，经过夜莺写入后要改查 `ident`；如果 test 已有非空 `ident`，夜莺会保留显式 `ident` 和 `agent_hostname`。

不要靠配置文件推测最终结果，直接从后端列出标签：

```promql
count by (agent_hostname, ident, instance, server, db) (postgresql_datid)
```

对于 MySQL：

```promql
count by (agent_hostname, ident, instance, address) (mysql_global_status_uptime)
```

对于 Redis：

```promql
count by (agent_hostname, ident, instance, address) (redis_uptime_in_seconds)
```

没有出现的标签不会因为 Dashboard 使用了它就自动生成。比如大盘按 `instance` 过滤，而实际指标只有 `address`，变量查询自然为空。

## 8. `ident`、`instance`、`server` 和 `agent_hostname` 不要混用

这些标签常被误认为都是“实例”：

| 标签 | 常见来源 | 典型用途 |
| --- | --- | --- |
| `agent_hostname` | Categraf 自动附加；直写其他 TSDB 时通常保留 | 区分采集 Agent |
| `ident` | 显式标签优先；不存在非空值时，夜莺由 `agent_hostname` 重命名得到 | 夜莺主机/对象标识相关大盘 |
| `instance` | 用户在 `labels` 中显式设置，或目标生态产生 | 稳定业务实例标识 |
| `server` | PostgreSQL 等插件自动生成，可能受 `outputaddress` 影响 | 区分数据库连接目标 |
| `address` | MySQL、Redis 等插件连接地址标签 | 显示真实目标地址 |

例如 PostgreSQL 配置：

```toml
[[instances]]
address = "host=db.example.com port=5432 user=categraf password=<PASSWORD> dbname=postgres sslmode=require"
outputaddress = "postgres-main"
labels = { instance = "postgres-main", env = "prod" }
```

这里 `outputaddress` 主要影响插件输出的 `server` 标签；`labels.instance` 是另一个独立标签。大盘使用哪个，就必须保证后端有哪个。

对于主机指标，标签还取决于写入路径。例如 Categraf 只输出 `system_load1{agent_hostname="monitor-01"}`、没有非空 `ident` 时，经过夜莺接入后通常查询 `system_load1{ident="monitor-01"}`，直写 VictoriaMetrics 等 TSDB 时则通常查询 `system_load1{agent_hostname="monitor-01"}`。如果原指标已有非空 `ident`，夜莺不会用 `agent_hostname` 覆盖它，并会保留 `agent_hostname`。显式配置的业务 `instance` 是另一个标签，不应与这次重命名混为一谈。

不要直接把 Dashboard 中所有 `server` 批量替换成 `instance`。先确认每个指标族是否都包含目标标签，再决定统一标签策略。

## 9. 如何逐步验证 Dashboard 变量

以仓库中的 PostgreSQL 大盘为例，夜莺变量定义包括：

```text
server = label_values(postgresql_datid,server)
db     = label_values(postgresql_datid{server="$server"},db)
```

Grafana 大盘也使用同样的变量查询，并通过 `${datasource}` 选择数据源。

注意：`label_values()` 是 Dashboard 模板变量函数，不是标准 PromQL。不要把它直接发给 `/api/v1/query` 判断 PromQL 是否正确。

变量排查应拆成三步。

### 第一步：基准指标是否存在

```promql
postgresql_datid
```

### 第二步：目标标签是否存在

```promql
count by (server) (postgresql_datid)
```

### 第三步：用实际值验证下游变量的 selector

```promql
count by (db) (
  postgresql_datid{server="postgres-main"}
)
```

三步都有结果时，`label_values(postgresql_datid{server="$server"},db)` 才有条件生成 db 下拉值。

MySQL by-ident 大盘的变量基准指标是：

```text
label_values(mysql_global_status_uptime, ident)
```

如果真实指标没有 `ident`，应改用 by-instance 大盘，或者先建立一致的标签方案，而不是在页面上反复刷新。

## 10. 级联变量为什么会让整个 Dashboard 变空

级联关系：

```text
datasource
  -> server
      -> db
          -> panels
```

任一上游变量为空，下游都会空。常见原因：

- `datasource` 没选中；
- `server` 保留了导出环境中的旧值；
- `server` 变量改了，但 `db` 变量没有刷新；
- URL 中保存了不存在的 `var-server`；
- 多选或 All 返回正则字符串，但下游使用精确匹配 `=`；
- 变量有显示名和值映射，面板实际拿到的不是预期值。

修复顺序：

1. 先选 datasource；
2. 刷新 server 变量并选择一个真实值；
3. 再刷新 db 变量；
4. 清除浏览器 URL 中旧变量参数；
5. 最后刷新面板。

变量支持多选或 All 时，PromQL 通常应使用正则匹配：

```promql
mysql_up{instance=~"$instance"}
```

只允许单选时可以使用精确匹配：

```promql
postgresql_up{server="$server"}
```

不要在不理解变量展开值的情况下机械替换 `=` 和 `=~`。先查看 Query inspector 中实际执行的 PromQL。

## 11. 从裸指标逐层还原面板 PromQL

假设面板查询是：

```promql
increase(postgresql_xact_commit{server="$server",db="$db"}[5m])
```

逐层验证：

```promql
postgresql_xact_commit
```

```promql
postgresql_xact_commit{server="postgres-main"}
```

```promql
postgresql_xact_commit{server="postgres-main",db="app"}
```

```promql
increase(postgresql_xact_commit{server="postgres-main",db="app"}[5m])
```

这样可以准确判断：

- 指标名是否存在；
- server 是否匹配；
- db 是否匹配；
- range function 是否因为窗口或样本数为空。

复杂公式也要拆开。例如缓存命中率：

```promql
postgresql_blks_hit / (postgresql_blks_hit + postgresql_blks_read)
```

分别查询分子和分母。某一边缺失，或者两边标签无法匹配时，二元运算找不到配对序列，结果是空向量。只有两边成功匹配且分母为零时才进入浮点除零语义：非零值除以零得到 `+Inf` 或 `-Inf`，`0 / 0` 得到 `NaN`。

## 12. `rate()` 和 `increase()` 为什么比裸指标更容易空

区间函数需要范围向量内有足够样本。下面情况会造成空结果或不稳定：

- Categraf 采集周期 60 秒，面板却使用很短的窗口；
- `interval_times` 把实例周期放大；
- 数据刚开始采集，窗口内只有一个样本；
- 页面时间范围过短；
- 序列刚因标签变化而重新创建；
- Counter 重置或长时间断点。

先检查样本间隔：

```promql
count_over_time(postgresql_xact_commit[10m])
```

再根据实际周期选择窗口。经验上 range 应明显大于采集周期，并至少覆盖两个以上样本。

Grafana 大盘如果使用 `$__rate_interval`，还要检查数据源的 scrape interval 设置是否符合 Categraf 实际采集周期。Categraf 是 remote write 发送方，不代表 Grafana 能自动知道每个 input 的真实 `interval_times`。

## 13. 指标名变化和采集开关如何影响 Dashboard

Dashboard 查询的是固定指标名，但 Categraf 可以在采集侧改变结果：

- `metrics_name_prefix` 给指标增加前缀；
- relabel 改写 `__name__`；
- `metrics_pass` 只保留部分指标；
- `metrics_drop` 丢弃部分指标；
- 插件开关关闭某个指标族；
- 数据库权限导致部分查询失败；
- Categraf 版本升级后指标名或标签发生变化；
- 使用其他 exporter 采集时采用另一套命名。

先列出实际指标名。Prometheus 兼容后端可使用元数据/series API，或者在查询页面按插件前缀搜索。然后对比 Dashboard JSON：

```shell
rg -o '"expr"\s*:\s*"[^"]+' inputs/postgresql/dashboard.json | head -n 60
rg -o '"expr"\s*:\s*"[^"]+' inputs/postgresql/dashboard_grafana.json | head -n 60
```

推荐从与运行二进制同一版本或同一发布包取得 Dashboard。只升级大盘、不升级采集器，或者只升级采集器、继续使用很旧的大盘，都可能产生兼容问题。

## 14. 区分 No data、查询错误、零值、NaN 和 Inf

面板看起来“空”，实际可能是几种不同状态：

| 状态 | 含义 | 排查方向 |
| --- | --- | --- |
| No data / 空 vector | PromQL 没返回序列 | 数据源、时间、标签、指标名 |
| Query error | PromQL、数据源或后端报错 | 错误详情、API 响应 |
| 值为 0 | 有序列且业务值确实为零 | 面板阈值、单位、业务语义 |
| NaN | 两个已匹配的值执行 `0 / 0`，或其他明确产生 NaN 的计算 | 分别检查分子和分母的值 |
| `+Inf` / `-Inf` | 已匹配的非零值除以零 | 检查分母为什么为零及符号 |

缺失序列和标签无法匹配属于空向量，不属于 `NaN`。不要把零值当成无数据，也不要用 `or vector(0)` 过早填补空结果，这会把“采集断了”和“业务值为零”混在一起。

Grafana 使用 Query inspector 查看实际请求、展开后的 PromQL、返回 JSON 和耗时。夜莺可以使用面板编辑/查询预览或数据源查询页面查看原始结果，具体入口随版本可能不同。

## 15. Query 有结果，面板为什么仍然空

如果 Query inspector 已经返回序列，继续检查展示层：

- transform 是否过滤掉字段或行；
- stat 面板的 calculation 是否选择了不存在的字段；
- value field 是否仍叫 `Value`；
- legend、rename 或 regex 是否意外删除全部字段；
- 表格 panel 是否隐藏了所有列；
- unit、decimals 或阈值是否让值看起来异常；
- 面板是否被 repeat 变量生成到另一个 row；
- panel override 是否只匹配旧字段名；
- null value 处理是否隐藏断点。

最快的定位方式是临时复制面板，删除 transform、override 和复杂计算，只保留原始 PromQL 与时间序列展示。如果基础面板有数据，再逐项恢复展示配置。

## 16. 夜莺 Dashboard 的重点检查项

夜莺大盘通常使用：

```text
inputs/<plugin>/dashboard.json
```

导入后重点检查：

1. Dashboard 绑定或选择的数据源是否正确；
2. 数据源查询租户是否与 writer 一致；
3. `configs.var` 中的变量 definition 是否有对应指标和标签；
4. 变量刷新后是否选中真实值；
5. panel 的 `datasourceCate` 与实际数据源类别是否一致；
6. PromQL 中的变量使用精确匹配还是正则匹配；
7. 夜莺版本是否支持该 Dashboard JSON schema 和 panel 版本。

检查变量：

```shell
jq '.configs.var' inputs/postgresql/dashboard.json
```

检查面板表达式：

```shell
jq -r '.. | objects | .targets? // empty | .[]? | .expr? // empty' \
  inputs/postgresql/dashboard.json | head -n 80
```

如果从其他环境导出的 Dashboard 带有固定数据源 ID、业务组或旧变量默认值，导入后要清理环境专用绑定。

## 17. Grafana Dashboard 的重点检查项

Grafana 大盘通常使用：

```text
inputs/<plugin>/dashboard_grafana.json
inputs/<plugin>/*_grafana.json
```

导入时重点检查：

1. 选择 Prometheus 类型的数据源；
2. `datasource` 变量是否指向正确 UID；
3. 查询变量的 datasource 是否是 `${datasource}`；
4. 变量 Refresh 设置是否会在加载或时间范围变化时刷新；
5. Dashboard URL 是否带着旧的 `var-*` 参数；
6. Explore 中同一数据源能否查到裸指标；
7. Query inspector 展开的 PromQL 是否符合预期。

查看 Grafana 变量：

```shell
jq '.templating.list[] | {
  name,
  type,
  datasource,
  definition,
  query,
  current,
  includeAll,
  multi
}' inputs/postgresql/dashboard_grafana.json
```

大盘导出文件中的 `current` 可能为空，也可能保留导出环境值。生产发布前建议清理不可复用的数据源 UID 和实例默认值。

## 18. 如何快速判断是变量问题还是 PromQL 问题

使用一个实际变量值替换模板：

```promql
# Dashboard 原始表达式
postgresql_numbackends{server="$server",db="$db"}

# 手工替换后的验证表达式
postgresql_numbackends{server="postgres-main",db="app"}
```

结果判断：

| 手工值查询 | Dashboard 查询 | 结论 |
| --- | --- | --- |
| 有数据 | 无数据 | 变量值、展开或刷新问题 |
| 无数据 | 无数据 | 标签、指标名、时间或采集问题 |
| 有数据 | Query inspector 也有数据 | 面板 transform/计算问题 |
| 查询报错 | 查询报错 | PromQL 语法或后端兼容问题 |

正则变量还要检查特殊字符。实例值中包含点号、冒号等字符时，Grafana 通常会按变量格式转义；手工拼接正则或使用错误格式选项可能改变匹配语义。

## 19. 一套可以直接执行的修复流程

以 PostgreSQL 大盘为例：

```shell
# 1. 确认导入的是正确平台文件
ls -l inputs/postgresql/dashboard.json \
      inputs/postgresql/dashboard_grafana.json

# 2. 查看变量依赖
jq '.configs.var' inputs/postgresql/dashboard.json
jq '.templating.list' inputs/postgresql/dashboard_grafana.json

# 3. 查看大盘使用的指标和标签
rg -n 'postgresql_|label_values' inputs/postgresql/dashboard*.json | head -n 100
```

然后在 Dashboard 实际数据源中依次执行：

```promql
postgresql_up
```

```promql
postgresql_datid
```

```promql
count by (server, db, agent_hostname) (postgresql_datid)
```

```promql
postgresql_numbackends{server="<ACTUAL_SERVER>",db="<ACTUAL_DB>"}
```

最后回到 Dashboard：

```text
选择正确 datasource
  -> 清除旧 URL 变量
  -> 刷新 server
  -> 选择真实 server
  -> 刷新 db
  -> 选择真实 db
  -> 打开 Query inspector
  -> 对比展开后的 PromQL
```

如果只有区间函数为空，再检查：

```promql
count_over_time(postgresql_xact_commit{server="<ACTUAL_SERVER>",db="<ACTUAL_DB>"}[10m])
```

样本数量不足时，扩大 range 或修复采集间隔，不要先改成固定零值。

## 20. 常见错误速查

| 现象 | 最可能原因 | 优先动作 |
| --- | --- | --- |
| 导入即报格式错误 | 夜莺/Grafana 文件混用或 schema 不兼容 | 选择正确 JSON 和平台版本 |
| datasource 下拉为空 | 没有 Prometheus 类型数据源或变量绑定错误 | 配置数据源、检查 UID |
| datasource 正常，所有变量空 | 基准指标或变量标签不存在 | 查裸指标、count by label |
| server 有值，db 为空 | 级联 selector 不匹配 | 用实际 server 查 db |
| 变量保留旧实例 | 导出值、URL 参数或缓存 | 清 current、URL 变量并刷新 |
| 裸指标有，面板无 | 标签筛选、指标名、range 或 transform | 逐层拆 PromQL |
| 一组高级面板空 | 插件开关、数据库权限或指标族未采集 | test、日志、插件配置 |
| Gauge 有，rate 空 | 窗口内样本不足 | 检查采集周期和 range |
| 结果为空向量 | 序列缺失或左右向量标签无法匹配 | 拆分公式、检查 labels 和 vector matching |
| 结果为 NaN | 两边匹配且发生 `0 / 0` | 检查分子、分母是否都为零 |
| 结果为 `+Inf` / `-Inf` | 两边匹配且非零值除以零 | 检查分母为何为零 |
| Query 有结果，图不显示 | transform、字段或计算设置 | 复制简化面板 |
| 一台实例正常，另一台空 | 标签值不一致或变量没包含 | count by、变量 regex |
| 最近 15 分钟空，1 天有 | 采集中断、时钟或时间范围 | timestamp、remote write |
| 导入旧大盘后大量空 | 指标/标签版本不兼容 | 使用同版本 Dashboard |

## 21. 常见问题

**为什么 `label_values()` 在查询 API 中报错？**

因为它是夜莺/Grafana 模板变量查询函数，不是标准 PromQL。查询 API 中先用裸指标和 `count by (label)` 验证。

**为什么变量下拉有值，选中后仍然 No data？**

打开 Query inspector 看变量展开后的表达式。常见原因是单选/多选与 `=`/`=~` 不匹配，或者变量保留了旧值。

**为什么 PostgreSQL 的 `server` 和我配置的 `instance` 不一样？**

`server` 是 PostgreSQL 插件标签，可能由连接地址或 `outputaddress` 生成；`instance` 通常是用户自定义标签。它们互不替代。

**为什么 MySQL by-ident 大盘变量为空？**

先检查 `mysql_global_status_uptime` 是否真的有 `ident` 标签。没有时使用 by-instance 大盘，或统一标签设计后再调整 Dashboard。

**为什么只看到 `up`，其他面板没有数据？**

`up` 只证明连接或健康检查。其他指标可能因权限、采集开关、目标版本或 metrics filter 缺失。回到 test 和 Categraf 日志验证面板所需指标。

**夜莺和 Grafana 的 Dashboard 可以共用同一份 JSON 吗？**

不能直接共用。两者的 JSON schema、变量、面板和数据源字段不同，应使用各自文件。

**可以把空结果统一改成 0 吗？**

不建议。只有业务语义明确“缺失等价于零”时才使用补零表达式。否则会掩盖采集、写入和变量故障。

## 22. 如何减少 Dashboard 无数据问题

- 从运行中 Categraf 对应版本取得 Dashboard；
- 导入前记录 Dashboard 依赖的指标和变量标签；
- 给多实例配置稳定的 `instance`、`cluster`、`env` 等标签；
- 明确 `agent_hostname`、`ident`、`instance`、`server` 的职责；
- 分别记录“经夜莺写入”和“直写 TSDB”时的最终标签，不能假定两条链路都存在 `ident`；
- Dashboard 发布前分别在夜莺和 Grafana 测试，不混用 JSON；
- 数据源、writer 和查询端统一记录集群与租户；
- 变量尽量使用稳定、低基数的基准指标；
- 级联变量控制层数，并为上游空值提供清晰提示；
- rate/increase 窗口按实际采集周期设置；
- Dashboard JSON 进入版本控制，清理固定数据源 UID 和测试实例；
- 每次插件指标或标签变更时同步评审 Dashboard 和告警规则；
- 验收时保存“裸指标、变量、核心面板”三层结果。

一个 Dashboard 的最小验收标准：

```text
正确数据源能查裸指标
  + 变量能列出真实实例
  + 核心面板 PromQL 有结果
  + 时间范围和刷新正常
  + 没有测试环境专用绑定
```

## 23. 小结

Dashboard 空白时，最有效的不是先改 JSON，而是逐层减少条件：

```text
复杂面板 PromQL
  -> 替换为实际变量值
  -> 去掉区间函数和公式
  -> 去掉标签 selector
  -> 回到裸指标
```

如果裸指标也没有，返回 remote write；裸指标有但标签不匹配，修变量与标签；PromQL 有结果但面板不显示，再查 transform 和计算。

归根结底，Dashboard 只是已有时序数据的一组查询模板。把“数据源、时间、标签、变量、PromQL、展示”六层分开验证，夜莺和 Grafana 的大多数 No data 问题都能快速定位。

下一篇将继续讲：`Categraf 多实例配置与标签设计：如何避免实例混淆和高基数`。

---

**内容更新时间**：2026-07-20

**证据边界**：Dashboard 文件命名、变量和示例 PromQL 来自当前 Categraf 仓库中的夜莺与 Grafana JSON；夜莺、Grafana 和具体 Prometheus 兼容后端的 UI 入口、变量实现及 schema 支持可能随部署版本变化，应以实际环境为准。
