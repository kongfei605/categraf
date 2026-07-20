# Categraf Community Article Plan

本文档用于规划一组面向开源社区的 Categraf 使用文章，目标是降低新用户上手成本，并持续放大 Categraf 在监控采集、Dashboard、告警和生产实践场景中的影响力。

## 目标

- 帮助新用户快速完成采集、存储、查询、Dashboard 展示的最小闭环。
- 用插件实战文章覆盖常见中间件、数据库、Kubernetes 和主机监控场景。
- 突出 Categraf 与夜莺、VictoriaMetrics、Grafana 等生态的协同能力。
- 沉淀可复用的配置、指标解释、Dashboard 导入和排障经验。

## 内容主线

### 1. 新手上手

这一组文章解决第一次接触 Categraf 时的核心问题：如何快速跑起来，如何确认采集有效，如何看见数据。

建议文章：

1. `Categraf 是什么：一篇文章讲清楚它适合什么场景`
2. `10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics`
3. `Categraf 入门：采集 Linux 主机指标并接入夜莺`
4. `Categraf + VictoriaMetrics：从采集到查询的最小闭环`
5. `Categraf 配置文件结构详解：global、writer、inputs 怎么理解`
6. `Categraf 常见启动失败与排查方法`

### 2. 插件实战

这一组文章围绕 `inputs` 目录展开。每篇聚焦一个真实监控场景，提供最小配置、核心指标解释、Dashboard 导入和常见问题。

优先级建议：

1. Linux 基础监控：`cpu`、`mem`、`disk`、`diskio`、`net`、`system`
2. 数据库和缓存：`mysql`、`redis`、`postgresql`、`mongodb`、`oracle`
3. 消息队列和中间件：`kafka`、`rabbitmq`、`zookeeper`、`elasticsearch`
4. Kubernetes：`kubelet`、`kube_state_metrics`、`kube_proxy`
5. 基础设施和硬件：`snmp`、`ipmi`、`redfish`、`switch_legacy`

示例标题：

- `用 Categraf 监控 MySQL：采集配置、核心指标和 Dashboard`
- `用 Categraf 监控 Redis：连接数、内存、慢查询与告警建议`
- `用 Categraf 监控 Kafka：Broker、Topic、Consumer Lag 怎么看`
- `用 Categraf 监控 Kubernetes：kubelet、kube-state-metrics 与 kube-proxy`
- `用 Categraf 监控交换机和硬件：SNMP、IPMI、Redfish 实战`

### 3. Dashboard 与告警

这一组文章重点放在“采集后如何看图、如何告警”。Categraf 仓库已经提供夜莺 Dashboard 和 Grafana Dashboard，可以作为重要传播素材。

建议文章：

1. `Categraf Dashboard 使用指南：夜莺和 Grafana 如何导入`
2. `如何读懂 Categraf Linux 主机监控大盘`
3. `MySQL 监控大盘里哪些指标最重要`
4. `Redis 监控大盘里应该关注哪些风险信号`
5. `从 Dashboard 到告警：Categraf 指标如何配置告警规则`
6. `Grafana 用户如何复用 Categraf 的监控大盘`

### 4. 深度原理与生产实践

这一组文章用于建立项目的技术权威感，适合在基础实战内容积累一定读者后持续发布。

建议文章：

1. `Categraf 的采集模型：input、processor、writer 如何协作`
2. `Categraf 和 Telegraf、node_exporter 的差异`
3. `如何为 Categraf 编写一个新的 input 插件`
4. `高基数标签问题：Categraf 采集配置中的常见坑`
5. `生产环境如何部署 Categraf：配置管理、升级和回滚`
6. `Categraf 在大规模主机监控中的性能与稳定性实践`

## 第一阶段选题

第一阶段建议先写 6 篇，目标是把“安装、采集、存储、查询、Dashboard”闭环打通，并覆盖最常见的数据库和缓存场景。

1. `Categraf 是什么：一篇文章讲清楚它适合什么场景`
2. `10 分钟跑起 Categraf + 夜莺 + VictoriaMetrics`
3. `Categraf Linux 主机监控实战：配置、指标和大盘`
4. `Categraf Grafana Dashboard 使用指南`
5. `Categraf MySQL 监控实战`
6. `Categraf Redis 监控实战`

## 第二阶段选题（探测类）

第二阶段继续围绕高频插件实战展开。当前优先补充探测类插件，因为它们更容易在生产环境形成“配置 -> 指标 -> 大盘 -> 告警”的闭环，也适合作为 Categraf 黑盒监控能力的传播素材。

1. `Categraf HTTP 响应监控实战：可用性、状态码、延迟和证书`
2. `Categraf TCP/UDP 网络探测实战：端口连通性、响应时间和告警`
3. `Categraf DNS 查询监控实战：解析结果、解析耗时和可用性`

当前 3 篇探测类文章已完成。数据库深入系列中的 MongoDB 测试环境、MongoDB 监控实战和 PostgreSQL 监控实战均已完成。新手与排障补充系列已经完成配置、启动、采集、写入和 Dashboard 展示五篇文章；后续按既定顺序继续完成多实例与标签、systemd 与容器部署、升级与回滚 3 篇，再进入 Kafka、Elasticsearch、RabbitMQ、Kubernetes 等中间件系列。

## 第三阶段选题（数据库深入）

第三阶段先补 MongoDB。MongoDB 的部署形态和采集方式比较多，文章开写前要先把几种常见模式跑通并验证 Dashboard，不要只覆盖单机示例。

建议顺序：

1. `MongoDB Docker 测试环境搭建：单节点、副本集和分片集群`（已完成）
2. `Categraf MongoDB 监控实战：单节点、副本集、分片集群的采集配置和大盘`（已完成）
3. `Categraf PostgreSQL 监控实战：连接、事务、缓存、锁和慢查询指标`（已完成）

MongoDB 文章已完成以下验证：

- 单节点 MongoDB 采集；
- 副本集 Primary、Secondary、复制延迟和成员状态采集；
- 分片集群 `mongos`、config server、shard replica set 的采集边界；
- 认证场景下监控账号配置；
- `collect_all = true` 下库表和索引采集的权限要求；
- 多实例标签设计，比如 `cluster`、`topology`、`component`、`instance`、`replset`；
- 夜莺和 Grafana Dashboard 文件位置；
- 常用告警建议，包括实例不可用、复制延迟、连接数、锁、缓存和慢操作。

PostgreSQL 文章已完成 PostgreSQL 16 容器采集、夜莺 Dashboard 导入和真实指标截图验证，并补充了锁等待自定义 SQL、`pg_stat_statements`、高基数风险和告警语义说明。

## 第四阶段选题（新手与排障补充）

这一阶段集中解决跨插件、可复用的新手问题和排障问题，不重复 MySQL、Redis、PostgreSQL 等插件实战文章中的局部 FAQ。文章按照“会配置 -> 能启动 -> 能采集 -> 能写入 -> 能展示 -> 可维护”的路径依次发布。

| 顺序 | 文章选题 | 重点内容 | 状态 |
| --- | --- | --- | --- |
| 1 | `Categraf 配置文件结构详解：global、writer、heartbeat、inputs 与四类 Agent` | 配置目录、主配置、插件配置、采集周期、全局标签、writer、heartbeat 和配置覆盖关系 | 已完成 |
| 2 | `Categraf 启动失败排查：TOML、配置路径、权限和 systemd 常见错误` | TOML 解析、配置目录、文件权限、systemd 工作目录、版本兼容和日志查看 | 已完成 |
| 3 | `Categraf 插件没有指标：从 test 模式到数据库权限的完整排查` | 插件启用、`[[instances]]`、`--inputs`、网络、认证、过滤条件和采集日志 | 已完成 |
| 4 | `Categraf 已采到指标但后端查不到：remote write 链路排查` | writer 地址、发送队列、HTTP 状态、重试、网络、时间偏差、后端和数据源 | 已完成 |
| 5 | `Categraf Dashboard 没有数据：数据源、变量、标签和 PromQL 排查` | 时间范围、数据源变量、实例标签、指标改名、PromQL 和 Dashboard 版本兼容 | 已完成 |
| 6 | `Categraf 多实例配置与标签设计：如何避免实例混淆和高基数` | 多个 `[[instances]]`、稳定实例标识、`labels`、`outputaddress` 和高基数控制 | 待写 |
| 7 | `Categraf 使用 systemd 和容器部署的常见坑` | 配置挂载、网络命名空间、容器内 `127.0.0.1`、权限、重启策略和日志持久化 | 待写 |
| 8 | `Categraf 升级与回滚实践：配置备份、灰度验证和版本兼容` | 升级前备份、测试验证、插件配置差异、灰度节点、二进制与配置回滚 | 待写 |

固定写作顺序：

```text
配置结构
  -> 启动失败
  -> 插件没有指标
  -> remote write 后端无数据
  -> Dashboard 无数据
  -> 多实例与标签
  -> systemd 和容器部署
  -> 升级与回滚
```

前 4 篇构成第一批，已经打通“配置、启动、采集、写入”排障闭环；后 4 篇构成第二批，Dashboard 无数据排查已经完成，后续继续覆盖多实例、部署和升级维护。每完成一篇，就在上表中把状态更新为“已完成”，并同步更新 `community-article-writing-workflow.md` 的当前产出清单。

## 第五阶段选题（中间件与 Kubernetes）

新手与排障补充系列完成后，再继续扩展消息队列、搜索和 Kubernetes 等常见组件。

1. `Categraf Kafka 监控实战：Broker、Topic、Consumer Lag 怎么看`
2. `Categraf Elasticsearch 监控实战：集群健康、节点、索引和 JVM 指标`
3. `Categraf RabbitMQ 监控实战：队列积压、连接、通道和消息速率`
4. `Categraf Kubernetes 监控实战：kubelet、kube-state-metrics 与 kube-proxy`

## 单篇文章模板

每篇文章建议保持固定结构，便于批量产出，也方便社区贡献者复用。

```text
1. 这个插件或场景解决什么问题
2. 适用场景
3. 最小可用配置
4. 启动与验证
5. 核心指标解释
6. Dashboard 导入
7. 常见问题
8. 生产建议
```

## 写作原则

- 优先写“用户照着做就能跑起来”的实战内容。
- 每篇文章都要包含可复制的配置片段和验证命令。
- 指标解释要结合真实排障场景，而不是简单罗列 metric name。
- Dashboard 文章要同时覆盖夜莺和 Grafana 用户。
- 标题尽量面向搜索和问题解决，例如“如何监控 MySQL”“如何导入 Dashboard”“如何排查采集失败”。
- 深度原理文章放在实战内容之后，用来承接已经入门的读者。

## 发布节奏建议

- 第一阶段每周发布 1 到 2 篇，先完成上手闭环和 Linux、MySQL、Redis 三个高频场景。
- 第二阶段完成 HTTP、TCP/UDP、DNS 三类探测文章。
- 第三阶段优先完成 MongoDB 和 PostgreSQL 数据库深入文章。
- 第四阶段依次完成 8 篇新手与排障补充文章，先完成配置、启动、采集、写入闭环，再覆盖 Dashboard、多实例、部署和升级。
- 第五阶段围绕 Kafka、Elasticsearch、Kubernetes、RabbitMQ 等常见组件继续扩展。
- 再往后发布原理、性能、插件开发和生产实践文章，提升项目技术影响力。
