package clipboard

import (
	"bytes"
	"errors"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/aymanbagabas/go-osc52/v2"
)

// Seams for tests: the platform and the terminal writers used by the OSC 52
// fallback. Production code always uses the real values.
var (
	// detectGOOS reports the target platform for clipboard dispatch.
	detectGOOS = func() string { return runtime.GOOS }

	// openControllingTTY opens a handle for writing escape sequences directly to
	// the controlling terminal.
	openControllingTTY = func() (io.WriteCloser, error) {
		return os.OpenFile("/dev/tty", os.O_WRONLY, 0)
	}

	// stderrWriter and stdoutWriter are the last-resort OSC 52 sinks.
	stderrWriter io.Writer = os.Stderr
	stdoutWriter io.Writer = os.Stdout
)

// clipboardTool is one candidate native clipboard writer for a platform.
type clipboardTool struct {
	// name is the executable looked up on PATH.
	name string
	args []string
	// env lists additional "KEY=value" entries appended to the process env.
	env []string
}

// nativeTools returns the native clipboard writers to try, in order, for the
// given platform and environment. Terminal escape sequences (OSC 52) are handled
// separately because they need no external executable.
func nativeTools(goos string, getenv func(string) string, wsl bool) []clipboardTool {
	switch goos {
	case "darwin":
		return []clipboardTool{{
			name: "pbcopy",
			env:  []string{"LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8"},
		}}
	case "windows":
		return []clipboardTool{
			{name: "powershell.exe", args: windowsPowerShellArgs()},
			{name: "cmd.exe", args: []string{"/c", "chcp 65001 >NUL && clip.exe"}},
		}
	}

	var tools []clipboardTool
	if wsl {
		tools = append(tools, clipboardTool{name: "powershell.exe", args: windowsPowerShellArgs()})
	}
	if getenv("WAYLAND_DISPLAY") != "" {
		tools = append(tools, clipboardTool{name: "wl-copy", args: []string{"--type", "text/plain;charset=utf-8"}})
	}
	if getenv("DISPLAY") != "" {
		// -t UTF8_STRING avoids Latin-1 garbled text.
		tools = append(tools,
			clipboardTool{name: "xclip", args: []string{"-in", "-selection", "clipboard", "-t", "UTF8_STRING"}},
			clipboardTool{
				name: "xsel",
				args: []string{"--input", "--clipboard"},
				env:  []string{"LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8"},
			},
		)
	}
	return tools
}

func windowsPowerShellArgs() []string {
	return []string{
		"-NoProfile",
		"-NonInteractive",
		"-Command",
		"[Console]::InputEncoding = [System.Text.Encoding]::UTF8; Set-Clipboard -Value ([System.Console]::In.ReadToEnd())",
	}
}

// WriteAll writes UTF-8 text (including Chinese characters and Emojis) to the system clipboard
func WriteAll(text string) error {
	switch detectGOOS() {
	case "darwin":
		return writeNative(text, "darwin")
	case "windows":
		return writeNative(text, "windows")
	default:
		return writeLinux(text)
	}
}

// writeNative runs the first working native writer for the platform. When none
// is available it falls back to OSC 52 (macOS and Windows terminals both accept
// it, and it keeps the command usable from a remote shell).
func writeNative(text, goos string) error {
	for _, tool := range nativeTools(goos, os.Getenv, false) {
		if runTool(tool, text) == nil {
			return nil
		}
	}
	return writeOSC52(text)
}

// Linux / Unix implementation (WSL, Wayland, X11, OSC 52)
func writeLinux(text string) error {
	for _, tool := range nativeTools("linux", os.Getenv, isWSL()) {
		if runTool(tool, text) == nil {
			return nil
		}
	}

	// OSC 52 fallback for SSH, remote terminals, tmux, and headless environments.
	if err := writeOSC52(text); err == nil {
		return nil
	}

	return errors.New("no working clipboard utility found (please install xclip, wl-clipboard, or xsel, or use an OSC 52 compatible terminal)")
}

// runTool pipes text into one clipboard executable. A missing executable is
// reported immediately so the caller can try the next candidate.
func runTool(tool clipboardTool, text string) error {
	if _, err := exec.LookPath(tool.name); err != nil {
		return err
	}
	cmd := exec.Command(tool.name, tool.args...)
	if len(tool.env) > 0 {
		cmd.Env = append(os.Environ(), tool.env...)
	}
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func writeOSC52To(out io.Writer, text string) error {
	seq := osc52.New(text)
	if os.Getenv("TMUX") != "" {
		seq = seq.Tmux()
	} else if strings.HasPrefix(os.Getenv("TERM"), "screen") {
		seq = seq.Screen()
	}
	_, err := seq.WriteTo(out)
	return err
}

// writeOSC52 writes the OSC 52 ANSI escape sequence to the terminal.
// This allows copying text directly to the client's local clipboard even over SSH / tmux / headless environments.
func writeOSC52(text string) error {
	// Try writing directly to controlling terminal (/dev/tty).
	if tty, err := openControllingTTY(); err == nil {
		defer tty.Close()
		if err := writeOSC52To(tty, text); err == nil {
			return nil
		}
	}
	if err := writeOSC52To(stderrWriter, text); err == nil {
		return nil
	}
	if err := writeOSC52To(stdoutWriter, text); err == nil {
		return nil
	}
	return errors.New("failed to write OSC 52 sequence to terminal")
}

func isWSL() bool {
	if _, err := os.Stat("/proc/sys/fs/binfmt_misc/WSLInterop"); err == nil {
		return true
	}
	if b, err := os.ReadFile("/proc/version"); err == nil {
		if bytes.Contains(bytes.ToLower(b), []byte("microsoft")) || bytes.Contains(bytes.ToLower(b), []byte("wsl")) {
			return true
		}
	}
	return false
}
