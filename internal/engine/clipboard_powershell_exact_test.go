package engine

import (
	"context"
	"os/exec"
	"strings"
	"testing"
)

// Exercise the actual helper script, replacing only the clipboard source. Never
// touches the user's clipboard. In WSL include powershell.exe's directory in PATH.
func TestPowerShellClipboardReaderPreservesExactTrailingNewlines(t *testing.T) {
	powershell, err := exec.LookPath("powershell.exe")
	if err != nil {
		t.Skip("requires Windows PowerShell or WSL bridge")
	}
	args := nativeClipboardReaders[len(nativeClipboardReaders)-1].args
	script := args[len(args)-1]
	for _, text := range []string{"plain", "line\n", "line\r\n", "first\nsecond\n\n", ""} {
		t.Run(strings.ReplaceAll(text, "\n", "_"), func(t *testing.T) {
			mock := "function Get-Clipboard { param([switch]$Raw); return '" + strings.ReplaceAll(text, "'", "''") + "' }; "
			output, err := exec.CommandContext(context.Background(), powershell, "-NoProfile", "-NonInteractive", "-Command", mock+script).Output()
			if err != nil {
				t.Fatal(err)
			}
			if string(output) != text {
				t.Fatalf("clipboard bytes changed: got %q want %q", output, text)
			}
		})
	}
}
