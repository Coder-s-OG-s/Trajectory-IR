//go:build windows

package console

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRevealArgsSelectsExistingFile(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "demo.tir")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, args := revealArgs(file)
	if len(args) != 1 || args[0] != "/select,"+file {
		t.Fatalf("want /select,file got %v", args)
	}
}

func TestWindowsShellGetsNewConsole(t *testing.T) {
	cmd := exec.Command("powershell.exe", "-NoLogo")
	applyLocalProcAttr(cmd)
	if cmd.SysProcAttr == nil || cmd.SysProcAttr.CreationFlags&createNewConsole == 0 {
		t.Fatal("powershell must start in a new console window")
	}
	if cmd.SysProcAttr.HideWindow {
		t.Fatal("powershell window must not be hidden")
	}
	helper := exec.Command("cmd.exe", "/c", "start", "", `C:\`)
	applyLocalProcAttr(helper)
	if helper.SysProcAttr == nil {
		t.Fatal("cmd start needs process attributes")
	}
	if helper.SysProcAttr.CreationFlags&createNewConsole != 0 {
		t.Fatal("cmd helper must not flash a console")
	}
}

func TestOpenShellArgsWindowsUsesStart(t *testing.T) {
	dir := t.TempDir()
	name, args := openShellArgs(dir)
	if !strings.Contains(strings.ToLower(name), "cmd") {
		t.Fatalf("name=%s", name)
	}
	joined := strings.Join(args, " ")
	if !strings.Contains(joined, "start") || !strings.Contains(joined, "/D") || !strings.Contains(joined, dir) {
		t.Fatalf("args=%v", args)
	}
}
