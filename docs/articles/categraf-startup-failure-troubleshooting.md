---
title: "Categraf 启动失败排查：TOML、配置路径、权限和 systemd 常见错误"
description: "本文从 Categraf 当前源码和 Linux 部署流程出发，给出启动失败的完整排查顺序，覆盖配置目录、TOML 语法与编码、文件权限、systemd 服务、二进制架构、版本兼容、端口冲突和日志定位。"
image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.svg"
og_image: "https://download.flashcat.cloud/blog-monitor-agent-categraf-introduction.png"
keywords: ["Categraf", "Categraf启动失败", "TOML", "systemd", "配置路径", "权限排查", "监控采集"]
author: "快猫星云"
date: "2026-07-17T00:00:00+08:00"
tags: ["Categraf", "Troubleshooting", "systemd"]
---

Categraf 安装完成后，最让新用户困惑的情况往往不是“某个指标不对”，而是服务刚启动就退出：`--status` 显示 stopped，systemd 反复拉起失败，或者终端只看到一条很长的 TOML 错误。

这类问题看起来很多，实际大多发生在几个固定阶段：二进制执行、配置目录定位、主配置解析、writer 初始化、Agent 创建和可选服务监听。只要按照启动顺序检查，通常不用反复修改插件配置，更不需要先去折腾 Dashboard。

本文基于 Categraf 当前仓库中的 `main.go`、`config/config.go`、`pkg/cfg`、`agent`、`writer` 和 Linux service 安装代码，整理一套可以直接执行的启动排障流程。配置层级的完整说明可以先参考：[Categraf 配置文件结构详解](/blog/categraf-configuration-structure-guide/)。

## 核心要点

- 先区分“进程启动失败”和“进程已运行但插件没指标”。单个 `input.*` 配置失败通常只跳过该插件，不一定让 Categraf 退出。
- 推荐先执行 Categraf 自带的 `--status`，再查看 systemd journal；`--status` 只能说明服务状态，不能证明配置有效。
- 默认配置目录是二进制旁边的 `conf`。二进制放在 `/opt/categraf/categraf` 时，默认读取 `/opt/categraf/conf`。
- 配置目录顶层的 TOML、JSON、YAML 会在主配置初始化阶段加载。`config.toml`、`logs.toml` 任意一个无法读取或解析，都可能导致进程直接退出。
- 推荐用 `--install`、`--start`、`--stop`、`--status`、`--remove` 管理服务。安装逻辑会生成 unit、启用服务并执行 `daemon-reload`。

## 1. 先判断是不是真的“启动失败”

排障前先给现象分类。下面几种情况处理路径完全不同：

| 现象 | 是否属于本文范围 | 下一步 |
| --- | --- | --- |
| 二进制无法执行 | 是 | 检查文件、权限和 CPU 架构 |
| 服务启动后立即变成 stopped/failed | 是 | 查 journal 和前台错误 |
| 报 `failed to init config` | 是 | 查配置目录、TOML 和权限 |
| 进程正常运行，但某个 input 没启动 | 否 | 查插件配置、实例、认证和网络 |
| `--test` 有指标，但后端查不到 | 否 | 查 writer 和 remote write 链路 |
| 后端有指标，但 Dashboard 空白 | 否 | 查数据源、变量、标签和 PromQL |

Categraf 的主配置在进程启动早期加载；这一阶段失败会直接退出。插件配置则由 Metrics Agent 后续逐个加载，单个插件解析、初始化或实例检查失败时，通常只记录错误并跳过该插件。

因此，判断启动是否成功不能只看“某个插件没有指标”。先确认进程和 Agent 是否仍然存活：

```shell
cd /opt/categraf
sudo ./categraf --status
systemctl is-active categraf
```

如果服务是 active，本文后面的主配置排查仍可参考，但重点应转向下一篇“插件没有指标”的排障流程。

## 2. 最短排查路径

遇到服务启动失败，可以先按下面顺序执行，不要一上来就重装：

```text
二进制能否执行
    |
    v
Categraf --status 与 systemd 状态
    |
    v
journal 中第一条致命错误
    |
    v
unit 的 ExecStart / WorkingDirectory / User
    |
    v
配置目录、config.toml、权限
    |
    v
前台 --test 验证
    |
    v
TOML 编码、语法和字段类型
    |
    v
writer、可选 Agent、端口和版本
```

对应命令：

```shell
cd /opt/categraf

./categraf --version
sudo ./categraf --status

systemctl status categraf --no-pager -l
journalctl -u categraf -b -n 200 --no-pager

systemctl show categraf \
  -p FragmentPath \
  -p ExecStart \
  -p WorkingDirectory \
  -p User
```

优先处理 journal 中最早出现的致命错误。后面的 “Start request repeated too quickly” 往往只是 systemd 多次重试后的结果，不是最初原因。

## 3. 为什么 `--status` 正常执行，服务仍然起不来

Categraf 的服务管理参数在主配置初始化之前处理：

```shell
sudo ./categraf --status
sudo ./categraf --start
sudo ./categraf --stop
```

所以即使 `config.toml` 已经损坏，`--status` 仍可能正常返回 “stopped”。这只能证明：

- 当前二进制可以运行；
- 能调用系统服务管理接口；
- 系统中存在或不存在名为 `categraf` 的服务。

它不能证明配置可以解析，也不能证明 Agent 可以启动。真正的配置错误要看：

```shell
journalctl -u categraf -b -n 200 --no-pager
```

如果服务反复重启，使用倒序或精确时间范围更容易找到第一条错误：

```shell
journalctl -u categraf -b --since "10 minutes ago" --no-pager
journalctl -u categraf -b -r --no-pager
```

## 4. 用前台启动拿到最直接的错误

systemd 日志已经足够时，不必重复前台运行。如果 journal 只有笼统状态，或者需要确认修改后的配置，先停止正式服务，再前台验证：

```shell
cd /opt/categraf
sudo ./categraf --stop
./categraf --test --inputs cpu
```

这个命令有三层价值：

1. 验证二进制和主配置能否加载；
2. 验证 local provider 能否发现 `input.cpu`；
3. 验证最基础的指标采集能否执行。

常见结果：

- 立即出现 `failed to init config`：先修配置路径、语法或权限；
- 出现 `failed to init writer`：检查 writer URL、TLS 文件和相关字段；
- 能持续打印 CPU 指标：主进程和基础 Metrics Agent 基本正常；
- 进程运行但目标插件报错：这已经进入插件级排障，不再是主进程启动问题。

需要注意，`--test` 只阻止普通指标进入 writer，并不会自动关闭 heartbeat、Logs Agent、Prometheus Agent 和 Ibex Agent。完全隔离测试时，应复制一份配置并关闭这些可选模块，避免测试进程连接生产端点或同时访问 WAL。

## 5. 配置目录为什么经常找错

Categraf 的 `--configs` 默认值是 `conf`。程序启动时会主动把当前工作目录切换到二进制所在目录，所以推荐布局为：

```text
/opt/categraf/
├── categraf
└── conf/
    ├── config.toml
    ├── logs.toml
    └── input.*/
```

在这个布局中，下面两条命令读取的是同一个目录：

```shell
/opt/categraf/categraf
/opt/categraf/categraf --configs /opt/categraf/conf
```

如果目标目录中没有 `config.toml`，错误通常很明确：

```text
F! failed to init config: configuration file(.../config.toml) not found
```

按下面顺序确认：

```shell
test -x /opt/categraf/categraf
test -f /opt/categraf/conf/config.toml
find /opt/categraf/conf -maxdepth 2 -type f | sort

systemctl show categraf -p ExecStart -p WorkingDirectory
```

最常见的配置路径错误有四种：

- 只复制了二进制，没有复制 `conf`；
- 二进制移动到新目录，但 systemd unit 仍指向旧路径；
- 手工传入了错误的 `--configs`；
- systemd 环境变量和手工终端环境不同。

如果配置确实放在其他目录，使用清晰的占位路径：

```shell
/opt/categraf/categraf --configs /path/to/categraf/conf
```

## 6. `config.toml` 存在，为什么仍然报主配置错误

Categraf 不只读取 `config.toml`。主配置初始化时会加载配置目录顶层的 TOML、JSON 和 YAML 文件。默认发行目录中，至少有：

```text
/opt/categraf/conf/config.toml
/opt/categraf/conf/logs.toml
```

顶层 TOML 会合并后再解析。因此以下情况都可能让主进程启动失败：

- `config.toml` TOML 语法错误；
- `logs.toml` TOML 语法错误；
- 顶层新增 TOML 与已有文件重复定义同一 table；
- 某个文件不可读；
- 文件不是 UTF-8，或者包含 NULL 字节；
- 字段类型无法转换到 Categraf 配置结构。

这也解释了一个常见误区：`config.toml` 看起来没问题，不代表整个主配置目录没问题。排查时要列出顶层全部配置：

```shell
find /opt/categraf/conf -maxdepth 1 -type f \
  \( -name '*.toml' -o -name '*.json' -o -name '*.yaml' -o -name '*.yml' \) \
  -print | sort
```

`input.*` 目录中的配置不会在这一步合并进主配置，而是在 Metrics Agent 启动后按插件分别读取。单个 input 配置损坏通常会看到 `failed to load configuration of plugin`，但主进程可能仍然运行。

## 7. TOML 最容易出错的地方

常见 TOML 问题包括：

**字符串少了引号**

```toml
[[writers]]
url = "http://127.0.0.1:17000/prometheus/v1/write
```

**table 头缺少右方括号**

```toml
[global.labels
region = "shanghai"
```

**把 TOML 和 YAML 写法混在一起**

```toml
# 错误示意：这不是 TOML 数组写法
providers:
  - local
```

**duration 单位无法解析**

```toml
# 错误示意
interval = "15seconds"

# 正确
interval = "15s"
```

Categraf 的 duration 支持整数秒、浮点秒和 Go duration，例如 `15`、`0.5`、`"500ms"`、`"2m"`。不要自行创造 `seconds`、`minutes` 等单位。

**在多个顶层文件重复定义 table**

例如 `config.toml` 已经有 `[global]`，又在 `custom.toml` 中重新写一个 `[global]`。两个文件单独检查都可能合法，合并后却会成为重复 table。

修改前建议保留行号和备份：

```shell
cp -a /opt/categraf/conf /opt/categraf/conf.backup
nl -ba /opt/categraf/conf/config.toml | sed -n '1,220p'
nl -ba /opt/categraf/conf/logs.toml | sed -n '1,220p'
```

## 8. 如何检查 UTF-8、NULL 字节和 TOML 语法

Categraf README 中记录过一种典型错误：配置文件在错误的打包工具或编辑流程中变成 UTF-16，启动时提示：

```text
files cannot contain NULL bytes; probably using UTF-16; TOML files must be UTF-8
```

先检查文件类型：

```shell
file /opt/categraf/conf/config.toml
file /opt/categraf/conf/logs.toml
```

如果服务器有 Python 3.11 及以上版本，可以逐个检查顶层 TOML 的 UTF-8、NULL 字节和语法：

```shell
python3 - <<'PY'
from pathlib import Path
import tomllib

root = Path("/opt/categraf/conf")
files = sorted(root.glob("*.toml"))

for path in files:
    data = path.read_bytes()
    if b"\x00" in data:
        raise SystemExit(f"NULL byte found: {path}")
    text = data.decode("utf-8")
    tomllib.loads(text)
    print(f"OK: {path}")

# Categraf 会合并顶层 TOML，再补一次合并检查。
merged = "\n".join(path.read_text(encoding="utf-8") for path in files)
tomllib.loads(merged)
print("OK: merged top-level TOML")
PY
```

这个脚本只能检查 TOML 和编码，不能验证所有 Categraf 字段语义。最终仍应以前台启动结果为准：

```shell
cd /opt/categraf
./categraf --test --inputs cpu
```

## 9. 文件权限应该怎么查

权限问题不只有“文件能不能读”。Linux 访问一个文件时，还要求父目录具有可遍历权限。启动阶段至少要检查：

- `/opt/categraf/categraf` 可执行；
- `/opt`、`/opt/categraf`、`conf` 可遍历；
- `config.toml`、`logs.toml` 和 TLS 文件可读；
- 自定义日志文件的父目录可写；
- Logs Agent 的 `run_path` 可写；
- Prometheus Agent 的 `wal_storage_path` 可写；
- Ibex Agent 的 `meta_dir` 可写。

先看 unit 以哪个用户运行：

```shell
systemctl show categraf -p User -p Group -p ExecStart
namei -l /opt/categraf/conf/config.toml
ls -l /opt/categraf/categraf /opt/categraf/conf/*.toml
```

systemd 的 `User` 为空时，系统级服务通常使用 root。自定义了运行用户时，应以同一个用户测试读取权限：

```shell
SERVICE_USER=$(systemctl show categraf -p User --value)
test -n "$SERVICE_USER" || SERVICE_USER=root

sudo -u "$SERVICE_USER" test -x /opt/categraf/categraf
sudo -u "$SERVICE_USER" test -r /opt/categraf/conf/config.toml
sudo -u "$SERVICE_USER" test -r /opt/categraf/conf/logs.toml
```

启用可选 Agent 时再检查写权限：

```shell
sudo -u "$SERVICE_USER" test -w /opt/categraf/run
sudo -u "$SERVICE_USER" test -w /opt/categraf/data-agent
sudo -u "$SERVICE_USER" test -w /opt/categraf/meta
```

不要为了快速绕过错误直接 `chmod -R 777 /opt/categraf`。正确做法是明确运行用户、目录属主和最小读写范围。

## 10. systemd 最常见的五类错误

推荐使用 Categraf 自带参数安装和管理服务：

```shell
cd /opt/categraf
sudo ./categraf --install
sudo ./categraf --start
sudo ./categraf --status
```

安装逻辑会生成 unit、启用服务并执行 `daemon-reload`。以下问题仍然比较常见。

**执行 `--install` 后忘了启动**

`--install` 不会自动启动进程，还需要：

```shell
sudo ./categraf --start
```

**先安装服务，后移动二进制**

unit 记录的是安装时的二进制绝对路径。移动后应重新安装：

```shell
cd /opt/categraf
sudo ./categraf --remove
sudo ./categraf --install
sudo ./categraf --start
```

**unit 已存在，再次执行 `--install`**

安装程序不会静默覆盖已有 unit。先用 `systemctl cat categraf` 确认来源；确实需要重建时，使用 `--remove` 后重新 `--install`，不要直接删除未知 unit。

**服务反复失败，触发启动频率限制**

先修复最早的配置或权限错误，再清理 failed 状态：

```shell
sudo systemctl reset-failed categraf
sudo ./categraf --start
sudo ./categraf --status
```

**手工 unit 与内置安装混用**

常规场景不要同时维护两套 unit。只有需要自定义 `User`、资源限制或安全加固时才手工修改；手工修改后由用户自己执行 `systemctl daemon-reload`。

## 11. 前台能启动，systemd 为什么失败

这是非常典型的环境差异问题。重点比较：

| 差异 | 手工终端 | systemd |
| --- | --- | --- |
| 运行用户 | 当前登录用户 | unit 的 `User` 或系统默认用户 |
| 环境变量 | shell profile 中的变量 | unit 和 EnvironmentFile |
| 配置路径 | 可能手工传入 `--configs` | unit 的 `ExecStart` |
| 工作目录 | shell 当前目录 | unit 设置后仍会被 Categraf 切到二进制目录 |
| 文件限制 | 当前 shell limit | unit 的 `LimitNOFILE` 等配置 |
| 日志 | 当前终端 | journal 或 `[log].file_name` |

直接检查 systemd 实际值：

```shell
systemctl cat categraf
systemctl show categraf \
  -p ExecStart \
  -p WorkingDirectory \
  -p User \
  -p Group \
  -p EnvironmentFiles
```

如果配置依赖 `${REGION}`、`${TOKEN}` 等环境变量，不要假设 systemd 会读取用户的 `.bashrc`。应通过受控的 EnvironmentFile、配置管理或密钥系统注入，并检查文件权限。

## 12. 二进制架构和版本兼容怎么查

在配置之前就失败时，先确认拿到的是正确二进制：

```shell
uname -m
file /opt/categraf/categraf
/opt/categraf/categraf --version
```

常见问题包括：

- 在 x86_64 主机上放了 ARM64 二进制，或反过来；
- 文件没有执行权限；
- 下载不完整或解压失败；
- 升级了二进制，却继续使用很久以前复制出来的配置模板；
- 回滚了二进制，但没有同步回滚新增配置字段或插件目录。

生产环境建议把“二进制 + conf”当成同一个发布单元。升级时从同一版本发行包取得配置模板，再把本地差异合并进去；不要用新二进制直接覆盖旧文件后立即全量重启。

版本兼容问题不一定表现为未知字段，也可能表现为字段类型、duration 格式、可选模块能力或插件配置结构不同。排查时记录：

```shell
/opt/categraf/categraf --version
sha256sum /opt/categraf/categraf
```

再与变更前的二进制和配置备份比较。

## 13. writer、可选 Agent 和端口也可能让进程退出

主配置解析通过后，Categraf 还会初始化 writer 和各类 Agent。以下问题值得检查：

- writer 的 HTTPS/TLS 配置引用了不存在或不可读的证书文件；
- `[global].providers` 配置了不支持的 provider；
- Prometheus Agent 的 YAML 无法解析；
- Prometheus Agent 的 WAL 目录不可写；
- 启用 `[http]` 后监听地址已被其他进程占用；
- Prometheus Agent 的 Web 地址与其他服务冲突；
- 自定义证书和私钥不匹配或不可读。

端口检查：

```shell
ss -lntup
```

如果日志出现 `address already in use`，先根据配置确认是 `[http].address`、Prometheus Web 地址，还是日志 TCP/UDP listener，再定位占用进程。不要只换一个随机端口掩盖重复部署问题。

需要区分：普通 `[[writers]].url` 对应后端暂时不可达，通常不会阻止 Categraf 进程完成启动，而是在发送时持续记录 remote write 错误。这属于后续“已采到指标但后端查不到”的链路排障。

## 14. 常见错误与原因速查

| 日志或现象 | 最可能原因 | 优先检查 |
| --- | --- | --- |
| `configuration file(.../config.toml) not found` | 配置目录错误或只复制了二进制 | `ExecStart`、`--configs`、文件路径 |
| `failed to load configs of dir` | 顶层配置不可读、编码或语法错误 | `config.toml`、`logs.toml`、权限 |
| `files cannot contain NULL bytes` | UTF-16 或文件损坏 | `file`、重新以 UTF-8 保存 |
| `failed to init writer` | writer URL 或 TLS 初始化失败 | URL、CA、证书、私钥权限 |
| `unsupported input provider` | `[global].providers` 值不受支持 | 使用 `local` 或正确配置 `http` |
| `no valid running agents` | Metrics Agent 创建失败，其他 Agent 也未启用 | provider 配置和构建能力 |
| `permission denied` | 文件、父目录或运行目录权限不足 | unit 用户、`namei -l`、读写权限 |
| `address already in use` | HTTP/Prometheus/listener 端口冲突 | `ss -lntup`、重复进程 |
| `input: ... not supported` | `input.*` 名称与当前二进制不匹配 | 插件目录名、版本和构建标签 |
| `failed to load configuration of plugin` | 单个插件配置解析失败 | 对应 `input.<plugin>`，进程可能仍正常 |
| 服务不断 restart | `Restart=on-failure` 正在重试 | journal 中最早的致命错误 |

这个表用于缩小范围，最终仍要结合完整日志上下文判断。不要只截取最后一行，因为最后一行经常只是 systemd 的失败汇总。

## 15. 一套可复制的修复流程

下面是一套相对稳妥的处理顺序：

```shell
cd /opt/categraf

# 1. 记录版本和服务状态
./categraf --version
sudo ./categraf --status

# 2. 保存配置备份
sudo cp -a conf "conf.backup.$(date +%Y%m%d%H%M%S)"

# 3. 查看最近启动日志和实际 unit
journalctl -u categraf -b -n 200 --no-pager
systemctl show categraf -p ExecStart -p WorkingDirectory -p User

# 4. 停止重试中的服务
sudo ./categraf --stop

# 5. 修复路径、TOML、权限或版本问题后前台验证
./categraf --test --inputs cpu

# 6. 恢复服务
sudo systemctl reset-failed categraf
sudo ./categraf --start
sudo ./categraf --status

# 7. 再看启动后的日志
journalctl -u categraf -b -n 100 --no-pager
```

第 5 步没有通过前，不要反复执行第 6 步。否则 systemd 只会继续制造重复日志，掩盖最初错误。

如果修复涉及更换二进制位置或重建 unit，再使用：

```shell
sudo ./categraf --remove
sudo ./categraf --install
sudo ./categraf --start
sudo ./categraf --status
```

## 16. 常见问题

**为什么 `[log].file_name` 指向的日志文件是空的？**

主配置解析和 writer 初始化发生在 Agent 正式运行日志初始化之前。进程如果在更早阶段退出，错误可能只出现在终端或 systemd journal。先查 `journalctl -u categraf`。

**为什么执行 `--install` 后 `--status` 还是 stopped？**

`--install` 负责写入 unit、启用服务和刷新 systemd，但不会自动启动。继续执行 `sudo ./categraf --start`。

**为什么重新执行 `--install` 提示服务已经存在？**

安装逻辑不会覆盖已有 unit。先用 `systemctl cat categraf` 确认内容；需要重建时执行 `--remove`，再执行 `--install`。

**使用内置 `--install` 后还要 `systemctl daemon-reload` 吗？**

不需要。安装和卸载流程会刷新 systemd。只有手工新建或修改 unit 时，才需要自己执行 `daemon-reload`。

**为什么手工运行正常，systemd 启动失败？**

优先比较运行用户、环境变量、`ExecStart`、配置路径和文件权限。特别是 shell 中存在的环境变量，systemd 默认不一定能继承。

**为什么配置错误修复后还是提示启动过快？**

systemd 可能已经触发启动频率限制。修复根因后执行 `systemctl reset-failed categraf`，再使用 `./categraf --start`。

**为什么进程 active，但目标插件没有指标？**

这不是主进程启动失败。下一步应检查 `input.<plugin>` 是否存在、是否有有效实例、`--inputs` 是否过滤、目标网络和认证是否正常。

**writer 后端连接失败会让 Categraf 启动失败吗？**

后端暂时不可达通常不会阻止进程启动；TLS 初始化、URL 构造等本地配置错误则可能在 writer 初始化阶段失败。网络写入错误将在下一篇 remote write 排障文章中展开。

## 17. 如何减少生产环境启动失败

生产环境可以用下面的约束降低故障概率：

- 二进制和 `conf` 作为同一个版本化发布单元；
- 在测试节点先执行 `--test --inputs cpu`，再灰度重启；
- 配置进入版本控制，密钥通过受控方式注入；
- 修改前保留可立即恢复的配置和二进制；
- 服务统一使用 `--install`、`--start`、`--stop`、`--status`、`--remove` 管理；
- 启用可选 Agent 时显式规划 WAL、状态和任务目录权限；
- 监控 Categraf 进程状态、重启次数和本机磁盘；
- 不在所有节点同时升级，先观察一小批节点的 journal 和指标。

对于大规模环境，启动成功只是第一步。还应把“配置版本、二进制版本、变更批次、回滚包”一起纳入配置管理和发布系统。

## 18. 小结

Categraf 启动排障可以压缩成一句话：**先看服务状态，再找 journal 中第一条致命错误，然后沿“二进制 -> unit -> 配置目录 -> 顶层配置 -> 权限 -> writer 与 Agent”逐段验证。**

最重要的边界是：

```text
主配置失败 -> 进程通常直接退出
单个 input 失败 -> 通常只跳过该插件
writer 后端不可达 -> 进程通常仍然运行
Dashboard 无数据 -> 已经是更下游的问题
```

把这四类问题分开，排障效率会高很多。下一篇继续讲：[Categraf 插件没有指标：从 test 模式到数据库权限的完整排查](/blog/categraf-plugin-no-metrics-troubleshooting/)。

---

**内容更新时间**：2026-07-17

**证据边界**：启动顺序、配置加载、服务参数和错误边界来自当前 Categraf 仓库源码及默认配置；systemd 命令以常见 Linux 发行版为例，自定义构建、旧版本或非 systemd 系统的表现可能不同。
