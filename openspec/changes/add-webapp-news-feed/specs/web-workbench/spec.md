## ADDED Requirements

### Requirement: WebApp 首页必须展示 SoSoValue 推荐新闻
WebApp 首页 MUST 在顶部标注区域展示 SoSoValue 推荐新闻，数据 MUST 来自后端本地缓存。

#### Scenario: 成功加载推荐新闻
- **WHEN** 用户打开 WebApp 首页且数据库中存在 24 小时内的推荐新闻
- **THEN** 页面顶部展示推荐新闻区域
- **AND** 每条新闻至少展示标题、来源、发布时间和外链

#### Scenario: 推荐新闻暂不可用
- **WHEN** 数据库中不存在可展示新闻且后端无法同步 SoSoValue 数据
- **THEN** 页面顶部显示明确的不可用状态
- **AND** 页面不得展示假新闻或静默使用陈旧默认内容

### Requirement: WebApp 底部必须展示新闻 ticker
WebApp MUST 在页面最底部展示横向滚动新闻 ticker，用于持续滚动显示最新新闻标题。

#### Scenario: 成功加载 ticker
- **WHEN** 数据库中存在 24 小时内的 ticker 新闻
- **THEN** 页面底部展示固定的横向滚动条
- **AND** 新闻标题按发布时间倒序循环滚动

#### Scenario: ticker 暂不可用
- **WHEN** 数据库中不存在可展示 ticker 内容
- **THEN** 页面底部展示明确的不可用状态或隐藏 ticker
- **AND** 不得展示假新闻

### Requirement: 后端必须缓存 SoSoValue 新闻数据
后端 MUST 通过服务端同步 SoSoValue 新闻 API，并将归一化后的新闻写入数据库。

#### Scenario: 同步成功
- **WHEN** SoSoValue API 返回推荐新闻
- **THEN** 后端将新闻按外部 ID 与 feed 类型去重写入数据库
- **AND** WebApp 新闻接口从数据库返回数据

#### Scenario: 清理过期新闻
- **WHEN** 新闻发布时间或入库时间超过 24 小时
- **THEN** 后端自动删除该新闻记录
- **AND** WebApp 接口不返回该记录

### Requirement: SoSoValue 请求必须受月度额度保护
后端 MUST 记录 SoSoValue API 的月度请求量，并在达到安全阈值后停止继续请求第三方 API。

#### Scenario: 正常额度内请求
- **WHEN** 当前月份请求量低于安全阈值
- **THEN** 后端可以按配置间隔请求 SoSoValue API
- **AND** 每次请求成功或失败都计入月度用量

#### Scenario: 达到安全阈值
- **WHEN** 当前月份请求量达到安全阈值
- **THEN** 后端停止请求 SoSoValue API
- **AND** WebApp 继续读取数据库中未过期新闻

### Requirement: 新闻展示必须保持首页布局稳定
新闻展示 MUST 在不同视口宽度下不遮挡登录区、布局控制区或已有工作台模块。

#### Scenario: 桌面宽屏展示
- **WHEN** 用户在桌面宽屏打开首页
- **THEN** 推荐新闻位于 Logo 与登录区之间的顶部空白区域
- **AND** 底部 ticker 固定在页面底部且不遮挡主要模块内容

#### Scenario: 移动或窄屏展示
- **WHEN** 用户在窄屏打开首页
- **THEN** 推荐新闻自动换到顶部内容流中的独立行
- **AND** 底部 ticker 文本不溢出容器
