# Claude Code 重构交接（2026-10-16）

用户要求先总结、提交并交接，改用 Claude Code 接续。Codex 在本交接后停止新增实现。
整体目标仍是继续推进重构，**没有宣布全量完成**。本文件是本批次的入口；后续须重新核对
Git/远端/运行时，不能只相信历史文字或把测试通过当作生产环境已升级。

## 0. Claude Code 接续后的最新状态（2026-10-16 晚，优先于下文）

同一工作树与分支 `codex/sqlite-c-advisory-foundation`；本节之前的HEAD为
`bf6d7f7343dfdd33f724c15099a4e2dab6ff06f7`（ls-remote精确核验），本交接提交以实际
`git rev-parse HEAD` 为准。`origin/main` 仍为 `24e55ba`。接续时先复现了下文第3节基线，结果一致。

| 提交 | 内容 |
| --- | --- |
| `52786f8` test / `fc289a0` docs | in-flight C step取消控制（读/写事务），合同第9节；WAL确定性调度未编写，如实保留未复现/open |
| `7a1bf82` build / `a376fc9` docs | go-sqlite3 v1.14.22→v1.14.36（SQLite 3.51.3），两项库级red同一断言转绿，合同第10节 |
| `6e008a9` docs / `97e9560` test / `6aeda8b` ci / `d1d774f` build / `bf6d7f7` docs | OS/C APK/secdb发布门与运行时zlib 1.3.2-r1构建期修复，合同第11、12节 |

当前测试：完整db包全绿；backend `go test ./...` 29包、`go vet ./...` 通过；关键race
`./api ./db ./services/...` 通过（api需 `-timeout 40m`，约1020s，默认10分钟会整体超时）；frontend按lock
`npm ci` 后788/788与build通过；门测试128/128（Node 26与本地pinned Node 22）；实际Go/npm扫描、Compose、
隔离存储HTTP合同通过。本会话沙箱禁止本地端口监听和写默认Go缓存：含端口的测试需在沙箱外运行，Go命令用
`GOCACHE=$TMPDIR/go-build`。

仍开放：WAL-reset确定性调度（未复现/open）；三视口浏览器、原生Linux arm64候选、新旧卷/portable、可信双平台
发布及OCI/binary/APK核验（合入main后由Actions执行，新增的两平台OS门会在推送前运行）；openssl 3.5.9-r0
已进签名索引但secdb暂无对应修复，未升级；parser/设备/全量矩阵其余项。下一步由用户决定是否按
`openreader-docker-release` 合入main；发布后核验两平台镜像的zlib 1.3.2-r1与SQLite 3.51.3。

Git：上述提交均已推送。镜像：未构建、未推送，最近已验证发布仍为fc4692c。生产：未访问、未修改。

**发布更新（取代上段的镜像状态）**：经用户确认，main快进至 `186c762`，可信Actions `38896494238` 全部成功，
`ghcr.io/changshengyu/openreader:186c762` 与 `latest` 为index
`sha256:bfda54269b4cf53bab46f1debdd3d9bef640f23c4de5a7f6b248ed081c43f83c`；两平台按digest独立核验zlib
1.3.2-r1与SQLite 3.51.3（详见合同第13节），本记录提交只改文档、不触发新镜像。生产仍未访问，实际运行版本
未知；下一步可做设备/三视口回归，WAL调度仍开放。

## 1. 从正确的工作树继续

当前有效目录：

```text
/Users/yuchangsheng/.codex/worktrees/sqlite-c-advisory-foundation/OpenReader-dev
```

分支 `codex/sqlite-c-advisory-foundation`。本交接文档提交前，代码/测试 HEAD 为
`b2f3cd9ad8417037553e5566513e344b8582c666`，工作树干净，已推送并通过 ls-remote 精确核验。
本交接本身将成为该分支的后继文档提交，以实际 `git rev-parse HEAD` 为准。
远端 main 本轮核验为 `24e55bab922e6c55206ef0a64d7fd64c02374da4`。

**不要误用旧工作树**：原始 `/Users/yuchangsheng/Documents/OpenReader-dev` 仍在旧 main
`2dfe80e`；聊天初始目录 `.../worktrees/f01e/OpenReader-dev` 也不是当前实现位置。
没有删除、重置、清理旧工作树或用户数据，不要对它们执行 reset/clean/prune。

先完整读取原仓库指令及匹配的 repo skills（这些本地指令不在新工作树的 tracked 文件里）：

```text
/Users/yuchangsheng/Documents/OpenReader-dev/AGENTS.md
/Users/yuchangsheng/Documents/OpenReader-dev/.agents/skills/
```

本批匹配：readerdev-compat-inventory、api-contract-compat、data-migration-compat、
openreader-backend、openreader-security、openreader-regression。发布时另读
openreader-docker-release；前端、解析器变化另读相应 skills。用户未授权子代理。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。
严格按 **合同/矩阵 → 独立实际红测 → 实现 → 全量回归 → 真实浏览器/卷 → 提交推送/适时发布**。
保护 API、认证、多用户、旧数字 Reader URL、进度/书签、逻辑备份、portable-v1/v2，
保留 data/cache/library；不做破坏迁移，不禁用 WAL 或改为单连接来绕过持久性问题。

用户已经明确允许向现有 changshengyu/openreader 推送并发布，但不是允许绕过发布门或修改生产。
生产在另一台 Mac，生产操作/SSH/升级/数据写入不在本轮范围内。

## 2. 本批完成的独立检查点

| 提交 | 内容 | 边界 |
| --- | --- | --- |
| `5bb35cf5d49f47c550e6083c78f0cbd664716581` | SQLite/WAL/OS-C 合同、矩阵及相关文档 | 在新增测试前独立推送核验 |
| `21abd585480113aa004e7edb84122487404c848d` | 两项真实 SQLite 库级红测及三项正常控制 | 在任何依赖实现前独立推送核验 |
| `3982b9f31333d31fcf095a6170910670e4bec765` | WAL test-only VFS 调度方式补充合同 | 已推送；调度探针尚未编写/执行 |
| `b2f3cd9ad8417037553e5566513e344b8582c666` | 连接生命周期、运行时特征、worker 故障/回收控制及记录 | 已推送核验；仍为测试阶段，不改应用/依赖 |

本批入口：

- `docs/compat/sqlite-wal-os-c-advisory-fixed-baseline-second-audit-p2-contract.md`，尤其第6–8节。
- `docs/compat/refactor-audit-matrix.md`，整体待办继续以矩阵为准。
- `docs/security-review-checklist.md`。
- `backend/db/sqlite_c_maintenance_contract_test.go`。
- `backend/db/sqlite_c_harness_contract_test.go`。

没有修改 go.mod/go.sum、应用 Go 文件、Dockerfile、workflow 或前端；没有发布本批新镜像。

## 3. 实际测试状态：有意保留两项红灯

当前 driver `github.com/mattn/go-sqlite3 v1.14.22`，嵌入 C SQLite `3.45.1`。
应用最低 Go 已是 `1.26.9`，AGENTS 里的 Go1.24 是历史描述，不可倒退。
从 backend 运行 Go，会按 go.mod 使用 Go1.26.9；仓库根目录直接 go version 可能是宿主1.26.5。

两项自有小表/空表反例稳定失败：

1. NOT NULL 空聚合返回 `(0,0)`，期望 `(1,0)`。
2. 两行冗余 REPLACE/双 ON CONFLICT 后表扫描计数2、索引路径计数3。

这是条件**库级 SQL 正确性问题**，不是已经证明当前 API 可达的漏洞，不是 WAL-reset 或 zlib
溢出复现，未碰用户数据库。不要为了让现有分支变绿而删除、跳过或改弱断言。

当前7项正常控制通过：绑定值/单 UPSERT、普通 NULL/空聚合、WAL 双连接 rollback/checkpoint/
reopen、准备语句长短 Unicode/NULL/空串复用、提前 Rows.Close 后复用、取消查询/事务后原值及
单连接池恢复、实际 C runtime SQL 特征查询。新增三项 driver 控制连续10轮通过。

4项 worker 保护验证连续3轮通过：17KiB 输出触发16KiB上限、2秒工作超时、SIGINT、SIGTERM。
验证准确拒绝原因、父测试正常失败退出1、自有 worker PID 已消失。仅向本次 Start 所得 PID
或新建自有进程组发信号，不按名称批量杀进程。保护失败/编译失败/启动失败不算库级红测。

完整 db 包：既有12个 root 测试通过，仅上述两叶失败，3.082秒。当前切片 race：同样2红/
7正常绿/4保护绿，9.962秒，未见 DATA RACE。`go vet ./db` 通过。既有 Go/npm 审计门的离线
合同测试109/109通过。这不是本轮实际全量依赖扫描或全镜像无漏洞证明。

原生 Linux arm64/官方 Go1.26.9/CGO1 同代码测试：768MiB、2CPU、128PID 硬限，源码及
module cache 只读、自有临时容器，仍2红/7正常绿/4保护绿，2.101秒。容器已自动移除。
实际 SQL 查询确认 version/source ID 与链接 C 一致；FTS3、FTS3_PARENTHESIS、RTREE、
THREADSAFE=1 存在，FTS5 未开启。是源码测试 binary，不是给发布服务增加 SQL 调试入口。
本轮检查未发现该 fixture 遗留 worker；所有已知测试/推送进程均已终态。

本批没有重跑全 Go、前端/build、浏览器或新旧卷：只改测试/文档，且仍有明确预期库级红灯。
这些门**未被豁免**，实现及发布时仍全部必须验证，不可红灯合入 main/发布。

复核命令（前两条当前应精确失败于两项已知库级反例，不可伪称通过）：

```bash
cd /Users/yuchangsheng/.codex/worktrees/sqlite-c-advisory-foundation/OpenReader-dev/backend
go test ./db -count=1 -v
go test -race ./db -run '^TestSQLiteCMaintenance(HarnessFaults|BoundedBehavior|OwnedWorker)$' -count=1 -v
go vet ./db
```

```bash
cd /Users/yuchangsheng/.codex/worktrees/sqlite-c-advisory-foundation/OpenReader-dev
node --test scripts/check-go-advisories.test.mjs scripts/check-frontend-advisories.test.mjs
```

本地原始证据保留（临时目录不保证长期存在；永久结论已写进仓库合同）：

```text
/private/tmp/openreader-sqlite-c-lifecycle-race.log
/private/tmp/openreader-sqlite-c-lifecycle-full-db.log
/private/tmp/openreader-sqlite-c-lifecycle-linux-arm64.log
/private/tmp/openreader-sqlite-c-lifecycle-gates.log
/private/tmp/openreader-sqlite-c-current-handoff.md
/private/tmp/openreader-frontend-advisory-current-handoff.md
```

后两个临时交接文件有历史段落，SQLite 临时文件尚停留更早检查点；本文件及最新合同第8节优先。

## 4. 下一步必须继续，而非已完成的事项

### SQLite/WAL

已核验最低候选 driver `v1.14.36` / SQLite `3.51.3`，精确 driver commit
`4ea2a9fb44c7ecc6eee436d26a32ab34a79a87ce`；候选仅缓存下载，没有应用到模块图。
校验和 `h1:EyQAxPedU6e4/lGrYcT2+1yVcOLT7ERS6jvTYKzfzgY=`。
不盲选更高版本跨到3.53语义系列；如改变候选先补合同与维护证据。

WAL-reset 必要条件（WAL+多连接）与当前应用相符，但**仍未复现临界顺序/损坏**。
普通并发绿灯、Go race、版本下限或现有两项 SQL 红测均不能替代该证明。

官方修复 SQLite commit `3c7a03032f9c69feb832b96447701bee6fa3ba81`、Fossil
`5aadfbbdd83b051b5f4a3f06eb60cf9b37aa1d62e676584719e6f785ecf148c7` 及
`test/walrestart.test` 已读，未执行下载的外部测试/PoC。机制为获取 read-lock0 后复核
live/cached WAL salt，避免回填已重置的新 WAL。

当前 C 的 `sqlite3WalCheckpoint` 读取 header 后、实际 checkpoint 前，header 变化且 VFS v3
时自然调用 xUnfetch。第7节已独立约定：仅在自有文件句柄复制/包装公开 I/O 方法表，底层
操作原样转发；在该回调让另一个自有连接执行正常短事务，控制顺序。不得改 SQLite 算法、
WAL/header、伪造 I/O 成功或使用用户数据。必须观测回调进入、salt/有效帧真实重置、后续较大
事务、checkpoint/reopen/integrity 与提交值；ABI/前提未满足不算损坏红灯。

**尚未编写临时 C 探针、仓库 test-only C helper 或 WAL 调度测试**。先安全、有界验证可行性，
再固定实际 Go 嵌入 C 测试；独立编译探针不是发布 binary/生产证明。参考官方公开接口：
[file_control](https://sqlite.org/c3ref/file_control.html)、
[io_methods](https://sqlite.org/c3ref/io_methods.html)。任何 helper 只能由 `_test` 引入，
不得进生产 main 依赖图，不增加 SQL 调试 API。需重新核对实际/候选公开 ABI。

当前取消控制只覆盖读行边界取消、事务自动回滚/连接恢复，**未证明正在执行 C step 时中断**。
这项仍需有界且可观测的 in-flight 控制，不能以重复当前绿测结案。

### OS/C

两份既有发布平台实际层/manifest/APK origin 已只读核验：16安装包/10origin，官方 Alpine
v3.23 main/community secdb 加原生 apk version 规则比较，不能用 semver/字符串比较替代。
当前已列待维护项 zlib `1.3.2-r0` → `1.3.2-r1`，维护者补丁针对非阻塞 gzwrite 错误路径；
没有当前 Go API 调用到 C gzwrite 的证据，不能称生产 RCE。5个未列 origin 是 unknown，不是 clean。
builder/Node 嵌入 C/复制 CA/其余 C 条件也未整体完成。

当时核验的官方 Alpine3.23.6、Go1.26.9-alpine3.23、Node22-alpine3.23 digest 未变，重拉同 tag
不能证明已修复。后续必须重新取证。若官方 base 尚无修复，合同只允许构建期同3.23系列、
签名校验的官方精确 zlib APK；两架构实际包字节/哈希/签名仍未核验。禁止运行时 apk upgrade、
全量 latest 或关闭签名校验。OS/C 门先写失败关闭的工具合同测试，再实现，不能先加忽略项。

原始盘点：`/private/tmp/openreader-os-c-inventory-eESDVj/report.json` 和
`/private/tmp/openreader-os-c-inventory-final.log`；临时只读原型
`/private/tmp/openreader-os-c-inventory.mjs` 不是已落库的正式发布门。

### 整体及发布

API/格式/挂载目录/多用户安全差异仅维持已签收适配，不扩大重构范围来规避测试。
整个 parser CPU/scan/峰值FD/late-tx、设备回归及全量矩阵仍开放，不把当前依赖切片当全项目完成。
实施后逐项跑同断言红转绿、正常控制、全Go/关键race/vet、前端/build、真实HTTP+SQLite/
三视口、旧DB/新旧卷/portable/备份/跨用户/重启、可信Actions双平台发布及实际OCI/binary/APK核验。

## 5. 已发布与生产状态，必须分开

最近已验证的应用发布是 **fc4692c474df30d1f0427cba57e7ebc67c91b156**，不是本批测试提交。
当时可信 Actions `38868093878` 成功；标签 `ghcr.io/changshengyu/openreader:fc4692c`
及当时的 latest 对应 OCI index：

```text
sha256:74ef48fd0c9f5547fff9787fb97763fff22e3500a376be05d5379f19dd8af765
```

当时两平台配置/binary/C来源/63份前端字节独立核验，实际拉回 ARM 镜像通过三视口 Reader/
原生上传及新旧卷/逻辑备份/重启；前端788/788、审计门109/109等证据在前端交接文件。
这是历史已验证交付，**本轮没有重新核验 Registry 的 latest 或生产实际运行 commit**。
main 的24e55ba为其后文档记录，不代表新的应用镜像。

用户曾确认早期旧书章节 `chapter-write` 409 故障已经恢复，该事件关闭；不要重复宣称仍在
修它、把当前 SQLite SQL fixture 当成当时生产故障，或再次要求用户升级来验证本批未发布内容。
生产另一台 Mac 的 commit 当前未知，未执行生产更改。后续每次报告继续分列 Git、镜像、生产。
