package clipboard

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
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

func TestNativeToolsDispatchPerPlatform(t *testing.T) {
	env := func(values map[string]string) func(string) string {
		return func(key string) string { return values[key] }
	}

	tests := []struct {
		name string
		goos string
		env  map[string]string
		wsl  bool
		want []string
	}{
		{
			name: "darwin uses pbcopy",
			goos: "darwin",
			want: []string{"pbcopy"},
		},
		{
			name: "windows prefers powershell then clip",
			goos: "windows",
			want: []string{"powershell.exe", "cmd.exe"},
		},
		{
			name: "headless linux has no native tool",
			goos: "linux",
			want: nil,
		},
		{
			name: "x11 without wayland",
			goos: "linux",
			env:  map[string]string{"DISPLAY": ":0"},
			want: []string{"xclip", "xsel"},
		},
		{
			name: "wayland before x11",
			goos: "linux",
			env:  map[string]string{"DISPLAY": ":0", "WAYLAND_DISPLAY": "wayland-0"},
			want: []string{"wl-copy", "xclip", "xsel"},
		},
		{
			name: "wsl powershell first",
			goos: "linux",
			env:  map[string]string{"DISPLAY": ":0"},
			wsl:  true,
			want: []string{"powershell.exe", "xclip", "xsel"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tools := nativeTools(tt.goos, env(tt.env), tt.wsl)
			var got []string
			for _, tool := range tools {
				got = append(got, tool.name)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("unexpected tools %v, want %v", got, tt.want)
			}
			for i := range tt.want {
				if got[i] != tt.want[i] {
					t.Fatalf("tool order mismatch at %d: got %v, want %v", i, got, tt.want)
				}
			}
		})
	}
}

func TestNativeToolsVerifyUTF8AndClipboardFlags(t *testing.T) {
	tools := nativeTools("linux", func(string) string { return "" }, true)
	if len(tools) != 1 || tools[0].name != "powershell.exe" {
		t.Fatalf("expected only the WSL powershell writer, got %v", tools)
	}
	args := strings.Join(tools[0].args, " ")
	if !strings.Contains(args, "InputEncoding") || !strings.Contains(args, "Set-Clipboard") {
		t.Fatalf("powershell writer must set UTF-8 input encoding and Set-Clipboard, got %q", args)
	}

	x11 := nativeTools("linux", func(key string) string {
		if key == "DISPLAY" {
			return ":0"
		}
		return ""
	}, false)
	xclipArgs := strings.Join(x11[0].args, " ")
	if !strings.Contains(xclipArgs, "UTF8_STRING") {
		t.Fatalf("xclip must request UTF8_STRING, got %q", xclipArgs)
	}
	xselArgs := strings.Join(x11[1].args, " ")
	if !strings.Contains(xselArgs, "--clipboard") {
		t.Fatalf("xsel must write the clipboard selection, got %q", xselArgs)
	}
	if !slicesContain(x11[1].env, "LANG=en_US.UTF-8") {
		t.Fatalf("xsel must force a UTF-8 locale, got %v", x11[1].env)
	}
}

// slicesContain reports whether values contains want.
func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestRunToolSkipsMissingExecutable(t *testing.T) {
	if err := runTool(clipboardTool{name: "llm-tui-does-not-exist"}, "text"); err == nil {
		t.Fatal("expected a lookup error for a missing executable")
	}
}

// fakeTerminal installs OSC 52 sinks that capture escape sequences instead of
// writing to the real stdout/stderr/tty.
func fakeTerminal(t *testing.T) *bytes.Buffer {
	t.Helper()
	var captured bytes.Buffer
	origTTY, origErr, origOut := openControllingTTY, stderrWriter, stdoutWriter
	t.Cleanup(func() {
		openControllingTTY, stderrWriter, stdoutWriter = origTTY, origErr, origOut
	})

	t.Setenv("TMUX", "")
	t.Setenv("TERM", "xterm-256color")
	openControllingTTY = func() (io.WriteCloser, error) {
		return errWriteCloser{&captured}, nil
	}
	stderrWriter = &captured
	stdoutWriter = &captured
	return &captured
}

type errWriteCloser struct{ io.Writer }

func (errWriteCloser) Close() error { return nil }

func TestWriteNativeFallsBackToOSC52(t *testing.T) {
	for _, goos := range []string{"darwin", "windows"} {
		t.Run(goos, func(t *testing.T) {
			// No clipboard executable exists on this host for the other platform,
			// so the native writers must fail and OSC 52 must take over.
			t.Setenv("PATH", t.TempDir())
			captured := fakeTerminal(t)

			if err := writeNative("fallback text", goos); err != nil {
				t.Fatalf("writeNative(%s) should fall back to OSC 52, got %v", goos, err)
			}
			if captured.Len() == 0 {
				t.Fatal("expected an OSC 52 sequence, got nothing")
			}
		})
	}
}

func TestWriteAllDispatchesDarwinAndWindows(t *testing.T) {
	tests := []struct {
		name string
		goos string
		tool string
	}{
		{name: "darwin uses pbcopy", goos: "darwin", tool: "pbcopy"},
		{name: "windows uses powershell", goos: "windows", tool: "powershell.exe"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			outputFile := filepath.Join(dir, "clipboard.txt")
			script := filepath.Join(dir, tt.tool)
			if err := os.WriteFile(script, []byte("#!/bin/sh\ncat > \"$CLIP_OUT\"\n"), 0o755); err != nil {
				t.Fatalf("failed to create fake %s: %v", tt.tool, err)
			}
			t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
			t.Setenv("CLIP_OUT", outputFile)

			orig := detectGOOS
			t.Cleanup(func() { detectGOOS = orig })
			detectGOOS = func() string { return tt.goos }

			if err := WriteAll("dispatch text"); err != nil {
				t.Fatalf("WriteAll(%s) failed: %v", tt.goos, err)
			}
			got, err := os.ReadFile(outputFile)
			if err != nil {
				t.Fatalf("fake %s did not receive input: %v", tt.tool, err)
			}
			if string(got) != "dispatch text" {
				t.Fatalf("unexpected clipboard content %q", got)
			}
		})
	}
}

func TestWriteOSC52PrefersControllingTTY(t *testing.T) {
	captured := fakeTerminal(t)

	if err := writeOSC52("tty text"); err != nil {
		t.Fatalf("writeOSC52 failed: %v", err)
	}
	if got := captured.String(); got != osc52.New("tty text").String() {
		t.Fatalf("unexpected OSC 52 sequence %q", got)
	}
}

func TestWriteOSC52FallsBackToStderrWhenTTYFails(t *testing.T) {
	captured := fakeTerminal(t)
	openControllingTTY = func() (io.WriteCloser, error) {
		return nil, errors.New("no controlling terminal")
	}

	if err := writeOSC52("stderr text"); err != nil {
		t.Fatalf("writeOSC52 failed: %v", err)
	}
	if got := captured.String(); got != osc52.New("stderr text").String() {
		t.Fatalf("unexpected OSC 52 sequence %q", got)
	}
}

func TestWriteOSC52ReportsAllSinksFailing(t *testing.T) {
	fakeTerminal(t)
	openControllingTTY = func() (io.WriteCloser, error) {
		return nil, errors.New("no controlling terminal")
	}
	stderrWriter = failingWriter{}
	stdoutWriter = failingWriter{}

	if err := writeOSC52("text"); err == nil {
		t.Fatal("expected an error when every OSC 52 sink fails")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("write failed") }

func TestWriteLinuxReportsNoUtilityFound(t *testing.T) {
	fakeTerminal(t)
	t.Setenv("PATH", t.TempDir())
	t.Setenv("DISPLAY", "")
	t.Setenv("WAYLAND_DISPLAY", "")
	openControllingTTY = func() (io.WriteCloser, error) {
		return nil, errors.New("no controlling terminal")
	}
	stderrWriter = failingWriter{}
	stdoutWriter = failingWriter{}

	err := writeLinux("text")
	if err == nil || !strings.Contains(err.Error(), "no working clipboard utility found") {
		t.Fatalf("expected a missing-utility error, got %v", err)
	}
}
