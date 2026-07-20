---
title: "Categraf PostgreSQL 监控实战：连接、事务、缓存、锁和慢查询指标"
description: "本文介绍如何使用 Categraf 采集 PostgreSQL 指标，包括监控账号、实例配置、事务与缓存指标、锁等待自定义 SQL、pg_stat_statements、夜莺和 Grafana Dashboard，以及告警建议。"
image: "https://download.flashcat.cloud/categraf/categraf-n9e-postgresql-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-postgresql-dashboard.jpg"
keywords: ["Categraf", "PostgreSQL监控", "PostgreSQL指标", "pg_stat_statements", "Nightingale", "Grafana"]
author: "快猫星云"
date: "2026-07-16T00:00:00+08:00"
tags: ["Categraf", "PostgreSQL", "Monitoring"]
---

PostgreSQL 出现性能问题时，只看主机 CPU、内存和磁盘通常不够。连接是否耗尽、事务是否大量回滚、缓存命中率是否下降、临时文件是否暴增、检查点是否过于频繁，以及哪些 SQL 消耗时间最多，都需要从数据库内部统计视图里找答案。

Categraf 的 `postgresql` 插件会连接 PostgreSQL，读取 `pg_stat_database` 和 `pg_stat_bgwriter` 等统计视图，并把结果转换成 Prometheus 风格指标。开启 `pg_stat_statements` 后，它还可以采集语句级调用次数、执行时间、返回行数和块读写时间。

本文用 PostgreSQL 16、Categraf 和夜莺完成了一次真实采集验证，并给出可以直接复用的配置、Dashboard、PromQL 和排障思路。

## 核心要点

- `postgresql_up = 1` 只表示 Categraf 能连接并认证成功；仍要结合日志确认统计视图和自定义 SQL 都能查询。
- 默认采集覆盖连接、事务、缓存、行活动、临时文件、死锁、冲突、后台写入和检查点等指标。
- `outputaddress` 会成为稳定的 `server` 标签，建议为每个实例显式设置，避免连接串变化导致时序漂移。
- 当前锁等待不是默认指标，可以通过 `[[instances.metrics]]` 执行只读 SQL 补充。
- 慢 SQL 指标需要 PostgreSQL 预加载 `pg_stat_statements`，并设置 `enable_statement_metrics = true`。
- 语句级指标带有 `query` 标签，存在高基数风险；生产环境应设置 `statement_metrics_limit`，并评估后端容量。

## 1. PostgreSQL 插件采什么

插件默认读取两组 PostgreSQL 统计信息。

第一组来自 `pg_stat_database`，按数据库输出：

- 当前连接数 `postgresql_numbackends`；
- 提交和回滚事务 `postgresql_xact_commit`、`postgresql_xact_rollback`；
- 缓存命中和磁盘读取 `postgresql_blks_hit`、`postgresql_blks_read`；
- 返回、读取、插入、更新、删除的行数；
- 临时文件数量和大小；
- 死锁、查询取消冲突；
- 开启 `track_io_timing` 后的块读写耗时。

第二组来自后台写入统计视图，输出定时/请求检查点、检查点写入与同步耗时、不同来源写出的缓冲区数量等指标。

PostgreSQL 17 把部分字段从 `pg_stat_bgwriter` 拆到了 `pg_stat_checkpointer`。Categraf 会自动查询新视图，并映射回原有指标名，因此现有 Dashboard 不需要为 PostgreSQL 17 单独改一套查询。

## 2. 创建只读监控账号

不要让 Categraf 使用应用账号或超级用户。可以创建独立账号并授予 PostgreSQL 内置的监控角色：

```sql
CREATE ROLE categraf WITH LOGIN PASSWORD '<PASSWORD>';
GRANT pg_monitor TO categraf;
ALTER ROLE categraf SET default_transaction_read_only = on;
GRANT CONNECT ON DATABASE appdb TO categraf;
```

`pg_monitor` 适合读取数据库运行状态，`default_transaction_read_only` 可以降低误操作风险。若自定义 SQL 要读取业务表，还需要对相应 schema 和表单独授予 `USAGE`、`SELECT`；不要为了方便直接授予超级用户权限。

先从 Categraf 所在机器验证网络和账号：

```shell
psql "host=127.0.0.1 port=5432 user=categraf dbname=appdb sslmode=disable" \
  -c 'select current_database(), current_user;'
```

生产环境建议使用 `sslmode=verify-full`，同时配置可信 CA。`sslmode=disable` 只适合可信内网或本地测试。

## 3. 配置 Categraf

PostgreSQL 插件配置文件位于：

```text
conf/input.postgresql/postgresql.toml
```

一个最小可用配置如下：

```toml
[[instances]]
address = "host=127.0.0.1 port=5432 user=categraf password=<PASSWORD> dbname=appdb sslmode=disable"
outputaddress = "prod-postgresql-01"
databases = ["appdb"]
max_lifetime = "5m"
prepared_statements = true
enable_statement_metrics = false
labels = { environment = "production", topology = "single" }
```

几个配置项需要特别注意：

| 配置项 | 作用 | 建议 |
| --- | --- | --- |
| `address` | PostgreSQL 连接串 | 显式填写 `dbname` 和 `sslmode` |
| `outputaddress` | 输出指标的 `server` 标签 | 使用稳定、可读的实例名 |
| `databases` | 只采集指定数据库 | 与 `ignored_databases` 二选一 |
| `ignored_databases` | 排除数据库 | 常用于排除 `template0`、`template1` |
| `max_lifetime` | 采集连接最长存活时间 | 可按代理和数据库策略设置 |
| `prepared_statements` | 是否使用预处理语句 | PgBouncer transaction 模式下设为 `false` |
| `disable_pg_stat_database` | 跳过数据库级统计 | 默认不要关闭 |
| `disable_pg_stat_bgwriter` | 跳过后台写入/检查点统计 | 默认不要关闭 |

监控多个实例时，增加多个 `[[instances]]`，并确保每个实例的 `outputaddress` 唯一。密码不应提交到 Git；实际部署可由配置管理或密钥系统渲染配置，并把文件权限限制为仅运行用户可读。

## 4. 启动并验证采集链路

修改配置后，先用测试模式检查插件输出：

```shell
./categraf --test --inputs postgresql
```

如果使用 systemd，再重启服务并查看日志：

```shell
sudo systemctl restart categraf
sudo systemctl status categraf --no-pager
journalctl -u categraf -n 100 --no-pager
```

随后在夜莺、Prometheus 或 VictoriaMetrics 查询：

```promql
postgresql_up
postgresql_numbackends
postgresql_xact_commit
postgresql_blks_hit
```

这次实测使用 PostgreSQL 16 容器，Categraf 通过宿主机映射端口连接数据库，并写入夜莺的 Prometheus 兼容后端。后端可以查到 `server="postgres16-lab"`、`db="lianhua"` 的 `postgresql_up = 1`，连接、事务、缓存和后台写入指标也能持续更新。

完整链路是：

```text
PostgreSQL -> Categraf postgresql input -> remote write -> 时序库 -> 夜莺 Dashboard
```

## 5. 连接与事务指标怎么看

**实例存活**

```promql
postgresql_up
```

值为 `0` 时优先检查地址、端口、密码、`pg_hba.conf`、TLS 和网络策略。值为 `1` 但某些面板没有数据时，应继续检查 Categraf 日志中的 SQL 权限或视图兼容性错误。

**当前连接数**

```promql
postgresql_numbackends{db="appdb"}
```

该指标要结合 `max_connections`、连接池上限和业务基线判断。默认插件没有输出 `max_connections`，可以通过自定义 SQL 补充，或者在告警阈值中使用已知容量。

**事务提交和回滚速率**

```promql
sum by (server, db) (rate(postgresql_xact_commit[5m]))
sum by (server, db) (rate(postgresql_xact_rollback[5m]))
```

这两个值是累计计数器，应使用 `rate()` 或 `increase()` 观察窗口内变化。回滚突然增多，常见原因包括应用异常、约束冲突、锁超时、语句超时或发布后的代码问题。

可以进一步看回滚占比：

```promql
100 * sum by (server, db) (rate(postgresql_xact_rollback[5m]))
/
clamp_min(
  sum by (server, db) (
    rate(postgresql_xact_commit[5m]) + rate(postgresql_xact_rollback[5m])
  ),
  1
)
```

## 6. 缓存、临时文件与行活动

**缓存命中率**

```promql
100 * postgresql_blks_hit
/
clamp_min(postgresql_blks_hit + postgresql_blks_read, 1)
```

命中率下降不等于一定要立即加内存。还要结合工作集变化、全表扫描、索引设计、查询计划和系统页缓存判断。新建数据库或低流量窗口也可能让比例暂时失真。

**临时文件增长**

```promql
increase(postgresql_temp_files{db="appdb"}[15m])
increase(postgresql_temp_bytes{db="appdb"}[15m])
```

临时文件快速增长通常意味着排序、Hash 或聚合超过 `work_mem`，也可能来自缺少索引的大查询。不要仅凭该指标全局调高 `work_mem`，因为它可能被单个查询的多个执行节点并发使用。

**行活动**

```promql
rate(postgresql_tup_inserted[5m])
rate(postgresql_tup_updated[5m])
rate(postgresql_tup_deleted[5m])
rate(postgresql_tup_fetched[5m])
```

行写入速率可以帮助识别批处理和流量突增，`tup_fetched`、`tup_returned` 的变化则适合与查询量、缓存命中率和慢 SQL 一起分析。

## 7. 检查点和 I/O 指标

Dashboard 中的检查点相关指标包括：

```promql
rate(postgresql_checkpoints_timed[5m])
rate(postgresql_checkpoints_req[5m])
rate(postgresql_checkpoint_write_time[5m])
rate(postgresql_checkpoint_sync_time[5m])
rate(postgresql_buffers_checkpoint[5m])
rate(postgresql_buffers_clean[5m])
rate(postgresql_buffers_backend[5m])
```

请求型检查点持续增多，通常需要结合 WAL 生成速率、`max_wal_size` 和写入突发排查。`buffers_backend` 偏高表示后端进程自己承担了较多缓冲区写出，也应与 bgwriter、检查点和磁盘延迟一起分析。

`postgresql_blk_read_time` 和 `postgresql_blk_write_time` 依赖 PostgreSQL 的 `track_io_timing`。如果该参数关闭，这两项通常为 0：

```sql
SHOW track_io_timing;
```

开启 I/O timing 会增加少量计时开销，是否开启应先在目标环境评估。查看趋势时同样要用 `rate()` 或 `increase()`，不要直接对累计值设置固定阈值。

## 8. 用自定义 SQL 采集锁等待

默认的 `pg_stat_database` 指标包含累计死锁数 `postgresql_deadlocks` 和查询取消冲突 `postgresql_conflicts`，但不能直接回答“当前有多少会话正在等锁”。

可以在同一个实例下增加自定义 SQL：

```toml
[[instances.metrics]]
mesurement = "locks"
label_fields = ["datname", "mode"]
metric_fields = ["waiting"]
timeout = "3s"
request = '''
SELECT
  COALESCE(d.datname, 'shared') AS datname,
  l.mode,
  COUNT(*) FILTER (WHERE NOT l.granted)::float8 AS waiting
FROM pg_locks l
LEFT JOIN pg_database d ON d.oid = l.database
GROUP BY 1, 2;
'''
```

输出指标名为：

```promql
postgresql_locks_waiting
```

注意，当前配置字段名就是 `mesurement`，需要按插件实现保留这个拼写。生产环境还可以增加锁类型或业务标签，但不要把 PID、完整 SQL、事务 ID 等高变化字段直接放进 label。

死锁告警应看时间窗口内的增量：

```promql
increase(postgresql_deadlocks[5m]) > 0
```

只写 `postgresql_deadlocks > 0` 会在历史上发生过一次死锁后持续触发，直到统计被重置，不适合直接用于生产告警。

## 9. 开启 pg_stat_statements 观察慢 SQL

语句级指标默认关闭。要开启它，先在 PostgreSQL 配置中预加载扩展：

```conf
shared_preload_libraries = 'pg_stat_statements'
```

修改后需要重启 PostgreSQL，再连接 Categraf 使用的数据库创建扩展：

```sql
CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
```

然后修改 Categraf 配置：

```toml
enable_statement_metrics = true
statement_metrics_limit = 100
```

插件会输出五组累计指标：

```text
postgresql_statements_calls_total
postgresql_statements_exec_milliseconds_total
postgresql_statements_rows_total
postgresql_statements_block_read_milliseconds_total
postgresql_statements_block_write_milliseconds_total
```

它们带有 `server`、`db`、`user`、`datname` 和 `query` 标签。可以按规范化 SQL 查询最近 5 分钟的平均执行时间：

```promql
sum by (server, datname, query) (
  rate(postgresql_statements_exec_milliseconds_total[5m])
)
/
clamp_min(
  sum by (server, datname, query) (
    rate(postgresql_statements_calls_total[5m])
  ),
  0.001
)
```

这里的“慢 SQL”来自累计语句统计，不是逐条慢日志。`query` 虽然经过换行和制表符归一化，但仍可能产生大量时序。`statement_metrics_limit` 会按累计执行时间排序后限制本次采集的语句数，生产环境不要设置为无限；还要制定统计重置、保留周期和敏感 SQL 文本处理策略。

本次截图环境没有为了文章重启已有业务容器，因此只展示默认数据库与后台写入指标；语句级指标按上述方式作为可选增强能力启用。

## 10. 导入夜莺和 Grafana Dashboard

Categraf 仓库提供两份 PostgreSQL 大盘：

```text
inputs/postgresql/dashboard.json
inputs/postgresql/dashboard_grafana.json
```

其中 `dashboard.json` 用于夜莺，`dashboard_grafana.json` 用于 Grafana。夜莺大盘已经提供 `datasource`、`server` 和 `db` 变量，导入后按顺序选择数据源、实例和数据库即可。

本次实测大盘如下，连接、缓存命中率、死锁、冲突、事务、行活动、临时文件、I/O、检查点和后台写入面板均来自真实采集数据：

![Categraf PostgreSQL 夜莺监控大盘](https://download.flashcat.cloud/categraf/categraf-n9e-postgresql-dashboard.jpg)

如果大盘没有数据，按下面顺序检查：

1. 查询 `postgresql_up` 是否存在且等于 1；
2. 检查 `server` 变量是否选中了 `outputaddress` 对应值；
3. 检查 `db` 变量是否选中实际业务库；
4. 确认 Dashboard 的 Prometheus 数据源正确；
5. 查看 Categraf 日志是否有权限、视图或 SQL 错误；
6. 确认查询时间范围覆盖 Categraf 开始上报后的时间。

后台写入指标的 `db` 标签固定为 `postgres`，并不属于某个业务数据库。因此 Dashboard 的 bgwriter/checkpointer 面板不应再按业务库变量过滤；本文配套大盘已处理这一点。

## 11. 告警规则怎么设计

仓库中的 `inputs/postgresql/alerts.json` 提供了实例不可用、读写时间、死锁和缓存命中率规则，可作为导入模板。生产使用前应按版本、负载和统计语义调整，而不是直接照搬阈值。

建议至少覆盖这些场景：

**实例不可用**

```promql
postgresql_up == 0
```

建议持续 1 到 2 分钟再触发，以减少发布、网络抖动或数据库短暂重启造成的噪声。

**死锁增加**

```promql
increase(postgresql_deadlocks[5m]) > 0
```

**缓存命中率持续偏低**

```promql
100 * postgresql_blks_hit
/
clamp_min(postgresql_blks_hit + postgresql_blks_read, 1) < 90
```

阈值不能跨所有数据库一刀切。分析型数据库、批处理窗口、新库和 OLTP 核心库的合理基线可能完全不同。

**临时文件异常增长**

```promql
increase(postgresql_temp_bytes[15m]) > 1073741824
```

示例阈值是 1 GiB/15 分钟，只用于说明写法，应根据实例规格和日常基线调整。

**当前锁等待**

```promql
sum by (server, datname) (postgresql_locks_waiting) > 0
```

锁等待是否需要立即告警取决于持续时间和业务 SLO。短暂等待很常见，建议设置持续时间，并结合应用延迟、活跃会话和阻塞链处理。

## 12. 常见问题

**`postgresql_up = 0`，但手工连接正常**

确认 Categraf 和手工测试是否运行在同一台机器、使用同一地址和 TLS 参数。容器内的 `127.0.0.1` 指向容器自身，宿主机端口映射也可能只绑定在回环地址。

**`postgresql_up = 1`，但只有少量指标**

查看日志中的 `failed to execute Query`。常见原因是监控账号权限不足、指定数据库不存在、统计视图受版本影响，或者自定义 SQL 超时。

**`blk_read_time`、`blk_write_time` 一直为 0**

先执行 `SHOW track_io_timing;`。关闭时不会累计块 I/O 计时。

**开启 statement metrics 后日志报找不到视图**

确认 `shared_preload_libraries` 已包含 `pg_stat_statements`、PostgreSQL 已重启，并在 Categraf 连接的数据库里执行过 `CREATE EXTENSION pg_stat_statements`。

**通过 PgBouncer 连接时报 prepared statement 错误**

如果 PgBouncer 使用 transaction pool mode，把 `prepared_statements` 设置为 `false`。

**Dashboard 的检查点和后台写入面板为空**

这些指标的 `db` 标签是 `postgres`。如果 PromQL 同时过滤了业务库名，就会查不到数据；使用仓库中的最新版 Dashboard，或移除这些面板上的业务库过滤条件。

## 13. 生产环境建议

- 每个 PostgreSQL 实例使用独立只读监控账号，并定期轮换密码；
- 显式设置稳定的 `outputaddress`，再用 `environment`、`region`、`cluster`、`topology` 等低基数标签补充上下文；
- 用 `databases` 或 `ignored_databases` 控制采集范围，避免模板库和无关数据库污染大盘；
- 对自定义 SQL 设置较短 `timeout`，先在从库或测试环境验证执行计划和权限；
- 语句级指标限制条数，评估 `query` 标签的基数、敏感信息和存储成本；
- 告警使用 `rate()`、`increase()` 处理累计计数器，并按业务基线设置持续时间；
- Dashboard、告警、配置和 PostgreSQL 参数应一起纳入版本管理和变更评审。

Categraf 的 PostgreSQL 插件可以先用很小的配置跑通“连接、采集、上报、看图”闭环，再按实际问题逐步增加锁等待、连接容量和语句级指标。这样既能获得足够的数据库可观测性，也能控制采集开销和时序基数。
