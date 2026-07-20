---
title: "10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics"
description: "本文介绍如何使用 Docker Compose 快速启动 Categraf、夜莺和 VictoriaMetrics，完成从主机指标采集、remote write 写入、PromQL 查询到 Dashboard 展示的最小监控闭环。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Nightingale", "VictoriaMetrics", "Docker Compose", "监控入门", "Prometheus remote write"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "Nightingale", "VictoriaMetrics", "Monitoring"]
---

上一篇文章讲了 [Categraf 是什么](/blog/what-is-categraf/)。如果只看概念，很难真正理解一个采集器的价值。最好的方式还是动手跑一遍：采集一批指标，写入时序库，在页面上查到，再导入 Dashboard 看图。

## 核心要点

- 本文用 Docker Compose 跑起 Categraf、夜莺、VictoriaMetrics、MySQL 和 Redis。
- 验证顺序建议固定为：容器状态、Categraf 日志、VictoriaMetrics 指标查询、夜莺页面查询、Dashboard 展示。
- Dashboard 没有数据时，先查后端是否有 `system_load1`、`cpu_usage_idle`、`mem_used_percent` 等原始指标。
- 这套演示环境对应生产链路中的最小闭环：采集对象 -> Categraf -> remote write -> 时序库 -> Dashboard / Alerting。

本文用 Docker Compose 快速启动一套最小监控环境：

- Categraf：负责采集主机指标；
- Nightingale：负责监控页面、Dashboard、告警等能力；
- VictoriaMetrics：负责存储时序指标；
- MySQL / Redis：夜莺依赖的元数据库和缓存。

跑完这篇文章，你会得到一个完整链路：

```text
Host metrics
    |
    v
Categraf
    |
    v
Nightingale / remote write
    |
    v
VictoriaMetrics
    |
    v
Nightingale Dashboard
```

下面开工。

## 1. 准备环境

本机需要提前安装 Docker 和 Docker Compose。确认命令如下：

```shell
docker --version
docker compose version
```

如果能正常打印版本号，就可以继续。

本文推荐直接使用夜莺仓库里的 Docker Compose 示例，因为它已经把夜莺、VictoriaMetrics、Categraf 以及依赖服务放在一起了，适合快速体验。

```shell
git clone https://github.com/ccfos/nightingale.git
cd nightingale/docker/compose-bridge
docker compose up -d
```

这个 Compose 会启动几个容器：

- `mysql`
- `redis`
- `victoriametrics`
- `nightingale`
- `categraf`

可以用下面的命令确认容器状态：

```shell
docker compose ps
```

正常情况下，几个容器都应该处于 `running` 或 `up` 状态。

## 2. 打开夜莺页面

Compose 启动后，夜莺默认监听 `17000` 端口：

```text
http://127.0.0.1:17000
```

默认账号以部署包或启动日志输出为准。首次登录后，建议立即修改默认密码。

```text
用户名：root
密码：请查看启动日志或部署说明
```

登录后，可以先不用急着看 Dashboard。第一步应该先确认服务是否真的起来了、Categraf 是否已经开始采集。

## 3. 确认 Categraf 是否正常启动

查看 Categraf 日志：

```shell
docker logs -f categraf
```

如果看到类似下面的信息，说明 Categraf 已经开始运行：

```text
input: local.cpu started
input: local.mem started
input: local.disk started
input: local.system started
agent started
```

刚启动时，Categraf 可能会先等待夜莺服务可用，所以看到短暂的连接失败不用紧张。等夜莺完全启动后，Categraf 会继续运行。

## 4. 确认 VictoriaMetrics 中已经有指标

VictoriaMetrics 默认监听 `8428` 端口。我们可以直接查一个最基础的主机指标，比如 `system_load1`：

```shell
curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=system_load1'
```

如果返回里有 `success` 和一条结果，说明指标已经写入 VictoriaMetrics。

再查一下 CPU 和内存：

```shell
curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=100-cpu_usage_idle{cpu="cpu-total"}'

curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=mem_used_percent'
```

这一步很关键。很多监控问题不是 Dashboard 的问题，而是采集或写入链路没有打通。只要 VictoriaMetrics 里能查到指标，说明“采集 -> 上报 -> 存储”这条链路已经通了。

## 5. 在夜莺里查看指标

回到夜莺页面，可以在即时查询或指标查询页面里搜索：

```text
system_load1
cpu_usage_idle
mem_used_percent
```

能查到这些指标，就说明夜莺已经可以读取 VictoriaMetrics 中的数据。

此时你已经完成了最小闭环：

```text
Categraf 采集 Linux 主机指标
        |
        v
写入 VictoriaMetrics
        |
        v
夜莺查询指标
```

后面再导入 Dashboard，本质上就是把这些 PromQL 查询组织成图表。

## 6. 导入 Linux 主机 Dashboard

Categraf 仓库的 `inputs/system` 目录下提供了 Linux 主机监控大盘：

```text
inputs/system/dashboard.json
```

如果你使用夜莺，可以在夜莺页面里导入这份 Dashboard。导入后，打开大盘，选择对应的机器标识，就可以看到 CPU、内存、磁盘、网络、系统负载等基础监控图表。

如果你使用 Grafana，也可以导入对应的 Grafana 版本：

```text
inputs/system/dashboard_grafana.json
```

Grafana 中导入后，选择 Prometheus 兼容数据源即可。这里的数据源可以指向 VictoriaMetrics：

```text
http://127.0.0.1:8428
```

如果 Grafana 是通过 Docker 启动的，容器内访问宿主机 VictoriaMetrics 时，地址可能需要写成：

```text
http://host.docker.internal:8428
```

导入后，可以先看 Linux 主机概览区域，确认 CPU、内存、运行时长等基础信息已经展示出来。

![Categraf Linux 主机 Grafana 大盘概览](https://download.flashcat.cloud/categraf/categraf-grafana-linux-overview.jpg)

## 7. Categraf 配置在哪里

在 Docker Compose 示例里，Categraf 的配置会挂载到容器内的配置目录。生产环境中，Categraf 常见配置结构大致是这样：

```text
conf/
  config.toml
  input.cpu/cpu.toml
  input.mem/mem.toml
  input.disk/disk.toml
  input.diskio/diskio.toml
  input.net/net.toml
  input.system/system.toml
```

其中：

- `config.toml` 是主配置，包含全局采集频率、writer、日志、心跳等配置；
- `input.*/*.toml` 是各个插件的配置；
- writer 决定指标写到哪里；
- inputs 决定采什么。

如果你只是想验证某几个插件能不能采到数据，可以用 `--test` 模式：

```shell
./categraf --test --inputs system:cpu:mem
```

`--test` 模式只打印采集结果，不会真正上报数据，很适合排查采集配置。

## 8. 常见问题

**页面打不开**

先确认容器是否启动：

```shell
docker compose ps
```

再看夜莺日志：

```shell
docker logs -f nightingale
```

如果 MySQL 或 Redis 还没 ready，夜莺会等待一会儿。第一次启动需要初始化数据库，可能要多等几十秒。

**Categraf 日志里连接夜莺失败**

刚启动时出现短暂连接失败是正常的，因为 nightingale 容器还没完全 ready。等夜莺监听 `17000` 和 `20090` 端口后，Categraf 会继续运行。

可以确认端口：

```shell
curl -I http://127.0.0.1:17000
```

**VictoriaMetrics 查不到指标**

优先检查 Categraf 是否启动，接着检查 writer 地址是否正确。然后看 Categraf 日志里有没有写入失败、网络失败、鉴权失败等错误。

也可以先用 `--test` 模式确认采集本身没有问题。如果本地能采到，但 VictoriaMetrics 查不到，问题多半在写入链路。

**Dashboard 没有数据**

Dashboard 没有数据时，不要先怀疑 Dashboard。建议按这个顺序排查：

1. VictoriaMetrics 里能不能查到原始指标；
2. 夜莺或 Grafana 的数据源是否配置正确；
3. Dashboard 变量是否选中了正确的 `ident`、`instance`、`job` 等标签；
4. Dashboard 里的 PromQL 和实际标签是否匹配。

## 9. 下一步

到这里，你已经跑通了 Categraf + 夜莺 + VictoriaMetrics 的最小闭环。

这套环境虽然是 Docker Compose 启动的演示环境，但链路和生产环境是一致的：

```text
采集对象 -> Categraf -> remote write -> 时序库 -> Dashboard / Alerting
```

下一步建议继续做两件事：

1. 把 Linux 主机基础监控大盘导入夜莺或 Grafana，熟悉 CPU、内存、磁盘、网络等核心图表；
2. 选择一个真实中间件，比如 MySQL 或 Redis，配置对应 input 插件，跑通业务组件监控。

后续文章我们会继续讲：

- [Categraf Linux 主机监控实战：配置、指标和大盘](/blog/linux-host-monitoring-by-categraf/)
- `Categraf Grafana Dashboard 使用指南`
- `Categraf MySQL 监控实战`
- `Categraf Redis 监控实战`

先把链路跑通，再逐步增加插件和告警规则，这是使用 Categraf 最稳妥的入门方式。
