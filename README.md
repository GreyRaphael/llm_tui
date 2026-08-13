# ⚡ LLM TUI: Provider Manager & Chat Laboratory

[English](README.md) | [简体中文](README-CN.md)

`llm_tui` is a fast, modern, terminal-based user interface (TUI) application written in **Go** for auto-probing, managing, and interactively testing **LLM Provider API endpoints** (supporting **OpenAI Chat API**, **OpenAI Responses API**, and **Anthropic Messages API**).

Built using the **Charm.sh** ecosystem (`bubbletea`, `lipgloss`, `viewport`, `textarea`) and a **pure-Go SQLite database** (`modernc.org/sqlite`, CGO-free), `llm_tui` allows developers and AI engineers to quickly verify provider connectivity, benchmark latency/tokens, and debug custom JSON request payloads side-by-side.

---

## ✨ Features

- **🔍 Automated Capability Probing & Setup Wizard**:
  - Connects to any OpenAI or Anthropic compatible Base URL.
  - API keys are optional: leave empty for unauthenticated providers / local servers.
  - Automatically queries the `/models` endpoint (with root-URL fallback) to discover available models; if discovery fails, prompts you to enter the exact model ID manually.
  - Probes endpoints concurrently with realistic timeouts (30s) to detect active capabilities (`openai_chat`, `openai_responses`, `anthropic_messages`).
- **💾 Pure Go SQLite Storage (CGO-Free)**:
  - Local database (`providers.db`) located alongside the binary.
  - Stores provider configs, custom payload templates, model choices, and reasoning preferences.
  - Auto-fills saved API Keys when matching Base URLs.
- **🧪 Side-by-Side Split-Pane Chat Laboratory**:
  - **Left Pane (Request Payload Editor)**: Live multi-line JSON payload editor with active focus. Press `Ctrl+S` to send requests instantly without leaving the editor.
  - **Right Pane (Response Viewport)**: Displays HTTP status code, latency, token usage (Prompt, Completion, Total), tokens-per-second (TPS) throughput, and formatted JSON response with smooth `PgUp` / `PgDn` page scrolling.
- **🤖 Embedded Model Switcher**:
  - Press `Alt+M` in the Chat Laboratory to open an embedded model picker viewport in the left panel.
  - Displays up to 20 models with centered scrolling pointers (`👉`). Selection updates model choice and saves to SQLite automatically.
- **🧠 Flexible Reasoning Effort Shortcuts**:
  - Press `Alt+1` to `Alt+4` to toggle Reasoning Effort (`none`, `low`, `high`, `max`).
  - Automatically formats `reasoning_effort` for OpenAI Chat, `reasoning` for OpenAI Responses, and `output_config.effort` for Anthropic Messages.
- **⚡ Real-Time SSE Streaming & Reasoning Process Visualization**:
  - Native support for typewriter-style real-time streaming via HTTP SSE (`text/event-stream`).
  - Auto-detects `"stream": true/false` payload mode (toggle it manually anytime with `Alt+S`): Pretty JSON for non-stream, live typewriter display for stream mode with dedicated **💭 Thinking Process** and **💬 Response Content** cards.
- **🎨 Terminal Glamour Markdown Rendering & Code Syntax Highlighting**:
  - Integrated Charm `glamour` Markdown renderer engine.
  - Automatically highlights code blocks (Python, Go, SQL, JSON, etc.), header tags, bold text, and lists.
- **📐 Responsive Word Wrap & Auto Reflow**:
  - Dynamically calculates Viewport width and performs real-time text reflow during terminal window resizing, eliminating horizontal border overflow.
- **🔑 Cross-Platform Lossless Unicode & UTF-8 Clipboard**:
  - Solves Chinese and Emoji clipboard garbled text issues across Linux (X11 `UTF8_STRING` / Wayland `wl-copy`), WSL (PowerShell UTF-8), macOS, and Windows natively.
  - `Ctrl+Y`: One-click copy Request Payload; `Ctrl+U`: One-click copy Response JSON.

---

## 🚀 Installation & Quick Start

### Option 1: Download Pre-built Release Binaries

Download the archive for your OS/Architecture from the [GitHub Releases](../../releases) (each archive contains the `llm_tui` binary, or `llm_tui.exe` on Windows):

- **Linux (x86_64)**: `llm_tui-<version>-linux-amd64.tar.gz`
- **Linux (ARM64)**: `llm_tui-<version>-linux-arm64.tar.gz`
- **macOS (Apple Silicon)**: `llm_tui-<version>-darwin-arm64.tar.gz`
- **macOS (Intel)**: `llm_tui-<version>-darwin-amd64.tar.gz`
- **Windows (x86_64)**: `llm_tui-<version>-windows-amd64.zip`

Extract and run:
```bash
# Linux / macOS
tar -xzf llm_tui-<version>-linux-amd64.tar.gz   # or -darwin-arm64 / -darwin-amd64
./llm_tui

# Windows (PowerShell)
Expand-Archive llm_tui-<version>-windows-amd64.zip
.\llm_tui.exe
```

### Option 2: Build from Source

Requires Go 1.25+:

```bash
git clone https://github.com/GreyRaphael/llm_tui.git
cd llm_tui
go build -o llm_tui .
./llm_tui
```

---

## ⌨️ Keybindings Reference

### Provider Manager View
| Key | Action |
| --- | --- |
| `n` | Add New Provider (Launch Setup Wizard) |
| `Enter` / `t` | Open Selected Provider in Chat Laboratory |
| `d` | Delete Selected Provider Record (press twice to confirm) |
| `↑` / `k`, `↓` / `j` | Navigate Provider Cards |
| `q` | Quit Application |

### Setup Wizard View
| Key | Action |
| --- | --- |
| `Tab` / `Shift+Tab` | Switch Input Fields |
| `↑` / `k`, `↓` / `j` | Select Model or API Type |
| `Enter` | Proceed to Next Step / Confirm Selection |
| `Esc` | Cancel and Return to Provider Manager |

### Chat Laboratory View (Split-Pane)
| Key | Action |
| --- | --- |
| `Ctrl+S` | Send Request Payload & View Response |
| `Alt+S` | Toggle Streaming Mode (`stream: true/false`) in Payload JSON |
| `Tab` / `Shift+Tab` | Switch Focus between Left Pane (Request) & Right Pane (Response) |
| `Alt+M` | Toggle Embedded Model Switcher Menu |
| `Alt+1` - `Alt+4` | Switch Reasoning Effort (`1: none`, `2: low`, `3: high`, `4: max`) |
| `Ctrl+Y` | Copy Request JSON Payload to Clipboard |
| `Ctrl+U` | Copy Response JSON Payload to Clipboard |
| `PgUp` / `PgDn` | Scroll Response Viewport Page by Page |
| `Esc` | Return to Provider Manager |

---

## 🧱 Architecture & Tech Stack

- **UI Framework**: [github.com/charmbracelet/bubbletea](https://github.com/charmbracelet/bubbletea)
- **Styling**: [github.com/charmbracelet/lipgloss](https://github.com/charmbracelet/lipgloss)
- **Components**: [bubbles/textarea](https://github.com/charmbracelet/bubbles), [bubbles/viewport](https://github.com/charmbracelet/bubbles), [bubbles/spinner](https://github.com/charmbracelet/bubbles)
- **Database**: [modernc.org/sqlite](https://modernc.org/sqlite) (Pure Go SQLite driver, NO CGO required)
- **Clipboard Integration**: [github.com/atotto/clipboard](https://github.com/atotto/clipboard)

---

## 📄 License

MIT License. Developed for pairing with AI coding agents and LLM endpoint benchmarking.

