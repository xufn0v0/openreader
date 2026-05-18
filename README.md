# OpenReader

OpenReader 是一个自部署、轻量级、多端同步的小说阅读器。当前仓库已经按项目交接文档配置为：

- 后端：Go 1.22、Gin、GORM、SQLite WAL、Gorilla WebSocket、goquery
- 前端：Vue 3、Vite、Vue Router、Pinia、Axios
- 部署：Docker 多阶段构建，单容器运行，`data/` 保存 SQLite，`cache/` 保存章节缓存，`library/` 保存导入原文件

## 已可验收功能

- 本地导入：支持 `.txt`、`.text`、`.md`、`.epub`
- TXT 分章：识别 `第一章`、`第1章`、`第十回` 等中文章节标题
- EPUB 分章：读取 `container.xml`、OPF manifest/spine，并按 spine 顺序导入章节
- 分类：创建分类，导入时选择分类，书架按分类/未分组过滤
- 章节缓存：导入后章节正文写入 `cache/`，数据库只保存元数据、目录、缓存路径和阅读状态
- 导入归档：原始书籍会落到 `library/data/用户名/书名_作者/`，同目录生成 `bookSource.json` 与 `chapters.json`
- 书签：阅读页创建书签，书籍详情页展示并可跳回章节位置
- 阅读模式：滚动、翻页、分页三种模式，翻页模式支持上一页/下一页
- 进度：保存章节、偏移、百分比和阅读模式，后端保留 WebSocket 同步通道

## 本地开发

后端：

```bash
cd backend
go mod tidy
go run .
```

前端：

```bash
cd frontend
npm install
npm run dev
```

默认地址：

- 前端：`http://localhost:5173`
- 后端：`http://localhost:8080`
- 健康检查：`http://localhost:8080/api/health`

验证：

```bash
cd backend
go test ./...

cd ../frontend
npm run build
npm audit --audit-level=moderate
```

## Docker / OrbStack

```bash
cp .env.example .env
docker compose up --build
```

推荐使用 OrbStack 或 Docker Desktop 提供 Docker 运行时。安装并启动后，确认命令可用：

```bash
docker version
docker compose version
```

服务会监听 `http://localhost:8080`，并从 Go 服务直接托管前端静态文件。

常用部署命令：

```bash
# 构建镜像
docker compose build

# 后台启动
docker compose up -d

# 查看日志
docker compose logs -f openreader

# 停止服务
docker compose down
```

生产部署前建议把 `.env` 中的 `OPENREADER_JWT_SECRET` 改成足够长的随机字符串。

需要持久化的目录：

- `data/`：SQLite 数据库，保存用户、分类、书架、书签、阅读进度
- `cache/`：按章节拆分后的正文缓存，供阅读页快速加载
- `library/`：用户导入的原始书籍文件及可迁移目录文件，例如 `library/data/yuchangsheng/御仙1-86_/御仙1-86.txt`

`docker-compose.yml` 已默认挂载这三个目录。以后重建镜像或换机器部署时，把 `data/`、`cache/`、`library/` 一起带走即可。

## 当前环境状态

已通过 Homebrew 安装本地开发工具：

- Go：`go1.26.3 darwin/arm64`
- Node：`v26.0.0`
- npm：`11.12.1`

Docker 运行时预留给 OrbStack；当前尚未检测到 `docker` 命令。安装 OrbStack 后即可执行 Docker 部署命令。
