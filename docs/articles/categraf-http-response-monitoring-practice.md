---
title: "Categraf HTTP 响应监控实战：可用性、状态码、延迟和证书"
description: "本文介绍如何使用 Categraf http_response 插件做 HTTP/HTTPS 黑盒探测，包括探测配置、结果码、状态码、分阶段耗时、HTTPS 证书过期时间、Dashboard 和告警建议。"
image: "https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg"
keywords: ["Categraf", "http_response", "HTTP监控", "HTTPS监控", "接口可用性", "证书监控", "Nightingale", "Grafana"]
author: "快猫星云"
date: "2026-07-03T00:00:00+08:00"
tags: ["Categraf", "HTTP", "Monitoring"]
---

应用监控不能只看进程是否存在，也不能只看端口是否打开。很多线上故障的表现是：服务进程还在，端口也能连上，但 HTTP 状态码异常、响应内容不符合预期、接口耗时升高，或者 HTTPS 证书即将过期。

Categraf 的 `http_response` 插件就是为这类场景准备的。它从外部发起 HTTP/HTTPS 请求，把可用性、状态码、响应耗时、DNS/TCP/TLS/首包阶段耗时和证书过期时间整理成指标，再写入夜莺、VictoriaMetrics 或其他 Prometheus 兼容后端。

## 核心要点

- `http_response` 适合做 HTTP/HTTPS 黑盒探测，重点回答“这个 URL 现在能不能正常访问”。
- 最基础的指标是 `http_response_result_code`、`http_response_response_code` 和 `http_response_response_time_ms`。
- HTTPS 目标会额外输出 `http_response_cert_expire_timestamp`，可以用于证书过期告警。
- 如果需要判断业务是否真的正常，可以配置期望状态码、响应正文子串或正则表达式。
- Dashboard 没数据时，先查 `http_response_result_code`，再看 `target` 标签是否和大盘变量一致。

## 1. http_response 解决什么问题

HTTP 探测通常用于这些场景：

- 对外 API、健康检查接口、登录页、网关入口的可用性监控；
- 业务接口是否返回预期状态码；
- 响应内容是否包含期望字符串；
- 接口响应时间是否持续升高；
- DNS、TCP 建连、TLS 握手、首包等待哪个阶段变慢；
- HTTPS 证书还有多少天过期。

它和应用内部指标是互补关系。应用内部指标能告诉你服务内部状态，HTTP 黑盒探测能告诉你“从探测点看过去，这个入口是否真的可用”。

## 2. 最小配置

插件配置通常放在：

```text
conf/input.http_response/http_response.toml
```

最小配置如下：

```toml
[[instances]]
targets = [
    "https://example.com/api/health",
    "https://example.com/login"
]
method = "GET"
response_timeout = "5s"
labels = { region = "cn", product = "portal" }
```

如果不同目标的请求方法、超时时间、Header 或校验规则不同，建议拆成多个 `[[instances]]`：

```toml
[[instances]]
targets = [
    "https://example.com/api/health"
]
method = "GET"
expect_response_status_codes = "200"
expect_response_substring = "ok"

[[instances]]
targets = [
    "https://example.com/api/orders/_health"
]
method = "POST"
body = '''
{"check": true}
'''
headers = ["Content-Type", "application/json"]
expect_response_status_codes = "200|204"
```

如果接口需要认证，建议通过配置管理系统下发凭据，文章、工单和截图中不要出现真实值。探测类配置里可以使用 Basic Auth、Header 或代理等能力，但生产环境要把凭据和普通配置分开管理。

## 3. 结果码怎么理解

`http_response_result_code` 是探测结果码，表示 Categraf 对这次请求的判断：

| 结果码 | 含义 |
| --- | --- |
| `0` | 成功 |
| `1` | 连接失败 |
| `2` | 超时 |
| `3` | DNS 解析失败 |
| `4` | 地址错误 |
| `5` | 响应内容不匹配 |
| `6` | HTTP 状态码不匹配 |

生产告警通常先从这个指标开始：

```promql
http_response_result_code != 0
```

如果只是单纯关心 HTTP 状态码，可以看：

```promql
http_response_response_code
```

两者的区别是：`response_code` 是服务端返回的 HTTP 状态码，`result_code` 是探测插件综合网络、超时、状态码和内容校验之后给出的结果。

## 4. 延迟指标怎么看

最常用的整体耗时指标是：

```promql
http_response_response_time_ms
http_response_response_time
http_response_total_cost
```

其中 `http_response_response_time_ms` 和 `http_response_total_cost` 单位是毫秒，`http_response_response_time` 单位是秒。

如果需要定位慢在哪个阶段，可以看分阶段耗时：

```promql
http_response_dns_request
http_response_tcp_connect
http_response_tls_handshake
http_response_first_byte
```

这些指标分别对应 DNS 解析、TCP 建连、TLS 握手和首包等待时间。排障时可以按下面的思路判断：

- DNS 耗时升高：检查域名解析、DNS 服务、跨网络解析路径；
- TCP 建连耗时升高：检查网络质量、目标端口、负载均衡和防火墙；
- TLS 握手耗时升高：检查证书链、TLS 配置、目标服务负载；
- 首包耗时升高：更可能是后端应用处理慢、数据库慢或队列堆积。

使用 IP 直连、连接复用或非 HTTPS 请求时，部分阶段耗时可能为 `-1`，这表示该阶段没有采到有效值，不要直接当作异常。

## 5. HTTPS 证书监控

HTTPS 目标成功建立 TLS 连接时，会输出：

```promql
http_response_cert_expire_timestamp
```

这个指标是证书过期时间戳。通常会转换成剩余天数：

```promql
(http_response_cert_expire_timestamp - time()) / 86400
```

告警可以从 7 天或 14 天开始，具体阈值取决于证书签发和变更流程。如果证书轮换需要走审批、灰度或多环境发布，建议提前量更长。

## 6. 导入 Dashboard

`http_response` 插件目录下提供了夜莺和 Grafana Dashboard：

```text
inputs/http_response/dashboard.json
inputs/http_response/dashboard_grafana.json
```

使用夜莺时导入 `dashboard.json`；使用 Grafana 时导入 `dashboard_grafana.json`。

导入后重点检查：

- 数据源是否选中正确；
- `target` 变量是否有值；
- 健康状态是否为 `UP`；
- 状态码是否符合预期；
- 响应时间是否在可接受范围；
- HTTPS 证书剩余天数是否正常。

下面是测试环境中使用 Categraf 采集 HTTP 探测指标后导入夜莺 Dashboard 的效果：

![Categraf HTTP Response 夜莺大盘](https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg)

这个大盘把所有 Target 的健康状态、状态码、响应时间、证书剩余天数和单个 Target 的趋势放在一起，适合值班人员快速判断入口是否正常。

## 7. 告警规则怎么配

Categraf 仓库中提供了告警规则：

```text
inputs/http_response/alerts.json
```

内置规则覆盖两个典型风险：

- HTTP 地址探测失败；
- HTTPS 证书即将在一周内过期。

基础告警可以这样写：

```promql
http_response_result_code != 0
```

证书告警可以这样写：

```promql
(http_response_cert_expire_timestamp - time()) / 86400 <= 7
```

生产环境建议按业务重要性分层：

- 核心入口：持续 1 分钟失败就告警；
- 普通后台接口：持续 3 到 5 分钟失败再告警；
- 跨地域探测：结合探测点标签，避免单个探测点网络抖动造成误报；
- 证书过期：至少提前 7 天告警，重要域名建议提前 14 到 30 天。

## 8. 常见问题

**Dashboard 没有数据怎么办？**

先查：

```promql
http_response_result_code
```

如果没有结果，说明采集链路或写入链路还没打通。再检查 Categraf 日志、writer 地址、插件是否启用，以及 `targets` 是否配置正确。

**状态码是 200，但 result_code 不是 0？**

通常是配置了响应内容校验或期望状态码，但实际响应不匹配。检查 `expect_response_substring`、`expect_response_regular_expression`、`expect_response_status_code` 和 `expect_response_status_codes`。

**为什么证书指标没有数据？**

`http_response_cert_expire_timestamp` 只在 HTTPS 目标且成功建立 TLS 连接时输出。HTTP 目标、TLS 握手失败、目标不可达时都可能没有这个指标。

**分阶段耗时为什么是 -1？**

部分阶段没有发生或没有采到有效时间时会是 `-1`。比如非 HTTPS 请求没有 TLS 握手，连接复用时 TCP 建连阶段也可能没有新的耗时。

## 9. 生产建议

HTTP 探测要少而关键。不要把所有业务接口都放进黑盒探测，优先选择入口页、健康检查接口、核心 API 和证书域名。

推荐的落地方式是：

1. 先配置核心 URL，确认 `http_response_result_code = 0`；
2. 再加入状态码和响应内容校验；
3. 导入 Dashboard，确认 `target` 变量和图表都有数据；
4. 配置探测失败和证书过期告警；
5. 根据业务等级设置不同的持续时间和通知策略。

这样既能覆盖入口可用性，又不会让探测流量和告警噪音失控。
