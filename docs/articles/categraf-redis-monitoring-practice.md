---
title: "Categraf Redis 监控实战：配置、指标、大盘和告警"
description: "本文介绍如何使用 Categraf 采集 Redis 指标，包括实例配置、INFO 指标、慢查询、自定义命令、Grafana Dashboard、告警规则和常见问题排查。"
image: "https://download.flashcat.cloud/blog-redis-monitoring-by-categraf.svg"
og_image: "https://download.flashcat.cloud/blog-redis-monitoring-by-categraf.png"
keywords: ["Categraf", "Redis监控", "Redis指标", "Grafana", "Nightingale", "缓存监控"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "Redis", "Monitoring"]
---

Redis 监控看起来简单：连上 Redis，执行 `INFO`，解析返回结果。但真正落地到生产环境时，还是会遇到很多细节：实例如何标识、命中率怎么看、内存碎片率要不要告警、慢查询是否采集、主从延迟如何判断、Dashboard 没数据怎么排查。

Categraf 的 `redis` 插件就是围绕这些问题设计的。它会连接 Redis 实例，执行 `INFO`、`PING` 等命令，把返回结果整理成监控指标，再写入夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端。

## 核心要点

- Redis 插件默认指标带有 `redis_` 前缀，比如 `redis_up`、`redis_used_memory`、`redis_keyspace_hitrate`。
- 多实例监控时，建议通过 `labels` 设置稳定的 `instance` 标签，方便 Dashboard、告警和实例筛选。
- Redis 监控至少要关注存活状态、PING 延迟、内存使用、命中率、拒绝连接、驱逐、复制状态和慢查询。
- 慢查询采集需要开启 `gather_slowlog = true`，不建议在不了解慢日志规模时贸然打开。
- Dashboard 没有数据时，先查 `redis_up`、`redis_ping_use_seconds`、`redis_scrape_use_seconds`。

## 1. Redis 插件如何采集

Redis 插件的基本原理是：

```text
Categraf -> Redis PING / INFO -> 指标解析 -> remote write -> 时序库
```

插件会采集几类数据：

- 基础状态：实例是否可用、PING 延迟、采集耗时；
- Server：运行时间等；
- Memory：内存使用、RSS、Lua 内存、最大内存、碎片率；
- Stats：连接数、命令数、QPS、命中率、拒绝连接、过期和驱逐；
- Persistence：RDB、AOF 相关状态；
- Clients：连接客户端和阻塞客户端；
- Replication：主从角色、复制偏移、复制延迟；
- CPU：Redis 进程 CPU 消耗；
- Cluster：集群模式状态；
- Keyspace：各 DB 的 key 数、过期 key 数、平均 TTL；
- Command Stats：按命令统计调用次数和耗时；
- Slow Log：慢查询日志；
- Custom Commands：自定义命令结果。

这些指标能覆盖 Redis 的大多数日常运行状态。

## 2. 最小配置

Redis 插件配置通常放在：

```text
conf/input.redis/redis.toml
```

最小配置如下：

```toml
[[instances]]
address = "127.0.0.1:6379"
username = ""
password = ""
labels = { instance = "prod-redis-01:6379" }
```

如果要监控多个 Redis 实例，就增加多个 `[[instances]]`：

```toml
[[instances]]
address = "10.23.25.2:6379"
username = ""
password = ""
labels = { instance = "prod-redis-a:6379" }

[[instances]]
address = "10.23.25.3:6379"
username = ""
password = ""
labels = { instance = "prod-redis-b:6379" }
```

这里最重要的是 `instance` 标签。Redis 实例经常以地址区分，但生产环境中地址可能变化，建议使用稳定、可读的实例标识。

## 3. 先验证基础指标

启动 Categraf 后，先在后端查询基础指标：

```promql
redis_up
redis_ping_use_seconds
redis_scrape_use_seconds
```

预期结果：

- `redis_up = 1` 表示 PING 成功；
- `redis_ping_use_seconds` 表示 PING 延迟；
- `redis_scrape_use_seconds` 表示单次采集耗时。

如果这些指标没有数据，优先检查：

- Redis 地址和端口是否正确；
- 用户名和密码是否正确；
- Categraf 所在机器能否访问 Redis；
- Redis 是否限制了来源 IP；
- Categraf 日志中是否有认证、超时或连接失败。

## 4. 常用指标怎么看

Redis 指标很多，建议先看下面这些。

**实例存活**

```promql
redis_up
```

这是最基础的可用性指标，适合做实例不可用告警。

**PING 延迟**

```promql
redis_ping_use_seconds
```

PING 延迟能反映 Redis 响应是否变慢，但它不能替代业务读写延迟。延迟异常时，需要结合网络、CPU、慢查询和命令统计一起看。

**内存使用**

```promql
redis_used_memory
redis_used_memory_rss
redis_maxmemory
redis_mem_fragmentation_ratio
```

内存监控要同时看逻辑使用量、RSS 和最大内存限制。`mem_fragmentation_ratio` 偏高时，可能和内存碎片、数据结构、释放行为有关。

**命中率**

```promql
redis_keyspace_hitrate
```

命中率低不一定代表 Redis 异常，也可能是业务访问模式变化、缓存过期策略不合理、预热不足或 key 设计问题。

**拒绝连接**

```promql
rate(redis_rejected_connections[1m])
```

拒绝连接通常和最大连接数、客户端连接池、突发流量有关。这个指标出现增长时，需要及时排查。

**驱逐 key**

```promql
rate(redis_evicted_keys[1m])
```

如果 Redis 已设置最大内存，驱逐可能是正常策略的一部分。但如果业务不接受缓存被动淘汰，就需要重点关注。

**阻塞客户端**

```promql
redis_blocked_clients
```

阻塞客户端增多时，可能和阻塞命令、慢查询、下游消费异常有关。

## 5. Keyspace 和命令统计

Keyspace 指标来自 `INFO keyspace`，常见指标包括：

```promql
redis_keyspace_keys
redis_keyspace_expires
redis_keyspace_avg_ttl
```

它们会带有 `db` 标签，比如 `db0`、`db1`。这些指标适合观察不同 DB 的 key 数、过期 key 数和平均 TTL。

命令统计来自 `INFO commandstats`，常见指标包括：

```promql
redis_cmdstat_calls
redis_cmdstat_usec
redis_cmdstat_usec_per_call
redis_cmdstat_failed_calls
redis_cmdstat_rejected_calls
```

它们会带有 `command` 标签，比如 `get`、`set`、`hget`。排查 Redis 压力时，命令统计往往比单纯看 QPS 更有价值，因为它能告诉你是哪类命令消耗更高。

## 6. 慢查询和自定义命令

Redis 慢查询只有在配置开启后才采集：

```toml
gather_slowlog = true
```

开启后会生成类似指标：

```promql
redis_slow_log
```

慢查询指标通常带有 `client_addr`、`client_name`、`log_id`、`cmd` 等标签。这里要注意标签基数，尤其是命令内容、客户端地址很多时，不要让慢查询采集制造过高的时序压力。

Redis 插件也支持自定义命令结果：

```text
redis_exec_result_<metric>
```

自定义命令适合少量、稳定、可转换为数值的结果。不建议把复杂业务查询、返回大量内容的命令放到监控采集中执行。

## 7. 导入 Dashboard

Redis 插件目录下提供了夜莺和 Grafana Dashboard：

```text
inputs/redis/dashboard.json
inputs/redis/dashboard_grafana.json
```

如果使用 Grafana，导入：

```text
inputs/redis/dashboard_grafana.json
```

该 Grafana Dashboard 使用 `instance` 变量筛选实例。导入后重点检查：

- `datasource` 是否选中正确数据源；
- `instance` 变量是否有值；
- `redis_up` 是否正常；
- 内存、命中率、连接、命令统计、复制状态等面板是否有数据。

如果 `instance` 变量为空，通常说明采集配置中没有设置 `labels = { instance = "..." }`，或者实际指标里的标签名和 Dashboard 查询不匹配。

下面是测试环境中使用 Categraf 采集 Redis 后导入 Grafana Dashboard 的效果：

![Categraf Redis Grafana 大盘](https://download.flashcat.cloud/categraf/categraf-grafana-redis-overview.jpg)

## 8. 告警规则怎么配

Categraf 仓库中提供了 Redis 告警规则：

```text
inputs/redis/alerts.json
```

覆盖的典型问题包括：

- Redis 节点故障；
- PING 延迟过高；
- Redis 内存使用率较高；
- 出现拒绝连接；
- Redis 刚刚重启；
- 命中率较低；
- 驱逐率较高。

导入后建议根据业务场景调整阈值。比如缓存型 Redis 和队列型 Redis，对命中率、驱逐、延迟的容忍度完全不同。

## 9. 常见问题

**Q1：`redis_up = 1` 是否代表 Redis 完全正常？**

不是。`redis_up = 1` 只代表 PING 成功。内存、连接、命中率、驱逐、复制和慢查询仍然可能异常。

**Q2：Redis Dashboard 没有实例下拉值怎么办？**

先查 `redis_up` 是否有数据，再看指标标签里是否有 `instance`。如果没有，建议在 Categraf 配置里通过 `labels` 设置稳定的 `instance` 标签。

**Q3：命中率低一定是故障吗？**

不一定。命中率要结合业务场景看。新上线、缓存预热、短 TTL、访问模式变化都会导致命中率下降。

**Q4：慢查询采集是否应该默认开启？**

不一定。慢查询指标可能带来较多标签，需要评估 Redis 慢日志规模和标签基数。建议先在重点实例上验证，再决定是否推广。

## 10. 小结

Categraf Redis 监控的落地重点是：先保证基础采集稳定，再围绕业务风险补齐关键指标。

推荐顺序：

1. 配置 Redis 实例和稳定 `instance` 标签；
2. 查询 `redis_up`、`redis_ping_use_seconds`、`redis_scrape_use_seconds`；
3. 导入 Redis Grafana 或夜莺 Dashboard；
4. 重点观察内存、命中率、拒绝连接、驱逐、复制和命令统计；
5. 根据业务场景决定是否开启慢查询和自定义命令；
6. 导入告警规则并调整阈值。

Redis 的问题通常不是单一指标能解释的。把存活、延迟、内存、命中率、命令统计和业务访问模式放在一起看，才更容易判断问题来源。
