# sqliteviewer

一个可执行的 SQLite 可视化工具，后端使用 Go+gin，前端使用 Vue3，支持在服务器上部署并通过浏览器查看／编辑 SQLite 数据。

## 功能特性

- 通过 `-db` 指定任意 SQLite 数据库文件并启动内置 HTTP 服务
- 自动嵌入前端资源，开箱即用（可选 `-static` 覆盖自定义前端目录）
- 列出所有用户表、分页查看数据
- 行级操作：新增、编辑、删除
- 导出表数据为 `CSV / JSON / SQL`

## 运行要求

- Go 1.21+（模块中声明的是 1.25，但向下兼容 1.21即可）
- Node.js 18+ & pnpm 8+（仅在需要修改前端时）

## 本地开发

```bash
# 安装依赖
pnpm install --dir frontend

# 重新构建前端（输出到 internal/server/ui/dist 以便 Go embed）
pnpm --dir frontend build

# 运行后端
go run ./cmd/sqliteviewer -db /path/to/your.db
```

访问 `http://localhost:8080` 即可打开界面。

## 运行参数

| 参数 | 说明 | 默认值 |
|------|------|--------|
| `-db` | **必填**，SQLite 文件路径 | 无 |
| `-addr` | HTTP 服务监听地址 | `:8080` |
| `-static` | 可选，覆盖默认嵌入的前端目录 | 空（使用内置） |

示例：

```bash
sqliteviewer -db ./example.db
sqliteviewer -db ./example.db -addr 0.0.0.0:9000
sqliteviewer -db ./example.db -static ./frontend/dist
```

## 项目结构

```
cmd/sqliteviewer        程序入口、启动参数
internal/server         gin HTTP 服务、SQLite 操作、静态资源嵌入
frontend                Vue3 + Vite 前端项目
```

## 数据修改注意

- 依赖 SQLite 隐式 `rowid` 进行编辑和删除；如果数据表使用 `WITHOUT ROWID`，暂不支持编辑。
- 后端未做鉴权，请勿直接暴露在不可信网络。

## License

MIT (如需其他协议请自行修改)。

