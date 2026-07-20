---
title: "Categraf 是什么：一篇文章讲清楚它适合什么场景"
description: "Categraf 是一款开源的 All-in-One 监控数据采集器，支持主机、中间件、数据库、Kubernetes、网络设备等多种监控对象，兼容 Prometheus 生态，并提供夜莺和 Grafana Dashboard。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "监控采集器", "Nightingale", "Prometheus", "VictoriaMetrics", "Grafana", "可观测性"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "Monitoring", "Observability"]
---

做监控系统，很多人一开始关注的是服务端：用 Prometheus、VictoriaMetrics、夜莺、Grafana，还是其他系统。但真正落地到生产环境时，很快会遇到另一个问题：**数据到底怎么采？**

Linux 主机要采，MySQL、Redis、Kafka、Elasticsearch 要采，Kubernetes 要采，交换机、IPMI、SNMP 也要采。每个对象找一个 exporter，看起来也能跑，但时间一长，就会发现机器上 agent 越来越多，配置越来越散，升级和排障越来越麻烦。

[Categraf](https://github.com/flashcatcloud/categraf) 要解决的就是这个问题：用一个统一的采集器，尽量覆盖常见监控对象，把指标采集、日志采集、数据上报、Dashboard 和告警规则这些事情串起来，让用户更快建立监控闭环。

## 核心要点

- Categraf 是开源的 All-in-One 监控数据采集器，定位是“采集和上报”，不是时序数据库或 Dashboard 系统。
- 它适合需要统一采集 Linux、数据库、中间件、Kubernetes、网络设备等多类对象的团队。
- Categraf 兼容 Prometheus remote write，可接入夜莺、VictoriaMetrics、Prometheus 兼容存储和 Grafana。
- 仓库中的 `inputs` 目录把插件配置、README、Dashboard 和告警规则放在一起，便于复用监控实践。

## 1. Categraf 是什么

Categraf 是一款开源的 All-in-One 监控数据采集器，由快猫星云开源和维护。

一句话概括：

> Categraf 负责把各种基础设施、中间件、数据库、云原生组件和业务系统的监控数据采集起来，然后通过 Prometheus remote write 等协议写入后端时序库或监控系统。

它可以采集的对象很多，比如：

- 主机：CPU、内存、磁盘、磁盘 IO、网络、进程、系统负载等；
- 数据库和缓存：MySQL、Redis、PostgreSQL、MongoDB、Oracle 等；
- 消息队列和中间件：Kafka、RabbitMQ、Zookeeper、Elasticsearch 等；
- Kubernetes：kubelet、kube-state-metrics、kube-proxy、cadvisor 等；
- 网络和硬件：SNMP、IPMI、Redfish、交换机等；
- 自定义场景：Prometheus scrape、exec、HTTP 探测、TCP 探测、日志采集等。

如果你熟悉 Telegraf，可以把 Categraf 理解成一个更贴近 Prometheus / 夜莺生态、同时沉淀了大量监控最佳实践的采集器。

## 2. 为什么需要一个 All-in-One 采集器

Prometheus 生态里有大量 exporter，这是很宝贵的积累。但在生产环境里，完全依赖“一个对象一个 exporter”的方式，通常会带来几个问题。

第一个问题是进程数量多。

比如一台机器上既要监控 Linux，又要监控 MySQL、Redis、Nginx、业务 HTTP 探测，如果每种对象都部署一个 exporter，很快就会有一堆采集进程。进程多了以后，安装、升级、配置下发、日志排查都会变复杂。

第二个问题是配置分散。

不同 exporter 的配置方式、启动参数、标签习惯、指标命名不完全一致。刚开始能接受，规模上来之后，团队需要投入不少精力维护这些差异。

第三个问题是缺少统一的 Dashboard 和告警沉淀。

采到指标只是第一步。真正让用户觉得“监控可用”，还需要能看到大盘，知道哪些指标重要，能配置告警。很多 exporter 只解决采集问题，后面的 Dashboard、告警、指标解释仍然要用户自己补齐。

Categraf 的思路是：常见采集能力尽量收敛到一个 agent 里，同时在仓库中配套维护采集说明、Dashboard 和告警规则，让用户少做重复劳动。

## 3. Categraf 和 Prometheus、VictoriaMetrics、夜莺、Grafana 是什么关系

Categraf 是采集器，不是完整的监控服务端。

一个典型链路是这样的：

```text
Linux / MySQL / Redis / Kafka / Kubernetes
        |
        v
    Categraf
        |
        v
Prometheus remote write
        |
        v
Nightingale / VictoriaMetrics / Prometheus compatible storage
        |
        v
Dashboard / Alerting / OnCall
```

如果使用夜莺，Categraf 可以把指标写到夜莺后端，并通过夜莺查看指标、导入 Dashboard、配置告警。

如果使用 VictoriaMetrics，Categraf 可以通过 remote write 把指标写入 VictoriaMetrics，再用夜莺或 Grafana 做查询和展示。

如果团队已经习惯 Grafana，也可以导入 Categraf 仓库中提供的 Grafana Dashboard，直接基于 Prometheus 兼容数据源查看监控图表。

![Categraf Linux 主机 Grafana 大盘概览](https://download.flashcat.cloud/categraf/categraf-grafana-linux-overview.jpg)

所以，Categraf 并不要求用户放弃现有监控生态。它更像是监控体系里的“数据入口”，负责把数据稳定、规范地采上来。

## 4. Categraf 适合哪些场景

如果你的环境里只有很少几个服务，已有 exporter 也运行得很好，Categraf 不一定是必须的。

但如果你遇到下面这些情况，Categraf 会比较适合：

- 机器数量较多，希望减少 agent 和 exporter 的维护成本；
- 同时监控主机、数据库、中间件、Kubernetes、网络设备等多类对象；
- 希望用统一方式管理采集配置、标签和数据上报；
- 希望采集后直接有 Dashboard 和告警规则可以参考；
- 后端使用夜莺、VictoriaMetrics、Prometheus 兼容存储或 Grafana；
- 环境横跨物理机、虚拟机、Kubernetes、混合云或多云；
- 团队希望把监控最佳实践沉淀成标准配置，而不是每个项目各自摸索。

一个常见例子是中小规模平台团队：既要管 Linux 主机，又要管 MySQL、Redis、Kafka、Elasticsearch，还要管 Kubernetes。此时如果每类对象都维护独立 exporter，配置和升级成本会越来越高。用 Categraf 收敛后，部署和排障路径会更统一。

## 5. Categraf 的核心特点

Categraf 的特点可以概括成几个关键词。

**统一采集**

主机、中间件、数据库、Kubernetes、网络设备等监控对象，尽量用一个 agent 完成采集。这样部署模型更简单，运维人员排障时也不用在多个 exporter 之间来回切换。

**兼容 Prometheus 生态**

Categraf 支持 remote write，可以把指标写入夜莺、VictoriaMetrics、Prometheus 兼容存储等后端。对于已经使用 PromQL、Grafana、VictoriaMetrics 的团队，接入成本比较低。

**内置最佳实践**

采集器不应该把所有能采的数据都无脑上报。高基数标签、无意义指标、字符串字段等问题，都会给后端时序库带来压力。Categraf 的设计倾向是在采集侧就做一些约束和整理，减少用户踩坑。

**配套 Dashboard 和告警**

Categraf 的 `inputs` 目录下，很多插件都配套了 Dashboard 和告警规则。用户不是只拿到一堆指标名，而是可以直接导入夜莺或 Grafana 看图，再根据自己的环境调整。

**适合开源共建**

每个 input 插件都有独立目录，代码、README、Dashboard、告警规则都放在一起。社区贡献时，可以围绕一个具体监控对象持续完善采集说明、指标解释、大盘和告警。

## 6. Categraf 不是什么

理解一个工具，也要知道它不解决什么问题。

Categraf 不是时序数据库。它不会长期存储监控指标，指标需要写到夜莺后端、VictoriaMetrics、Prometheus 兼容存储等系统中。

Categraf 不是 Dashboard 系统。它提供 Dashboard JSON，但实际展示需要夜莺或 Grafana。

Categraf 也不是 OnCall 平台。它可以采集指标并辅助告警配置，但告警通知、认领、升级、排班等流程，通常需要夜莺、FlashDuty 或其他告警协同系统来完成。

把边界说清楚，架构就很容易理解：Categraf 负责采集和上报，后端系统负责存储、查询、展示、告警和协同。

## 7. 从哪里开始使用

如果你是第一次接触 Categraf，建议不要一开始就研究所有配置项。最好的方式是先跑通一个最小闭环。

推荐路径：

1. 部署夜莺或 VictoriaMetrics 作为后端；
2. 部署 Categraf；
3. 先开启 Linux 主机基础监控；
4. 用 `--test` 模式确认本机指标能采到；
5. 确认后端能查到 `system_load1`、`cpu_usage_idle`、`mem_used_percent` 等指标；
6. 导入 Linux 主机 Dashboard；
7. 再逐步接入 MySQL、Redis、Kafka、Kubernetes 等插件。

比如本地验证采集数据，可以使用：

```shell
./categraf --test --inputs system:cpu:mem:disk
```

如果命令行能打印出指标，说明采集侧基本正常。后续重点就是检查 writer 配置、后端写入地址和 Dashboard 导入。

## 8. 常见问题

**Q1：Categraf 和 exporter 是什么关系？**

Categraf 可以理解为把很多常见 exporter 能力收敛到一个 agent 里，同时保留 Prometheus 生态的上报方式。它不排斥 exporter，生产环境也可以让 Categraf 和已有 exporter 共存。

**Q2：Categraf 必须和夜莺一起使用吗？**

不是。Categraf 可以通过 remote write 写入 Prometheus 兼容后端，也可以配合 Grafana 使用。夜莺提供的是监控页面、Dashboard、告警和协同能力。

**Q3：第一次使用 Categraf 应该先监控什么？**

建议先从 Linux 主机基础监控开始，确认 CPU、内存、磁盘、网络、系统负载这些指标能采集、写入和展示，再逐步接入 MySQL、Redis、Kafka、Kubernetes 等插件。

## 9. 后续文章会讲什么

这篇文章先讲清楚 Categraf 是什么、为什么需要它、适合什么场景。

后续我们会继续围绕实战展开：

- [10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics](/blog/quickstart-categraf-nightingale-victoriametrics/)
- [Categraf Linux 主机监控实战：配置、指标和大盘](/blog/linux-host-monitoring-by-categraf/)
- [Categraf Grafana Dashboard 使用指南](/blog/categraf-grafana-dashboard-guide/)
- [Categraf MySQL 监控实战：配置、指标、大盘和告警](/blog/categraf-mysql-monitoring-practice/)
- [Categraf Redis 监控实战：配置、指标、大盘和告警](/blog/categraf-redis-monitoring-practice/)
- [Categraf HTTP 响应监控实战：可用性、状态码、延迟和证书](/blog/categraf-http-response-monitoring-practice/)
- [Categraf TCP/UDP 网络探测实战：端口连通性、响应时间和告警](/blog/categraf-net-response-monitoring-practice/)
- [Categraf DNS 查询监控实战：解析结果、解析耗时和可用性](/blog/categraf-dns-query-monitoring-practice/)
- [MongoDB Docker 测试环境搭建：单节点、副本集和分片集群](/blog/mongodb-docker-lab-for-categraf/)
- [Categraf MongoDB 监控实战：单节点、副本集、分片集群的采集配置和大盘](/blog/categraf-mongodb-monitoring-practice/)
- [Categraf PostgreSQL 监控实战：连接、事务、缓存、锁和慢查询指标](/blog/categraf-postgresql-monitoring-practice/)
- [Categraf 配置文件结构详解：global、writer、heartbeat、inputs 与四类 Agent](/blog/categraf-configuration-structure-guide/)
- [Categraf 启动失败排查：TOML、配置路径、权限和 systemd 常见错误](/blog/categraf-startup-failure-troubleshooting/)
- [Categraf 插件没有指标：从 test 模式到数据库权限的完整排查](/blog/categraf-plugin-no-metrics-troubleshooting/)
- [Categraf 已采到指标但后端查不到：remote write 链路排查](/blog/categraf-remote-write-troubleshooting/)
- [Categraf Dashboard 没有数据：数据源、变量、标签和 PromQL 排查](/blog/categraf-dashboard-no-data-troubleshooting/)

监控系统的价值，不在于组件名字有多复杂，而在于故障发生时能不能快速看到现象、定位原因、通知到人。Categraf 的目标就是把“采集”这件事做得更统一、更可维护，并且和 Dashboard、告警规则一起形成可复用的监控实践。
