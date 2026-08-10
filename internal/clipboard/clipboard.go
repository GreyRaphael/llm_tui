package clipboard

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
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
	return cmd.Run()
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

// Linux / Unix implementation (WSL, Wayland, X11)
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

	return errors.New("no working clipboard utility found (please install xclip, wl-clipboard, or xsel)")
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
