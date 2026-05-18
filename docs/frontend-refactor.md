# OpenReader 前端重构总结

## 项目概况

**OpenReader** — 自部署轻量级多端同步小说阅读器。

- 后端：Go 1.22、Gin、GORM、SQLite WAL
- 前端：Vue 3、Vite、Vue Router、Pinia、Element Plus
- 部署：Docker 多阶段构建

---

## 第一阶段：Naive UI → Element Plus 迁移

reader3 原项目用 Element UI (Vue2)，本项目对应方案是 Element Plus (Vue3)。
所有 9 个文件从 Naive UI 替换为 Element Plus：

| 文件 | 主要组件 | 说明 |
|------|---------|------|
| `main.js` | — | 全局注册 Element Plus + 中文 locale + 全部图标 |
| `App.vue` | `el-container` / `el-aside` / `el-menu` | 深色侧边栏布局，参考 reader3 导航区 |
| `Login.vue` | `el-form` + `el-input` | 登录/注册页 |
| `Home.vue` | `el-tabs` / `el-card` / `el-upload` | 分类标签 + 书架网格 + 导入书籍 |
| `BookDetail.vue` | `el-card` / `el-dialog` | 书籍信息 + 书签列表 + 目录 + 换源弹窗 |
| `Search.vue` | `el-input` / `el-checkbox` / `el-card` | 远程搜索 + 书源选择 + 侧边栏搜索联动 |
| `Sources.vue` | `el-table` / `el-switch` / `el-dialog` | 书源管理 + 启停 + 导入导出 + 调试面板 |
| `LocalStore.vue` | `el-table` | 本地书仓列表 + 批量导入 |
| `Reader.vue` | `el-drawer` / `el-slider` / `el-color-picker` / `el-radio-group` | 目录/设置抽屉 + 字号/亮度/行高/主题 |

---

## 第二阶段：阅读器翻页模式修复

### 问题

翻页模式下一次只翻一部分，翻不全。

### 根因

用 CSS `columns` 多列布局 + `translateX` 切页。CSS `column-width` 是浏览器**建议值**，实际渲染列宽往往不等于设定值，导致 `translateX` 位移量和列宽不匹配。

### 解决方案

参照 reader3 非 slide-reader 模式的分页机制：**按可视区高度垂直分页 + translateY 切页**。

```
页数   = ceil(body高度 / viewport高度)
切页   = translateY(-页码 × viewport高度)
```

位移量和页高完全一致，翻页精准不会翻一半。

### 三种阅读模式

| 模式 | 机制 | 翻页操作 |
|------|------|---------|
| 滚动 | `overflow-y: auto` 自由滚动 | 自由滚动 |
| 翻页 | `overflow: hidden` + `translateY` 逐屏切页 | 到首/尾自动切章 |
| 分页 | 同翻页 | 同翻页 |

### 附加修复

键盘 `←` `→` 从翻章改为翻页（`PageUp` / `PageDown` / `Space` 也是翻页）。`Home` 跳到章首，`End` 跳到章尾，`Esc` 关面板/返回详情页。

---

## 改动清单

```
frontend/
├── package.json              # naive-ui → element-plus + @element-plus/icons-vue
├── src/
│   ├── main.js               # Element Plus 全局注册 + 中文 + 图标
│   ├── App.vue               # el-* 侧边栏布局，参考 reader3
│   ├── styles/global.css     # 精简至 ~10 行
│   ├── composables/
│   │   └── useKeyboard.js    # 左右键 → 翻页（非翻章）
│   ├── views/
│   │   ├── Login.vue         # Element Plus 重写
│   │   ├── Home.vue          # Element Plus 重写
│   │   ├── BookDetail.vue    # Element Plus 重写
│   │   ├── Search.vue        # Element Plus 重写 + 侧边栏搜索联动
│   │   ├── Sources.vue       # Element Plus 重写
│   │   ├── LocalStore.vue    # Element Plus 重写
│   │   └── Reader.vue        # Element Plus 重写 + 翻页机制修复
```

---

## 构建状态

`npm run build` 通过，无编译错误。

---

## 本地开发

```bash
cd frontend && npm run dev    # 前端 http://localhost:5173
cd backend && go run .        # 后端 http://localhost:8080
```
