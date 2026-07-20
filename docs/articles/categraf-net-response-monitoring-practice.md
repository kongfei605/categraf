---
title: "Categraf TCP/UDP 网络探测实战：端口连通性、响应时间和告警"
description: "本文介绍如何使用 Categraf net_response 插件做 TCP/UDP 网络探测，包括端口连通性、响应匹配、结果码、响应时间、Dashboard 和告警建议。"
image: "https://download.flashcat.cloud/categraf/categraf-n9e-net-response-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-net-response-dashboard.jpg"
keywords: ["Categraf", "net_response", "TCP监控", "UDP监控", "端口监控", "网络探测", "Nightingale", "Grafana"]
author: "快猫星云"
date: "2026-07-03T00:00:00+08:00"
tags: ["Categraf", "Network", "Monitoring"]
---

很多基础设施故障最早表现为“端口连不上”：数据库端口被防火墙拦截、Redis 监听异常、内部服务端口没有起来、跨机房链路抖动、UDP 服务没有响应。此时只看主机 CPU、内存、进程状态是不够的，需要从调用方视角做网络探测。

Categraf 的 `net_response` 插件用于 TCP/UDP 探测。它可以检查本机端口是否监听，也可以检查远端端口是否可达；如果协议允许，还可以发送一段字符串并校验返回内容。

## 核心要点

- `net_response` 适合做 TCP/UDP 端口级黑盒探测。
- 最核心的指标是 `net_response_result_code` 和 `net_response_response_time`。
- TCP 探测可以只检查连接是否成功，也可以发送字符串并匹配响应。
- UDP 探测通常需要配置 `send` 和 `expect`，否则很难判断服务是否真的可用。
- Dashboard 没数据时，先查 `net_response_result_code`，再检查 `target`、`protocol`、`source` 等标签。

## 1. net_response 解决什么问题

`net_response` 关注的是网络层和协议入口层的可达性，适合这些场景：

- 远端 TCP 端口是否能连通；
- 本机进程是否监听指定端口；
- Redis、MySQL、SSH、网关、内部 RPC 端口是否可达；
- UDP 服务是否能收到请求并返回预期内容；
- 跨机房、跨网段、跨安全组访问是否正常；
- 探测点到目标端口的连接耗时是否升高。

它和 `http_response` 的边界很清晰：如果目标是 HTTP/HTTPS URL，优先用 `http_response`；如果目标只是主机端口、TCP/UDP 协议或没有 HTTP 语义，使用 `net_response` 更直接。

## 2. 最小配置

插件配置通常放在：

```text
conf/input.net_response/net_response.toml
```

最小 TCP 探测配置如下：

```toml
[[instances]]
targets = [
    "10.23.25.10:3306",
    "10.23.25.11:6379",
    "localhost:22",
    ":9090"
]
protocol = "tcp"
timeout = "1s"
labels = { region = "cn", product = "core" }
```

这里的目标含义是：

- `10.23.25.10:3306`：探测远端主机的 MySQL 端口；
- `10.23.25.11:6379`：探测远端 Redis 端口；
- `localhost:22`：探测本机 SSH 端口；
- `:9090`：省略主机时，会转换为 `localhost:9090`。

如果不同目标属于不同业务，建议拆成多个 `[[instances]]`，或者使用 `mappings` 给目标追加标签。

## 3. 使用 mappings 补充业务标签

告警里只有 IP 和端口时，值班人员往往还要再查 CMDB 才知道影响哪个业务。`net_response` 支持给目标追加标签：

```toml
[mappings]
"10.23.25.10:3306" = { service = "order-mysql", env = "prod" }
"10.23.25.11:6379" = { service = "order-redis", env = "prod" }

[[instances]]
targets = [
    "10.23.25.10:3306",
    "10.23.25.11:6379"
]
protocol = "tcp"
labels = { region = "cn" }
```

这样告警事件和时序数据里会带上 `service`、`env`、`region` 等标签，更容易路由通知和定位责任方。

## 4. TCP 响应匹配

默认 TCP 探测只判断连接是否成功。如果目标协议支持简单交互，可以配置发送内容和期望返回：

```toml
[[instances]]
targets = [
    "10.23.25.20:12345"
]
protocol = "tcp"
timeout = "1s"
read_timeout = "1s"
send = "ping"
expect = "pong"
labels = { service = "internal-rpc" }
```

这种方式适合非常简单、稳定的协议探测。不要把复杂业务请求放在这里做，也不要发送会改变服务状态的命令。监控探测应该尽量只读、轻量、可重复。

## 5. UDP 探测

UDP 没有连接语义，单纯“发出去”不能证明服务可用。因此 UDP 探测通常需要配置 `send` 和 `expect`：

```toml
[[instances]]
targets = [
    "10.23.25.30:8125"
]
protocol = "udp"
timeout = "1s"
read_timeout = "1s"
send = "ping"
expect = "pong"
labels = { service = "udp-health" }
```

如果 UDP 服务本身不会返回响应，就不适合用 `net_response` 判断业务可用性。此时更合适的做法是从服务端暴露指标，或者增加一个专门的健康检查接口。

## 6. 结果码怎么理解

`net_response_result_code` 是探测结果码：

| 结果码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 超时 |
| `2` | 连接失败 |
| `3` | 读取失败 |
| `4` | 返回内容不匹配 |

最常用的可用性查询是：

```promql
net_response_result_code
```

基础告警可以这样写：

```promql
net_response_result_code != 0
```

如果要看连接耗时：

```promql
net_response_response_time
```

该指标单位是秒。失败时通常会是 `-1`，表示这次探测没有得到有效响应时间。

## 7. 导入 Dashboard

`net_response` 插件目录下提供了夜莺和 Grafana Dashboard：

```text
inputs/net_response/dashboard.json
inputs/net_response/dashboard_grafana.json
```

另外还提供了一组按 `ident` 变量查看的 Dashboard：

```text
inputs/net_response/dashboard-by-ziv.json
inputs/net_response/dashboard-by-ziv_grafana.json
```

使用夜莺时可以先导入 `dashboard.json`，使用 Grafana 时导入 `dashboard_grafana.json`。导入后重点检查：

- 数据源是否正确；
- `net_response_result_code` 是否有数据；
- `target` 标签是否和图表查询一致；
- 如果使用按 `ident` 过滤的大盘，确认采集指标里确实有 `ident` 标签；
- 探测失败的目标是否能在表格或趋势图中看到。

下面是测试环境中使用 Categraf 采集 TCP 端口探测指标后导入夜莺 Dashboard 的效果：

![Categraf Net Response 夜莺大盘](https://download.flashcat.cloud/categraf/categraf-n9e-net-response-dashboard.jpg)

这个大盘把所有 Target 的健康状态、响应时间、当前明细和单个 Target 趋势放在一起，适合快速确认端口连通性和网络抖动。

## 8. 告警规则怎么配

Categraf 仓库中提供了告警规则：

```text
inputs/net_response/alerts.json
```

内置规则核心 PromQL 是：

```promql
net_response_result_code != 0
```

生产环境建议按目标类型设置不同策略：

- 核心数据库、缓存、网关端口：持续 1 分钟失败即可告警；
- 后台任务、低频内部服务端口：持续 3 到 5 分钟失败再告警；
- 跨地域链路：结合 `source`、`region` 或探测点标签，避免单点网络抖动造成误报；
- UDP 探测：只有在服务明确会返回可匹配响应时再配置告警。

如果目标端口本身有主备或多副本，告警表达式可以按业务维度聚合，不要只盯单个端口。例如多个探测点同时失败时再升级告警，可以减少网络局部抖动带来的噪音。

## 9. 常见问题

**端口能 telnet 通，但 net_response 失败？**

先确认 Categraf 所在机器和你手工测试的机器是不是同一个网络位置。再检查 `protocol`、`timeout`、`send`、`expect` 配置。如果配置了响应匹配，连接成功但返回内容不匹配也会被判定失败。

**为什么 response_time 是 -1？**

探测失败、超时、连接失败或读取失败时，插件没有有效耗时结果，`net_response_response_time` 可能为 `-1`。排查时应该先看 `net_response_result_code`。

**UDP 不返回内容，能不能监控？**

不建议用 `net_response` 判断这种 UDP 服务是否可用。UDP 无连接，服务不响应时，探测端很难知道请求是否被正确处理。更可靠的方式是让服务暴露指标或提供健康检查响应。

**Dashboard 变量为空怎么办？**

先查：

```promql
net_response_result_code
```

确认真实指标里有哪些标签。如果大盘按 `target` 过滤，而你的数据没有对应标签，需要调整 Dashboard 查询或采集标签。

## 10. 生产建议

网络探测不是越多越好。建议把它用在关键依赖和关键路径上：

- 应用到数据库、缓存、消息队列的端口；
- 跨网段、跨机房、跨云网络访问；
- 网关、负载均衡、内部 RPC 入口；
- 需要从调用方视角确认可达性的关键服务。

落地时可以按这个顺序推进：

1. 先做 TCP 端口连通性探测；
2. 给每个目标补充 `service`、`env`、`region` 等标签；
3. 导入 Dashboard，确认结果码和响应时间有数据；
4. 再为少量协议稳定的目标增加 `send` / `expect`；
5. 最后按业务等级配置告警持续时间和通知路由。

这样既能及时发现网络和端口问题，又不会把探测系统变成新的告警噪音来源。
