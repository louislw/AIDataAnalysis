# 自然语言数据分析：Go + SQLite 方案

用自然语言提问，系统在授权范围内查询 SQLite，返回带来源的中文分析。后端是 Go，应用数据和被分析的数据都是 SQLite 文件。前端是 Vue 3 + Element Plus。模型只通过第三方 OpenAI 兼容接口调用，本方案不包含模型部署。

## 1. 产品行为

用户选择一个虚拟人物后提问。人物决定四件事：能看哪些表、系统提示和业务口径、行级过滤、哪些列是敏感列。

一次成功的回答必须同时满足：

- 至少执行过一次成功的 `SELECT`
- 正文是 Markdown，含结论和必要的表格
- 单独给出依据：执行的 SQL、涉及的表、时间等过滤条件、返回行数、是否被截断

本轮没有成功查询时，只允许两种结果：

- **澄清**：口径有歧义时，只提一个问题，不给数字结论
- **失败**：工具或 SQL 失败，说明原因，不编造分析

同一会话的后续问题能接上上文。送进模型的是最近几轮的「问题 + 执行过的 SQL + 一行结果摘要」，不送整表数据。

## 2. 技术选型

| 层 | 选择 | 说明 |
|---|---|---|
| 后端 | Go 1.23 | 单二进制 |
| HTTP | `chi` | 路由和中间件 |
| 应用库 | `modernc.org/sqlite` | 纯 Go，无需 CGO。文件 `data/app.db`，WAL |
| 业务库 | 同一个驱动，只读打开 | 管理员登记的 `.sqlite` 文件 |
| SQL 访问 | `database/sql` | 每个业务库一个只读连接池 |
| 密码 | `bcrypt` | |
| 登录态 | `golang-jwt/jwt/v5` | |
| 模型 | 自写的薄 HTTP 客户端 | `POST {base}/chat/completions`，解析 `tool_calls` |
| 前端 | Vue 3 + Vite + Vue Router + Pinia + Element Plus | `marked` 渲染 Markdown |

不引入 LangChain、Eino 或 ORM。Agent 循环、工具和 SQL 守卫都在本仓库里，便于把「没查到就不许下结论」写死。

业务库第一期只有 SQLite。连接放在 `datasource.Opener` 后面，以后要加别的数据库只加驱动，不改 Agent。

## 3. 部署形态

```
浏览器
  Vue 3 + Element Plus
        │  /api  JSON + JWT
Go 进程  :8081
  ├─ data/app.db            用户、人物、口径、会话、表字段说明
  ├─ data/sources/*.sqlite  被分析的业务库（只读）
  └─ HTTPS                  第三方 Chat Completions API
```

不再需要单独的 MySQL，也不再需要 llama.cpp。密钥和模型名来自环境变量。

## 4. 相对原 Java 版改掉的行为

这些是功能设计，不是实现细节。

**目录放进提示词。** 人物保存或表注释变更时，生成一份目录：授权表、字段、业务说明、敏感列名单、指标口径、示例 SQL、禁止事项。提问时目录随系统提示一起发送。模型直接写 SQL。`listTables` 和 `getTableSchema` 仍保留，只在目录不够时使用。

**失败不降级成臆测。** tool call 格式错误只重试有限次。SQL 执行错误把数据库报错交回模型，让它在剩余轮次里改写。轮次用尽或无法形成合法查询时，返回失败。不存在「去掉工具再让模型写一段分析」。

**样本和合计分开。** 明细查询在 SQL 上限制行数，并在依据里标记截断。用户问合计、占比、排名时，模型必须写聚合 SQL。工具结果里写明「这是前 N 行明细，不是全量」。

**敏感列有实际效果。** 敏感列不出现在给模型的目录说明里。语法检查要求：敏感列只能作为 `SUM` / `COUNT` / `AVG` / `MIN` / `MAX` 的参数出现，不能出现在 SELECT 明细、`GROUP BY`、`ORDER BY` 或条件里。违规 SQL 直接拒绝，并把原因回给模型。

**多轮上下文。** 同一 `sessionId` 把最近 6 轮的问题、SQL 和一行摘要放进消息。换主题用「新对话」，旧口径不会串进来。

**授权补全。** 人物要对角色授权，也可以再授权到具体用户。管理员始终可用。没勾选任何表就是不能查。

**行级条件。** 人物可配置一条过滤，例如 `dept_id = {{user.dept}}`。保存时做语法检查，只允许列名、占位符、比较和 `AND` / `OR`。执行时在内存库里为每张授权表建临时视图，过滤已经写在视图里。模型只能查这些视图。

**回答带依据。** 每条助手消息保存 SQL、表名、行数、是否截断、推理步骤和 `kind`（`answer` / `clarify` / `error`）。

## 5. 查询是怎么被圈住的

业务文件以 `mode=ro` 打开。每次执行使用独立的内存库，不把写权限带进业务文件：

```
:memory:
  ATTACH 'data/sources/hr.sqlite' AS src   -- 只读
  CREATE TEMP VIEW employee AS
    SELECT id, name, dept_id, salary       -- 敏感列是否露出由语法守卫决定
    FROM src.employee
    WHERE dept_id = 'D3';                  -- 人物行级条件，已替换成字面量
```

SQLite authorizer 再拦一层：

- 拒绝 `INSERT` / `UPDATE` / `DELETE` / `DDL` / `ATTACH` / `DETACH` / `PRAGMA` 写操作
- 拒绝读取 `src` 下的基表，只允许查询临时视图
- 视图名与授权表一致，模型写的 SQL 仍使用原来的表名

语句使用 `context` 超时。驱动层 `SetMaxRows` 之外，还在 SQL 守卫里对非聚合查询补上 `LIMIT`。单条语句，不允许多语句。

登记数据源时，路径必须落在 `DATA_ROOT/sources` 之下，拒绝 `..` 和符号链接逃逸。

## 6. 应用库表

`data/app.db` 由嵌入的 SQL 迁移创建。

```sql
CREATE TABLE sys_user (
  id            INTEGER PRIMARY KEY,
  username      TEXT NOT NULL UNIQUE,
  password_hash TEXT NOT NULL,
  role          TEXT NOT NULL CHECK (role IN ('ADMIN', 'USER')),
  display_name  TEXT NOT NULL DEFAULT '',
  dept          TEXT NOT NULL DEFAULT '',
  enabled       INTEGER NOT NULL DEFAULT 1,
  created_at    TEXT NOT NULL
);

CREATE TABLE datasource (
  id         INTEGER PRIMARY KEY,
  name       TEXT NOT NULL,
  file_name  TEXT NOT NULL,          -- 仅文件名，位于 DATA_ROOT/sources
  status     INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE persona (
  id            INTEGER PRIMARY KEY,
  name          TEXT NOT NULL,
  description   TEXT NOT NULL DEFAULT '',
  system_prompt TEXT NOT NULL DEFAULT '',
  prohibitions  TEXT NOT NULL DEFAULT '',
  row_filter    TEXT NOT NULL DEFAULT '',
  datasource_id INTEGER REFERENCES datasource(id),
  status        INTEGER NOT NULL DEFAULT 1,
  catalog_text  TEXT NOT NULL DEFAULT '',
  created_at    TEXT NOT NULL,
  updated_at    TEXT NOT NULL
);

CREATE TABLE persona_table_grant (
  persona_id INTEGER NOT NULL,
  table_name TEXT NOT NULL,
  PRIMARY KEY (persona_id, table_name)
);

CREATE TABLE persona_role_grant (
  persona_id INTEGER NOT NULL,
  role_code  TEXT NOT NULL,
  PRIMARY KEY (persona_id, role_code)
);

CREATE TABLE persona_user_grant (
  persona_id INTEGER NOT NULL,
  user_id    INTEGER NOT NULL,
  PRIMARY KEY (persona_id, user_id)
);

CREATE TABLE table_meta (
  datasource_id INTEGER NOT NULL,
  table_name    TEXT NOT NULL,
  custom_comment TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  is_hidden     INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (datasource_id, table_name)
);

CREATE TABLE column_meta (
  datasource_id INTEGER NOT NULL,
  table_name    TEXT NOT NULL,
  column_name   TEXT NOT NULL,
  custom_comment TEXT NOT NULL DEFAULT '',
  description   TEXT NOT NULL DEFAULT '',
  is_sensitive  INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (datasource_id, table_name, column_name)
);

CREATE TABLE persona_metric (
  id         INTEGER PRIMARY KEY,
  persona_id INTEGER NOT NULL,
  name       TEXT NOT NULL,
  definition TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE persona_example (
  id         INTEGER PRIMARY KEY,
  persona_id INTEGER NOT NULL,
  question   TEXT NOT NULL,
  sql_text   TEXT NOT NULL,
  sort_order INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE chat_session (
  id         TEXT PRIMARY KEY,
  user_id    INTEGER NOT NULL,
  persona_id INTEGER NOT NULL,
  title      TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE chat_message (
  id          INTEGER PRIMARY KEY,
  session_id  TEXT NOT NULL,
  role        TEXT NOT NULL CHECK (role IN ('USER', 'ASSISTANT')),
  content     TEXT NOT NULL,
  kind        TEXT NOT NULL DEFAULT '',   -- answer / clarify / error
  sql_text    TEXT NOT NULL DEFAULT '',
  tables_used TEXT NOT NULL DEFAULT '[]',
  row_count   INTEGER,
  truncated   INTEGER NOT NULL DEFAULT 0,
  reasoning   TEXT NOT NULL DEFAULT '',
  created_at  TEXT NOT NULL
);
```

隐藏表不进入目录，也不能被授权。修改表注释、字段注释、敏感标记、口径或示例后，重写该人物的 `catalog_text`。

## 7. 后端模块

```
cmd/server/main.go
internal/
  config/       环境变量
  http/         路由、JWT、请求体
  auth/
  store/        app.db 迁移与读写
  catalog/      从业务库 sqlite_master、PRAGMA table_info 和 meta 表生成目录
  datasource/   路径校验、只读打开、连接缓存
  sqlguard/     单条 SELECT、抽表名、敏感列、补 LIMIT
  query/        内存库 + ATTACH + 临时视图 + authorizer + 执行
  agent/        循环、提示词、多轮消息、kind 判定
  llm/          ChatClient
web/            Vue 构建产物，Go 在生产模式直接托管
```

### Agent 循环

1. 确认人物启用，当前用户通过角色授权或用户授权。管理员跳过这项检查。
2. 组装系统提示：人物设定、目录、禁止事项、输出要求（要么澄清一个问题，要么基于查询结果作答）。
3. 追加本轮用户问题和最近 6 轮摘要。
4. 调用模型，工具为 `executeSql`、`listTables`、`getTableSchema`。
5. 出现 tool call：校验参数，执行，把结果追加为 tool message。SQL 报错原文返回。然后回到第 4 步。
6. 没有 tool call：若本轮已有成功查询，`kind=answer`；若正文是澄清且不含数据表，`kind=clarify`；否则 `kind=error`。
7. 循环超过 `AGENT_MAX_LOOP`（默认 6）则为 `error`。
8. 写入 `chat_message`。用户消息和助手消息各一条。

`executeSql` 的返回文本包含：行数、是否截断、Markdown 表格（单元格过长则截断）。聚合查询不强制追加 `LIMIT`。

### 模型客户端

```
LLM_BASE_URL   例如 https://api.example.com/v1
LLM_API_KEY
LLM_MODEL
LLM_TIMEOUT    默认 120s
```

只依赖 Chat Completions 的 `messages`、`tools`、`tool_calls`。供应商需要支持 function calling。`GET /api/health` 不访问模型，避免探活消耗额度。

## 8. HTTP API

除登录和健康检查外，请求带 `Authorization: Bearer`。管理接口要求 `ADMIN`。

| 方法 | 路径 | 说明 |
|---|---|---|
| POST | `/api/auth/login` | 返回 `token`、`username`、`role`、`displayName` |
| POST | `/api/auth/register` | 注册为 `USER` |
| GET | `/api/auth/me` | |
| GET/PUT | `/api/auth/users`、`/api/auth/users/{id}` | 角色、部门、启用 |
| GET | `/api/health` | 进程存活 |
| GET/POST | `/api/datasources` | 登记或列出。只接收文件名 |
| POST | `/api/datasources/test` | 只读打开并读取表数量 |
| PUT/DELETE | `/api/datasources/{id}` | |
| GET | `/api/datasources/{id}/tables` | `sqlite_master` + 注释 + 隐藏标记 |
| GET | `/api/datasources/{id}/tables/{t}/columns` | `PRAGMA table_info` + 注释 + 敏感标记 |
| GET | `/api/datasources/{id}/tables/{t}/preview` | 预览也走敏感列掩码，最多 100 行 |
| PUT | `/api/datasources/{id}/tables/{t}/meta` | |
| PUT | `/api/datasources/{id}/tables/{t}/columns/{c}/meta` | |
| GET | `/api/personas/mine` | 当前用户可用的人物 |
| CRUD | `/api/personas` | 含 `systemPrompt`、`prohibitions`、`rowFilter`、`datasourceId` |
| PUT | `/api/personas/{id}/tables` | 表授权，空数组表示不能查 |
| PUT | `/api/personas/{id}/roles` | |
| PUT | `/api/personas/{id}/users` | 用户授权，空数组表示不额外限制到人 |
| PUT | `/api/personas/{id}/metrics` | 整体替换口径列表 |
| PUT | `/api/personas/{id}/examples` | 整体替换示例问题 |
| POST | `/api/personas/{id}/catalog/rebuild` | 重算目录 |
| GET/POST | `/api/personas/{id}/sessions` | 会话列表、新建 |
| GET/DELETE | `/api/sessions/{id}` | 含消息；只能访问自己的 |
| POST | `/api/sessions/{id}/query` | body：`{ "question": "..." }` |

`/query` 响应：

```json
{
  "kind": "answer",
  "content": "markdown",
  "sql": "SELECT ...",
  "tables": ["employee"],
  "rowCount": 12,
  "truncated": false,
  "reasoning": "...."
}
```

`kind` 为 `clarify` 或 `error` 时，`sql` 为空字符串，`content` 是问题或失败说明。

## 9. 前端

Vue Router 页面：

| 路由 | 角色 | 内容 |
|---|---|---|
| `/login` | 公开 | 登录、注册 |
| `/` | 登录 | 重定向到最近使用的人物 |
| `/p/:personaId` | 登录 | 问答 |
| `/admin/datasources` | ADMIN | 数据源列表、登记、测试 |
| `/admin/datasources/:id` | ADMIN | 左表右字段：注释、隐藏、敏感、预览 |
| `/admin/personas` | ADMIN | 人物列表 |
| `/admin/personas/:id` | ADMIN | 四个页签：设定、授权、口径与示例、行级条件 |

问答页布局：

- 左侧：人物列表；选中后是该人物的会话列表和「新对话」
- 中间：消息。助手消息用 `kind` 区分结果、澄清和失败
- 依据区默认展开 SQL、表、行数和截断说明。分析步骤放在折叠面板
- 输入框在本轮请求完成前禁用。超时与后端 `LLM_TIMEOUT` 对齐

管理页保存口径、示例、注释或授权后，调用目录重建，并提示目录已更新。表授权的空状态文案是「未授权任何表，该人物不能查询」。

视觉使用 Element Plus 默认体系，侧栏深色、内容区白色。不另做设计系统。

开发时 Vite 把 `/api` 代理到 `http://127.0.0.1:8081`。生产由 Go 托管 `web/dist`。

## 10. 配置

```
APP_ADDR=:8081
DATA_ROOT=./data
JWT_SECRET=
LLM_BASE_URL=
LLM_API_KEY=
LLM_MODEL=
LLM_TIMEOUT=120s
AGENT_MAX_LOOP=6
AGENT_MAX_ROWS=50
AGENT_HISTORY_TURNS=6
BOOTSTRAP_ADMIN_PASSWORD=
BOOTSTRAP_USER_PASSWORD=
```

首次启动且没有任何用户时，创建 `admin`（ADMIN）和 `user`（USER），密码取自上述变量。密码不写进代码库。

## 11. 实现顺序

1. **应用壳。** 迁移、配置、登录注册、用户的角色和部门。此时可以登录。
2. **数据源与目录素材。** 登记 SQLite 文件、只读测试、表和字段、注释、隐藏、敏感、预览掩码。
3. **人物。** 授权、口径、示例、行级条件校验、目录生成。
4. **安全执行。** 内存库、临时视图、authorizer、语法守卫、行数限制。用固定 SQL 测越权表、写语句、敏感列和行级条件，先不接模型。
5. **Agent 与会话。** 接第三方 API，打通提问、澄清、失败、多轮和依据字段。
6. **前端。** 登录、问答三栏、数据源、人物四个页签。用真实接口走完「登记库 → 配人物 → 提问 → 看到 SQL」。

每步都可以单独运行。第 4 步的测试不依赖外网。

## 12. 本期不做

- 模型部署、本地推理、按难度切换模型
- MySQL 或其他业务库驱动
- 流式输出、导出 Excel
- 把完整结果表送回模型
- 通用行权限引擎。只有人物上的一条过滤条件
- 兼容原 Java 接口。前后端一起换，按本文件的路径实现
