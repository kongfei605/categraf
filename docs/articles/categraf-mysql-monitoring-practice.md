---
title: "Categraf MySQL 监控实战：配置、指标、大盘和告警"
description: "本文介绍如何使用 Categraf 采集 MySQL 指标，包括账号权限、实例配置、核心指标、Grafana Dashboard、告警规则和常见问题排查。"
image: "https://download.flashcat.cloud/blog-mysql-monitoring-by-categraf.svg"
og_image: "https://download.flashcat.cloud/blog-mysql-monitoring-by-categraf.png"
keywords: ["Categraf", "MySQL监控", "MySQL指标", "Grafana", "Nightingale", "数据库监控"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "MySQL", "Monitoring"]
---

MySQL 是最常见的数据库监控对象之一。对数据库来说，只知道机器 CPU、内存、磁盘是否正常还不够，还要看到连接数、慢查询、锁等待、InnoDB Buffer Pool、复制延迟、Binlog 大小、库表空间等数据库内部状态。

Categraf 的 `mysql` 插件通过连接 MySQL 实例并执行内置 SQL，把这些状态整理成 Prometheus 风格指标，再写入夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端。

这篇文章讲 MySQL 监控的完整落地路径：账号权限怎么给、Categraf 怎么配、先看哪些指标、Dashboard 怎么选、告警规则怎么用。

## 核心要点

- Categraf MySQL 插件的前提是能连上数据库并成功 `Ping`，`mysql_up = 1` 表示连接和认证成功。
- MySQL 监控账号建议至少具备 `PROCESS`、`REPLICATION CLIENT` 和目标库表的 `SELECT` 权限。
- 多实例监控时，强烈建议通过 `labels` 设置稳定的 `instance` 标签，方便 Dashboard 和告警聚合。
- Dashboard 没有数据时，先查 `mysql_up`、`mysql_scrape_use_seconds`、`mysql_global_status_threads_connected`、`mysql_version_info`。
- 可选采集项很多，不建议一次性全开；库表大小、复制状态、Processlist、自定义 SQL 应按场景启用。

## 1. MySQL 插件采什么

Categraf 的 `mysql` 插件覆盖几类常见监控信息：

- 基础可用性：实例是否可连接、单次采集耗时；
- 全局状态：连接数、慢查询、问题数、查询数、临时表、锁等待等；
- 全局变量：最大连接数、表缓存、慢查询阈值、只读状态等；
- InnoDB：Buffer Pool、锁等待、事务、日志等待等；
- Processlist：按状态或用户统计连接；
- 库表空间：库级、表级数据和索引空间；
- 复制状态：主从 / 副本延迟、IO 线程、SQL 线程；
- Binlog：Binlog 文件数量和总大小；
- 自定义 SQL：把业务查询结果转成指标。

这意味着 MySQL 插件不是只做“端口探活”，而是面向数据库运行状态做结构化采集。

## 2. 创建监控账号

如果希望采集大多数内置指标，建议为监控账号授予这些权限：

```sql
GRANT PROCESS, REPLICATION CLIENT ON *.* TO 'categraf'@'%';
GRANT SELECT ON *.* TO 'categraf'@'%';
```

权限和采集内容的关系大致如下：

| 采集内容 | 典型 SQL | 常见权限 |
| --- | --- | --- |
| 存活探测 | `Ping()` | 可登录即可 |
| 全局状态 / 变量 | `SHOW GLOBAL STATUS` / `SHOW GLOBAL VARIABLES` | 依 MySQL 版本而异 |
| InnoDB 状态 | `SHOW ENGINE INNODB STATUS` | `PROCESS` |
| Processlist | `information_schema.processlist` | `PROCESS` |
| 库表大小 | `information_schema.tables` | 目标库表 `SELECT` |
| 复制状态 | `SHOW SLAVE STATUS` / `SHOW REPLICA STATUS` | `REPLICATION CLIENT` |
| Binlog 大小 | `SHOW BINARY LOGS` | `REPLICATION CLIENT` |

权限不足时，不一定表现为 `mysql_up = 0`。更常见的现象是：实例能连上，但某些指标缺失，并且 Categraf 日志里出现模块级查询错误。

## 3. 最小配置

MySQL 插件配置通常放在：

```text
conf/input.mysql/mysql.toml
```

最小可用配置如下：

```toml
[[instances]]
address = "127.0.0.1:3306"
username = "categraf"
password = "<PASSWORD>"
timeout_seconds = 3
labels = { instance = "prod-mysql-01:3306" }
```

几个细节要注意：

- `address` 以 `.sock` 结尾时才会使用 Unix socket；
- `localhost` 不会自动切换成 socket 连接；
- `labels` 不是 MySQL 插件专属字段，但多实例场景强烈建议设置；
- `instance` 标签应该稳定，不要频繁变化，否则 Dashboard 和告警聚合会变乱。

如果要监控多个实例，增加多个 `[[instances]]` 即可：

```toml
[[instances]]
address = "10.2.3.6:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "prod-mysql-a:3306" }

[[instances]]
address = "10.2.6.9:3306"
username = "categraf"
password = "<PASSWORD>"
labels = { instance = "prod-mysql-b:3306" }
```

## 4. 先验证基础指标

启动 Categraf 后，不要先看 Dashboard。建议先在后端查询几个基础指标：

```promql
mysql_up
mysql_scrape_use_seconds
mysql_global_status_threads_connected
mysql_version_info
```

预期结果：

- `mysql_up = 1`；
- `mysql_scrape_use_seconds` 有值；
- `mysql_global_status_threads_connected` 有值；
- `mysql_version_info` 出现，并带有 `version`、`innodb_version`、`version_comment` 等标签。

如果这些指标都有数据，说明最基础的采集链路已经通了：

```text
MySQL -> Categraf -> remote write -> 时序库 -> 查询 / Dashboard
```

## 5. 常用指标怎么看

MySQL 指标很多，刚开始可以先关注下面几类。

**实例存活**

```promql
mysql_up
```

`mysql_up = 0` 表示连接或认证失败。它不代表所有模块都采集成功，只代表基础连接状态。

**连接数**

```promql
mysql_global_status_threads_connected
mysql_global_variables_max_connections
```

连接数要结合最大连接数看。如果长期接近上限，需要排查连接池配置、慢 SQL、应用泄漏连接等问题。

**慢查询**

```promql
rate(mysql_global_status_slow_queries[1m])
```

慢查询出现时，不要只看监控值，还要结合 MySQL 慢日志、SQL 指纹和业务发布时间线。

**查询量**

```promql
rate(mysql_global_status_queries[1m])
rate(mysql_global_status_questions[1m])
```

查询量突增可能来自业务流量，也可能来自批处理、定时任务或异常重试。

**InnoDB Buffer Pool**

```promql
mysql_global_status_buffer_pool_pages_utilization
mysql_global_status_buffer_pool_bytes_dirty
```

Buffer Pool 相关指标适合判断缓存使用、脏页压力和数据库内存配置是否合理。

**复制延迟**

```promql
mysql_slave_status_seconds_behind_master
mysql_slave_status_seconds_behind_source
```

复制延迟只在启用复制状态采集且当前实例返回相关字段时出现。

## 6. 可选采集项怎么开

MySQL 插件有不少可选开关，不建议全部无脑打开。

常用开关包括：

| 配置项 | 作用 | 建议 |
| --- | --- | --- |
| `extra_status_metrics` | 扩展 `SHOW GLOBAL STATUS` 指标 | 需要更细状态分析时开启 |
| `extra_innodb_metrics` | 扩展 InnoDB 指标 | 排查 InnoDB 性能问题时开启 |
| `gather_processlist_processes_by_state` | 按状态统计连接 | 排查连接堆积时开启 |
| `gather_processlist_processes_by_user` | 按用户统计连接 | 多业务共用实例时有价值 |
| `gather_schema_size` | 采集库级空间 | 建议生产开启，但注意权限 |
| `gather_table_size` | 采集表级空间 | 表很多时要评估采集成本 |
| `gather_slave_status` | 采集主从状态 | 有复制链路时开启 |
| `gather_replica_status` | 采集新版 replica 状态 | MySQL 8 或新版本语义场景可考虑 |
| `gather_binary_logs` | 额外采集新版 Binlog 指标 | 需要观察 Binlog 空间时开启 |

如果你的实例很多，建议先从基础指标和 Dashboard 必需指标开始，再按问题场景逐步打开扩展模块。

## 7. 导入 Dashboard

Categraf 的 MySQL 插件目录下提供了多份 Dashboard：

```text
inputs/mysql/dashboard-by-ident_grafana.json
inputs/mysql/dashboard-by-instance_grafana.json
inputs/mysql/dashboard-by-aws-rds_grafana.json
```

如果是 Categraf 直接采集自建 MySQL，建议优先选择：

```text
inputs/mysql/dashboard-by-instance_grafana.json
```

导入 Grafana 后，重点检查：

- `datasource` 是否选中正确数据源；
- `instance` 变量是否有值；
- `mysql_up` 是否正常；
- 连接数、查询量、慢查询、InnoDB、复制状态等面板是否有数据。

如果 `instance` 变量为空，通常说明采集配置没有打 `instance` 标签，或者 Dashboard 变量使用的标签和实际指标标签不一致。

下面是测试环境中使用 Categraf 采集 MySQL 后导入 Grafana Dashboard 的效果：

![Categraf MySQL Grafana 大盘](https://download.flashcat.cloud/categraf/categraf-grafana-mysql-overview.jpg)

## 8. 告警规则怎么配

Categraf 仓库中提供了 MySQL 告警规则：

```text
inputs/mysql/alerts.json
```

其中覆盖的典型问题包括：

- MySQL 实例不可用；
- MySQL 刚刚发生重启；
- 连接数超过阈值；
- 慢查询出现；
- 打开文件句柄过多；
- 复制 IO 线程异常；
- 复制 SQL 线程异常；
- 复制延迟过高；
- InnoDB log waits 异常。

建议先导入作为模板，再结合实例规格、业务峰值、主从架构和数据库等级调整阈值。

数据库告警尤其要注意持续时间。比如瞬时连接数升高不一定需要立刻叫醒人，但连接数长期接近上限、复制延迟持续扩大，就应该提高优先级。

## 9. 自定义 SQL 指标

MySQL 插件支持自定义 SQL，把业务查询结果转成指标。它适合补充一些业务强相关的数据，比如队列表积压、任务状态数量、租户数等。

示例：

```toml
[[instances.queries]]
mesurement = "users"
metric_fields = ["total"]
label_fields = ["service"]
timeout = "3s"
request = '''
SELECT 'billing' AS service, COUNT(*) AS total FROM users;
'''
```

注意这里的字段名是 `mesurement`，这是当前实现中的历史拼写。配置时应按插件 README 中的字段名写。

自定义 SQL 要控制好频率、超时和结果基数。不要把高基数字段直接做标签，也不要让监控 SQL 影响业务库性能。

## 10. 常见问题

**Q1：`mysql_up = 1`，但 Dashboard 仍然有很多面板没数据，为什么？**

`mysql_up = 1` 只说明连接和认证成功。某些面板依赖 `PROCESS`、`REPLICATION CLIENT`、`SELECT` 权限，或者依赖可选采集开关。需要结合 Categraf 日志和具体指标名排查。

**Q2：MySQL 8 里还会看到 `mysql_slave_` 前缀吗？**

会。插件兼容 `SHOW REPLICA STATUS`，但当前部分输出指标名前缀仍保持 `mysql_slave_...`，这是历史兼容行为。

**Q3：表级空间采集是否一定要开？**

不一定。表数量很多时，表级空间采集可能增加查询开销。可以先开库级空间，必要时再对重点实例开启表级空间。

**Q4：AWS RDS 应该用哪份 Dashboard？**

如果是 AWS RDS 场景，可以优先看 `inputs/mysql/dashboard-by-aws-rds_grafana.json`。自建 MySQL 更适合按 `instance` 或 `ident` 的 Dashboard。

## 11. 小结

Categraf MySQL 监控的关键不是“采尽可能多的指标”，而是先把基础链路跑稳，再按数据库场景逐步补充扩展采集。

推荐落地顺序：

1. 创建最小权限可用的监控账号；
2. 配置 `[[instances]]`，并设置稳定 `instance` 标签；
3. 查询 `mysql_up`、`mysql_scrape_use_seconds`、`mysql_version_info`；
4. 导入 MySQL Grafana 或夜莺 Dashboard；
5. 根据复制、库表空间、Processlist、自定义 SQL 等场景开启可选模块；
6. 导入告警规则并调整阈值。

下一篇文章继续讲 Redis 监控。
