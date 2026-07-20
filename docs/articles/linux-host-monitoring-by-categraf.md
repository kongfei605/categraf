---
title: "Categraf Linux 主机监控实战：配置、指标和大盘"
description: "本文介绍如何使用 Categraf 采集 Linux 主机基础监控指标，包括 CPU、内存、磁盘、磁盘 IO、网络、系统负载和进程数，并导入夜莺或 Grafana Dashboard 完成主机监控闭环。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Linux监控", "主机监控", "CPU监控", "内存监控", "磁盘监控", "Grafana", "Nightingale"]
author: "快猫星云"
date: "2026-06-30T00:00:00+08:00"
tags: ["Categraf", "Linux", "Monitoring"]
---

前两篇文章分别讲了 [Categraf 是什么](/blog/what-is-categraf/)，以及如何 [10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics](/blog/quickstart-categraf-nightingale-victoriametrics/)。这篇开始进入具体监控场景。

主机监控是所有监控体系的地基。无论上层跑的是 MySQL、Redis、Kafka，还是 Kubernetes，最后都绕不开 CPU、内存、磁盘、网络、系统负载这些基础指标。

本文介绍如何用 Categraf 做 Linux 主机监控，重点包括：

- 应该开启哪些 input 插件；
- 常用配置项怎么理解；
- 核心指标怎么看；
- 如何导入夜莺和 Grafana 大盘；
- Dashboard 没有数据时怎么排查。

## 核心要点

- Linux 主机监控建议至少覆盖 CPU、内存、磁盘空间、磁盘 IO、网络、系统负载和进程状态。
- Categraf 推荐先用 `--test` 模式验证本机能采到指标，再检查 remote write 和后端查询。
- 大盘没有数据时，优先排查原始指标、数据源、变量标签和过滤条件，而不是直接修改 Dashboard。
- 磁盘挂载点和网卡名称最容易产生环境差异，生产环境需要结合容器、Kubernetes 和虚拟网卡情况调整过滤规则。

## 1. Linux 主机监控需要采什么

一台 Linux 主机最基础的监控，通常至少要覆盖这几类：

- CPU：使用率、iowait、system、user、steal 等；
- 内存：使用率、cache、buffer、swap 等；
- 磁盘空间：分区使用率、inode 使用率；
- 磁盘 IO：读写 IOPS、吞吐、IO 等待、util；
- 网络：网卡流量、丢包、错误包；
- 系统负载：load1、load5、load15、uptime；
- 进程和内核：进程数、上下文切换、中断数、文件句柄等。

Categraf 里这些能力不是放在一个巨大插件里，而是拆成多个 input 插件。这样做的好处是配置更清晰，也便于按需启用或禁用。

常见组合如下：

```text
cpu
mem
disk
diskio
net
netstat
system
kernel
processes
linux_sysctl_fs
```

如果只是快速验证，先开启 `cpu`、`mem`、`disk`、`diskio`、`net`、`system` 就够了。生产环境建议把 `netstat`、`kernel`、`processes`、`linux_sysctl_fs` 也一起采集，Dashboard 和告警规则会更完整。

## 2. 配置文件在哪里

Categraf 的配置目录通常是 `conf`。Linux 主机监控相关配置文件大致在这些路径：

```text
conf/config.toml
conf/input.cpu/cpu.toml
conf/input.mem/mem.toml
conf/input.disk/disk.toml
conf/input.diskio/diskio.toml
conf/input.net/net.toml
conf/input.netstat/netstat.toml
conf/input.system/system.toml
conf/input.kernel/kernel.toml
conf/input.processes/processes.toml
conf/input.linux_sysctl_fs/linux_sysctl_fs.toml
```

`config.toml` 是主配置，主要关心两类配置：

- 全局采集频率；
- 指标写入地址，也就是 writer。

input 插件目录里的 toml 文件负责具体采什么、怎么采。

比如 CPU 插件配置很简单：

```toml
# # collect interval
# interval = 15

# # whether collect per cpu
# collect_per_cpu = false
```

默认情况下，CPU 插件会采集整机汇总指标。如果开启 `collect_per_cpu = true`，会额外采集每个 CPU 核心的指标。对大多数主机监控场景来说，先看整机汇总就够了；只有排查 CPU 核心不均衡时，才需要打开每核采集。

## 3. 磁盘和网卡配置要特别注意

Linux 主机监控里，最容易因为环境差异产生噪音的是磁盘和网卡。

磁盘空间插件配置示例：

```toml
# # By default stats will be gathered for all mount points.
# # Set mount_points will restrict the stats to only the specified mount points.
# mount_points = ["/"]

ignore_fs = ["tmpfs", "devtmpfs", "devfs", "iso9660", "overlay", "aufs", "squashfs", "nsfs", "CDFS", "fuse.juicefs"]

ignore_mount_points = ["/boot", "/var/lib/kubelet/pods"]
```

这里有两个常见建议。

第一，容器环境里会有很多临时挂载点，如果不忽略，Dashboard 里会出现大量不关心的分区。

第二，Kubernetes 节点上 `/var/lib/kubelet/pods` 下面可能会出现很多 pod 相关挂载点，建议根据实际情况过滤掉，避免分区图表过于混乱。

网卡插件配置示例：

```toml
# # setting interfaces will tell categraf to gather these explicit interfaces
# interfaces = ["eth0"]

# enable_loopback_stats=true
# enable_link_down_stats=true
```

如果机器上有很多虚拟网卡、容器网卡、隧道网卡，建议通过 `interfaces` 明确指定要采集的网卡，或者在 Dashboard 变量和 PromQL 中做过滤。否则一台 Kubernetes 节点可能会暴露非常多网卡序列，影响可读性。

## 4. 先用 test 模式验证采集

正式启动前，建议先用 `--test` 模式看一眼采集结果：

```shell
./categraf --test --inputs cpu:mem:disk:diskio:net:system
```

如果你只想看 CPU 和内存：

```shell
./categraf --test --inputs cpu:mem
```

`--test` 模式不会把指标写到后端，只会把采集结果打印出来。它适合回答一个最基础的问题：**这台机器本地到底能不能采到指标？**

如果 `--test` 都采不到，先不要排查夜莺、VictoriaMetrics 或 Grafana。问题大概率在本机权限、配置路径、插件配置或运行环境。

## 5. 启动 Categraf

确认本地采集正常后，就可以启动 Categraf。

如果使用二进制直接运行：

```shell
./categraf --configs ./conf
```

如果使用 systemd，通常会把 Categraf 做成系统服务。启动后可以看日志确认 input 是否正常启动：

```shell
journalctl -u categraf -f
```

或者如果是 Docker Compose 示例环境：

```shell
docker logs -f categraf
```

正常情况下，会看到类似信息：

```text
input: local.cpu started
input: local.mem started
input: local.disk started
input: local.diskio started
input: local.net started
input: local.system started
agent started
```

## 6. 在后端确认指标

采集器启动后，下一步是确认后端能查到数据。

如果后端是 VictoriaMetrics，可以直接查询：

```shell
curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=system_load1'
```

继续查几个典型指标：

```shell
curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=100-cpu_usage_idle{cpu="cpu-total"}'

curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=mem_used_percent'

curl -sG 'http://127.0.0.1:8428/api/v1/query' \
  --data-urlencode 'query=disk_used_percent'
```

如果这些指标都有返回，说明链路已经通了：

```text
Categraf -> remote write -> VictoriaMetrics / Nightingale 后端
```

## 7. 核心指标怎么看

Linux 主机监控指标很多，刚开始不需要全部记住。先关注下面这些就够了。

**CPU 使用率**

常用 PromQL：

```promql
100 - cpu_usage_idle{cpu="cpu-total"}
```

这个指标表示整机 CPU 使用率。进一步排查时，可以看：

- `cpu_usage_user`: 用户态 CPU；
- `cpu_usage_system`: 内核态 CPU；
- `cpu_usage_iowait`: 等待 IO 的 CPU 时间；
- `cpu_usage_steal`: 虚拟化环境中被宿主机抢占的时间。

如果 CPU 使用率高，但 `iowait` 也高，问题可能不在 CPU 本身，而在磁盘或网络 IO。

**系统负载**

常用指标：

```promql
system_load1
system_load5
system_load15
```

负载要结合 CPU 核数看。一般来说，如果 load 长时间明显高于 CPU 逻辑核数，就要关注。load 高但 CPU 使用率不高时，要重点看 IO 等待、进程阻塞和上下文切换。

**内存使用率**

常用指标：

```promql
mem_used_percent
```

Linux 内存不能只看 used，还要理解 cache、buffer。内存使用率高不一定代表异常，但如果 swap 开始明显增长，或者业务进程出现 OOM，就需要深入排查。

**磁盘空间**

常用指标：

```promql
disk_used_percent
disk_inodes_used / disk_inodes_total
```

磁盘有两类常见风险：空间用满和 inode 用满。日志目录、临时目录、容器镜像目录都容易出问题。

**磁盘 IO**

常用指标：

```promql
rate(diskio_reads[1m])
rate(diskio_writes[1m])
rate(diskio_read_bytes[1m])
rate(diskio_write_bytes[1m])
rate(diskio_io_time[1m]) / 10
```

其中 `rate(diskio_io_time[1m]) / 10` 常用来近似观察磁盘 util。磁盘 util 长时间接近高位时，业务延迟可能会明显上升。

**网络流量和错误**

常用指标：

```promql
rate(net_bytes_recv[1m]) * 8
rate(net_bytes_sent[1m]) * 8
rate(net_drop_in[1m])
rate(net_drop_out[1m])
rate(net_err_in[1m])
rate(net_err_out[1m])
```

流量要结合网卡带宽看。丢包和错误包即使数值不大，也值得关注，尤其是在数据库、消息队列、Kubernetes 节点上。

## 8. 导入夜莺 Dashboard

Categraf 仓库里已经提供 Linux 主机监控大盘：

```text
inputs/system/dashboard.json
```

这份大盘面向夜莺，可以直接导入。它覆盖了：

- 主机数量总览；
- CPU 使用率 TopN；
- 内存使用率 TopN；
- 磁盘分区使用率 TopN；
- 单机 CPU、内存、磁盘、磁盘 IO、网络、进程等图表。

导入后，打开大盘，选择 `ident` 变量，就可以查看具体主机。

如果大盘没有数据，先不要急着改大盘。建议先在即时查询里查：

```promql
system_load1
mem_used_percent
100 - cpu_usage_idle{cpu="cpu-total"}
```

如果这些查询有数据，大盘变量大概率没有选对；如果查询本身没数据，说明采集或写入链路还没通。

## 9. 导入 Grafana Dashboard

如果团队习惯使用 Grafana，可以导入同目录下的 Grafana 版本：

```text
inputs/system/dashboard_grafana.json
```

导入时选择 Prometheus 兼容数据源即可。这个数据源可以是 VictoriaMetrics，也可以是其他兼容 Prometheus 查询 API 的后端。

如果 Grafana 以容器方式运行，而 VictoriaMetrics 运行在宿主机，Grafana 数据源地址常见写法是：

```text
http://host.docker.internal:8428
```

导入后关注两个变量：

- `datasource`: Grafana 数据源；
- `ident`: 主机标识。

如果 `ident` 变量没有值，通常说明 `system_load1` 这类基础指标没有写入，或者数据源选错了。

导入成功后，可以先看 CPU 和内存区域，确认基础趋势图是否有数据。

![Categraf Linux 主机 CPU 和内存大盘](https://download.flashcat.cloud/categraf/categraf-grafana-linux-cpu-memory.jpg)

接着再看磁盘 IO、网卡流量和 TCP / UDP 连接状态，这些图表通常是排查主机性能问题时最常用的入口。

![Categraf Linux 主机磁盘和网卡大盘](https://download.flashcat.cloud/categraf/categraf-grafana-linux-disk-network.jpg)

## 10. 告警规则怎么配

Linux 主机告警可以从下面几类开始：

- CPU 使用率持续过高；
- load 持续高于 CPU 核数；
- 内存使用率过高或 swap 明显增长；
- 磁盘空间使用率过高；
- inode 使用率过高；
- 磁盘 IO util 长时间过高；
- 网卡丢包或错误包增长；
- 主机长时间无数据上报。

Categraf 仓库中也提供了 Linux 相关告警规则：

```text
inputs/system/alerts-linux.json
```

建议先导入作为参考，再根据自己的机器规格和业务特点调整阈值。比如数据库机器、计算节点、普通应用机器，对 CPU、IO、磁盘空间的阈值要求通常不一样。

## 11. 常见排查顺序

Linux 主机监控排障时，建议按下面顺序来。

**第一步，确认本机能采到**

```shell
./categraf --test --inputs cpu:mem:disk:diskio:net:system
```

如果这里没有数据，说明问题在本机采集侧。

**第二步，确认 Categraf 正常运行**

```shell
ps aux | grep categraf
```

或者：

```shell
systemctl status categraf
```

**第三步，确认后端能查到**

```promql
system_load1
mem_used_percent
disk_used_percent
```

如果本机能采到，但后端查不到，重点检查 writer 地址、网络连通性、鉴权配置。

**第四步，确认 Dashboard 变量**

很多大盘没有数据，是因为变量没选对。比如 `ident`、`instance`、`job` 这些标签，要和实际指标标签匹配。

**第五步，确认过滤规则**

磁盘、网卡相关图表经常带过滤条件。如果你修改过挂载点、网卡名或标签，可能需要同步调整 PromQL。

## 12. 常见问题

**Q1：Linux 主机监控最小应该开启哪些 Categraf 插件？**

快速验证时可以先开启 `cpu`、`mem`、`disk`、`diskio`、`net`、`system`。生产环境建议补充 `netstat`、`kernel`、`processes`、`linux_sysctl_fs`，这样大盘和告警规则会更完整。

**Q2：Grafana Dashboard 导入后没有主机列表怎么办？**

先确认数据源能查到 `system_load1`。如果原始指标有数据，再检查 Dashboard 变量使用的标签是否和实际指标标签一致，比如 `ident`、`instance`、`job`。

**Q3：磁盘或网卡图表太乱怎么办？**

优先从采集配置和 Dashboard 变量两侧处理。磁盘可以过滤临时挂载点、容器挂载点和 Kubernetes pod 挂载目录；网卡可以明确指定需要采集或展示的物理网卡。

## 13. 小结

Linux 主机监控是 Categraf 最基础、也最应该先跑通的场景。

建议新用户按这个顺序推进：

1. 开启 `cpu`、`mem`、`disk`、`diskio`、`net`、`system` 等基础插件；
2. 用 `--test` 模式确认本机采集正常；
3. 在 VictoriaMetrics、夜莺或其他后端确认指标能查到；
4. 导入 `inputs/system/dashboard.json` 或 `inputs/system/dashboard_grafana.json`；
5. 根据自己的主机类型调整磁盘、网卡过滤和告警阈值。

先把主机监控做好，再继续接入 MySQL、Redis、Kafka、Kubernetes 等组件，整个监控体系会稳很多。
