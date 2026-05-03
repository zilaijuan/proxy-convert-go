# Proxy Convert Go 重构设计文档

## 1. 背景

当前项目已经具备完整的基础能力：订阅链接提取、协议解析、SQLite 存储、节点验证、Clash/V2Ray 导出、定时任务和 HTTP API。现有代码可以运行，测试基线通过，但模块边界仍然偏松散，部分业务逻辑、基础设施逻辑和 HTTP 处理逻辑混在一起。

本次重构的目标不是重写项目，而是在保持现有功能兼容的前提下，逐步整理结构，让后续新增订阅源、协议类型、验证策略和导出格式更容易。

## 2. 当前主要问题

### 2.1 extractor 职责过重

`internal/extractor/extractor.go` 同时负责：

- 创建 HTTP client 并请求远程内容
- 解析 GitHub/raw 文本订阅
- 解析 V2rayse HTML / Nuxt 数据
- 识别和解码 base64
- 去重
- 输出日志

这些职责混在一起后，单元测试难写，也不利于新增其他订阅源。

### 2.2 service 与具体实现耦合

`ExtractorService` 直接创建 `GitHubExtractor` 和 `V2rayseExtractor`，并在方法内部读取全局配置。这样会带来几个问题：

- 测试时很难替换真实网络请求
- 业务服务依赖具体来源，而不是抽象能力
- 配置热更新逻辑分散在业务层调用处

### 2.3 handlers 文件过大

`internal/handlers/handlers.go` 中集中注册了所有路由，并直接写入大量匿名 handler。随着 API 增长，这个文件会越来越难维护。

另外，当前 `:id` 参数绑定方式存在风险：局部变量不会被正确写回，相关 API 可能始终使用默认值。

### 2.4 共享逻辑重复

以下逻辑在多个位置重复或分散：

- status 查询参数解析
- 文本响应拼接
- links 去重
- HTTP 内容读取
- base64 判断和解码
- 默认配置构造

### 2.5 全局状态较多

目前存在几类全局状态：

- config 的全局配置和 watcher
- logger 的全局 logger
- extractor 中的临时可变字段

全局状态不是必须全部移除，但应该把业务代码对全局状态的依赖收敛到明确边界。

## 3. 重构目标

### 3.1 保持现有功能兼容

重构后应保持以下能力不变：

- 导入 URL 订阅
- 导入文本订阅
- 从配置中的 V2rayse URL 提取节点
- 从配置中的 GitHub/raw URL 提取节点
- 解析 SS、VMess、VLESS、Trojan、Hysteria2、AnyTLS
- 存储、查询、删除、统计链接
- 验证节点
- 导出 Clash YAML / JSON
- 导出 V2Ray 原始链接文本
- 定时执行提取、验证和清理任务

### 3.2 新增来源只需要新增一个代码文件

定时提取能力应从“固定调用 V2rayse 和 GitHub 两个方法”重构为“遍历一组已注册的提取来源”。新增一个需要特殊解析的网站时，理想步骤应只有：

1. 在 `internal/extractor/sources/` 下新增一个文件，例如 `example.go`。
2. 在该文件中实现该网站自己的页面解析逻辑。
3. 在该文件的 `init()` 或显式注册函数中注册 source。
4. 在 `config.yaml` 中增加对应 source 配置。

除此之外，以下步骤应完全复用现成流程：

- HTTP 获取
- 超时控制
- 日志记录
- 错误汇总
- 提取结果去重
- 代理协议解析
- fingerprint 生成
- 数据库写入
- 导入统计
- 定时任务编排

### 3.3 提升模块边界清晰度

建议把项目逐步整理为以下方向：

```text
internal/
  app/             # 应用装配，负责 wire services、routes、scheduler
  config/          # 配置加载、默认值、热更新
  database/        # SQLite 实现
  domain/          # 核心领域类型，例如 Link、Proxy、ImportResult
  extractor/       # 订阅源提取、注册和内容解析
  handlers/        # HTTP 层，参数解析和响应转换
  logger/          # 日志实现
  parser/          # 代理协议解析
  scheduler/       # 定时任务编排
  service/         # 业务用例层
```

第一阶段不一定需要一次性新增所有目录。尤其是 `domain/` 和 `app/` 可以等依赖关系稳定后再引入。

### 3.4 提升可测试性

核心业务应尽量依赖接口，而不是直接依赖网络、数据库或全局配置。例如：

```go
type SourceExtractor interface {
    Extract(ctx context.Context, urls []string) ([]string, error)
}

type LinkRepository interface {
    AddLink(link string, status int, fingerprint string, tag string) (int64, error)
    GetAllLinks(statuses []int, limit int, offset int) ([]database.Link, error)
}
```

实际接口应以当前代码使用面为准，避免为了抽象而抽象。

## 4. 提取来源扩展设计

### 4.1 核心思想

把所有站点提取逻辑统一抽象成 `Source`。每个 `Source` 只负责一件事：从配置给定的 URL 中提取出原始代理链接列表。

通用流程由 `ExtractorService` 负责：

```text
读取配置中的 sources
  -> 根据 source.type 从 registry 找到实现
  -> 使用统一 Fetcher 获取网页或订阅内容
  -> 调用 source.Extract 解析代理 URL
  -> 汇总所有 source 的结果
  -> 全局去重
  -> parser.ParseLink
  -> 生成 fingerprint/name
  -> database.AddLink
  -> 返回 ImportResult
```

### 4.2 推荐目录结构

```text
internal/extractor/
  extractor.go          # Source 接口、SourceConfig、ExtractResult 等公共类型
  registry.go           # RegisterSource / GetSource / ListSources
  runner.go             # 遍历配置 sources，执行所有来源
  fetcher.go            # Fetcher 接口和 HTTPFetcher 实现
  content_parser.go     # 通用订阅文本解析
  base64.go             # base64 判断和解码 helper
  dedupe.go             # 去重 helper
  sources/
    github.go           # GitHub/raw source
    v2rayse.go          # V2rayse source
```

后续新增来源时，例如新增 `FooBar` 网站，只新增：

```text
internal/extractor/sources/foobar.go
```

### 4.3 Source 接口

推荐接口：

```go
type Source interface {
    Type() string
    Extract(ctx context.Context, req SourceRequest) ([]string, error)
}

type SourceRequest struct {
    URLs    []string
    Fetcher Fetcher
    Options map[string]string
}

type Fetcher interface {
    Fetch(ctx context.Context, url string) (string, error)
}
```

设计原则：

- `Type()` 用来匹配配置里的 `type`。
- `Extract()` 返回原始代理链接，不负责入库。
- `Fetcher` 由外部注入，source 不自己创建 `http.Client`。
- `Options` 用于给少数来源传特殊参数，避免每新增来源就改全局 config struct。

### 4.4 Registry 设计

推荐实现一个轻量 registry：

```go
var sources = map[string]Source{}

func RegisterSource(source Source) {
    sources[source.Type()] = source
}

func GetSource(sourceType string) (Source, bool) {
    source, ok := sources[sourceType]
    return source, ok
}
```

每个来源文件自己注册：

```go
func init() {
    extractor.RegisterSource(NewV2rayseSource())
}
```

如果不希望使用 `init()`，也可以在 `sources/register.go` 中集中注册。但为了满足“新增一个代码文件”的目标，推荐使用 `init()` 自动注册。

### 4.5 配置设计

当前配置：

```yaml
extractor:
  v2rayseURLs:
    - https://test.v2rayse.com/live-node
    - https://test.v2rayse.com/free-node
  githubURLs:
    - https://cdn.jsdmirror.com/gh/arshiacomplus/v2rayExtractor/mix/sub.html
```

建议迁移为：

```yaml
extractor:
  sources:
    - type: v2rayse
      enabled: true
      urls:
        - https://test.v2rayse.com/live-node
        - https://test.v2rayse.com/free-node

    - type: github
      enabled: true
      urls:
        - https://cdn.jsdmirror.com/gh/arshiacomplus/v2rayExtractor/mix/sub.html

    - type: foobar
      enabled: true
      urls:
        - https://example.com/proxy-page
      options:
        selector: "#proxy-list"
```

推荐结构体：

```go
type ExtractorConfig struct {
    Sources []SourceConfig `yaml:"sources"`
}

type SourceConfig struct {
    Type    string            `yaml:"type"`
    Enabled bool              `yaml:"enabled"`
    URLs    []string          `yaml:"urls"`
    Options map[string]string `yaml:"options"`
}
```

为了兼容现有配置，第一阶段可以同时支持旧字段：

```go
type ExtractorConfig struct {
    Sources    []SourceConfig `yaml:"sources"`
    V2rayseURLs []string      `yaml:"v2rayseURLs"`
    GitHubURLs  []string      `yaml:"githubURLs"`
}
```

加载配置时，如果 `sources` 为空，就把旧字段转换成两个默认 source。

### 4.6 新增来源示例

新增一个普通文本或 base64 订阅来源时，只需要写类似文件：

```go
package sources

import (
    "context"
    "proxy-convert/internal/extractor"
)

type FooBarSource struct{}

func NewFooBarSource() *FooBarSource {
    return &FooBarSource{}
}

func (s *FooBarSource) Type() string {
    return "foobar"
}

func (s *FooBarSource) Extract(ctx context.Context, req extractor.SourceRequest) ([]string, error) {
    parser := extractor.NewContentParser()
    var links []string

    for _, url := range req.URLs {
        content, err := req.Fetcher.Fetch(ctx, url)
        if err != nil {
            return nil, err
        }
        links = append(links, parser.ParseContent(content)...)
    }

    return extractor.Dedupe(links), nil
}

func init() {
    extractor.RegisterSource(NewFooBarSource())
}
```

如果某个网站需要特殊 HTML/JSON 解析，只在这个文件中实现，最终仍然返回 `[]string`。

### 4.7 Runner 设计

`runner` 负责执行所有 source：

```go
type Runner struct {
    fetcher Fetcher
}

func (r *Runner) Run(ctx context.Context, configs []SourceConfig) ([]string, error) {
    var allLinks []string

    for _, cfg := range configs {
        if !cfg.Enabled {
            continue
        }

        source, ok := GetSource(cfg.Type)
        if !ok {
            // 记录 unknown source，继续处理其他 source
            continue
        }

        links, err := source.Extract(ctx, SourceRequest{
            URLs:    cfg.URLs,
            Fetcher: r.fetcher,
            Options: cfg.Options,
        })
        if err != nil {
            // 记录该 source 失败，继续处理其他 source
            continue
        }

        allLinks = append(allLinks, links...)
    }

    return Dedupe(allLinks), nil
}
```

这里建议单个来源失败不影响其他来源，最终在日志或 `ImportResult` 中记录失败来源。

### 4.8 Service 调用方式

重构后不再需要：

```go
ExtractFromV2rayse()
ExtractFromGitHub()
```

推荐改成统一方法：

```go
func (s *ExtractorService) ExtractFromSources(ctx context.Context, sources []extractor.SourceConfig) (ImportResult, error)
```

为了兼容旧调用，可以临时保留：

```go
func (s *ExtractorService) ExtractFromV2rayse() error
func (s *ExtractorService) ExtractFromGitHub() error
```

但内部都转到统一的 `ExtractFromSources`。

### 4.9 新增来源时的开发约束

每个 source 文件应遵守：

- 不直接写数据库。
- 不直接解析代理协议。
- 不自己创建长期 HTTP client。
- 不负责全局去重。
- 不读取全局配置。
- 只返回提取到的原始代理 URL。
- 可以使用公共 helper：`ContentParser`、`Dedupe`、base64 helper、Fetcher。

这样才能保证“新增一个来源文件”不会把通用流程重新写一遍。

## 5. 建议后的核心分层

### 5.1 HTTP 层：handlers

职责：

- 注册路由
- 解析 path/query/body 参数
- 调用 service
- 转换 HTTP 响应

不应该负责：

- 直接访问数据库
- 解析代理协议
- 拼装复杂业务数据
- 创建 extractor/verifier

建议结构：

```text
internal/handlers/
  handlers.go        # RegisterRoutes 入口
  link_handler.go    # links API
  import_handler.go  # import API
  clash_handler.go   # clash/v2ray export API
  response.go        # Response 结构和 helper
  params.go          # parseIDParam / parseStatuses
```

### 5.2 业务层：service

职责：

- 编排业务流程
- 调用 parser 生成 fingerprint/name
- 调用 repository 写入或查询数据
- 调用 extractor 获取链接
- 调用 verifier 进行验证
- 调用 exporter 生成配置

不应该负责：

- HTTP 参数解析
- 远程页面的具体 HTML 提取细节
- 数据库 SQL 细节

建议先保留现有 service 文件，再逐步拆分内部依赖。

### 5.3 提取层：extractor

建议拆分为：

```text
internal/extractor/
  extractor.go          # SourceExtractor 接口和公共类型
  registry.go           # 来源注册表
  runner.go             # 执行所有来源的通用流程
  client.go             # HTTP 内容获取
  content_parser.go     # 普通订阅文本解析
  base64.go             # base64 判断和解码 helper
  dedupe.go             # 去重 helper
  sources/
    github.go           # GitHub/raw 订阅源
    v2rayse.go          # V2rayse 订阅源
```

核心接口：

```go
type Extractor interface {
    Extract(ctx context.Context, urls []string) ([]string, error)
}

type Fetcher interface {
    Fetch(ctx context.Context, url string) (string, error)
}
```

`GitHub`、`V2rayse` 和后续新增来源都应该只关心“如何从内容中提取链接”，HTTP 请求、导入、去重、入库由公共流程完成。

### 5.4 解析层：parser

`parser` 当前边界相对清晰，建议保持为纯函数包：

- 输入代理链接
- 输出 `Proxy`
- 输出 fingerprint

短期只建议补测试和小修，不建议大改。

### 5.5 数据层：database

当前 `database.DB` 直接暴露 SQL 方法。短期可以保留，后续如果 service 测试成本变高，再引入 repository interface。

建议优先改进：

- 将重复 SQL 拼接 helper 化
- 区分唯一约束错误和其他数据库错误
- 给迁移逻辑补测试或至少补注释

### 5.6 配置层：config

当前配置热更新对业务层是全局可见的。建议短期保留 `config.Get()`，但把它收敛在 scheduler/app 边界，减少 service 内部直接读取全局配置。

中期可以考虑：

```go
type Provider interface {
    Get() *Config
}
```

这样 service 既能支持热更新，也更容易测试。

## 6. 推荐迁移阶段

### 阶段 0：建立安全基线

目标：确认重构前系统可编译、可测试。

任务：

- 使用项目内 Go cache 跑测试
- 记录当前 API 行为
- 确认当前工作区未提交改动的归属
- 给关键 bug 风险点补测试

验收：

```powershell
$env:GOCACHE=(Resolve-Path .gocache).Path
go test ./...
```

### 阶段 1：修正 handlers 并拆出参数解析

目标：先处理低风险、高收益的 HTTP 层问题。

任务：

- 新增 `handlers/params.go`
- 实现 `parseIDParam(c *gin.Context) (int, bool)`
- 实现 `parseStatuses(c *gin.Context, defaultStatuses []int) ([]int, bool)`
- 修复 `/api/links/:id`、`PUT /api/links/:id`、`DELETE /api/links/:id`
- 用 `strings.Builder` 替代循环字符串拼接

验收：

- 现有测试通过
- 新增 handler 参数解析测试通过
- API 路径行为不变，除修复 ID 解析外

### 阶段 2：引入 extractor source registry

目标：把定时提取从“固定 V2rayse/GitHub 两套流程”改为“遍历已注册 sources 的统一流程”。

任务：

- 新增 `Source`、`SourceRequest`、`SourceConfig`
- 新增 `RegisterSource`、`GetSource`
- 新增 `Runner`
- 新增 `Fetcher` 和 `HTTPFetcher`
- 将 GitHub 提取逻辑移动到 `extractor/sources/github.go`
- 将 V2rayse 提取逻辑移动到 `extractor/sources/v2rayse.go`
- 将 `GitHubExtractor.fetchContent` 改为依赖 `Fetcher`
- 将 `V2rayseExtractor.fetchHTML` 改为依赖 `Fetcher`
- 移除 `V2rayseExtractor.base64Strings` 字段，改为局部变量返回
- 抽出公共 `Dedupe`
- 抽出公共 base64 helper
- 支持 `extractor.sources` 新配置
- 兼容旧的 `v2rayseURLs` 和 `githubURLs`

验收：

- extractor 单元测试不需要真实网络
- 空 URL 时仍使用当前默认源
- 提取结果顺序保持稳定
- 新增一个 fake source 文件即可接入 runner
- 单个 source 失败不影响其他 source 继续执行

### 阶段 3：整理 ExtractorService

目标：让 service 只依赖统一 source runner，不再感知具体来源类型。

任务：

- `ExtractorService` 通过构造函数注入 `extractor.Runner`
- 新增 `ExtractFromSources(ctx, sources)` 统一入口
- 将 `ExtractFromV2rayse` 和 `ExtractFromGitHub` 暂时保留为兼容包装
- 将 `config.Get()` 调用尽量移到 scheduler 或 app 装配处
- 为 `importLinks` 引入结构化返回值

建议结果：

```go
type ImportResult struct {
    Imported int
    Existing int
    Failed   int
}
```

验收：

- service 测试可使用 fake runner/fake repository
- 导入逻辑行为保持一致

### 阶段 4：调整 scheduler 定时提取入口

目标：定时任务只调用一次统一提取流程。

任务：

- scheduler 每次执行时读取最新 `config.Get().Extractor.Sources`
- 调用 `ExtractorService.ExtractFromSources`
- 移除定时任务中分别调用 V2rayse/GitHub 的流程
- 保留日志中每个 source 的成功/失败统计

验收：

- 当前两个来源仍按定时任务执行
- 配置增加第三个 source 后无需改 scheduler

### 阶段 5：拆分 handlers 文件

目标：降低单文件复杂度。

任务：

- 按 API 领域拆分文件
- 保留 `RegisterRoutes` 作为统一入口
- 统一 JSON 响应 helper

验收：

- 路由表保持兼容
- 编译和测试通过

### 阶段 6：可选的 app 装配层

目标：让 `main.go` 更薄。

建议引入：

```text
internal/app/
  app.go
```

职责：

- 加载 config
- 初始化 database
- 初始化 services
- 注册 routes
- 启动 scheduler
- 管理 shutdown

验收：

- `main.go` 只负责调用 app 启动
- 优雅关闭行为不变

## 7. 设计后的请求流程

### 7.1 从 URL 导入

```text
HTTP POST /api/links/import
  -> handlers.ImportFromURL
  -> service.ExtractorService.ImportFromURL
  -> extractor.ContentParser.ParseContent
  -> parser.ParseLink / parser.GetNodeFingerprint
  -> database.AddLink
  -> HTTP JSON Response
```

### 7.2 定时提取

```text
scheduler
  -> config.Get().Extractor.Sources
  -> service.ExtractorService.ExtractFromSources
  -> extractor.Runner.Run
  -> registry.GetSource(source.type)
  -> source.Extract
  -> fetcher.Fetch
  -> service.importLinks
  -> database.AddLink
```

### 7.3 新增来源

```text
新增 internal/extractor/sources/foo.go
  -> 实现 Source
  -> init 注册 Source
  -> config.yaml 增加 type: foo
  -> scheduler 自动执行
  -> service 自动导入数据库
```

### 7.4 导出 Clash

```text
HTTP GET /api/clash
  -> handlers.ClashHandler
  -> service.ClashService.ExportClashConfigYAML
  -> database.GetAllLinks
  -> parser.ParseLink
  -> YAML response
```

## 8. 测试策略

### 8.1 保留现有测试

当前已有：

- `internal/parser/parser_test.go`
- `internal/service/verifier_service_test.go`

这些测试应在每个阶段保持通过。

### 8.2 新增测试建议

优先新增：

- `internal/extractor/content_parser_test.go`
- `internal/extractor/v2rayse_test.go`
- `internal/extractor/github_test.go`
- `internal/extractor/registry_test.go`
- `internal/extractor/runner_test.go`
- `internal/handlers/params_test.go`
- `internal/service/extractor_service_test.go`

重点覆盖：

- base64 订阅文本解析
- 带注释/空行的订阅文本解析
- V2rayse `__NUXT_DATA__` 解析
- V2rayse fallback base64 提取
- source 注册和查找
- runner 跳过 disabled source
- runner 遇到未知 source 时不中断
- runner 遇到单个 source 失败时不中断其他 source
- URL path id 解析
- status 查询参数解析
- 重复链接导入统计

## 9. 风险与规避

### 9.1 当前工作区存在大量未提交改动

风险：重构时可能和已有改动混杂，导致难以回滚。

规避：

- 重构前先确认这些改动是否都要保留
- 每个阶段单独提交
- 每个阶段只改有限范围

### 9.2 提取逻辑依赖外部网站结构

风险：V2rayse 页面结构变化会导致提取失败。

规避：

- 用样例 HTML 写单元测试
- 将 `__NUXT_DATA__` 提取和 fallback 提取都独立测试
- 网络层和解析层分开，便于定位问题

### 9.3 配置热更新与 service 注入可能冲突

风险：如果 service 只保存启动时配置，热更新可能失效。

规避：

- 短期保留 `config.Get()` 在少数边界处
- 中期引入 `ConfigProvider`
- scheduler 每次执行任务前读取最新配置

### 9.4 数据库唯一约束语义不清

风险：当前导入逻辑把所有 `AddLink` 错误都当作已存在，可能掩盖真实数据库错误。

规避：

- 后续区分 duplicate error 和 unexpected error
- `ImportResult` 中增加 failed 统计

### 9.5 source 自动注册可能被遗漏

风险：如果新增 source 文件但没有被编译进项目，registry 中找不到该来源。

规避：

- 所有 source 文件统一放在 `internal/extractor/sources`
- app 装配层显式 blank import sources 包，或者在 extractor 包内集中引用
- 增加 `registry_test` 验证内置 source 已注册
- 启动时打印已注册 source 列表

## 10. 不建议第一轮做的事

第一轮不建议：

- 大规模移动所有目录
- 改数据库 schema
- 重写 parser
- 替换 Gin 框架
- 替换 SQLite 驱动
- 引入复杂依赖注入框架
- 一次性重写前端模板

这些改动收益不如 extractor/handlers/service 边界整理直接，风险也更高。

## 11. 建议最终验收标准

重构完成后，应满足：

- `go test ./...` 通过
- `go vet ./...` 通过
- 现有 API 路径兼容
- 导入 URL 和导入文本功能可用
- 定时任务仍能执行提取和验证
- extractor 单元测试不依赖真实网络
- 新增来源只需要新增一个 source 文件和配置项
- scheduler 不依赖具体来源类型
- handler 参数解析有测试覆盖
- `main.go` 或 app 装配逻辑清晰

推荐本地验证命令：

```powershell
$env:GOCACHE=(Resolve-Path .gocache).Path
go test ./...
go vet ./...
```

## 12. 推荐第一步

如果以“来源扩展性”为优先目标，建议先从阶段 2 开始做一条垂直切片：

1. 定义 `Source`、`SourceConfig`、`Registry`、`Runner`。
2. 先把 GitHub 接入新流程。
3. 再把 V2rayse 接入新流程。
4. 让 scheduler 调用统一 `ExtractFromSources`。
5. 保留旧配置兼容，确认现有行为不变。

如果以“先修 bug、降低风险”为优先目标，仍然可以先从阶段 1 开始：

1. 修复 handlers 的 `:id` 参数解析。
2. 抽出 status 查询解析。
3. 给参数解析补测试。
4. 确认 API 行为不变。

这一步风险最低，而且能先消除真实 bug。完成后再进入 extractor 拆分，会更稳。
