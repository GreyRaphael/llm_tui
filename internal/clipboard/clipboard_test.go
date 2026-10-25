package clipboard

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/aymanbagabas/go-osc52/v2"
)

func TestWriteAllUsesLinuxClipboardTool(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("linux clipboard dispatch test")
	}
	dir := t.TempDir()
	outputFile := filepath.Join(dir, "clipboard.txt")
	script := filepath.Join(dir, "xclip")
	if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > \"$CLIP_OUT\"\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake xclip: %v", err)
	}
	// On WSL, prevent the native PowerShell path from winning before xclip.
	powerShell := filepath.Join(dir, "powershell.exe")
	if err := os.WriteFile(powerShell, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("failed to create fake powershell.exe: %v", err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("DISPLAY", ":99")
	t.Setenv("WAYLAND_DISPLAY", "")
	t.Setenv("TMUX", "")
	t.Setenv("CLIP_OUT", outputFile)

	if err := WriteAll("你好 clipboard"); err != nil {
		t.Fatalf("WriteAll failed: %v", err)
	}
	got, err := os.ReadFile(outputFile)
	if err != nil {
		t.Fatalf("fake clipboard did not receive input: %v", err)
	}
	if string(got) != "你好 clipboard" {
		t.Fatalf("unexpected clipboard content %q", got)
	}
}

func TestWriteOSC52ToUnicodeAndTerminalModes(t *testing.T) {
	text := "你好，世界！LLM TUI 剪贴板测试 😀🚀🤖"
	tests := []struct {
		name string
		tmux string
		term string
		want string
	}{
		{name: "default", term: "xterm-256color", want: osc52.New(text).String()},
		{name: "tmux", tmux: "/tmp/tmux-1000/default", term: "xterm-256color", want: osc52.New(text).Tmux().String()},
		{name: "screen", term: "screen-256color", want: osc52.New(text).Screen().String()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Setenv("TMUX", tt.tmux)
			t.Setenv("TERM", tt.term)
			var out bytes.Buffer
			if err := writeOSC52To(&out, text); err != nil {
				t.Fatalf("writeOSC52To failed: %v", err)
			}
			if out.String() != tt.want {
				t.Fatalf("unexpected OSC52 sequence:\n got %q\nwant %q", out.String(), tt.want)
			}
		})
	}
}
