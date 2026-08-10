# ⚡ LLM TUI: 大模型 Provider 管理与测试终端实验室

[English](README.md) | [简体中文](README-CN.md)

`llm_tui` 是一款采用 **Go 语言** 编写的高性能、现代 Terminal (TUI) 命令行终端工具。用于自动探测、管理并交互式测试 **LLM Provider API 端点**（支持 **OpenAI Chat API**、**OpenAI Responses API** 以及 **Anthropic Messages API**）。

项目基于 **Charm.sh** 生态构建（`bubbletea`、`lipgloss`、`viewport`、`textarea`），搭配 **纯 Go 实现的 SQLite 数据库**（`modernc.org/sqlite`，无需 CGO 编译器依赖）。`llm_tui` 可帮助开发者与 AI 工程师快速验证 API 服务连通性、压测与对比延迟/Token 消耗，并实时调试自定义 JSON 请求 Payload。

---

## ✨ 核心特性

- **🔍 自动化能力探测与配置向导**:
  - 支持连接任意兼容 OpenAI 或 Anthropic 协议的 Base URL 端点。
  - 自动查询 `/models` 列表发现可用模型。
  - 并发探测 API 端点能力，自动识别激活的服务模式（`openai_chat`、`openai_responses`、`anthropic_messages`）。
- **💾 纯 Go SQLite 本地持久化 (CGO-Free)**:
  - 自动在可执行文件同级目录创建并维护 SQLite 数据库（`providers.db`）。
  - 存储 Provider 配置、自定义 Payload 模板、模型选择及 Reasoning Effort 偏好。
  - 匹配相同 Base URL 时自动填充已保存的 API Key。
- **🧪 左右分栏交互式测试实验室 (Chat Laboratory)**:
  - **左侧面板（Request Payload 编辑器）**: 具备实时的多行 JSON 编辑功能。按 `Ctrl+S` 可直接在编辑状态下发送请求，无需切换焦点。
  - **右侧面板（Response 响应视图）**: 实时展示 HTTP 状态码、延迟时间、Token 统计（Prompt, Completion, Total），以及格式化后的 JSON 响应，支持平滑的 `PgUp` / `PgDn` 按页滚动。
- **🤖 内置模型快速切换器**:
  - 在测试实验室中按下 `Alt+M` 即可在左侧面板唤起内置模型选择菜单。
  - 展示多达 20 个可用模型，具备指针指示（`👉`）。选择后自动更新配置并同步保存至 SQLite。
- **🧠 灵活的 Reasoning Effort 快捷切换**:
  - 按下 `Alt+1` 至 `Alt+4` 可快速切换推理强度（`none`, `low`, `high`, `max`）。
  - 自动适配 OpenAI Chat (`reasoning_effort`)、OpenAI Responses (`reasoning`) 与 Anthropic Messages (`output_config.effort`) 规范。
- **📋 一键剪贴板导出**:
  - `Ctrl+Y`: 一键复制原始 Request JSON Payload 到系统剪贴板。
  - `Ctrl+U`: 一键复制格式化后的 Response JSON 到系统剪贴板。
- **⚡ 实时 SSE 流式传输与思考过程 (Reasoning Process) 可视化**:
  - 原生支持基于 HTTP SSE (`text/event-stream`) 的打字机式实时流式渲染。
  - 自动提取并独立展示 DeepSeek-R1、QwQ、Claude 3.7 等推理/思考模型的 **💭 思考过程 (Thinking Process)** 与 **💬 响应内容 (Response Content)**。
  - 自动识别 GBK / GB2312 字符编码并无缝转换为 UTF-8，流传输结束后自动重构完整 JSON 以供 `Ctrl+U` 剪贴板复制。

---

## 🚀 安装与快速开始

### 方式一：下载预编译可执行文件

在 [GitHub Releases](../../releases) 页面中下载对应操作系统与架构的二进制文件：

- **Linux (x86_64)**: `llm_tui-linux-amd64`
- **macOS (Apple Silicon)**: `llm_tui-darwin-arm64`
- **macOS (Intel)**: `llm_tui-darwin-amd64`
- **Windows (x86_64)**: `llm_tui-windows-amd64.exe`

添加执行权限并运行：
```bash
chmod +x llm_tui-linux-amd64
./llm_tui-linux-amd64
```

### 方式二：从源码编译

开发环境需要 Go 1.22+：

```bash
git clone https://github.com/GreyRaphael/llm_tui.git
cd llm_tui
go build -o llm_tui .
./llm_tui
```

---

## ⌨️ 快捷键指南

### Provider 管理列表视图 (Manager View)
| 按键 | 功能说明 |
| --- | --- |
| `n` | 新建 Provider（启动配置向导） |
| `Enter` / `t` | 打开当前选中的 Provider 进测试实验室 |
| `d` | 删除选中的 Provider 记录（连续按两次确认删除） |
| `↑` / `k`, `↓` / `j` | 上下切换 Provider 卡片 |
| `q` | 退出程序 |

### 配置向导视图 (Setup Wizard View)
| 按键 | 功能说明 |
| --- | --- |
| `Tab` / `Shift+Tab` | 切换输入框焦点 |
| `↑` / `k`, `↓` / `j` | 选择模型或 API 类型 |
| `Enter` | 进入下一步 / 确认选择 |
| `Esc` | 取消并返回管理列表 |

### 测试实验室视图 (Chat Laboratory View - 左右分栏)
| 按键 | 功能说明 |
| --- | --- |
| `Ctrl+S` | 发送 Request Payload 并查看响应 |
| `Tab` / `Shift+Tab` | 在左侧面板（Request）与右侧面板（Response）之间切换焦点 |
| `Alt+M` | 唤起/关闭内置模型选择菜单 |
| `Alt+1` - `Alt+4` | 切换 Reasoning Effort（`1: none`, `2: low`, `3: high`, `4: max`） |
| `Ctrl+Y` | 复制 Request JSON Payload 到系统剪贴板 |
| `Ctrl+U` | 复制 Response JSON 到系统剪贴板 |
| `PgUp` / `PgDn` | 上下翻页滚动响应结果 |
| `Esc` | 返回 Provider 管理列表 |

---

## 🧱 架构与技术栈

- **UI 框架**: [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- **样式与布局**: [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)
- **组件库**: [bubbles/textarea](https://github.com/charmbracelet/bubbles), [bubbles/viewport](https://github.com/charmbracelet/bubbles), [bubbles/spinner](https://github.com/charmbracelet/bubbles)
- **数据库**: [modernc.org/sqlite](https://modernc.org/sqlite) (纯 Go 实现，无 CGO 依赖)
- **剪贴板集成**: [github.com/atotto/clipboard](https://github.com/atotto/clipboard)

---

## 📄 开源协议

MIT License.
