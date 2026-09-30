//go:build windows

package console

import (
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	createNewConsole       = 0x00000010
	detachedProcess        = 0x00000008
	createBreakawayFromJob = 0x01000000
)

func applyLocalProcAttr(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	base := strings.ToLower(filepath.Base(cmd.Path))
	if base == "." || base == "" {
		if len(cmd.Args) > 0 {
			base = strings.ToLower(filepath.Base(cmd.Args[0]))
		}
	}
	flags := uint32(detachedProcess | createBreakawayFromJob)
	if strings.Contains(base, "powershell") || base == "pwsh.exe" || base == "pwsh" {
		flags = createNewConsole | createBreakawayFromJob
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: flags,
	}
	if strings.Contains(base, "powershell") || base == "pwsh.exe" || base == "pwsh" {
		cmd.SysProcAttr.HideWindow = false
	}
}

func startOS(name string, args []string) error {
	cmd := exec.Command(name, args...)
	applyLocalProcAttr(cmd)
	if err := cmd.Start(); err != nil {
		cmd = exec.Command(name, args...)
		applyLocalProcAttr(cmd)
		if cmd.SysProcAttr != nil {
			cmd.SysProcAttr.CreationFlags &^= createBreakawayFromJob
		}
		return cmd.Start()
	}
	return nil
}
