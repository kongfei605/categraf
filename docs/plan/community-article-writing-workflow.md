# Categraf Community Article Writing Workflow

本文档沉淀 Categraf 社区文章的选题、写作、验证、截图、上传和同步流程。目标是让后续文章可以稳定复用同一套方法，而不是每篇文章临时摸索。

## 当前产出

当前已经完成 17 篇文章：

1. `what-is-categraf.md`
2. `quickstart-categraf-nightingale-victoriametrics.md`
3. `linux-host-monitoring-by-categraf.md`
4. `categraf-grafana-dashboard-guide.md`
5. `categraf-mysql-monitoring-practice.md`
6. `categraf-redis-monitoring-practice.md`
7. `categraf-http-response-monitoring-practice.md`
8. `categraf-net-response-monitoring-practice.md`
9. `categraf-dns-query-monitoring-practice.md`
10. `mongodb-docker-lab-for-categraf.md`
11. `categraf-mongodb-monitoring-practice.md`
12. `categraf-postgresql-monitoring-practice.md`
13. `categraf-configuration-structure-guide.md`
14. `categraf-startup-failure-troubleshooting.md`
15. `categraf-plugin-no-metrics-troubleshooting.md`
16. `categraf-remote-write-troubleshooting.md`
17. `categraf-dashboard-no-data-troubleshooting.md`

文章源文件维护在 Categraf 仓库：

```text
docs/articles/
```

发布到官网时，同步到 fc-site 仓库：

```text
~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/
```

## 计划文档

选题计划维护在：

```text
docs/plan/community-articles-plan.md
```

该文档回答“写什么”和“按什么顺序写”。本 workflow 文档回答“怎么写、怎么验证、怎么发布”。

如果本次文章涉及更新 Dashboard、采集真实指标、截图、上传图片到 S3/COS、插入 CDN 图片链接、同步官网博客，优先参考更细的发布流程：

```text
docs/plan/community-article-update-and-publish-workflow.md
```

更新原则：

- 每完成一个阶段，及时把已验证的选题补回计划文档。
- 如果临时调整选题，比如从 Kafka 改成 `http_response`、`net_response`，也要同步计划文档。
- 第一篇总览文章 `what-is-categraf.md` 末尾维护系列文章入口，新增文章后要补链接。

## 如何选题

优先选择能形成闭环的主题：

```text
场景问题 -> Categraf 配置 -> 指标验证 -> Dashboard -> 告警 -> 常见问题
```

选题优先级：

1. 高频场景：Linux、MySQL、Redis、HTTP 探测、TCP/UDP 探测。
2. 有配套 Dashboard 和告警规则的插件。
3. 容易搭建测试环境并能截图的插件。
4. 用户搜索意图明确的主题，比如“如何监控 MySQL”“HTTP 接口监控”“Redis Dashboard”。
5. 能体现 Categraf 和夜莺、Grafana、VictoriaMetrics 协同能力的主题。

每篇文章开始前先检查：

```shell
ls inputs/<plugin>/
sed -n '1,220p' inputs/<plugin>/README.md
sed -n '1,220p' conf/input.<plugin>/<plugin>.toml
```

重点看：

- README 是否已有指标解释；
- 是否有 `dashboard.json`；
- 是否有 `dashboard_grafana.json`；
- 是否有 `alerts.json`；
- 配置文件里有哪些关键项；
- Go 代码里真实输出的 metric、label、result code。

## 文章结构

插件实战文章默认使用下面结构：

```text
1. 场景和核心问题
2. 核心要点
3. 插件解决什么问题
4. 最小可用配置
5. 启动与验证
6. 核心指标解释
7. Dashboard 导入
8. 告警规则
9. 常见问题
10. 生产建议
```

写作要求：

- Markdown 标题从 H2 开始，即正文使用 `##`，不要从 `####` 开始。
- 标题要面向搜索和问题解决，不要只写插件名。
- 配置片段必须可复制，但不能包含真实密码、Token、AK/SK。
- 指标解释要结合排障场景，不要只罗列指标名。
- 每篇尽量包含 Dashboard 截图，优先使用真实测试环境截图。
- 图片引用使用公网 CDN URL，不引用本地路径。

Front matter 示例：

```yaml
---
title: "Categraf HTTP 响应监控实战：可用性、状态码、延迟和证书"
description: "本文介绍如何使用 Categraf http_response 插件做 HTTP/HTTPS 黑盒探测，包括探测配置、结果码、状态码、分阶段耗时、HTTPS 证书过期时间、Dashboard 和告警建议。"
image: "https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg"
keywords: ["Categraf", "http_response", "HTTP监控", "Nightingale", "Grafana"]
author: "快猫星云"
date: "2026-07-03T00:00:00+08:00"
tags: ["Categraf", "HTTP", "Monitoring"]
---
```

## 日期规则

发布时间按批次设置：

- 第一批 3 篇：`2026-06-30T00:00:00+08:00`
- 第二批 3 篇：`2026-07-01T00:00:00+08:00`
- 第三批从 `2026-07-03T00:00:00+08:00` 开始

后续如果一天发布多篇，保持同一天同一个日期即可。若需要模拟连续发布，再按天递增。

## 启动夜莺测试环境

优先复用已有测试环境。以下命令使用 `<TEST_HOST>`、`<SSH_USER>` 和 `/path/to/ssh-key.pem` 作为占位符。

登录：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST>
```

夜莺代码目录：

```text
~/go/src/github.com/ccfos/nightingale
```

推荐先用 bridge compose：

```shell
cd ~/go/src/github.com/ccfos/nightingale/docker/compose-bridge
docker compose up -d
docker compose ps
```

该 compose 默认包含：

- MySQL
- Redis
- VictoriaMetrics
- Nightingale
- Categraf

常用端口：

```text
Nightingale: http://<host>:17000
VictoriaMetrics: http://<host>:8428
Nightingale remote write / API: <host>:20090
```

如果需要主机网络模式，可以使用：

```shell
cd ~/go/src/github.com/ccfos/nightingale/docker/compose-host-network
docker compose up -d
docker compose ps
```

启动后先确认：

```shell
docker compose ps
docker compose logs --tail=100 nightingale
docker compose logs --tail=100 categraf
```

注意：文章中不要写测试环境真实登录密码。需要说明时使用“请查看部署说明或启动日志”。

## 启动 Grafana 测试环境

Grafana 用于验证 `dashboard_grafana.json`。如果已有 Grafana，优先复用；如果需要临时启动，可以在测试机上执行：

```shell
docker run -d \
  --name grafana \
  -p 3000:3000 \
  -e GF_SECURITY_ADMIN_USER=admin \
  -e GF_SECURITY_ADMIN_PASSWORD=<PASSWORD> \
  grafana/grafana:latest
```

Grafana 地址：

```text
http://<host>:3000
```

数据源选择 Prometheus 类型，URL 按部署位置填写：

```text
http://<host>:8428
```

如果 Grafana 和 VictoriaMetrics 在同一台测试机，通常可用：

```text
http://127.0.0.1:8428
```

导入 Dashboard：

```text
inputs/<plugin>/dashboard_grafana.json
```

导入后检查：

- datasource 是否正确；
- 变量是否有值；
- 核心指标是否有数据；
- 时间范围是否覆盖采集时间；
- 图表标题、单位、legend 是否清晰。

## 采集数据

优先用本机编译的 Categraf 采集真实目标，再写入测试环境夜莺或 VictoriaMetrics。这样便于调试配置，也避免污染远端测试机。

基本流程：

```shell
go build ./cmd/categraf
./categraf --test --inputs <plugin>
```

确认 `--test` 能打印指标后，再配置 writer 写入测试环境。

对于数据库、Redis、HTTP 探测等文章，至少采集几分钟，让 Dashboard 的趋势图有足够数据。截图前把页面时间范围设置为最近 15 分钟、最近 30 分钟或最近 1 小时。

敏感信息处理：

- 配置样例中使用 `<PASSWORD>`、`<TOKEN>`、`<SECRET>`。
- 不要把真实数据库密码、Redis 密码、COS 密钥写入文章。
- 不要把真实登录态 token、cookie、JWT 写入文章或提交记录。

## 导入夜莺 Dashboard

夜莺 Dashboard 通常在插件目录：

```text
inputs/<plugin>/dashboard.json
```

导入后检查：

- 数据源是否正确；
- 变量是否能列出目标；
- 核心图表是否有数据；
- 表格中是否能看到关键标签；
- 告警规则是否可导入或可参考。

告警规则通常在：

```text
inputs/<plugin>/alerts.json
```

文章中可以引用关键 PromQL，但不要完整展开大段 JSON。

## 截图要求

Dashboard 截图用于增强文章可信度。要求：

- 使用真实夜莺或 Grafana 页面截图。
- 图片格式统一使用 JPG。
- 文件名使用小写英文和连字符，不要出现 `-_` 连用。
- 图片放到 CDN 的 `categraf/` 目录。
- 截图前确认页面没有敏感信息。
- 截图前确认核心面板有数据，不要截空白大盘。

推荐文件名：

```text
categraf-n9e-<plugin>-dashboard.jpg
categraf-grafana-<plugin>-overview.jpg
```

示例：

```text
categraf-n9e-http-response-dashboard.jpg
categraf-grafana-mysql-overview.jpg
categraf-grafana-redis-overview.jpg
```

如果 Chrome 已授权，可以用 Chrome 打开页面后截完整页面。截图先保存为 PNG，再转 JPG：

```shell
sips -s format jpeg /tmp/categraf-n9e-http-response-dashboard.png \
  --out /tmp/categraf-n9e-http-response-dashboard.jpg
```

## 上传图片

图片上传到腾讯 COS，对外访问域名是：

```text
https://download.flashcat.cloud/
```

Categraf 文章统一上传到：

```text
categraf/
```

最终 URL 形如：

```text
https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg
```

本机已有上传脚本：

```text
~/local/tools/pub.sh
```

该脚本包含敏感配置，不能把内容写入文档、文章或提交信息。上传时只读取必要环境变量，然后用 COS SDK 上传到 `categraf/` 前缀。

上传后必须校验 CDN 可访问。该 CDN 需要 Referer：

```shell
wget --spider \
  --header="Referer: http://flashcat.cloud" \
  https://download.flashcat.cloud/categraf/<image>.jpg
```

预期看到：

```text
HTTP request sent, awaiting response... 200 OK
Remote file exists.
```

如果返回 404，不要在文章里引用该链接。

## 写入文章

文章先写到 Categraf 仓库：

```text
docs/articles/<slug>.md
```

命名规则：

- 使用小写英文和连字符；
- 尽量包含 `categraf` 和插件名；
- 不使用空格；
- 不出现 `-_` 连用；
- 文件名和最终 blog slug 保持一致。

示例：

```text
categraf-http-response-monitoring-practice.md
categraf-net-response-monitoring-practice.md
```

文章中的图片使用 Markdown：

```markdown
![Categraf HTTP Response 夜莺大盘](https://download.flashcat.cloud/categraf/categraf-n9e-http-response-dashboard.jpg)
```

不要使用本地图片路径。

## 同步到 fc-site

文章写好后复制到官网仓库：

```shell
cp docs/articles/<slug>.md \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/<slug>.md
```

如果修改了第一篇总览文章，也要同步：

```shell
cp docs/articles/what-is-categraf.md \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/what-is-categraf.md
```

fc-site 本地预览：

```shell
cd ~/go/src/github.com/flashcatcloud/fc-site
npm run dev
```

预览时重点检查：

- front matter 是否被正确解析；
- 图片是否显示；
- 标题层级是否正常；
- 代码块是否换行正常；
- 链接是否可点击；
- 移动端是否没有明显排版问题。

## 链接检查

写完后检查文章中的外链，尤其是图片链接。

列出 `download.flashcat.cloud` 链接：

```shell
rg -o "https://download\\.flashcat\\.cloud/[^)\\]\\s\\\"']+" docs/articles/<slug>.md
```

逐个用 Referer 校验：

```shell
wget --spider --header="Referer: http://flashcat.cloud" <url>
```

内部 blog 链接使用：

```text
/blog/<slug>/
```

新增文章后，更新：

```text
docs/articles/what-is-categraf.md
~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/what-is-categraf.md
```

## 安全检查

提交前必须做敏感信息扫描：

```shell
rg -n "password\\s*=|passwd|secret\\s*=|AKID|FC_COS|access_token|refresh_token|private_key|secret_key" \
  docs/articles \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/<slug>.md
```

如果命中真实敏感信息，必须替换为占位符：

```text
<PASSWORD>
<TOKEN>
<SECRET>
```

注意：

- 可以在配置样例里保留空密码字段，例如 `password = ""`，但不要出现真实密码。
- 不要把 COS 配置、私钥、cookie、JWT、数据库密码写入文章。
- 截图里如果出现敏感信息，要重新截图或裁剪。

## 格式检查

检查标题层级：

```shell
rg -n "^#{3,6}\\s" docs/articles/<slug>.md
```

插件实战文章正文应从 H2 开始。除非有明确需要，避免 H3/H4 过深层级。

检查 Markdown 和 diff 基本问题：

```shell
git diff --check -- docs/articles/<slug>.md docs/plan/community-articles-plan.md
git -C ~/go/src/github.com/flashcatcloud/fc-site diff --check -- content/chinese/blog/<slug>.md
```

检查文件名：

```shell
find docs/articles -name '*-_*' -o -name '*_-*'
```

如发现类似 `dashboard-3.8-_grafana.json` 这种 `-_` 连用，应改成更自然的命名。

## GEO 友好检查

如果需要按 GEO / AI 搜索友好方式 review，使用已安装的 `yao-geo-article-friendly` skill 对已有文章做检查和改写。

重点检查：

- 标题是否直接回答搜索意图；
- 开头是否快速说明读者能获得什么；
- 核心要点是否清晰；
- 指标解释是否有语义密度；
- 是否有足够的实操步骤和排障信息；
- 是否避免空泛营销表达；
- 是否有可验证的 Dashboard、指标和配置引用。

不要为了 GEO 编造数据、引用或结论。

## 提交流程

通常会涉及两个仓库：

```text
Categraf: ~/go/src/github.com/kongfei605/categraf
fc-site:  ~/go/src/github.com/flashcatcloud/fc-site
```

提交前检查：

```shell
git status --short
git -C ~/go/src/github.com/flashcatcloud/fc-site status --short
```

如果需要提交，commit message 使用 conventional 风格，例如：

```text
docs: add categraf http response and net response articles
```

只提交本次文章相关文件，不要顺手提交无关改动。

## 单篇文章完成清单

每篇文章完成前检查：

- [ ] 选题已在 `docs/plan/community-articles-plan.md` 中体现。
- [ ] 已阅读对应插件 README、配置、Dashboard、alerts。
- [ ] 已写入 `docs/articles/<slug>.md`。
- [ ] front matter 日期、标题、描述、关键词正确。
- [ ] 正文标题从 H2 开始。
- [ ] 配置样例没有真实密码、Token、密钥。
- [ ] Dashboard 截图是 JPG。
- [ ] 图片已上传到 `https://download.flashcat.cloud/categraf/`。
- [ ] 图片链接用 Referer 校验返回 200。
- [ ] 文章已同步到 fc-site。
- [ ] 第一篇总览文章的系列链接已更新。
- [ ] 敏感信息扫描通过。
- [ ] `git diff --check` 通过。
