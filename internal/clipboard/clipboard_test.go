package clipboard

import (
	"bytes"
	"testing"

	"github.com/aymanbagabas/go-osc52/v2"
)

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
