# Categraf Article Update And Publish Workflow

本文档记录“更新 Categraf 仓库文档和 Dashboard，再同步到官网博客”的完整流程。适用于新增或更新一篇社区文章，尤其是需要更新 Dashboard、采集真实指标、截图、上传图片到 S3/COS、把 CDN 链接插入文章的场景。

## 目标产物

一次完整更新通常包含这些产物：

```text
inputs/<plugin>/dashboard.json
inputs/<plugin>/dashboard_grafana.json
inputs/<plugin>/alerts.json
docs/articles/<slug>.md
docs/articles/what-is-categraf.md
docs/plan/community-articles-plan.md
docs/plan/community-article-writing-workflow.md
~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/<slug>.md
```

不是每篇文章都会修改所有文件。原则是只更新和本次文章直接相关的内容，不顺手提交无关改动。

## 1. 确认文章和插件素材

先确认文章 slug、插件目录、默认配置、大盘和告警规则：

```shell
slug="categraf-dns-query-monitoring-practice"
plugin="dns_query"

ls inputs/$plugin/
sed -n '1,220p' inputs/$plugin/README.md
sed -n '1,220p' inputs/$plugin/README_CN.md
sed -n '1,220p' conf/input.$plugin/$plugin.toml
find inputs/$plugin -maxdepth 1 -type f \( -name '*dashboard*' -o -name 'alerts.json' \)
```

重点确认：

- README 是否已有可复用的指标解释；
- `conf/input.<plugin>/<plugin>.toml` 中有哪些关键配置项；
- `inputs/<plugin>/dashboard.json` 是否存在，是否是夜莺可导入格式；
- `inputs/<plugin>/dashboard_grafana.json` 是否存在；
- `inputs/<plugin>/alerts.json` 是否存在；
- Go 代码里真实输出的 metric、label、result code 是否和文章一致。

如果 Dashboard 不存在或格式太简陋，先补 Dashboard，再写文章。文章里的 Dashboard 描述必须和实际 Dashboard 一致。

## 2. 更新 Dashboard

夜莺 Dashboard 默认放在：

```text
inputs/<plugin>/dashboard.json
```

Grafana Dashboard 默认放在：

```text
inputs/<plugin>/dashboard_grafana.json
```

夜莺 Dashboard 建议包含：

- 全局健康状态；
- 全局耗时或核心指标对比；
- 当前明细表格；
- 单目标详情；
- 单目标趋势图；
- 必要的变量，比如 `datasource`、`ident`、`target`、`domain`、`server`。

更新后先做 JSON 校验：

```shell
python3 -m json.tool inputs/<plugin>/dashboard.json >/tmp/<plugin>-dashboard-check.json
```

如果是手工从夜莺页面导出的 Dashboard，需要再检查：

- 数据源变量是否使用可复用写法，例如 `${datasource}`；
- PromQL 是否只依赖该插件真实输出的指标；
- 变量查询是否能列出目标；
- 面板单位、阈值、legend 是否清晰；
- 是否包含测试环境专用 IP、账号、Token、Cookie 或内部业务名。

## 3. 准备测试环境

优先复用已有夜莺测试实例。当前可用测试环境示例：

```text
Nightingale: http://<TEST_HOST>:17000
Remote write: http://<TEST_HOST>:17000/prometheus/v1/write
Prometheus query: http://<TEST_HOST>:9090/api/v1/query
SSH: ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST>
```

先确认服务可用：

```shell
curl -I http://<TEST_HOST>:17000/
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> 'docker ps --format "{{.Names}} {{.Ports}}" | head'
```

如果本机可以直接运行 Categraf，优先本机采集并写入测试夜莺。若需要在测试机就近采集，可以在远端创建临时配置目录，不要修改线上已有配置目录。

## 4. 采集真实指标

先用测试模式确认插件能输出指标：

```shell
./categraf --test --inputs <plugin>
```

如果需要远端临时采集，推荐方式是复制一份临时配置：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  'rm -rf /tmp/categraf-<plugin>-conf && mkdir -p /tmp/categraf-<plugin>-conf/input.<plugin>'

scp -i /path/to/ssh-key.pem conf/config.toml \
  <SSH_USER>@<TEST_HOST>:/tmp/categraf-<plugin>-conf/config.toml

scp -i /path/to/ssh-key.pem conf/input.<plugin>/<plugin>.toml \
  <SSH_USER>@<TEST_HOST>:/tmp/categraf-<plugin>-conf/input.<plugin>/<plugin>.toml
```

然后在临时配置中只改本次插件需要的内容：

- `[[writers]].url` 指向测试夜莺 remote write；
- 关闭不必要的 `heartbeat`；
- 只启用本次插件需要的 `[[instances]]`；
- 使用测试目标和占位凭据，不能写真实密码、Token、AK/SK；
- 补充 `labels`，例如 `region`、`probe`、`service`，便于截图和告警示例说明。

测试输出：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  '/opt/categraf/categraf -configs /tmp/categraf-<plugin>-conf -inputs <plugin> -test'
```

确认输出正常后，后台运行几分钟：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  'cd /opt/categraf && nohup ./categraf -configs /tmp/categraf-<plugin>-conf -inputs <plugin> >/tmp/categraf-<plugin>.log 2>&1 & echo $! >/tmp/categraf-<plugin>.pid'
```

用 PromQL 确认后端能查到数据：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  'curl -sS -G http://127.0.0.1:9090/api/v1/query --data-urlencode "query=<metric_name>"'
```

截图完成后停止临时采集进程：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  'kill $(cat /tmp/categraf-<plugin>.pid) 2>/dev/null || true'
```

如果 PID 文件指向启动 shell 而不是实际 Categraf 进程，需要按临时配置路径精确清理：

```shell
ssh -i /path/to/ssh-key.pem <SSH_USER>@<TEST_HOST> \
  'pkill -f "categraf.*-configs /tmp/categraf-<plugin>-conf" 2>/dev/null || true'
```

## 5. 导入夜莺 Dashboard

优先通过夜莺页面导入：

```text
Dashboards -> Import -> 选择 inputs/<plugin>/dashboard.json
```

导入后检查：

- Dashboard 标题是否清晰；
- 数据源变量是否选中 Prometheus 或 VictoriaMetrics；
- 变量是否能列出目标；
- 核心面板是否有数据；
- 表格是否能看到关键标签；
- 时间范围是否覆盖采集时间。

如果页面导入不方便，可以使用夜莺 API。接口和 payload 会随版本变化，操作前先从前端请求或当前实例接口确认，不要硬编码旧版本路径。

不推荐直接写数据库。只有在测试环境、已确认表结构、且页面/API 不可用时，才把直接写数据库作为临时兜底方案。直接写入前至少确认：

```shell
mysql -h<host> -P<port> -u<user> -p \
  -e "show columns from n9e_v6.board; show columns from n9e_v6.board_payload;"
```

直接写数据库不能用于生产环境。

## 6. 截图

截图前先处理页面状态：

- 时间范围设置为最近 15 分钟、30 分钟或 1 小时；
- 变量选中真实有数据的目标；
- 页面不要出现账号、Token、Cookie、内部密码、真实客户名；
- 图表不能空白；
- 表格不要展示敏感标签；
- 浏览器缩放保持 100%。

图片命名规则：

```text
categraf-n9e-<plugin>-dashboard.jpg
categraf-grafana-<plugin>-overview.jpg
```

示例：

```text
categraf-n9e-dns-query-dashboard.jpg
```

如果截图先得到 PNG，再转换成 JPG：

```shell
sips -s format jpeg /tmp/categraf-n9e-<plugin>-dashboard.png \
  --out /tmp/categraf-n9e-<plugin>-dashboard.jpg
```

截图后本地预览，确认文字不糊、没有裁切、没有敏感信息：

```shell
open /tmp/categraf-n9e-<plugin>-dashboard.jpg
```

如果无法稳定截取浏览器页面，可以用真实 PromQL 查询结果生成一张 Dashboard 风格配图作为临时替代，但文章中不要声称它是浏览器原始截图。优先级仍然是真实页面截图。

## 7. 上传图片到 S3/COS

图片对外访问域名：

```text
https://download.flashcat.cloud/
```

Categraf 文章图片统一上传到：

```text
categraf/
```

最终 URL：

```text
https://download.flashcat.cloud/categraf/<image>.jpg
```

本机已有上传脚本：

```text
~/local/tools/pub.sh
```

注意：

- 上传脚本或环境变量里可能包含 COS/S3 密钥，不能把密钥值写入文档、文章、提交信息或截图；
- 如果脚本只按文件 basename 上传，需要确认是否支持对象前缀；
- 上传完成后必须用 CDN URL 校验，不能只看命令是否返回成功。

如果已有脚本支持指定对象 key，直接上传到 `categraf/` 前缀：

```shell
~/local/tools/pub.sh upload /tmp/categraf-n9e-<plugin>-dashboard.jpg \
  categraf/categraf-n9e-<plugin>-dashboard.jpg
```

如果现有脚本只上传 basename，可以用 COS/S3 SDK 指定对象 key。示例只读取环境变量，不写密钥值：

```shell
source <path-to-cos-env>
source ~/venv/bin/activate
python3 - <<'PY'
import os
from qcloud_cos import CosConfig, CosS3Client

bucket = os.environ["FC_COS_BUCKET"]
config = CosConfig(
    Region=os.getenv("FC_COS_REGION", "ap-nanjing"),
    SecretId=os.environ["FC_COS_SECRETID"].strip(),
    SecretKey=os.environ["FC_COS_SECRETKEY"].strip(),
)
client = CosS3Client(config)
client.upload_file(
    Bucket=bucket,
    Key="categraf/categraf-n9e-<plugin>-dashboard.jpg",
    LocalFilePath="/tmp/categraf-n9e-<plugin>-dashboard.jpg",
)
PY
deactivate
```

校验 CDN：

```shell
wget --spider \
  --header="Referer: http://flashcat.cloud" \
  https://download.flashcat.cloud/categraf/categraf-n9e-<plugin>-dashboard.jpg
```

必须看到：

```text
HTTP request sent, awaiting response... 200 OK
Remote file exists.
```

如果是 `404 Not Found`，不要把链接写入文章。先确认对象 key 是否上传到了 `categraf/` 前缀。

## 8. 更新文章

文章源文件在：

```text
docs/articles/<slug>.md
```

如果文章使用图片，front matter 添加：

```yaml
image: "https://download.flashcat.cloud/categraf/categraf-n9e-<plugin>-dashboard.jpg"
og_image: "https://download.flashcat.cloud/categraf/categraf-n9e-<plugin>-dashboard.jpg"
```

正文 Dashboard 章节插入图片：

```markdown
![Categraf <Plugin> 夜莺大盘](https://download.flashcat.cloud/categraf/categraf-n9e-<plugin>-dashboard.jpg)
```

Dashboard 章节要写清楚：

- Dashboard 文件路径；
- 夜莺和 Grafana 是否都有；
- 导入后检查哪些变量；
- Dashboard 包含哪些核心面板；
- 大盘没数据时先查哪个原始指标。

如果新增文章，还要更新总览入口：

```text
docs/articles/what-is-categraf.md
```

新增一条内部链接：

```markdown
- [文章标题](/blog/<slug>/)
```

如果计划或当前产出状态变化，还要更新：

```text
docs/plan/community-articles-plan.md
docs/plan/community-article-writing-workflow.md
```

## 9. 同步到官网博客

文章写好后复制到 fc-site：

```shell
cp docs/articles/<slug>.md \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/<slug>.md
```

如果更新了总览文章，也同步：

```shell
cp docs/articles/what-is-categraf.md \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/what-is-categraf.md
```

本地预览：

```shell
cd ~/go/src/github.com/flashcatcloud/fc-site
npm run dev
```

预览重点：

- front matter 是否解析正常；
- 封面图和正文图是否显示；
- 标题层级是否正常；
- 代码块是否换行；
- 内部链接是否正确；
- 移动端是否有明显溢出。

## 10. 检查清单

检查图片链接：

```shell
rg -o "https://download\\.flashcat\\.cloud/[^)\\]\\s\\\"']+" docs/articles/<slug>.md
wget --spider --header="Referer: http://flashcat.cloud" <image-url>
```

检查标题层级：

```shell
rg -n "^#{3,6}\\s" docs/articles/<slug>.md
```

插件实战文章正文标题默认从 H2 开始，避免过深标题。

检查敏感信息：

```shell
rg -n "password\\s*=|passwd|secret\\s*=|AKID|FC_COS|access_token|refresh_token|private_key|secret_key" \
  docs/articles/<slug>.md \
  inputs/<plugin>/dashboard.json \
  ~/go/src/github.com/flashcatcloud/fc-site/content/chinese/blog/<slug>.md
```

检查 Markdown 和 JSON：

```shell
python3 -m json.tool inputs/<plugin>/dashboard.json >/tmp/<plugin>-dashboard-check.json
git diff --check -- docs/articles/<slug>.md inputs/<plugin>/dashboard.json
git -C ~/go/src/github.com/flashcatcloud/fc-site diff --check -- content/chinese/blog/<slug>.md
```

查看变更：

```shell
git status --short
git -C ~/go/src/github.com/flashcatcloud/fc-site status --short
```

## 11. 完成标准

一次文章更新完成前，至少满足：

- [ ] 插件 README、配置、代码、Dashboard、alerts 已核对。
- [ ] Dashboard JSON 已更新并通过 JSON 校验。
- [ ] 测试环境已采到真实指标。
- [ ] 夜莺或 Grafana Dashboard 已导入并确认有数据。
- [ ] 截图是 JPG，且没有敏感信息。
- [ ] 图片已上传到 `https://download.flashcat.cloud/categraf/`。
- [ ] 图片链接用 Referer 校验返回 200。
- [ ] 文章 front matter 已补 `image` 和 `og_image`。
- [ ] 正文已插入图片和 Dashboard 说明。
- [ ] 文章已同步到 fc-site。
- [ ] 总览文章、计划文档、workflow 状态按需更新。
- [ ] 敏感信息扫描通过。
- [ ] `git diff --check` 通过。
