---
title: "Categraf Grafana Dashboard 使用指南"
description: "本文介绍如何使用 Categraf 仓库中的 Grafana Dashboard，包括 Dashboard 文件选择、数据源配置、变量选择、导入验证和常见无数据问题排查。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Grafana", "Dashboard", "VictoriaMetrics", "Prometheus", "监控大盘"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "Grafana", "Monitoring"]
---

前几篇文章讲了 [Categraf 是什么](/blog/what-is-categraf/)、如何 [10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics](/blog/quickstart-categraf-nightingale-victoriametrics/)，以及如何做 [Linux 主机监控](/blog/linux-host-monitoring-by-categraf/)。

如果你的团队已经习惯 Grafana，那么接下来最自然的问题就是：Categraf 采到的数据，怎么在 Grafana 里看？

Categraf 仓库的 `inputs` 目录中，很多插件已经配套了 Grafana Dashboard。用户不需要从零写 PromQL，也不需要先猜哪些指标重要。更推荐的方式是：先导入官方配套大盘，确认变量、数据源和核心图表都能工作，再根据自己的业务场景做二次调整。

## 核心要点

- Categraf 的 Grafana Dashboard 通常放在各插件目录下，文件名多为 `dashboard_grafana.json` 或 `*-grafana.json`。
- Grafana 数据源需要使用 Prometheus 兼容数据源，可以指向 VictoriaMetrics、Prometheus 或其他兼容查询 API 的后端。
- Dashboard 没有数据时，优先检查原始指标、数据源、变量标签和时间范围，而不是先改图表。
- 不同插件的大盘变量不同，常见变量包括 `datasource`、`ident`、`instance`、`busigroup` 等。
- 生产环境建议把官方 Dashboard 当作基线模板，后续再按业务组、环境、实例标签进行裁剪。

## 1. Dashboard 文件在哪里

Categraf 的 Dashboard 和采集插件放在一起。比如：

```text
inputs/system/dashboard_grafana.json
inputs/mysql/dashboard-by-ident_grafana.json
inputs/mysql/dashboard-by-instance_grafana.json
inputs/redis/dashboard_grafana.json
inputs/kafka/dashboard_grafana.json
inputs/elasticsearch/dashboard_grafana.json
```

这种组织方式有一个好处：你要监控什么对象，就去对应 input 目录里找 README、配置示例、Dashboard 和告警规则。

比如 Linux 主机监控大盘在：

```text
inputs/system/dashboard_grafana.json
```

MySQL 按实例查看的大盘在：

```text
inputs/mysql/dashboard-by-instance_grafana.json
```

Redis 大盘在：

```text
inputs/redis/dashboard_grafana.json
```

如果某个目录里同时存在夜莺和 Grafana 两类 Dashboard，一般可以通过文件名区分：`dashboard.json` 多用于夜莺，`dashboard_grafana.json` 或带 `_grafana` 后缀的文件用于 Grafana。

下面是测试环境中导入 Linux、MySQL、Redis 三个 Grafana Dashboard 后的列表页：

![Categraf Grafana Dashboard 列表](https://download.flashcat.cloud/categraf/categraf-grafana-dashboard-list.jpg)

## 2. Grafana 数据源怎么配置

Categraf 负责采集和上报，不直接给 Grafana 提供查询接口。Grafana 需要连到 Prometheus 兼容的数据源。

常见链路是：

```text
Categraf
  |
  v
remote write
  |
  v
VictoriaMetrics / Prometheus compatible storage
  |
  v
Grafana Dashboard
```

如果你用的是本地 VictoriaMetrics，Grafana 数据源可以配置为：

```text
http://127.0.0.1:8428
```

如果 Grafana 运行在 Docker 容器里，而 VictoriaMetrics 在宿主机上，数据源地址常见写法是：

```text
http://host.docker.internal:8428
```

这里要注意，Grafana 数据源地址是从 Grafana 所在环境访问后端的地址，不一定等于你在浏览器里访问 VictoriaMetrics 的地址。

## 3. 导入 Dashboard 的推荐顺序

导入 Dashboard 前，不建议直接从“页面有没有图”开始判断是否成功。更稳妥的顺序是：

1. 确认 Categraf 已经启动；
2. 确认后端能查到原始指标；
3. 在 Grafana 配好 Prometheus 兼容数据源；
4. 导入对应插件的 `_grafana.json` 文件；
5. 在 Dashboard 变量中选择正确的数据源和实例；
6. 查看核心面板是否有数据；
7. 再检查细分图表和 TopN 图表。

以 Linux 主机监控为例，可以先在后端查：

```promql
system_load1
mem_used_percent
100 - cpu_usage_idle{cpu="cpu-total"}
```

这些查询有数据，再导入 `inputs/system/dashboard_grafana.json`。导入后选择对应的 `datasource` 和 `ident`，通常就能看到主机概览。

![Categraf Linux 主机 Grafana 大盘概览](https://download.flashcat.cloud/categraf/categraf-grafana-linux-overview.jpg)

## 4. 常见 Dashboard 变量怎么理解

Grafana Dashboard 是否有数据，很多时候取决于变量是否选对。

Categraf 当前常见 Dashboard 变量包括：

| 变量 | 常见含义 | 排查建议 |
| --- | --- | --- |
| `datasource` | Grafana 数据源 | 先确认数据源测试通过，并且能查询 PromQL |
| `ident` | 主机标识，常用于 Linux 主机类大盘 | 查询 `system_load1` 看实际标签是否包含目标 `ident` |
| `instance` | 实例标识，常用于 MySQL、Redis 等中间件 | 建议在 Categraf 配置中通过 `labels` 设置稳定的 `instance` 标签 |
| `busigroup` | 业务组，常用于夜莺或带业务分组的大盘 | 没有业务组标签时可先选择全部或空值 |
| `region` | 区域，常见于云厂商或 RDS 类大盘 | 对照实际指标标签确认是否存在 |

如果变量下拉框没有值，通常不是 Grafana 导入失败，而是对应的变量查询没有查到标签。此时应该回到数据源里查原始指标。

## 5. MySQL 和 Redis 大盘怎么选

MySQL 插件目录下有多份 Grafana Dashboard：

```text
inputs/mysql/dashboard-by-ident_grafana.json
inputs/mysql/dashboard-by-instance_grafana.json
inputs/mysql/dashboard-by-aws-rds_grafana.json
```

如果你的 MySQL 是由 Categraf 直接采集，优先看 `dashboard-by-instance_grafana.json` 或 `dashboard-by-ident_grafana.json`。如果是 AWS RDS 场景，再看 `dashboard-by-aws-rds_grafana.json`。

Redis 插件目录下的 Grafana Dashboard 是：

```text
inputs/redis/dashboard_grafana.json
```

Redis 大盘按 `instance` 变量查看具体实例，所以配置 Categraf 时建议显式加上实例标签：

```toml
labels = { instance = "prod-redis-01:6379" }
```

这样后续在 Grafana 里筛选会更直观，也避免直接用地址作为唯一识别方式时产生混乱。

## 6. Dashboard 没有数据怎么排查

建议按下面顺序排查。

**第一步，确认时间范围**

Grafana 默认时间范围如果太短，而 Categraf 采集周期较长，可能看不到数据。可以先切到最近 1 小时或最近 6 小时。

**第二步，确认数据源**

在 Grafana Explore 里查一个最基础的指标。Linux 主机查 `system_load1`，MySQL 查 `mysql_up`，Redis 查 `redis_up`。

**第三步，确认变量标签**

如果原始指标有数据，但 Dashboard 变量没有值，说明变量查询依赖的标签和实际指标标签不匹配。比如 MySQL 大盘按 `instance` 查询，但采集配置没有设置 `labels = { instance = "..." }`。

**第四步，确认 Dashboard 版本**

同一个插件可能有多份 Dashboard。比如 MySQL 有按 `ident`、按 `instance`、按 AWS RDS 的版本，导错版本会导致变量和标签不匹配。

**第五步，确认指标名**

如果你修改过采集配置、禁用了某些模块，或者使用了其他采集器生成的指标，Dashboard 中的 PromQL 可能查不到对应指标。此时需要回到插件 README，对照指标名和采集开关。

## 7. 什么时候需要改 Dashboard

官方 Dashboard 适合作为起点，但生产环境通常还需要调整。

建议优先改这几类内容：

- 业务相关的变量，比如环境、业务组、集群、机房；
- TopN 面板的排序和过滤条件；
- 不同机器规格对应的阈值线；
- 不适合当前环境的挂载点、网卡、库表过滤；
- 团队内部常用的跳转链接或文档链接。

不建议一开始就大幅改 PromQL。先保证原始指标和变量稳定，再逐步调整图表展示，否则排查时很难判断问题来自采集、存储、变量还是 PromQL。

## 8. 常见问题

**Q1：Categraf 的 Grafana Dashboard 能直接用于 Prometheus 吗？**

可以，只要 Grafana 数据源是 Prometheus 兼容查询接口即可。VictoriaMetrics、Prometheus 以及其他兼容 PromQL 的后端都可以作为数据源。

**Q2：为什么导入后变量为空？**

最常见原因是数据源里没有对应指标，或者指标里没有 Dashboard 变量依赖的标签。先在 Explore 里查原始指标，再看标签是否包含 `ident`、`instance` 等字段。

**Q3：夜莺 Dashboard 和 Grafana Dashboard 可以混用吗？**

不能直接混用。两者都是 JSON，但格式不同。夜莺使用 `dashboard.json`，Grafana 使用 `_grafana.json` 或类似命名的文件。

**Q4：是否应该把官方 Dashboard 直接用于生产？**

可以作为生产基线，但建议先在测试环境导入验证，再根据业务标签、机器规格、告警阈值和团队排查习惯做调整。

## 9. 小结

Categraf 的 Grafana Dashboard 价值不只是“省去画图时间”，更重要的是把采集插件、指标命名、变量标签和排查路径放到一个可复用模板里。

使用时建议遵循一个简单原则：

```text
先确认原始指标，再导入 Dashboard；先选对变量，再判断图表是否正常。
```

这样可以避免把采集问题、数据源问题、变量问题和图表问题混在一起排查。

下一篇文章继续进入具体中间件场景，讲如何用 Categraf 做 MySQL 监控。
