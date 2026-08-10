package clipboard

import (
	"testing"
)

func TestClipboard_ChineseAndEmoji(t *testing.T) {
	testText := "你好，世界！LLM TUI 剪贴板测试 😀🚀🤖"

	err := WriteAll(testText)
	if err != nil {
		t.Skipf("Skipping clipboard test (no active clipboard utility or DISPLAY): %v", err)
	}

	got, err := ReadAll()
	if err != nil {
		t.Skipf("Skipping clipboard read test: %v", err)
	}

	if got != testText {
		t.Errorf("Clipboard text mismatch:\n  got:  %q\n  want: %q", got, testText)
	}
}
