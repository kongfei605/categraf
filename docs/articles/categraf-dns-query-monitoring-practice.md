---
title: "Categraf DNS 查询监控实战：解析结果、非预期 IP、解析耗时和可用性"
description: "本文介绍如何使用 Categraf dns_query 插件做 DNS 查询质量监控，包括 DNS 服务器配置、域名解析探测、非预期 IP 校验、结果码、响应码、解析耗时、Dashboard 和告警建议。"
image: "https://download.flashcat.cloud/categraf/categraf-n9e-dns-query-ip-check-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-dns-query-ip-check-dashboard.jpg"
keywords: ["Categraf", "dns_query", "DNS监控", "DNS解析", "DNS探测", "DNS IP校验", "Nightingale", "VictoriaMetrics"]
author: "快猫星云"
date: "2026-07-09T00:00:00+08:00"
tags: ["Categraf", "DNS", "Monitoring"]
---

DNS 故障经常表现得很隐蔽：应用进程正常，端口也能连通，但域名解析变慢、偶发超时、返回 `SERVFAIL` 或解析到了非预期结果，最终表现为接口访问慢、服务发现异常、跨地域访问失败。

Categraf 的 `dns_query` 插件用于持续探测 DNS 查询质量。它可以从指定探测点访问一个或多个 DNS 服务器，定期查询关键域名，把解析耗时、探测结果码、DNS 协议响应码和可选的非预期 IP 校验结果写入夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端。

## 核心要点

- `dns_query` 适合从探测点视角监控 DNS 服务器和关键域名解析质量。
- 最核心的指标是 `dns_query_result_code`、`dns_query_query_time_ms` 和 `dns_query_rcode_value`。
- 如果配置了 `expect_query_ips`，还可以通过 `dns_query_status_change` 和 `dns_query_status_change_detail` 发现解析结果中出现的非预期 IP。
- 可以手动指定 DNS 服务器，也可以从本机 `/etc/resolv.conf` 自动发现 DNS 服务器。
- 同一个插件实例可以同时探测多个 DNS 服务器和多个域名。
- Dashboard 没数据时，先查 `dns_query_result_code`，再检查 `server`、`domain`、`record_type` 标签；非预期 IP 校验面板没有数据时，再检查是否配置了 `expect_query_ips`。

## 1. dns_query 解决什么问题

DNS 监控通常用于这些场景：

- 内网 DNS 服务器是否可用；
- 公共 DNS 或云厂商 DNS 解析是否稳定；
- 关键业务域名是否能解析成功；
- 关键业务域名是否解析到了非预期 IP；
- DNS 查询耗时是否持续升高；
- `A`、`AAAA`、`CNAME`、`MX`、`TXT` 等记录是否能正常查询；
- 多个网络区域、机房或云上 VPC 到 DNS 服务器的访问质量是否一致。

它和 `http_response`、`net_response` 是互补关系。`http_response` 关注 HTTP/HTTPS 入口是否可用，`net_response` 关注 TCP/UDP 端口是否可达，`dns_query` 则专门回答“从这个探测点看过去，域名解析是否正常”。

## 2. 最小配置

插件配置通常放在：

```text
conf/input.dns_query/dns_query.toml
```

最小可用配置如下：

```toml
[[instances]]
auto_detect_local_dns_server = false
servers = ["223.5.5.5", "114.114.114.114"]
network = "udp"
domains = ["www.baidu.com", "www.example.com"]
record_type = "A"
port = 53
timeout = 2
labels = { region = "cn", probe = "office" }
```

这里的含义是：

- `servers`：要探测的 DNS 服务器列表；
- `network`：查询协议，通常使用 `udp`，必要时可以改成 `tcp`；
- `domains`：要查询的域名列表；
- `record_type`：查询记录类型，比如 `A`、`AAAA`、`CNAME`、`MX`、`TXT`；
- `timeout`：单次查询超时时间，单位是秒；
- `labels`：追加业务标签，便于后续按区域、探测点或业务过滤。

如果 `servers` 为空，并且希望直接使用本机配置的 DNS 服务器，可以开启自动发现：

```toml
[[instances]]
auto_detect_local_dns_server = true
servers = []
domains = ["www.example.com"]
record_type = "A"
timeout = 2
```

自动发现会读取本机 `/etc/resolv.conf`。这种方式适合监控“当前机器实际使用的 DNS 解析链路”，但如果要对比多个指定 DNS 服务器，建议显式配置 `servers`。

## 3. 多记录类型怎么配置

`dns_query` 的一个 `[[instances]]` 块对应一种 `record_type`。如果需要同时探测 `A` 记录和 `CNAME` 记录，建议配置多个实例：

```toml
[[instances]]
servers = ["223.5.5.5"]
domains = ["www.example.com", "api.example.com"]
record_type = "A"
timeout = 2
labels = { record_group = "address" }

[[instances]]
servers = ["223.5.5.5"]
domains = ["cdn.example.com"]
record_type = "CNAME"
timeout = 2
labels = { record_group = "alias" }
```

当前支持的记录类型包括：

```text
A, AAAA, ANY, CNAME, MX, NS, PTR, TXT, SOA, SPF, SRV
```

如果没有配置 `record_type`，插件默认查询 `NS`。如果没有配置 `domains`，插件默认查询根域 `.` 并使用 `NS` 类型。这适合做 DNS 服务器基础可用性探测，但业务域名监控建议显式配置域名和记录类型。

## 4. 校验是否出现非预期 IP

新版 `dns_query` 支持通过 `expect_query_ips` 配置预期解析 IP 列表。这个能力适合监控核心入口域名、服务发现域名、CDN 回源域名等场景，尤其适合发现域名被错误解析到非预期地址、灰度切换未按计划生效、不同 DNS 服务器返回结果不一致等问题。

示例配置：

```toml
[[instances]]
servers = ["223.5.5.5", "114.114.114.114"]
domains = ["www.example.com", "api.example.com"]
record_type = "A"
timeout = 2

[instances.expect_query_ips]
"www.example.com" = ["93.184.216.34"]
"api.example.com" = ["192.0.2.10", "192.0.2.11"]
```

配置后，插件会提取 DNS 回答中的 `A` 或 `AAAA` 记录，并和 `expect_query_ips` 做对比：

- `dns_query_status_change = 0`：当前解析 IP 没有发现超出预期列表的地址；
- `dns_query_status_change = 1`：当前解析结果里出现了不在预期列表中的 IP；
- `dns_query_status_change_detail`：只在异常时上报，标签里会带上 `diff` 和 `ips`，分别表示异常 IP 和当前解析到的 IP 列表；
- 基础指标在能提取到 IP 时，也会带上 `ips` 标签，便于在查询结果和表格中直接看到当前解析值。

这个校验主要面向 `A` 和 `AAAA` 记录。对于 `CNAME`、`MX`、`TXT` 等不会直接提取 IP 的记录类型，不建议配置 `expect_query_ips`。

## 5. 启动与验证

修改配置后，可以先用测试模式确认插件能采到指标：

```shell
./categraf --test --inputs dns_query
```

正常情况下会看到类似这些指标：

```text
dns_query_query_time_ms{server="223.5.5.5",domain="www.example.com",record_type="A"} 12.3
dns_query_result_code{server="223.5.5.5",domain="www.example.com",record_type="A"} 0
dns_query_rcode_value{server="223.5.5.5",domain="www.example.com",record_type="A"} 0
```

如果配置了 `expect_query_ips`，还会看到类似：

```text
dns_query_status_change{server="223.5.5.5",domain="www.example.com",record_type="A"} 0
dns_query_status_change_detail{server="223.5.5.5",domain="api.example.com",record_type="A",diff="203.0.113.10",ips="203.0.113.10"} 1
```

如果测试模式没有输出，优先检查：

- `servers` 是否为空；
- `auto_detect_local_dns_server` 是否符合预期；
- Categraf 所在机器是否能访问 DNS 服务器的 53 端口；
- `record_type` 是否写成了插件支持的类型；
- `domains` 是否配置了需要探测的域名。
- 如果非预期 IP 校验没有指标，检查 `expect_query_ips` 的域名 key 是否和 `domains` 中的域名完全一致。

测试通过后，再确认 writer 配置，把指标写入夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端。

## 6. 结果码怎么理解

`dns_query_result_code` 是插件探测结果码：

| 结果码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 超时 |
| `2` | 其他错误 |

最基础的可用性告警可以这样写：

```promql
dns_query_result_code != 0
```

这个指标回答的是“这次探测过程是否成功”。比如 DNS 服务器不可达、查询超时、记录类型配置错误、服务端返回非成功响应等，都会导致结果异常。

## 7. DNS 响应码怎么看

`dns_query_rcode_value` 是 DNS 协议层面的响应码。常见值包括：

| 响应码 | 含义 |
| --- | --- |
| `0` | `NOERROR`，查询成功 |
| `2` | `SERVFAIL`，服务端失败 |
| `3` | `NXDOMAIN`，域名不存在 |
| `5` | `REFUSED`，服务器拒绝查询 |

排障时建议同时看 `result_code` 和 `rcode_value`：

- `result_code != 0` 且没有 `rcode_value`：更可能是网络不可达、超时或本地查询错误；
- `result_code != 0` 且 `rcode_value = 2`：DNS 服务器返回了 `SERVFAIL`；
- `result_code != 0` 且 `rcode_value = 3`：域名不存在或查询的记录类型不存在；
- `result_code = 0` 且 `rcode_value = 0`：查询过程和 DNS 响应都正常。

如果业务域名不应该出现 `NXDOMAIN` 或 `SERVFAIL`，可以对 `dns_query_rcode_value` 单独配置告警。

## 8. 解析耗时怎么看

`dns_query_query_time_ms` 表示 DNS 查询耗时，单位是毫秒：

```promql
dns_query_query_time_ms
```

可以按 DNS 服务器、域名和记录类型查看：

```promql
avg by (server, domain, record_type) (dns_query_query_time_ms)
```

常见排障思路：

- 单个域名变慢：检查权威 DNS、域名配置、跨地域解析链路；
- 单个 DNS 服务器变慢：检查该 DNS 服务器负载、网络链路或上游递归解析；
- 某个探测点整体变慢：检查探测点所在机房、VPC、出口网络或本机 DNS 配置；
- 所有 DNS 服务器同时变慢：检查公共网络、上游解析服务或业务域名本身的 DNS 配置。

阈值要结合网络位置设置。内网 DNS 查询通常应该很快，跨地域、公网或链路复杂的场景可以适当放宽。

## 9. 导入 Dashboard

`dns_query` 插件目录下提供了夜莺 Dashboard：

```text
inputs/dns_query/dashboard.json
```

导入后重点检查：

- 数据源是否正确；
- `dns_query_query_time_ms` 是否有数据；
- `dns_query_result_code` 是否持续为 `0`；
- 如果配置了 `expect_query_ips`，`dns_query_status_change` 是否持续为 `0`；
- 大盘变量里的 `domain`、`server` 是否能列出探测目标；
- 图例里的 `domain`、`server`、`record_type` 是否符合预期；
- 时间范围是否覆盖 Categraf 实际采集时间。

当前 Dashboard 包含这些核心面板：

- `所有域名解析状态`：按 `domain`、`server`、`record_type` 展示探测成功或失败；
- `所有域名解析耗时`：按域名和 DNS 服务器对比解析耗时；
- `DNS 响应码`：展示 DNS 协议层面的 `rcode`；
- `当前 DNS 查询明细`：把结果码、响应码和耗时整理成表格；
- `非预期 IP 状态`：展示配置了预期 IP 的域名是否出现非预期解析结果；
- `非预期 IP 明细`：当解析结果中出现非预期 IP 时，展示当前 IP 和异常 IP；
- `单域名详情`：按变量查看指定域名和 DNS 服务器的状态趋势。

下面是测试环境中使用 Categraf 采集 DNS 查询指标，并配置非预期 IP 校验后导入夜莺 Dashboard 的效果：

![Categraf DNS Query 夜莺大盘和非预期 IP 校验](https://download.flashcat.cloud/categraf/categraf-n9e-dns-query-ip-check-dashboard.jpg)

如果 Dashboard 没有数据，先在即时查询里查：

```promql
dns_query_result_code
```

如果能查到数据但 Dashboard 为空，重点检查数据源、时间范围和图表查询表达式。`解析 IP 校验` 这一组面板是可选能力，只有配置了 `expect_query_ips` 后才会有数据。

## 10. 告警规则怎么配

基础可用性告警：

```promql
dns_query_result_code != 0
```

DNS 协议响应异常告警：

```promql
dns_query_rcode_value != 0
```

解析耗时告警可以从 2 秒和 5 秒两个层级开始：

```promql
dns_query_query_time_ms > 2000
```

```promql
dns_query_query_time_ms > 5000
```

如果配置了预期 IP 列表，建议增加非预期 IP 告警：

```promql
dns_query_status_change == 1
```

排查时可以查询明细指标查看异常 IP：

```promql
dns_query_status_change_detail == 1
```

生产环境建议按业务重要性分层：

- 核心入口域名：持续 1 分钟失败即可告警；
- 普通内部域名：持续 3 到 5 分钟失败再告警；
- 公共 DNS 对比探测：按 `server` 分组，避免单个 DNS 服务器异常被聚合掩盖；
- 多探测点部署：按 `probe`、`region` 等标签区分，避免局部网络问题被误判成全局故障；
- 解析耗时：内网 DNS 和公网 DNS 使用不同阈值，不要套用同一条规则。
- 非预期 IP 校验：核心域名可以单独告警，普通域名可以先观察趋势，避免正常调度、CDN 切换或灰度发布造成噪音。

如果同一个域名配置了多个 DNS 服务器，可以用聚合表达式判断“多数 DNS 服务器都失败”再升级告警，减少单点抖动造成的噪音。

## 11. 常见问题

**Dashboard 没有数据怎么办？**

先查：

```promql
dns_query_result_code
```

如果没有结果，说明采集或写入链路还没打通。检查 Categraf 日志、writer 地址、插件配置是否启用，以及 `servers` 和 `domains` 是否配置正确。

**为什么 result_code 是 1？**

`1` 表示查询超时。常见原因是 Categraf 所在机器访问不到 DNS 服务器、53 端口被防火墙拦截、DNS 服务器负载过高，或者 `timeout` 设置过短。

**为什么 result_code 是 2？**

`2` 表示其他错误。比如 DNS 服务器返回了非成功响应码、记录类型配置不支持、域名格式异常等。此时需要结合 Categraf 日志和 `dns_query_rcode_value` 判断。

**为什么没有 query_time_ms？**

查询失败或超时时，插件可能没有有效解析耗时，只会输出结果码。排查时先看 `dns_query_result_code`，确认探测过程成功后再分析耗时趋势。

**为什么没有 status_change？**

`dns_query_status_change` 只有在当前实例配置了 `expect_query_ips`，并且域名 key 命中 `domains` 中的域名时才会上报。这个能力主要用于 `A` 和 `AAAA` 记录；如果查询的是 `CNAME`、`MX`、`TXT` 等记录，不会提取 IP 做校验。

**为什么 status_change_detail 为空？**

`dns_query_status_change_detail` 只在发现非预期 IP 时上报。正常情况下只看 `dns_query_status_change = 0` 即可；出现 `1` 时，再用明细指标查看 `diff` 和 `ips` 标签。

**UDP 和 TCP 应该选哪个？**

大多数 DNS 查询使用 UDP 即可。如果需要验证 TCP 查询能力，或排查大响应包、网络策略、DNS 服务器 TCP 监听问题，可以把 `network` 改成 `tcp` 单独配置一组实例。

## 12. 生产建议

DNS 监控不要只放在 DNS 服务器本机。更有价值的做法，是在核心机房、云上 VPC、Kubernetes 节点、出口网关或业务调用方附近部署探测点，从真实访问路径观察解析质量。

域名选择也要分层：既要探测公共域名确认基础解析链路，也要探测自己的核心业务域名、服务发现域名、第三方依赖域名。对于核心入口，建议同时探测多个 DNS 服务器，并为稳定入口配置 `expect_query_ips`，保留 `server`、`domain`、`record_type`、`region`、`probe`、`ips` 等标签，方便故障时快速判断是某个域名异常、某个 DNS 服务器异常、解析结果出现非预期 IP，还是某个网络区域异常。
