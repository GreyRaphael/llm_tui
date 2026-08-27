package clipboard

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"github.com/aymanbagabas/go-osc52/v2"
)

// WriteAll writes UTF-8 text (including Chinese characters and Emojis) to the system clipboard
func WriteAll(text string) error {
	switch runtime.GOOS {
	case "darwin":
		return writeDarwin(text)
	case "windows":
		return writeWindows(text)
	default:
		return writeLinux(text)
	}
}

// ReadAll reads UTF-8 text from the system clipboard
func ReadAll() (string, error) {
	switch runtime.GOOS {
	case "darwin":
		return readDarwin()
	case "windows":
		return readWindows()
	default:
		return readLinux()
	}
}

// macOS implementation using pbcopy / pbpaste with explicit UTF-8 environment
func writeDarwin(text string) error {
	cmd := exec.Command("pbcopy")
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8")
	cmd.Stdin = strings.NewReader(text)
	if err := cmd.Run(); err == nil {
		return nil
	}
	return writeOSC52(text)
}

func readDarwin() (string, error) {
	cmd := exec.Command("pbpaste")
	cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}

// Linux / Unix implementation (WSL, Wayland, X11, OSC 52)
func writeLinux(text string) error {
	// 1. WSL environment check
	if isWSL() {
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Console]::InputEncoding = [System.Text.Encoding]::UTF8; Set-Clipboard -Value ([System.Console]::In.ReadToEnd())")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	// 2. Wayland check (wl-copy)
	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-copy"); err == nil {
			cmd := exec.Command("wl-copy", "--type", "text/plain;charset=utf-8")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	// 3. X11 xclip check (forcing -t UTF8_STRING to avoid Latin-1 garbled text)
	if os.Getenv("DISPLAY") != "" {
		if _, err := exec.LookPath("xclip"); err == nil {
			cmd := exec.Command("xclip", "-in", "-selection", "clipboard", "-t", "UTF8_STRING")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}

		// 4. X11 xsel check
		if _, err := exec.LookPath("xsel"); err == nil {
			cmd := exec.Command("xsel", "--input", "--clipboard")
			cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8")
			cmd.Stdin = strings.NewReader(text)
			if err := cmd.Run(); err == nil {
				return nil
			}
		}
	}

	// 5. OSC 52 fallback for SSH, remote terminals, tmux, and headless environments
	if err := writeOSC52(text); err == nil {
		return nil
	}

	return errors.New("no working clipboard utility found (please install xclip, wl-clipboard, or xsel, or use an OSC 52 compatible terminal)")
}

// writeOSC52 writes the OSC 52 ANSI escape sequence to the terminal.
// This allows copying text directly to the client's local clipboard even over SSH / tmux / headless environments.
func writeOSC52(text string) error {
	seq := osc52.New(text)
	if os.Getenv("TMUX") != "" {
		seq = seq.Tmux()
	} else if strings.HasPrefix(os.Getenv("TERM"), "screen") {
		seq = seq.Screen()
	}

	// Try writing directly to controlling terminal (/dev/tty)
	if tty, err := os.OpenFile("/dev/tty", os.O_WRONLY, 0); err == nil {
		defer tty.Close()
		if _, err := seq.WriteTo(tty); err == nil {
			return nil
		}
	}

	// Fallback to Stderr
	if _, err := seq.WriteTo(os.Stderr); err == nil {
		return nil
	}

	// Fallback to Stdout
	if _, err := seq.WriteTo(os.Stdout); err == nil {
		return nil
	}

	return errors.New("failed to write OSC 52 sequence to terminal")
}

func readLinux() (string, error) {
	if isWSL() {
		if _, err := exec.LookPath("powershell.exe"); err == nil {
			cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-Clipboard")
			out, err := cmd.Output()
			if err == nil {
				return strings.TrimRight(string(out), "\r\n"), nil
			}
		}
	}

	if os.Getenv("WAYLAND_DISPLAY") != "" {
		if _, err := exec.LookPath("wl-paste"); err == nil {
			cmd := exec.Command("wl-paste", "--no-newline")
			out, err := cmd.Output()
			if err == nil {
				return string(out), nil
			}
		}
	}

	if _, err := exec.LookPath("xclip"); err == nil {
		cmd := exec.Command("xclip", "-out", "-selection", "clipboard", "-t", "UTF8_STRING")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	if _, err := exec.LookPath("xsel"); err == nil {
		cmd := exec.Command("xsel", "--output", "--clipboard")
		cmd.Env = append(os.Environ(), "LANG=en_US.UTF-8", "LC_CTYPE=en_US.UTF-8")
		out, err := cmd.Output()
		if err == nil {
			return string(out), nil
		}
	}

	return "", errors.New("no working clipboard utility found")
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

// Windows native implementation
func writeWindows(text string) error {
	if _, err := exec.LookPath("powershell.exe"); err == nil {
		cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Console]::InputEncoding = [System.Text.Encoding]::UTF8; Set-Clipboard -Value ([System.Console]::In.ReadToEnd())")
		cmd.Stdin = strings.NewReader(text)
		return cmd.Run()
	}
	cmd := exec.Command("cmd.exe", "/c", "chcp 65001 >NUL && clip.exe")
	cmd.Stdin = strings.NewReader(text)
	return cmd.Run()
}

func readWindows() (string, error) {
	cmd := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", "[Console]::OutputEncoding = [System.Text.Encoding]::UTF8; Get-Clipboard")
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}
