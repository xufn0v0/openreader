# Reader 桌面运行标注合同盘点（P0）

状态：**implementation-complete / regression-validated**。合同 `4092ea2`、红测 `4897a62`、实现 `035762d`，
验证记录见第 10 节。提交只在本地，未推送、未合入 main；没有新镜像；生产未动。

固定上游：`changshengyu/reader-dev@fa22f271849d45f93349ae1636223e27b16a4691`。  
当前基线：`OpenReader@c1172d1`（`claude/sleepy-moser-50391e`，基于 `origin/main` `5bc38a3`；
`git diff 186c762 c1172d1 -- frontend backend` 为空，Reader 产品代码与已发布的 `186c762` 相同）。

## 1. 范围

- 对象：桌面 Reader 的 `.reader-page-head`。左侧 `.reader-running-chapter-title` 显示 `第 N 章 章节名`，
  右侧 `chapterLabel` 显示 `N / total`。
- “桌面”指非 mini interface：`shouldUseMiniInterface(reader.pageMode, width)` 为假，即 pageMode 不是
  `mobile` 且视口宽度大于 750px。1440×900 和 1024×1366 都是桌面。
- 适用于文本和 EPUB。音频与 CBZ 不渲染这个页头（`showRunningChapterHeader`），不在范围内。
- 起因：[EPUB 移动端章节计数合同](reader-epub-mobile-chapter-counter-fixed-baseline-second-audit-p0-contract.md)
  第 5 节把这一行记为 `unknown`；[运行标题合同](reader-running-chapter-header-device-feedback-p0-contract.md)
  第 5 条保留了桌面 `N / total`，但没有对照上游空 top-bar 分类。

## 2. 上游证据（fa22f271）

| 位置 | 行为 |
| --- | --- |
| `web/src/views/Reader.vue` L227–229 `.chapter > .top-bar` | `{{ $store.state.miniInterface ? title : "" }}`：桌面渲染空字符串。 |
| 同文件 L3395–3401 | 桌面 `.top-bar` 只有 `height: 44px; padding: 10px`，是正文流里的一条空白，随 `.chapter` 一起滚走。mini 的固定栏在 L3661–3675（`position: fixed`、30px + safe area、12px）。 |
| 同文件 L1–251（整个模板） | 不引用 `catalog`、`chapterIndex` 或 `readingBook`；任何视口都没有章节 `N / M` 计数。 |
| 同文件 L140–144、L692–700 | 右栏 `.tool-icon.progress-text` 显示 `readingProgress`，桌面只有 `N%`（mini 才加 `阅读进度: ` 前缀），点击打开缓存区。`readingProgress = parseInt((chapterIndex + 1) * 100 / catalog.length) + "%"`。 |
| 同文件 L241–248；`web/src/plugins/vuex.js` L451–453 | `.bottom-bar` 的 `第x/y页 N%` 和时间只在 `isSlideRead` 时渲染，而 `isSlideRead` 要求 `miniInterface && readMethod === "左右滑动"`，桌面永远不显示。 |
| `web/src/components/Content.vue` L62、L213、L231 | 文本正文开头渲染 `<h3 data-pos={0}>{title}</h3>`，连续滚动时每章一个；CBZ 不渲染。EPUB（L397–405 `renderEpub`）只有 iframe，没有这个 h3。 |
| `Reader.vue` L338–343 `activated`、L410–416 `watch.title` | 浏览器标签页标题设为 `书名 - 章节名`，章节名为空时只用书名。`web/src/views/Index.vue` L1188–1189 `mounted`、L1207–1208 `activated` 设为 `阅读`。 |

**结论**：上游桌面没有常驻的运行标注。当前章节名只出现在正文开头的 `<h3>`（随正文滚走）和浏览器标签页
标题里；书级进度只有右栏 `N%`；任何地方都没有章节 `N / total` 计数。

## 3. 当前 OpenReader

- `frontend/src/views/Reader.vue:146–149`：文本/EPUB 页头先渲染运行标题，再渲染
  `<span v-if="!isMobileReader">{{ chapterLabel }}</span>`。
- `Reader.vue:1419–1427`：`formatRunningChapterLabel`、`chapterLabel`（`` `${currentIndex + 1} / ${chapters.length || 1}` ``）、
  `runningChapterLabel`、`showRunningChapterHeader`。`chapterLabel` 只在第 148 行使用。
- `Reader.vue:2530–2541`：页头绝对定位在阅读框顶部，`justify-content: space-between`、14px、`padding: 10px 65px 0`、
  `pointer-events: none`；运行标题单行省略。`Reader.vue:2597–2604`：桌面文档滚动时页头改为 `position: fixed`，
  跟随可见阅读框（`437a940`）。
- `frontend/src/components/reader/ReaderDesktopProgress.vue:11`（由 `Reader.vue:218–231` 在桌面挂载）：右栏
  `.progress-box` 显示 `bookProgressLabel`，点击开关缓存区。百分比来自 `useReaderProgressControls.js:13–18`，
  包含章内比例。
- `frontend/src/components/reader/ReaderChapterContent.vue:63`：每章正文开头渲染 `<h3 data-pos="0">`，CBZ 隐藏，
  与上游一致。
- 浏览器标签页标题：`frontend/src` 里没有代码设置 `document.title`，标题始终是 `frontend/index.html:13` 的
  `OpenReader`。

## 4. 来源与用户要求的证据

| 提交 | 日期 | 与本项有关的内容 |
| --- | --- | --- |
| `131d3f8` | 2026-05-18 | 初始提交就有这个页头：左 `book?.title \|\| '阅读中'`，右 `chapterLabel`。同期 `docs/frontend-rewrite-plan.md` 的“阅读器独立导航”写的是“顶部：返回、书名、章节、同步/缓存状态”。这是重写初期的自有设计，不是上游行为，也没有用户要求的记录。 |
| `9b96c50` | 2026-09-13 | 真机反馈重开。矩阵行写“恢复章节运行标注并按用户要求补 1-based 章节序号”；运行标题合同第 6 条只把 1-based 序号列为用户明确要求的允许差异。第 5 条“右侧保留 `N / total`”既没有引用用户要求，也没有对照上游。 |
| `e1631d0` | 2026-09-14 | 实现：页头左侧改为运行标题，`chapterLabel` 改为只在桌面渲染。 |
| `9d0d4f4` | 2026-09-14 | [桌面分页合同](reader-desktop-pagination-device-feedback-p0-contract.md)第 28 行：“User explicitly requested the running chapter ordinal/title on desktop and mobile”，判为 `user-requested acceptable-change`，并要求文档滚动时桌面页头保持固定（第 42、49 行）。说的是序号和标题，没有提到计数。 |
| `5361e68` | 2026-10-16 | EPUB 移动端章节计数合同第 5 节把桌面一行记为 `unknown`。 |

检索范围：

- 所有本地分支的提交信息和 `docs/` 历史：`git log --all -S'N / total'`、`-S'章节计数'`，以及按“章节计数、
  章节序号、运行标注、章节标注、running header、chapter count”等关键词的 `--grep`；
- `docs/` 下的全部合同、gap analysis、矩阵、`docs/*plan*.md`、`docs/upstream-alignment-audit.md`；
- 交接文档 `claude-code-refactor-handoff-2026-10-16.md`；
- 主检出里的 `AGENTS.md`、`.agents/skills/`，以及 `/private/tmp/openreader-*-handoff.md`。

结果：

- 桌面运行标题（序号 + 章节名）有用户要求的书面记录：`9b96c50` 记录用户要求补 1-based 序号，并在第 1、5 条把
  同一标注用于桌面；`9d0d4f4` 直接写明用户要求桌面和移动都显示。
- 桌面 `N / total` 找不到任何用户要求。它唯一的来源是初始提交的自有设计，`9b96c50` 和 `e1631d0` 都只是沿用。
- 局限：原始对话不在仓库里，本盘点没有查阅，结论以仓库里的合同记录为准。如果用户确认曾要求在桌面显示
  计数，第 5 节第 2 行改为用户要求的 `acceptable-change`，第 6–8 节随之作废。

## 5. 差异分类

| 项目 | 上游桌面 | 当前桌面 | 判定 |
| --- | --- | --- | --- |
| 运行标题 `第 N 章 章节名`（页头左侧） | top-bar 为空；章节名只在正文 `<h3>` 和标签页标题里 | 页头常驻显示，文档滚动时固定在阅读框顶部 | `acceptable-change`：用户明确要求桌面与移动都显示运行序号和标题（桌面分页合同第 28 行、运行标题合同第 6 条），已签收，不重开 |
| 章节计数 `N / total`（页头右侧） | 任何视口都没有 | 桌面显示 | **`must-fix`**：上游没有对应行为，也不属于任何允许差异（运行时适配、多用户隔离、缓存加固、安全修复、用户要求的 UX 改进），只是重写初期自有设计的遗留。目标见第 6 节 |
| 书级进度 `N%` | 右栏 `.progress-text`，按章取整的百分比，点击打开缓存区 | 右栏 `.progress-box`，含章内比例的平滑百分比，点击打开缓存区 | 位置和入口 `aligned`；算法是[移动进度合同](reader-mobile-progress-p0-contract.md)已裁决的 `acceptable-change`（同一个 `bookProgressLabel`），不重开 |
| 正文章节标题 `<h3>` | 文本正文开头有；EPUB、CBZ 没有 | 同样渲染，CBZ 隐藏 | `aligned` |
| 页头位置、字号、颜色 | 44px 空白条，在正文流里 | 阅读框顶部绝对定位，文档滚动时固定；14px | 属于已签收的运行标题实现（`e1631d0`、`437a940`），不重开 |
| 浏览器标签页标题 | Reader 为 `书名 - 章节名`，Index 为 `阅读` | 始终是 `OpenReader` | Reader 部分为 `must-fix`（上游可见行为，没有允许差异的理由）；Index 用 `阅读` 还是产品名为 `unknown`。**不并入本切片**，另行盘点。运行标题合同第 5 条说“书名仍由浏览器标题提供”，与当前代码不符 |

为什么计数不算 `intentional-redesign`：没有任何文档把它当作有意的重新设计提出过。`9b96c50` 和 `e1631d0`
只是沿用旧页头的右半边，“保留现状”不是对照上游做出的设计决定。

## 6. 目标合同

1. 所有视口统一：文本与 EPUB 的 `.reader-page-head` 只包含 `.reader-running-chapter-title`，页头文本整体等于
   `第 N 章 章节名`（章节名为空时为 `第 N 章`）。
2. 任何视口都不渲染章节计数：删除 `Reader.vue:148` 的 `<span v-if="!isMobileReader">{{ chapterLabel }}</span>`，
   并删除随之失去用途的 `chapterLabel`（`Reader.vue:1425`）。不能改成 CSS 隐藏，也不能留在 DOM 里：
   `document.body.textContent` 中也不得出现计数。
3. 保持不变：运行标题的派生与更新时机（运行标题合同第 1、3 条）、音频/CBZ 例外、页头的绝对定位和文档滚动
   固定几何、`pointer-events: none`、单行省略、mini 的 30px + safe area 栏、右栏 `N%` 与缓存入口、正文 `<h3>`。
4. 不需要改 CSS：只剩一个子元素时，`justify-content: space-between` 把它放在起始端，和现在的左侧位置相同。
   长标题仍在 `.reader-running-chapter-title` 内单行省略，只是可用宽度不再扣掉计数和 16px 间距。
5. 不新增浏览器存储 key、API、数据库字段或持久文件。

## 7. 测试计划（后续阶段执行，本切片不写测试）

### 7.1 `frontend/tests/readerTextModeFrameContract.test.mjs`

- 现状：第 113 行的测试“reader exposes the current chapter number and title in the upstream running header”，
  第一条断言（第 114–118 行）要求运行标题之后还有 `<span v-if="!isMobileReader">{{ chapterLabel }}</span>`，
  失败消息是“the running header must show the active chapter label and retain desktop count”。
- 影响：这条断言固定的正是本合同判为 `must-fix` 的行为。按 CLAUDE.md“与上游冲突的 Reader 测试应删除或重写”，
  **重写**这条断言，测试本身保留：
  - 页头元素只包含运行标题：
    `/<header\s+v-if="showRunningChapterHeader"\s+class="reader-page-head">\s*<span class="reader-running-chapter-title">\{\{ runningChapterLabel \}\}<\/span>\s*<\/header>/`，
    消息改为“the running header shows only the running chapter label on every viewport”；
  - `assert.doesNotMatch(readerSource, /\{\{\s*chapterLabel\s*\}\}/)`：模板任何位置都不渲染计数；
  - `assert.doesNotMatch(readerSource, /const chapterLabel = computed/)`：删除失去用途的计数计算属性，避免以后被
    别处重新渲染。
  - 同一测试的其余断言原样保留：`runningChapterLabel` 的派生、`showRunningChapterHeader` 例外、mini 不得
    `display: none`、mini safe-area 几何、单行省略。
- 预期：当前实现上，重写后的第一条断言失败；实现后同一断言转绿。
- `frontend/tests/readerMobileProgressContract.test.mjs:27`（移动进度按钮不含 `chapterLabel`）不受影响。

### 7.2 `scripts/smoke/reader-epub-contract.mjs` 的桌面分支

- 现状：第 710–712 行在所有视口等待运行标题精确为 `第 3 章 第二章`；第 713–733 行的移动分支断言页头只有
  运行标题、body 中没有 `3 / 3`；第 734–736 行的桌面分支是
  `await page.waitForFunction(() => document.body.textContent.includes('3 / 3'))`。
- 影响：桌面 `3 / 3` 等待固定的是 `must-fix` 行为，实现后必然 30 秒超时，必须改写。这条等待原本要证明的是
  “跨 XHTML 链接后目录状态更新到第 3 行”，第 710–712 行的精确运行标题等待（序号 3 加第 3 行标题）已经覆盖，
  改写不会丢掉这层意图。
- 改写：
  - 把“页头运行标题可见”“页头文本只等于运行标题”“`document.body.textContent` 不含 `3 / 3`”三条断言从移动
    分支提到所有视口共用，失败消息去掉“mini”限定；
  - 移动端的 `第 x/y 页` 页码断言留在移动分支；
  - 桌面分支改为上游控制：`.reader-page-control .progress-box` 可见且文本匹配 `^\d+%$`（对应上游桌面右栏的
    `readingProgress`），证明去掉计数后桌面仍有书级进度。这条控制在当前实现上也应通过；如果不通过，先查原因，
    不能算作红测。
- 预期：当前实现上，1440×900 在“页头文本只等于运行标题”处失败，实际值为 `第 3 章 第二章3 / 3`；390×844 和
  360×800 照常通过。实现后三个视口全部通过。

### 7.3 `scripts/smoke/reader-continuous-contract.mjs`

- 现状：自 `e1631d0` 起，运行状态断言都读 `.reader-running-chapter-title`：第 152–179 行的初始采样检查运行标题
  文本、`pointer-events: none`、标题不超出页头、页头 `position: fixed`；第 229、254、265、296–299、414 行等待或
  读取跨章后的运行标题。没有任何断言读计数，也没有断言读 `.reader-page-head` 的整体文本。
- 影响：删除计数后，现有断言都不受影响，不需要改。第 157 行 `header.querySelector(...)` 依赖页头存在；本合同保留
  桌面页头，所以不会出错。反过来，如果有人按“上游桌面 top-bar 为空”把桌面页头整个删掉，这里会在 1440×900 和
  1024×1366 直接抛错，而那样做也违反第 5 节第 1 行已签收的用户要求。
- 补充（红测阶段加入）：在第 152 行起的 `initial` 采样里加 `headerText: header.textContent.trim()`，并断言
  `initial.headerText === initial.runningTitle`，覆盖 4 个视口 × scroll/scroll2。当前实现在 1440×900、1024×1366
  失败（页头多出 `1 / <总章数>`），390×844、360×800 已经通过。这样文本模式的桌面也有真实浏览器覆盖，不只靠 EPUB。
- 夹具第 0 章标题写着“不遮挡右侧进度”，指的是页头右侧的计数。计数删除后，第 178 行“标题不超出页头”的断言仍然
  有意义，夹具文字不必改。

### 7.4 回归与临时变异（实现阶段）

- frontend `npm test`（总数不变：单元测试只改写已有断言）、`npm run build`。
- 真实浏览器：EPUB 合同 1440×900、390×844、360×800；连续阅读合同 1440×900、1024×1366、390×844、360×800；
  另跑 `reader-text-modes-contract`（桌面文档滚动、固定阅读框和点击区）和 `reader-mobile-contract` 作为对照。
- 临时变异只在工作区修改并构建到独立目录，构建后立即还原，不提交：
  - M1：桌面重新渲染计数 → 单元合同、EPUB 1440×900、连续合同桌面视口都失败；
  - M2：计数移出页头、放进页面其他位置的隐藏元素 → 页头相等断言通过，EPUB 的 body 文本断言失败，证明“隐藏的
    计数也不允许”不是空话；
  - M3：桌面整个不渲染页头（照搬上游空 top-bar）→ EPUB 1440×900 在第 710 行运行标题等待处超时，连续合同桌面
    视口在读取页头时抛错，证明这些测试同时守住用户要求的运行标题。
- 跳过并写明原因：Go `test`/`vet`/`race`（后端不改）；依赖 advisory、Compose 与卷门（依赖、Dockerfile、数据格式都不改）。
- 会话沙箱禁止 loopback 监听，服务和 Playwright 需在沙箱外运行（见 EPUB 移动端章节计数合同第 9 节）。

## 8. 阶段

1. **合同**：本文件、gap analysis 与 matrix，独立 `docs:` 提交。
2. **红测**：按 7.1–7.3 改写和补充断言，记录每个视口的精确失败点。提交 `test: expose desktop reader chapter counter`。
3. **实现**：只改 `Reader.vue`，删除第 148 行的计数 span 和第 1425 行的 `chapterLabel`，让同一批断言转绿。
   提交 `fix: drop desktop reader chapter counter`。
4. **回归**：按 7.4 执行，包括三项临时变异。
5. **记录**：提交号、各视口结果、变异结果和状态标签写回本文件、gap analysis 与 matrix。
6. **发布**：实现提交改动 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；
   是否合入由用户决定。

## 9. 不重开

- 桌面与移动的运行标题 `第 N 章 章节名`、1-based 序号及其更新时机；桌面文档滚动时的固定页头几何（`437a940`）；
  mini safe-area 栏；音频/CBZ 例外。
- 书级进度百分比算法；移动端 `第 x/y 页`；EPUB 合同的移动端改写（`41fd3e6`）。
- 浏览器标签页标题（另行盘点）。
- API、数据库、浏览器存储 key、持久文件、镜像与生产：本切片都不改，不在本地构建或推送镜像，不做任何生产操作。

## 10. 实施与验证（2026-10-16）

| 提交 | 阶段 | 内容 |
| --- | --- | --- |
| `4092ea2` | 合同 | 本文件、gap analysis 与 matrix。 |
| `4897a62` | 红测 | 7.1：单元合同第一条断言改为“页头只包含运行标题”，并加两条 `doesNotMatch`（模板不渲染 `{{ chapterLabel }}`、没有 `const chapterLabel = computed`），其余断言不变。7.2：EPUB 合同把页头三条断言提到所有视口，移动端保留 `第 x/y 页`，桌面改为 `.reader-page-control .progress-box` 可见且匹配 `^\d+%$`。7.3：连续合同 `initial` 采样加 `headerText`，断言它等于 `runningTitle`。 |
| `035762d` | 实现 | `Reader.vue` 删除页头里的计数 span 和 `chapterLabel` 计算属性，共 2 行。没有改 CSS、API、存储或后端。 |

EPUB 合同里，按视口分支的断言放在“运行标题可见”（`:713`）之后、“页头文本只等于运行标题”（`:737`）之前。这样移动端
的断言顺序和 `41fd3e6` 相同（可见 → 页码 → 页头 → body）；桌面的 `N%` 控制（`:726`、`:731`）排在红点之前，红测
运行本身就证明这条控制在旧实现上通过，符合 7.2“控制在当前实现上也应通过”的要求。

**环境**：用本 worktree 构建的 Go 二进制加 `vite build` 生产前端。`git diff 4092ea2 035762d -- backend` 为空，
所有代码状态共用同一个二进制。每个代码状态单独起一个服务，各用独立的临时 data/cache/library 和随机开发 JWT
secret（未输出）。Go 1.26.9（`go.mod` 选择的工具链）、Node v26.4.0、Playwright 1.61.1 / Chrome Headless Shell
1228（149.0.7827.55）。会话沙箱禁止 loopback 监听，服务、浏览器合同和完整 `npm test` 都在沙箱外运行。第 2 节的
上游证据在 `fa22f271` 的临时浅克隆中复核（会话 scratchpad；沙箱不允许写 `/private/tmp/changshengyu-reader-dev`）。

**连续合同的逐视口运行**：提交的脚本一次运行里按 1440 → 390 → 360 → 1024 依次跑 scroll 和 scroll2，遇到第一处
失败就退出。为了逐视口记录红点，在会话 scratchpad 生成了临时副本，只改两处：Playwright 辅助模块改用绝对路径
导入；`main()` 的视口列表和模式列表改为从环境变量读取（模式默认仍是 scroll → scroll2）。断言、失败重试和进度
事务用例都没动。`4092ea2` 基线运行时工作区已是红测脚本，所以两个基线都用 `git show 4092ea2:<脚本>` 取出的副本：
EPUB 副本只改了导入路径；连续合同副本按上面的方法生成，并按原顺序传入四个视口。实现后的结论以提交的脚本原样
运行为准。

**单元合同**（`readerTextModeFrameContract.test.mjs`）与 frontend `npm test`：

| 代码状态 | 单元合同 | `npm test` |
| --- | --- | --- |
| `4897a62`（红测，产品代码同 `4092ea2`） | FAIL `:114`：页头只包含运行标题 | 787/788，唯一失败就是这一条 |
| `035762d` | PASS | 788/788 |

**EPUB 合同**（`reader-epub-contract.mjs`，每个视口单独运行）：

| 代码状态 | 1440×900 | 390×844 | 360×800 |
| --- | --- | --- | --- |
| `4092ea2`（旧脚本，三视口一次运行） | PASS | PASS | PASS |
| `4897a62`（红测） | FAIL `:737`：页头应只含运行标题，实际为 `第 3 章 第二章3 / 3`；之前的可见性和桌面 `N%` 控制都通过 | PASS | PASS |
| `035762d` | PASS | PASS | PASS |

`035762d` 上缺省 `SMOKE_VIEWPORTS` 的三视口一次运行也通过。

**连续阅读合同**（`reader-continuous-contract.mjs`）：

| 代码状态 | 1440×900 | 1024×1366 | 390×844 | 360×800 |
| --- | --- | --- | --- | --- |
| `4092ea2`（旧脚本，四视口一次运行） | PASS | PASS | PASS | PASS |
| `4897a62`（红测，逐视口） | FAIL `:178`：scroll 与 scroll2 的页头文本都是运行标题后面多出 `1 / 6` | FAIL `:178`，同左 | PASS | PASS |
| `035762d`（提交的脚本原样运行，四视口一次跑完） | PASS | PASS | PASS | PASS |

提交的红测脚本原样运行时停在 `1440/scroll :178`。

**对照合同**（`035762d`）：`reader-text-modes-contract` 通过（桌面文档滚动、固定阅读框和点击区）；
`reader-mobile-contract` 通过（desktop/mobile/adaptive-iPad/forced-mobile-iPad）。

**服务日志**：除 WebSocket 外没有 4xx/5xx，也没有 panic。401 全部来自 `/ws/sync`：走 mock 的合同（连续阅读、
文本模式、移动端）用的是假 token，而 `page.route` 拦不到 WebSocket，所以连接到真实服务时被拒。这些合同本来就会
过滤这类控制台错误，`4092ea2` 基线上也一样。

**临时变异**：每项都只改工作区里的 Reader.vue，先跑单元合同，再构建到独立目录，随后立即 `git checkout` 还原，
确认工作树与 HEAD 一致，没有提交。浏览器合同对 `035762d` 的脚本运行：

| 变异 | 单元合同 | EPUB 1440×900 | EPUB 390×844、360×800 | 连续 1440×900、1024×1366 | 连续 390×844、360×800 |
| --- | --- | --- | --- | --- | --- |
| M1：桌面在页头里用内联表达式重新渲染计数，不经过 `chapterLabel` | FAIL `:114` | FAIL `:737`，实际为 `第 3 章 第二章3 / 3` | PASS | FAIL `:178`，多出 `1 / 6` | PASS |
| M2：计数移出页头，恢复 `chapterLabel`，作为 `hidden` 元素在所有视口留在 DOM 里 | `:114` 通过，FAIL `:119`：模板不得渲染 `{{ chapterLabel }}` | FAIL `:742`：body 含 `3 / 3`（`:737` 页头相等已通过） | FAIL `:742`，同左 | PASS | PASS |
| M3：桌面整个不渲染页头（`v-if` 加上 `isMobileReader`，照搬上游空 top-bar） | FAIL `:114` | FAIL `:710`：运行标题等待 30 秒超时 | PASS | FAIL `:152`：`initial` 采样抛 `TypeError`，页头为 null（`:156` `header.getBoundingClientRect()`） | PASS |

- M1 没有直接恢复原来那一行，是为了证明单元合同守的是页头结构，而不只是名字匹配。原样恢复那一行就是
  `4092ea2` 的产品代码，红测阶段已经覆盖。
- M2 证明“隐藏的计数也不允许”有两道防线：EPUB 的 body 断言和单元合同。连续合同只比较页头文本，所以通过，与
  7.3 的设计一致。
- M3 证明这些测试同时守住用户要求的桌面运行标题：照上游把桌面页头整个删掉，单元合同、EPUB 1440×900 和连续合同
  的两个桌面视口都会失败。
- 三个变异服务的日志同样没有非 WebSocket 的 4xx/5xx，也没有 panic。

**其余门**：

- frontend `npm run build`：通过，只有既有的两条 Rolldown `#__PURE__` 注释位置警告。
- 跳过并说明原因：Go `test`/`vet`/`race`（后端无 diff）；依赖 advisory 门、Compose 与卷门（依赖、Dockerfile、
  数据格式都没改，卷门只在 Actions 上运行）；其他 Reader 浏览器合同（本切片只删掉页头的一个子元素；`scripts/smoke`
  中只有 EPUB 与连续合同读取 `.reader-page-head`，CBZ 不渲染页头）。
- `035762d` 改了 `frontend/**`，命中 `docker-publish.yml` 的路径过滤，合入 main 会触发镜像发布；是否合入由用户决定。

**状态**：Git 为 `claude/suspicious-ptolemy-1af85f` 上的本地提交（基于 `claude/nervous-saha-c99e83` 的 `4092ea2`，
其中包含 `claude/sleepy-moser-50391e` 的 4 个提交），未推送、未合入 main。镜像：未构建、未推送，已发布的镜像仍带
桌面计数。生产：未访问、未操作。

2026-10-17 补记：第 5 节最后一行留给另行盘点的浏览器标签页标题见
[`browser-document-title-fixed-baseline-second-audit-p0-contract.md`](browser-document-title-fixed-baseline-second-audit-p0-contract.md)。
其 Reader 部分已在 `claude/xenodochial-carson-5004c3` 上实现（`4a0aae8`，以本切片的 `9839270` 为基线，本地回归通过，
尚未发布）；本切片的提交、结论和状态不变。

## 11. 发布记录（2026-10-17）

经用户确认，`refs/heads/claude/xenodochial-carson-5004c3` 快进合入，main 由 `5bc38a3` 快进至
`2528d9c2f905f0098f98155aa733f9138f926052`（非 force，ls-remote 核验）。发布前本地门：Go 29 包与 vet、frontend
805/805 与 build、审计门测试 128/128、Go/npm 实际扫描、Compose；本地 Go 服务（临时数据）加生产前端跑 openreader-smoke
三视口、document-title 三视口 × navigation/auth/remote、EPUB 三视口、连续阅读、重新认证四视口、移动端（含 iPad）合同，
全部通过，访问日志无 5xx（401 只来自 mock 合同伪造 token 的 `/ws/sync`）。可信 Actions `39039046891` 成功（20m49s，
两平台 OS 门 pass-with-unknown-origins，新旧卷与 portable 备份通过）。`ghcr.io/changshengyu/openreader:2528d9c` 与
`latest` 指向 index `sha256:18b6c2992d9ef02bb89474186501afa8c9f72b3575bf60cfab1ba2d7ac9121fa`（amd64
`sha256:e47d2f6984e3075f0ef6ee4f9ba068ba9bdaf95eed2238424fbe54963a802906`，arm64
`sha256:bd61d51d650f10f58e08e82176bc2695743234a9dad7114df3f653cb93b7c090`）。按 digest 独立核验两平台：`/app/public`
63 个文件与本地同一提交的 build 逐字节一致，bundle 含 document.title 写入；zlib 1.3.2-r1、SQLite 3.51.3 与完整
revision 标签保持。生产未访问，实际运行版本未知。
本切片（去掉桌面 `N / total`，`035762d`）随同一镜像发布，状态 **released / awaiting-device-verification**。
