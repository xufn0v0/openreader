# 本地书导入持久提交全生命周期第二轮固定基准合同（P2）

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
审查日期：2026-10-10；OpenReader 源码基线：`9cb6a030b9b2c6c5e6f55d2b19dcc719b554571a`。  
状态：**core-regression-validated / Docker-published / follow-up-open / awaiting-device-verification**。

初始合同段落保留为历史；第8–9节记录独立红测，第10–11节记录 checkpoint，第12节为最终发布证据。

本提交只盘点合同，不修改应用或测试。源码窗口不是已复现漏洞，也不是生产故障结论。
已恢复的原书事件保持关闭；stage 应用 b2b32f7 已发布，不等于本项持久提交已签收。

## 1. 固定上游、共享入口和不可改变的结果

以下均以 `git show` 读取固定 SHA，而非 upstream 工作副本 HEAD：

- `BookController.kt:212–300`：认证、TXT/EPUB/UMD/CBZ 上传、namespace 工作文件、目录预览和
  同工作文件改规则；空目录仍可确认，preview 不保存书架。
- 同文件 `2263–2323`：LocalStore/WebDAV 权限、源文件存在检查，回到共同 preview/confirm 流。
- 同文件 `1189–1335#saveBook`：校验 origin/URL、caller namespace、本地原文件和 EPUB/CBZ 资源、
  book source 与 shelf 保存；提交 Book 包含 group。相同书名/作者合并已有阅读进度。
- `web/src/views/Index.vue:1512–1554`：只有保存成功才关闭导入框、提示成功并刷新书架。
- 同文件 `2310–2415`：统一分组应用于每个提交 Book，批量逐项保存、逐本确认和取消当前项继续。

固定上游没有证明 SQLite↔文件系统原子事务、request cancellation 或 exactly-once；其同名原文件
覆盖不能复制到 OpenReader。数字 Book ID、同名独立归档、唯一 reader-dev placeholder 原位恢复、
many-to-many 分组、失败保留 stage、多用户私有根和已签收 parser/Reader/backup 是允许技术适配。

本项覆盖四条 confirm 路由：`POST /api/imports/books`、`/api/imports/txt`、
`/api/local-store/import`、`/api/webdav/import`；raw/token、new/placeholder 两条 service 路径。
边界从已取得有界源 bytes/prepared catalogue 到：新归档分配→原文件→正文/资源→Book/Chapter/
分组事务→source/TOC→提交确认→成功响应/事件与原 stage best-effort consume。
共享 `RestoreExisting` 的 portable consumer 必须回归，不能用“只改 HTTP”留下第二套不安全归档。

## 2. 当前调用链与分类矩阵

| 生命周期 | 当前源码行为（9cb6a03） | 裁决 |
|---|---|---|
| archive allocation | `engine/library.go#ArchiveImportedBook/uniqueDirectory`：Stat 找空名字，绝对 MkdirAll/WriteFile；并发检查并非 exclusive directory ownership。 | **must-fix**：configured LibraryDir/data/user 原身份、独占分配新书目录，不能覆盖同时出现的同名对象。 |
| 原文件和初始失败 | 原文件直接绝对 WriteFile；失败在 Importer 接收 archive 之前，没有统一 partial compensation。 | **must-fix**：同一原 session 有界实际写、ctx、owned partial rollback，关闭全部 handles。 |
| 正文和 metadata | `importer.go` 事务中调用 Background `WriteChapterCache`，随后绝对 source/TOC JSON 写入；每个 helper 重新接收 pathname。 | **must-fix**：贯穿新归档的原 parent/book/entry identity；每步实际 I/O 使用 request context，不能把独立安全 helper 串接当完整生命周期。 |
| EPUB/CBZ resources | 先 `PrepareBookResources`，独立 source/root admission、extraction staging/rename；不接收 importer context/session。 | **must-fix**：导入初始化的实际源读取、展开、发布属于同归档 ownership/context；保持现有限额/marker/布局和已签收 Reader 服务。 |
| Book/Chapter | db.Transaction 无 request context；new Create/Save URL，placeholder reread 三字段后整行 Save。 | **must-fix**：同 request-context item transaction、owner/target 复验、guarded owned columns、无 fallback resurrection。 |
| 所选分组 | ImportRequest 只传 primary；四 handler 在 Book 提交后 `_ = setBookCategories(s.db,...)`。helper 先 delete 再逐行 create，错误被忽略。 | **must-fix**：全 selected memberships 和 legacy primary 属于同 item transaction，失败不能报成功或留下部分关系。 |
| token handoff | direct 在 category 后 consume；storage helper 在 category 前已经 consume。 | **must-fix**：只有包括所选分组的整个 item durable 才 consume；失败原 token/prepared 可重试。 |
| rollback | new/placeholder 两路径均 defer 绝对 `os.RemoveAll(archiveRoot)`。 | **must-fix**：原自有实体补偿，未知替换目录/成员/外部 bait 必须保留，不能以安全拒绝为理由盲删。 |
| response/event | 失败的分组写入仍可能有 201/200 Book 和 bookshelf_update；storage raw durability 可泄漏 err.Error。 | **must-fix**：仅 authoritative durable items；固定安全内部错误，先前成功项独立保留/通知。 |
| parser/old data | 既有 format/encoding/rule、Prepared Matches、空 TOC、旧两文件 stage、数字 URL、旧卷/portable。 | **aligned controls**：不重新设计，不以迁移/重新上传代替修复。 |

已签收 `local-book-archive-filesystem-lifecycle-...`@125fd93 的对象是**已导入** Reader、refresh、
export/delete，不包括这里的新 ArchiveImportedBook allocation 与 importer rollback。
已签收 stage b2b32f7 绑定 cache/bundle，不证明后续 library/SQL/category 原子取消。
PUT metadata/category、batch category 合同也不覆盖 import confirmation，不能用其绿灯代替本项。
旧 local-book-import-catalog P0 的 broad category-compensation claim 只保留为历史记录；本项重新取证。

## 3. API 和逐项 durable 合同

1. 保持 JWT/active-user、LocalStore/WebDAV capability、body/multipart/文件/rule/200项限额和
   validation priority、请求字段、原 numeric URL 和正常 response shape。direct 为 201 Book shelf
   projection；storage 为 200 `{imported:[{path,book|error}]}`，不新增全批 transaction。
2. 正常 parser、格式、raw overflow、invalid token、stage read/write 的既有 400/413/500/item
   mapping 不变。纯 SQL/archive/resource durability 的内部失败：direct 固定 500
   `{"error":"failed to import book"}`；storage 200 的当前项固定 `error:"failed to import book"`，
   不泄漏 PathError、宿主路径、用户名目录、SQLite、资源成员或内部 callback 文本。
3. category 前置不存在/foreign 继续既有 400 `category not found`；提交期间 category identity
   或数据库读写失败属于安全 durability failure，不静默降级为空分组或创建孤儿关联。
4. 本项实际归档/资源 I/O 或 SQL commit 前观察到 caller canceled/deadline，停止本请求后续项，
   整体固定 500 `{"error":"local book import canceled"}`，不新增 499。direct 可以在安全交付时
   加既有 `importToken`，不伪造新 token。原 stage cancellation 文案仍归原 stage service 所有。
5. 每 item 的 Book、Chapter、所选 BookCategory、legacy primary 在同一 transaction；source/TOC/
   正文/资源已完整写到本请求独占归档并在 commit 前复验原身份。写入、重载或 commit 失败都不
   留当前项 row/partial relation，不返回或广播该失败项。正常失败也不能 consume stage。
6. batch 第 k 项失败不撤销 1..k-1 的 durable Books；普通失败按既有规则继续，取消整请求停止。
   response/event 只能含实际成功集合，之前成功项在后来取消时仍准确通知。零成功不广播空成功。
7. SQLite 已确认 commit 后才允许成功通知/consume。commit 后 ctx/consume/cleanup 受阻不得伪造
   回滚或失败、删除已 durable Book、吞掉成功事件；原 stage unknown newcomer 保留。仍不承诺
   崩溃、跨进程或 retry-after-commit 的 exactly-once。

## 4. 新归档原实体、资源、取消与失败补偿

1. 文件工作前检查 ctx。LibraryDir 是 configured trust boundary；从原 trusted anchor 初始化合法
   缺失目录，绑定 library/data/safe-user 的原 opened identity，不把 resolved user root 作为新信任根。
   stable/late symlink、特殊文件、祖先 replacement、不可用原权限 fail closed；不修复/chmod 老卷。
2. 新书目录必须 exclusive/no-replace 创建，同名普通书仍按当前 friendly `_2/_3` 规则选择下一名字。
   不覆盖现存目录、原文件、metadata、资源或另一请求新实体；ownership 来自实际创建并验证的
   inode，不是“Stat 不存在”或“已接收 inode”。并发同名导入只可各得独立目录或安全失败。
3. 创建起持有一个原归档 session，贯穿原文件、content、resource temporary/marker/publication、
   source/TOC、durable 前复验和失败补偿。no-follow/nonblock regular handle 实际读取/写入；
   检查后不得按绝对 pathname 重开到 replacement。仅最终文件合法 rename 可以消费原 held bytes，
   但新同名实体不能被覆盖/删除。原祖先变更不能被另一 helper 的重新 admission 接受。
4. 原 bytes、EPUB/CBZ entry count/entry bytes/total expanded/text/chapter 等现有预算继续有效，
   实际 bounded read/write/extraction copy 前后和每章节/SQL步骤观察 caller ctx；所有限额 +1 与
   MaxInt64 饱和探测不溢出。完整 parser CPU/regex/HTML DOM interrupt 仍独立 unknown，最终检查
   或只给 db.WithContext 不等于实际归档 I/O 已取消。
5. metadata 和 cache 相对字段、原文件格式及 EPUB/CBZ marker/generation/capability 不变；新公开
   archive directory/file 保持现有 755/644 语义，不直接套 stage 私有 700/600。private temporary
   可以 600，发布前按既有目标权限；不改变旧目录/文件权限或对外 Reader 能力边界。
6. 失败先回滚当前项 SQL，只回收原 session 确认由本请求创建的 files/empty directories；递归回收
   需要原父 detach/no-replace 与原 tree identity/owned成员证明。未知 newcomer、额外成员、被
   替换的目录/link/special/外部 target 全保留。不能绝对 RemoveAll 当前同名书目录、user/data/root，
   也不追踪 rename 到其它 namespace 来修复。安全补偿不使用已取消 ctx 跟随未知对象；残留
   不伪称自动清理完成，不为它扫描/删用户历史目录。
7. 即使 allocation 后原文件写失败也要有 owned partial compensation。所有成功、取消、失败、
   collision、resource extraction 和 cleanup 路径关闭自有 fd；正常 cleanup 不泄漏 handles。
   无法强行中断 native syscall、进程崩溃、掉电/fsync/跨 FS 原子性不属于本合同证明。

## 5. Book、placeholder、分组与投影

- service 接收全 normalized requested category set；保留现有 stable positive dedup、categoryIds
  优先和 categoryId fallback。未选正 ID 的 new book 为未分组；placeholder 不选时保留已有
  memberships，explicit positive set 精确替换。空/省略不是新的“清空恢复书分组”操作。
- 在同 tx 重新验证 selected category 的 user ownership；关系删除/第一或后续插入错误、primary
  更新和最终重载错误均回滚当前 item。legacy primary 与 durable effective memberships 一致；
  旧 primary 只在无关系时 fallback，不能和非空 relations 合并制造 phantom 分组。
- placeholder 按 caller ID/source/URL/原 library 字段复验当前目标，不复活已删除/已重绑定目标；
  只更新本次 import 拥有的 parser/title/author/archive/catalogue 和 explicit category 列，保留
  当前其它 metadata、追更、时间语义和已有 progress/bookmark 既有 bookcatalog 重绑规则。
  不因“同名找到一个旧快照”Save 整行或改外用户同名书；两个歧义 placeholder 不误合并。
- 事务内得到 authoritative Book/effective relations，commit 后生成同一 shelf projection 用于
  response/event。失败重载不可空数组伪成功；不持久化 resource capability 或 host absolute paths。
- portable RestoreExisting 保持原 numeric identity、URL、category/progress/bookmark、format 和
  archive backup 规则；可以用显式 Background wrapper 支持非 HTTP consumer，但不绕开 shared
  原实体归档所有权。不能从 API ctx 绿灯推导 portable lifecycle 自动完成。

## 6. 必须先于实现的真实红测与控制

测试只使用 test-owned roots/SQLite/accounts；不改生产、不复制用户书库。nil-in-production seam
放在实际旧 allocation/write/resource/SQL/rollback 边界；每个 fixture 必须 fired、实际结果可观察、
worker 有界 join。编译失败、没触发 hook、睡眠概率、旧相邻绿灯不当 red。

1. 四 confirm 路由 × raw/token × new/placeholder：注入第一/第二 BookCategory create 和 delete/
   primary/最终 reload failure，证明当前 success、部分关系、消耗 token 或存续 Book；目标当前项
   rows/files/event 零新增或原 placeholder snapshot/progress/bookmark 保持，原 token 可重试。
2. 原 library/data/user stable link/special/000 和 admission 后 replacement：allocation/raw/chapter/
   metadata/resource 实际边界不得把 bait 写入外部或接受为 durable；外部 sentinel/hash/mode保持。
3. 新目录 Stat→mkdir collision、concurrent friendly name、raw write 部分失败和失败补偿前 same-name
   replacement/unknown member：只删自有实体，不覆盖/删除对手或历史归档，无 orphan owned handle。
4. 实际 raw/chapter/JSON/EPUB/CBZ resource read/write/copy 中和 SQL mutation 后 ctx 取消/deadline，
   不只 pre-canceled/final guard；回滚当前 item，保留 stage、旧 target 和前项已成功集合/通知。
5. placeholder lookup 后 delete/rebind 与无关 metadata/follow/category 更新；owner guarded updates
   无 fallback insert，不覆盖未提交列，positive selection 精确、空选择保留旧 membership。
6. controls：所有 formats/rules/GB18030/空 TOC、legacy prepared match no-reparse/miss fallback、
   mounted 源删除后的同 token retry、无分组/单/多/重复/零/foreign、两个用户同名、歧义 placeholder、
   多个批次的 earlier success、durable 后 cancel/consume 阻塞、旧两/三文件 stage与 FD/权限保持。
7. full Go/vet/focused+full race、frontend full/build、source-built Go+SQLite HTTP 和真实 Chromium
   1440×900/390×844/360×800 preview→confirm→shelf→精确 Reader，非 root native Linux两架构，
   同 SHA candidate fresh/historical/portable-v1/v2/backup/restart和 trusted Actions双架构发布。
   必须独立核验 exact/latest OCI index 和真实平台 config；本地成功不等于生产升级或设备签收。

## 7. 数据、实施顺序与仍未签收

无 schema/table/column/index/migration marker、API URL/field/前端 key、根目录、持久 cache/archive/
backup member 或环境变量改变。保留 data/cache/library/旧 URL/用户数据，不扫描、迁移、chmod、
删除或重建用户历史归档，不要求消费者本地编译镜像。

顺序：独立 contract/matrix/API/data/security 提交推送 → old app 精确 actual reds 独立提交推送 →
shared service/rooted archive/context tx/thin API →全部相邻与真实门→coherent candidate提交推送→
trusted Actions正式镜像→独立OCI→设备验收。故意 red 只放 feature，不能推 main发布。

允许差异：Go/SQLite、多用户、guarded columns、原 opened ownership、bounded I/O/ctx、安全内部错误、
失败保留未知实体和 durable 成功不伪回滚；不改变上游导入/分组/逐本成功可见结果。
本项测试、实现、全量/runtime/发布仍全部 pending。完整 parser CPU、跨进程 exactly-once、扫描
cardinality/depth/FD 总预算、桌面 click/wheel、dependency advisory、整体审计仍未完成。
本盘点时 Git 9cb6a03、已发布 Docker b2b32f7/index ed57cc69…、生产最近只读 db1ea21 分别记录；
没有生产升级/写入，原已恢复阅读故障不重开。

## 8. 独立初轮实际红测与工作树恢复（2026-10-15）

合同 `477587fa1cb7d88843a34780ffc4aec2a030395f` 六文档独立提交，官方 Git API 对象 SHA 和
远端 `codex/import-durability-inventory` 均精确核验后才添加测试；main 保持9cb6a03，应用仍b2。
本轮没有修改 application/helper/hook，全部实际失败使用 GORM 既有 callback 和自有临时文件。

旧 `/private/tmp/openreader-import-durability/OpenReader-dev` 的 `.git` 和 handoff 已缺失，Git 将
其标为失联；没有据此推断清理原因，也未 prune/delete/reset 旧目录。由仍在本地对象库和远端的
精确477提交新建受管理 worktree，重新读取 AGENTS/九个匹配 skills，确认同 SHA、clean、空
submodule、应用相对b2无差异；已有 stage collision 控制通过1.434s。这是已提交状态恢复，不能
声称未提交文件或临时日志完整连续。

`backend/api/local_import_durable_commit_lifecycle_contract_test.go` 在未修复应用运行：
`go test ./api -run '^TestLocalImportDurableCommit' -count=1 -v`，终态exit1/API10.787s，
**128个 leaf 行为失败 +8个独立正常控制通过**，无编译失败、panic 或未触发 fixture：

-48：4confirm routes × raw/token × new/placeholder × 所选第一/第二关系create或关系delete失败，
  当前仍成功响应、留 Book/Chapter/不完整关系/归档并广播，token路径还被consume。注入按实际
  CategoryID 身份而非 Create调用次数，不冻结未来单条/批量SQL实现。
-16：同入口组合，在实际 Book create/update 边界移开本请求归档、建立同名 unknown directory，
  再注入SQL失败；legacy absolute RemoveAll 删除 newcomer，而 held原文件保持。
-48：4routes × raw/token × new/placeholder × library/data/user初始symlink，外部自有bait目录
  被写入归档或被接受为持久Book；sentinel/link保持检查和数据库/通知/token断言实际失败。
-16：4routes × raw/token × new/placeholder，在真实 Chapter create 完成后取消 caller ctx，当前
  仍提交后续状态/归档/通知，不符合本项固定取消响应。不是只测 pre-canceled 或最后ctx检查。
-8正常控制：4routes × raw/token 的新书多分组/primary、实际Go Reader精确正文、一条成功事件和
  durable stage consume 全通过；它们不是红灯，也不证明完整 placeholder/资源/并发生命周期。

最终日志 `/private/tmp/openreader-import-durable-final-initial-red.log`，首轮64反例日志和中间128
日志分别保留，不用其旧耗时冒充最终测试源码。只使用test-owned roots/SQLite/accounts，无生产
写入或用户书库操作。此提交只可feature推送；故意red不能推main触发发布。初始所有权collision、
partial原文件/JSON/资源实际I/O取消、admission后ancestor/entry替换、unknown成员补偿、SQL最终
reload/primary失败、placeholder并发列/删除重绑定、earlier batch成功、FD/native/卷等门仍需继续。

同最终测试源码 `-race` 终态exit1/API78.770s，仍精确128行为失败/8控制通过，无 DATA RACE、
panic、编译或fixture未触发；它是race仪器下的真实red，不是应用race全绿。独立只跑8正常导入+
旧stage collision控制exit0（API1.500s/importstage1.308s）。本轮尚无应用实现或发布候选，因此
未把有意红灯的fullGo/frontend/browser/volume/OCI门写为通过；实现后必须重新完成全部门。

另独立只读生产health本日返回完整 `b2b32f7b40bb8cefc18d316aefb29865aede9872`、status ok、
build2026-10-10T09:13:53Z；此前db1ea21是历史事实，不能继续当当前运行值。本项尚未实现，故
生产已升级stage镜像不等于持久提交问题已解决；原已恢复书事件不重开，整体审计保持未完成。

## 9. 实际工作边界追加红测（2026-10-15，仍先于实现）

独立初轮red38c8f07远端精确核验后，只增加 nil-in-production 观察接缝与测试，所有原绝对
mkdir/write/resource/remove、contextless handler 和 Book/category 提交逻辑仍未修复。接缝观察
已发生的 allocation/Write/Close，不替换 parser结果或模拟成功 I/O。

最终补充运行 `go test ./engine ./services/localbook ./services/epubreader ./services/cbzreader
-run '^(TestImportArchiveAllocation|TestImportResource|TestImporterDurableWork)' -count=1 -v` 终态exit1，
**追加79个实际行为失败、15个已绿取消控制**：

- engine2：选名后真实同名目录/原文件被旧实现覆盖并认领；新目录创建后改000，非root实际原文件
  Write返回permission error但留下空归档。首个超长中文basename夹具在本机没有触发失败，已舍弃，
  不计red；最终EACCES夹具明确fired/实际错误，仍非rootLinux回归待后续完成。
- localbook48：TXT/EPUB/CBZ × new/RestoreExisting × raw/chapter/source/TOC实际写后 × book/user
  原目录换成含同一合法原bytes的普通replacement；后续独立helper重新接收并写入或提交替换归档。
- localbook13：已给DB合法caller ctx的28个取消案例中，13个在raw写后继续准备资源、source写后
  继续TOC、或恢复第一章写后继续第二章；context.Canceled有时被sql.ErrTxDone吞掉。另15个原本
  安全停止/回滚是控制，不计红灯；SQL ctx绿灯并不证明其前后原实体I/O完整安全。
- localbook6：raw实际写后添加unknown成员再取消，旧绝对RemoveAll误删unknown，new/restore及
  三格式均fired；不能把“新分配目录”当作后来所有成员的所有权。
- EPUB/CBZ各5：首entry实际写/close后换普通目录、symlink或加入冲突unknown成员（各3）；真实
  reader返回partial+UnexpectedEOF时writer按name删掉后来同名newcomer（各1）；MaxInt64 +1
  溢出造成静默空文件（各1）。外部sentinel和held原文件逐项观察，未把ZIP解析/编译失败当red。

最终日志 `/private/tmp/openreader-import-durable-supplement-final-red.log`：engine0.944s、
localbook3.273s、EPUB1.453s、CBZ1.927s；race同源码终态exit1，仍79行为red/15控制、无DATA RACE、
panic/编译或未触发fixture（0.533s/5.065s/0.921s/1.841s）。race日志独立保留。
既有正常portable archive、prepared snapshot、EPUB/CBZ导入/恢复资源控制通过；首个联合控制命令
CBZ包显示no tests to run，不计覆盖，另真正的CBZ immutable generation控制0.438s通过。

此轮只可独立feature checkpoint，不能推main发布。207累计red是128 API+79跨层真实反例，不能
写成207条API或全合同覆盖。完整actual-chunk ctx/预算、原始source打开竞态、native FD与partial
mkdir回收、元数据/分组最终重载、placeholder并发删除/列、batch早项成功和正式全量/卷/发布门
仍需继续；不从这些追加red或15控制推导完整模块已实现。

## 10. 共享实现及本地验证 checkpoint（2026-10-15，未发布）

独立合同477587f、初轮red38c8f07、追加red09d9fd6均精确推送核验后才实施。新增
`rootedfs.OwnedTree` 持有原configured/library/data/user/book目录链，exclusive/no-replace子目录、
32KiB bounded实际读写/源ReaderAt、owned文件补偿及empty-owned目录回收；不再按当前pathname
RemoveAll。未知replacement/member保留，所有自有scopes/readers关闭。公开目录/文件755/644，
derived completion marker仍600；stage沿用同native底层但仍700/600，不chmod已有实体。

engine新归档wrapper保留签名/友好suffix规则，Importer接收尚未handoff的原session；新建与
RestoreExisting收敛同一persist路径。EPUB/CBZ从同session已打开的regular source计算fingerprint、
有界ZIP读写和原格式marker，不重新按path接收资源根。Book/Chapter/selected memberships在同
caller-context transaction；category复验、placeholder owner/URL/archive guard、显式owned列更新、
进度/书签重绑和最终Book/relations重载全部在tx内。无Save/fallback resurrection、无ignored分组
follow-up。stage把最终原bundle guard带入commit；四confirm路由只做输入/错误映射和权威结果
投影，durable后才consume/通知，后项失败保留前项成功。

207实际反例与原正常控制初轮贯通全绿（API13.601s、service4.242s、engine2.370s、EPUB2.840s、
CBZ3.321s；该选择命令stage包no tests to run不计stage覆盖）。另新增实现控制：32个tx category
recheck/final Book reload真实失败回滚，8个placeholder未提交intro/canUpdate列合并及权威事件，
8个两个storage raw/token后项SQL失败/取消保留前项，API5.156s全过。这些是新实现控制，不冒充
旧实现red。native控制实际raw/chapter/JSON/EPUB/CBZ分块Read/Write中取消、source Read/ReadAt
取消、partial mkdir/unknown保留、精确/+1/MaxInt64、marker权限和120轮成功/取消FD释放均通过。
首个/dev/fd目录枚举夹具在macOS报bad file descriptor，不计产品失败/绿灯；改有界fcntl测量后
native/root2.537s、stage1.491s、EPUB1.796s、CBZ2.133s、localbook5.622s全通过。

最终源码完整Go终态通过（API118.894s），vet exit0；较早108.767s保留为中间版本，不替代最终。
frontend762/762（零skip）、Vite build、Compose通过；离线npm ci没有改lock或依赖版本。
源构建Go+SQLite隔离三账号HTTP：旧admin根/两普通用户、双storage、源删除后同token改规则/确认、
精确Reader正文/foreign404、Basic/curl和LocalStore邻接全通过。真实Chromium1228在1440×900、
390×844、360×800确认取消/单本改规则/分组/批量/逐本后，直接打开numeric Book Reader读取精确
正文，无章节mock/错误/横溢出；preview延迟fixture仍真实fetch/取消。自有server/browser均已关闭，
保留自有浏览器数据根，不操作生产书库。

同native测试Linux双架构静态编译成功；在已有Alpine3.22非root65532、断网/只读根/私有tmp中
Linuxarm64全包终态PASS。它不是新应用Go1.24/CGO candidate镜像证明；amd64目前仅编译，不
冒充runtime。完整API+localbook/stage/native/resource/engine race在25m预算真实运行（进程/handle
已核验live），尚未终态，不称全绿。候选同SHA Docker新旧卷/portable/backup/restart、trusted
Actions双架构发布与独立OCI仍待办，不能从旧stage镜像b2推导本项发布。

本机构建Go1.26.5/darwin-arm64；正式Go1.24/CGO Linux由candidate/trusted门另验。未加schema、
URL、服务环境变量、backup member或浏览器key。安全差异仅owned/ctx/固定内部错误/unknown
保留；纯parser CPU/regex/DOM、跨进程exactly-once、scan/peakFD预算、desktopclick/wheel、advisory
和整体审计仍未签收。生产再只读health仍完整b2b32f7/statusok，无生产写入或升级。

## 11. 精确候选本地终态与可信发布进度（2026-10-15）

应用提交 `b4ff9ca61bd4e761ab23dd50067c3e78ff10441f`；上述第10节的 race/候选卷 pending
是历史 checkpoint，不再作为当前状态。最终完整 API/localbook/importstage/rootedfs/EPUB/CBZ/
engine race 终态 exit0，分别为 998.650s/7.262s/4.863s/5.881s/2.298s/3.886s/3.348s。
日志 `/private/tmp/openreader-import-durable-full-race-final.log`；没有 DATA RACE 或悬挂本地测试。

同 SHA 本地 Go1.24/CGO Linux arm64 候选 `openreader-import-durable-local:b4ff9ca` 已核验完整
OCI revision，原始 sh fresh/historical 两门均首次运行终态 PASS：portable-v1/v2-assets、跨用户、
backup/restart，以及旧 TXT/EPUB/UMD/CBZ、relative-cache、owner、archive hash、进度/书签和
logical/portable restore。保留自有夹具与日志，全部容器已停止。更早 stage 候选0a的历史404仍为
未复现 unknown，不从本轮通过推断其根因。该本地候选 index 不冒充 GHCR 双架构 index。

所有本地门通过后，main 以非 force fast-forward 从9cb6a03更新到精确b4；可信
[Actions38745709308](https://github.com/changshengyu/openreader/actions/runs/38745709308)
已通过 backend/frontend/build/Compose、隔离 HTTP、native candidate、新旧卷/portable/backup门，
仍在双架构正式构建/推送。尚不能签收发布终态或独立远端 OCI config；没有重复派发发布。

按 rotate-worktree-safely skill 在里程碑交接后继续到原生受管理工作树
`/Users/yuchangsheng/.codex/worktrees/import-durable-published/OpenReader-dev`，同精确b4、clean、
空 submodule、完整任务交接与廉价 native/stage 控制通过。原工作树和普通 Git adapter 创建的后继
均保留；attach 不支持普通树时使用原生 create_worktree，不修改内部注册数据或删除旧树。

这是共享核心的回归签收，不是第6节所有控制或整个模块完成。placeholder lookup 后的独立外部
删除/重新绑定覆盖仍待补核；现有源码 guards 和列合并绿灯不能替代这些案例。完整 parser CPU/
regex/DOM interrupt、扫描/峰值FD预算、跨进程/崩溃 exactly-once、桌面 click/wheel 设备反馈、
dependency advisory 与整体审计仍独立未完成。非root Linux arm64 native 已运行、amd64本地仅
编译；CI 双架构构建也不能冒充两架构全部 native 测试运行。生产最新只读仍为完整b2b32f7，
不声称已升级b4或已获得设备签收，原书恢复事件保持关闭。

## 12. 可信双架构发布与独立 OCI 签收（2026-10-15）

第11节远端 in-progress 是历史进度。可信
[Actions38745709308](https://github.com/changshengyu/openreader/actions/runs/38745709308)
在完整 `b4ff9ca61bd4e761ab23dd50067c3e78ff10441f` 终态 success，所有 backend/frontend/build/
Compose、隔离 HTTP、native image、新旧卷/portable/backup、发布和平台检查步骤均 success。
状态 watch 客户端末次网络 EOF 不等于工作流失败；独立 GET 已核验同一运行/Job 的全部终态，
没有重派工作流。公开 Registry API 于 `2026-10-15T14:08:46.920Z` 校验实际响应字节 SHA256、
manifest header digest、两真实 Linux platform 的 manifest/config、OS/arch 和完整 revision：

- `ghcr.io/changshengyu/openreader:b4ff9ca` 与 `latest` 共用 OCI index
  `sha256:60e97193265ff47808a3910cae1d0bf8a8cec6a3c7b5b4a12efeb7bab144c6ee`；
- amd64 manifest `sha256:018ba1a38abd715fab512cacd68d5b53e9a8d2463766d3b5b7c11ba64cc5171a`，
  config `sha256:472715b469468889494f6ceba12297c493691b83397941a23e407ad39311c31b`；
- arm64 manifest `sha256:92f962345f44959bc392b2590f2a715d143cb1df7a2a8e06cdb273fab944a0f6`，
  config `sha256:bcb383faad0e0d14d37505e6814e776d75a97c7c0738f838af2553be911e4dae`；
- 两 config 均为完整b4 revision；没有把 attestation/unknown platform 当成实际架构，未输出凭证。

只读记录 `/private/tmp/openreader-import-durable-registry-b4ff9ca.json`；本地 candidate digest 与
远端 index 分开，不声称本机已强制拉取远端容器。发布后生产 GET `/api/health` 仍为
`b2b32f7b40bb8cefc18d316aefb29865aede9872`、status ok、build2026-10-10T09:13:53Z。
未访问/升级另一台 Mac 容器、未修改生产数据或挂载；浏览器刷新不是升级或真机签收。

完成的是第10–11节所述共享 durable core 和其已列实际控制。允许差异仍为 Go/SQLite、多用户、
原实体/ctx/owned columns、固定内部错误、未知实体保留与提交后成功不伪回滚；无 schema、根目录、
旧 URL、格式、备份、环境变量或前端 key 迁移。第11节列出的 placeholder 外部删除/rebind补核、
完整 parser CPU、scan/峰值FD、跨进程 exactly-once、设备反馈/advisory/整体审计继续开放，不能
将发布状态改写成全部合同或整体重构已完成。

## 13. 占位书目标身份补充覆盖（2026-10-15，独立合同后验证）

固定 SHA 上游 `src/main/java/com/htmake/reader/api/controller/BookController.kt:1189–1335`
已再次从 pinned raw source 读取：caller namespace 内按 name/author 找书，保存本地原文件、资源、
source/shelf，合并旧 durChapterIndex/title/time。没有从这些代码推导并发 SQLite 或删除复活语义。
OpenReader 自动路径 `readerDevLocalPlaceholder` 与显式 portable `RestoreExisting` 使用第5节目标
身份保护，这是多用户/SQLite 允许强化；现有源码有 guards，但不能以源码代替具体反例/控制。

| 边界 | 当前证据 | 裁决和测试计划 |
|---|---|---|
| 候选确定后、item tx前，外部目标已删除或变更owner/source/URL/library/original | b4的tx重读和原快照比较应拒绝；旧09也已有部分同类检查。 | `unknown coverage`，不是已证缺陷；四confirm×raw/token与三格式×自动/显式Restore均加实际已提交SQL控制，期望保留外部结果、无复活/新增章节关系/成功事件、新owned archive回收、token仍可重试。 |
| tx重读后、guarded UPDATE前，目标再失效 | b4有where guard/RowsAffected，无Save fallback；普通SQLite外部writer不能任意越过读锁。 | 只作为同tx故障注入控制，不冒充独立并发外部提交。若后续加入，单列时序/回滚断言，不与上一行混计。 |
| 正常可恢复目标 | 当前unowned intro/canUpdate和权威投影8控制已绿；原URL/progress/bookmark/分组合同不变。 | 邻接正常控制必须继续通过，不将新拒绝推广到所有逻辑恢复书。 |

外部控制使用真实test-owned SQLite提交，不在 importer tx 内伪造已发生外部commit；触发点必须
实际fired、位于tx前，比较的是外部变更提交后的完整DB快照，而不是要求撤销外部删除/重绑定。
变更归档字段时只指向自有bait文件，并比较全部旧文件hash。四路响应仍是原durability安全错误
（direct500/storage200-item），不增加404/409/499或新响应字段。显式Restore内部错误不得带来
成功Book返回；服务/API测试不调用生产或复制用户书库。

此盘点不改应用/测试、schema、backup/路径/环境变量或可见成功语义；先独立提交推送再添加
控制。若当前应用全绿，记录为实现后补充控制而非“先于b4的红测”，不为制造red删掉已有guard。
若出现真实失败，先归入具体must-fix合同、固定观察红测后再实施。完整解析CPU/扫描预算等
其它开放项仍独立保留，不把本控制集当整项重构完成。

上方仅合同段落在独立 `87e5a72e0f39b56bd35b3ef5fcdafcb35a1e9967` 提交、远端 feature SHA
精确核验后才添加两个测试文件，**未修改应用**。四confirm×raw/token×六种外部变更48个API
leaf、TXT/EPUB/CBZ×自动/显式Restore×六种变更36个service leaf全部通过。API在实际placeholder
chapter Count返回后、item tx前触发；service在实际raw-written后、item tx前触发，均独立SQLite
pool实际Exec、检查RowsAffected1，之后用原连接读取外部已提交快照。没有sleep概率或在tx内
伪造外部提交；每case必查fired。拒绝后快照完整保持、无返回Book、无新增归档或旧bait hash变化，
API零成功事件、原token仍可加载；外部变更没有被补偿撤销。

首次受限模式setup因既有Go构建缓存权限失败，不计行为red；授权同源重跑终态exit0，API4.783s/
service2.032s。focused race 同84 leaf以及8个API列合并、3个正常prepared/唯一恢复/歧义保护控制
全部通过（API36.181s/service3.421s），无DATA RACE、no-tests或未触发夹具。最终新增测试源码
fullGo终态通过（API119.156s/localbook8.860s），fullvet exit0。日志分别为
`/private/tmp/openreader-import-placeholder-target-run.log`、
`/private/tmp/openreader-import-placeholder-target-race.log`、
`/private/tmp/openreader-import-placeholder-full-go.log`。

这些是b4发布后补充的84项控制，**不是新的先于实现红测或新缺陷修复**，不改207旧red计数。
应用/前端/脚本/镜像配置与精确b4无差异，既有762前端、三视口、候选卷和可信发布证据属于同一
未变应用；本补测未重跑前端/浏览器/卷，也不声称新测试源码经历一轮新的Docker发布。
tx重读后失效的故障注入、完整CPU/scan/峰值FD/跨进程/设备/advisory与整体审计仍开放。
