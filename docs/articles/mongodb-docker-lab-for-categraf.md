---
title: "MongoDB Docker 测试环境搭建：单节点、副本集和分片集群"
description: "本文记录如何用 Docker Compose 搭建用于 Categraf MongoDB 监控验证的测试环境，覆盖单节点、三节点副本集、config server、shard replica set、mongos、认证账号和测试数据。"
keywords: ["MongoDB", "Docker Compose", "副本集", "分片集群", "Categraf", "监控测试环境"]
author: "快猫星云"
date: "2026-07-09T00:00:00+08:00"
tags: ["MongoDB", "Docker", "Categraf"]
---

MongoDB 监控文章不能只拿一个单机实例做示例。生产里的 MongoDB 常见形态至少包括单节点、副本集和分片集群，而 Categraf 的 `mongodb` 插件在这些拓扑下采集边界也不一样：`mongod`、`mongos`、config server、shard server 能看到的指标并不完全相同。

这篇文章先记录测试环境搭建过程，后续监控实战文章直接复用这个环境。

## 环境目标

本次测试环境部署在一台 Linux 测试机上，通过 Docker Compose 启动：

| 拓扑 | 组件 | 宿主机端口 |
| --- | --- | --- |
| 单节点 | `mongo-single` | `37017` |
| 普通副本集 | `mongo-rs-1` / `mongo-rs-2` / `mongo-rs-3` | `37117` / `37118` / `37119` |
| 分片 config server | `mongo-cfg-1` / `mongo-cfg-2` / `mongo-cfg-3` | `37217` / `37218` / `37219` |
| shard01 | `mongo-shard01-a` / `mongo-shard01-b` / `mongo-shard01-c` | `37317` / `37318` / `37319` |
| shard02 | `mongo-shard02-a` / `mongo-shard02-b` / `mongo-shard02-c` | `37417` / `37418` / `37419` |
| 路由层 | `mongos` | `37517` |

端口只绑定到 `127.0.0.1`，方便在同一台测试机上运行 Categraf，同时避免把测试 MongoDB 暴露到外部网络。

## 创建目录和 keyfile

先准备独立目录：

```shell
mkdir -p ~/categraf-mongodb-lab/{single-keyfile,replset-keyfile,sharded-keyfile,scripts,categraf-conf/input.mongodb}
openssl rand -base64 756 > ~/categraf-mongodb-lab/replset-keyfile/keyfile
cp ~/categraf-mongodb-lab/replset-keyfile/keyfile ~/categraf-mongodb-lab/single-keyfile/keyfile
cp ~/categraf-mongodb-lab/replset-keyfile/keyfile ~/categraf-mongodb-lab/sharded-keyfile/keyfile
chmod 400 ~/categraf-mongodb-lab/*-keyfile/keyfile
```

MongoDB 开启副本集认证时需要 keyfile。这里单节点也使用 keyfile，是为了让三类拓扑的认证方式保持一致。

## Docker Compose 配置

`docker-compose.yml` 使用 MongoDB 7.0 镜像。官方镜像启动时会切换到 `mongodb` 用户，如果直接把宿主机 0400 权限的 keyfile bind mount 进去，容器内用户可能读不到文件。因此这里在容器启动时复制 keyfile 到 `/tmp/mongo-keyfile`，再设置属主和权限。

下面是核心写法。完整环境可以按同样模式扩展到所有 config server 和 shard 节点。

```yaml
x-mongod: &mongod
  image: mongo:7.0
  entrypoint: ["/bin/bash", "-lc"]
  networks: [mongo-lab]

services:
  mongo-single:
    <<: *mongod
    container_name: categraf_mongo_single
    hostname: mongo-single
    command:
      - "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && exec gosu mongodb mongod --bind_ip_all --auth --keyFile /tmp/mongo-keyfile"
    ports:
      - "127.0.0.1:37017:27017"
    volumes:
      - single-data:/data/db
      - ./single-keyfile/keyfile:/etc/mongo-keyfile/keyfile:ro

  mongo-rs-1:
    <<: *mongod
    container_name: categraf_mongo_rs_1
    hostname: mongo-rs-1
    command:
      - "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && exec gosu mongodb mongod --bind_ip_all --replSet rs0 --auth --keyFile /tmp/mongo-keyfile"
    ports:
      - "127.0.0.1:37117:27017"
    volumes:
      - rs1-data:/data/db
      - ./replset-keyfile/keyfile:/etc/mongo-keyfile/keyfile:ro

  mongo-cfg-1:
    <<: *mongod
    container_name: categraf_mongo_cfg_1
    hostname: mongo-cfg-1
    command:
      - "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && exec gosu mongodb mongod --bind_ip_all --configsvr --replSet cfgRepl --port 27017 --auth --keyFile /tmp/mongo-keyfile"
    ports:
      - "127.0.0.1:37217:27017"
    volumes:
      - cfg1-data:/data/configdb
      - ./sharded-keyfile/keyfile:/etc/mongo-keyfile/keyfile:ro

  mongo-shard01-a:
    <<: *mongod
    container_name: categraf_mongo_shard01_a
    hostname: mongo-shard01-a
    command:
      - "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && exec gosu mongodb mongod --bind_ip_all --shardsvr --replSet shard01 --port 27017 --auth --keyFile /tmp/mongo-keyfile"
    ports:
      - "127.0.0.1:37317:27017"
    volumes:
      - shard01a-data:/data/db
      - ./sharded-keyfile/keyfile:/etc/mongo-keyfile/keyfile:ro

  mongos:
    <<: *mongod
    container_name: categraf_mongo_mongos
    hostname: mongos
    command:
      - "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && exec gosu mongodb mongos --bind_ip_all --configdb cfgRepl/mongo-cfg-1:27017,mongo-cfg-2:27017,mongo-cfg-3:27017 --keyFile /tmp/mongo-keyfile"
    ports:
      - "127.0.0.1:37517:27017"
    volumes:
      - ./sharded-keyfile/keyfile:/etc/mongo-keyfile/keyfile:ro
    depends_on:
      - mongo-cfg-1
      - mongo-cfg-2
      - mongo-cfg-3
      - mongo-shard01-a
      - mongo-shard01-b
      - mongo-shard01-c
      - mongo-shard02-a
      - mongo-shard02-b
      - mongo-shard02-c

networks:
  mongo-lab:
    name: categraf-mongodb-lab

volumes:
  single-data:
  rs1-data:
  rs2-data:
  rs3-data:
  cfg1-data:
  cfg2-data:
  cfg3-data:
  shard01a-data:
  shard01b-data:
  shard01c-data:
  shard02a-data:
  shard02b-data:
  shard02c-data:
```

实际搭建时，需要把 `mongo-rs-2`、`mongo-rs-3`、`mongo-cfg-2`、`mongo-cfg-3`、`mongo-shard01-b`、`mongo-shard01-c`、`mongo-shard02-a`、`mongo-shard02-b`、`mongo-shard02-c` 按同样模式补齐，只改 `container_name`、`hostname`、端口、volume 和 `--replSet` 名称。

## 为什么需要 bootstrap 模式

如果容器一开始就带 `--auth --keyFile` 启动，在某些镜像和启动方式下，localhost exception 不一定适合用来创建首个用户。更稳的做法是两阶段：

1. bootstrap 模式：不带 `--auth` 和 `--keyFile`，初始化副本集、分片和用户；
2. auth 模式：保留数据卷，切回带认证的 `docker-compose.yml`。

可以从认证版 Compose 派生一个 bootstrap Compose，把命令中的 keyfile 和 auth 参数去掉：

```shell
python3 - <<'PY'
from pathlib import Path

p = Path("docker-compose.yml")
s = p.read_text()
prefix = "cp /etc/mongo-keyfile/keyfile /tmp/mongo-keyfile && chmod 400 /tmp/mongo-keyfile && chown mongodb:mongodb /tmp/mongo-keyfile && "
s = s.replace(prefix, "")
s = s.replace(" --auth --keyFile /tmp/mongo-keyfile", "")
s = s.replace(" --keyFile /tmp/mongo-keyfile", "")
Path("docker-compose.bootstrap.yml").write_text(s)
PY
```

启动 bootstrap：

```shell
cd ~/categraf-mongodb-lab
docker compose -f docker-compose.bootstrap.yml up -d
```

## 初始化单节点和副本集

下面示例使用占位密码，实际执行时替换成测试环境密码，不要把真实密码写进文章或仓库。

```shell
ROOT_USER="root"
ROOT_PASS="<ROOT_PASSWORD>"
MON_USER="categraf"
MON_PASS="<MONITOR_PASSWORD>"
```

创建监控账号：

```javascript
const admin = db.getSiblingDB("admin");
admin.createUser({
  user: "root",
  pwd: "<ROOT_PASSWORD>",
  roles: [{ role: "root", db: "admin" }]
});
admin.createUser({
  user: "categraf",
  pwd: "<MONITOR_PASSWORD>",
  roles: [
    { role: "clusterMonitor", db: "admin" },
    { role: "read", db: "local" },
    { role: "readAnyDatabase", db: "admin" }
  ]
});
```

普通副本集初始化：

```shell
docker exec categraf_mongo_rs_1 mongosh --quiet --eval '
  rs.initiate({
    _id: "rs0",
    members: [
      { _id: 0, host: "mongo-rs-1:27017", priority: 2 },
      { _id: 1, host: "mongo-rs-2:27017", priority: 1 },
      { _id: 2, host: "mongo-rs-3:27017", priority: 1 }
    ]
  })'
```

等待选主：

```shell
docker exec categraf_mongo_rs_1 mongosh --quiet --eval 'rs.status().members.map(m => ({name:m.name,state:m.stateStr,health:m.health}))'
```

## 初始化分片集群

config server：

```shell
docker exec categraf_mongo_cfg_1 mongosh --quiet --eval '
  rs.initiate({
    _id: "cfgRepl",
    members: [
      { _id: 0, host: "mongo-cfg-1:27017" },
      { _id: 1, host: "mongo-cfg-2:27017" },
      { _id: 2, host: "mongo-cfg-3:27017" }
    ]
  })'
```

两个 shard replica set：

```shell
docker exec categraf_mongo_shard01_a mongosh --quiet --eval '
  rs.initiate({
    _id: "shard01",
    members: [
      { _id: 0, host: "mongo-shard01-a:27017" },
      { _id: 1, host: "mongo-shard01-b:27017" },
      { _id: 2, host: "mongo-shard01-c:27017" }
    ]
  })'

docker exec categraf_mongo_shard02_a mongosh --quiet --eval '
  rs.initiate({
    _id: "shard02",
    members: [
      { _id: 0, host: "mongo-shard02-a:27017" },
      { _id: 1, host: "mongo-shard02-b:27017" },
      { _id: 2, host: "mongo-shard02-c:27017" }
    ]
  })'
```

通过 `mongos` 添加 shard：

```shell
docker exec categraf_mongo_mongos mongosh --quiet --eval \
  'sh.addShard("shard01/mongo-shard01-a:27017,mongo-shard01-b:27017,mongo-shard01-c:27017")'

docker exec categraf_mongo_mongos mongosh --quiet --eval \
  'sh.addShard("shard02/mongo-shard02-a:27017,mongo-shard02-b:27017,mongo-shard02-c:27017")'
```

如果后续要让 Categraf 直连 shard 节点采集，也要在 shard replica set 上创建同名监控账号。推荐用副本集 URI，让客户端自动找到当前 Primary：

```shell
docker exec categraf_mongo_shard01_a mongosh --quiet \
  "mongodb://mongo-shard01-a:27017,mongo-shard01-b:27017,mongo-shard01-c:27017/admin?replicaSet=shard01" \
  --eval 'db.getSiblingDB("admin").createUser({
    user: "categraf",
    pwd: "<MONITOR_PASSWORD>",
    roles: [
      { role: "clusterMonitor", db: "admin" },
      { role: "read", db: "local" },
      { role: "readAnyDatabase", db: "admin" }
    ]
  })'
```

## 写入测试数据

单节点和普通副本集可以写入简单订单数据：

```javascript
const d = db.getSiblingDB("categraf_demo");
d.orders.createIndex({ user_id: 1, created_at: -1 });
for (let i = 0; i < 200; i++) {
  d.orders.updateOne(
    { order_id: i },
    {
      $set: {
        order_id: i,
        user_id: i % 20,
        amount: i * 3.14,
        status: i % 3 === 0 ? "paid" : "created",
        created_at: new Date(Date.now() - i * 60000)
      }
    },
    { upsert: true }
  );
}
```

分片集群可以创建一个简单分片集合：

```javascript
sh.enableSharding("categraf_demo");
db.getSiblingDB("categraf_demo").events.createIndex({ event_id: 1 });
sh.shardCollection("categraf_demo.events", { event_id: 1 });
for (let i = 0; i < 500; i++) {
  db.getSiblingDB("categraf_demo").events.updateOne(
    { event_id: i },
    { $set: { event_id: i, shard_key: i % 10, ts: new Date() } },
    { upsert: true }
  );
}
```

## 切回认证模式

bootstrap 初始化完成后，保留数据卷，切回认证版 Compose：

```shell
cd ~/categraf-mongodb-lab
docker compose -f docker-compose.bootstrap.yml down
docker compose up -d
```

注意这里不要加 `-v`，否则刚刚初始化的数据和用户会被删除。

## 验证监控账号

单节点：

```shell
docker exec categraf_mongo_single mongosh --quiet \
  -u categraf -p "<MONITOR_PASSWORD>" --authenticationDatabase admin \
  --eval 'db.adminCommand({ serverStatus: 1 }).ok'
```

普通副本集：

```shell
docker exec categraf_mongo_rs_1 mongosh --quiet \
  -u categraf -p "<MONITOR_PASSWORD>" --authenticationDatabase admin \
  --eval 'db.adminCommand({ replSetGetStatus: 1 }).ok'
```

`mongos`：

```shell
docker exec categraf_mongo_mongos mongosh --quiet \
  -u categraf -p "<MONITOR_PASSWORD>" --authenticationDatabase admin \
  --eval 'db.adminCommand({ serverStatus: 1 }).ok'
```

shard 直连：

```shell
docker exec categraf_mongo_shard01_a mongosh --quiet \
  -u categraf -p "<MONITOR_PASSWORD>" --authenticationDatabase admin \
  --eval 'db.adminCommand({ serverStatus: 1 }).ok'
```

这些命令返回 `1`，说明认证和基础监控权限正常。

## 本次验证结果

本次在测试机上完成了以下验证：

- 单节点 `mongo-single:37017`：监控账号可执行 `serverStatus`；
- 普通副本集 `rs0`：Primary / Secondary 状态正常，监控账号可执行 `replSetGetStatus`；
- 分片集群：`cfgRepl`、`shard01`、`shard02` 均完成选主，`mongos` 成功添加两个 shard；
- `mongos:37517`、`mongo-cfg-1:37217`、`mongo-shard01-a:37317`、`mongo-shard02-b:37418` 均可被 Categraf 直连采集；
- `collect_all = true` 场景下，监控账号需要 `readAnyDatabase` 或目标库 `read` 权限，否则库表发现会出现 `listCollections` 未授权。

## 常见问题

**keyfile bad file**

通常是容器内 MongoDB 进程读不到 bind mount 的 keyfile。不要简单把 keyfile 改成 0644。更稳的做法是在容器启动时复制到容器内路径，再设置 `mongodb:mongodb` 属主和 0400 权限。

**等待固定节点成为 Primary 导致脚本卡住**

副本集选主不保证落在指定容器。初始化脚本不要写死等待 `mongo-shard02-a` 成为 Primary，应该判断任一成员为 `PRIMARY`，或者用 replica set URI 让客户端自动连接当前 Primary。

**通过 mongos 创建的用户不能用于直连 shard**

如果 Categraf 只采集 `mongos`，通过 `mongos` 创建监控账号即可。如果还要直连 shard server 采集副本集和节点指标，需要在 shard replica set 上也创建或补齐同名监控账号。

**`collect_all = true` 权限不足**

`clusterMonitor` 和 `read local` 可以覆盖很多基础指标，但 `collect_all` 会打开库表和索引发现。要么给监控账号补 `readAnyDatabase`，要么只对需要的库表配置 `coll_stats_namespaces`、`index_stats_collections` 并授予目标库 `read` 权限。
