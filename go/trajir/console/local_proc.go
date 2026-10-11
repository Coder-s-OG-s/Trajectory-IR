//go:build !windows

package console

import "os/exec"

func applyLocalProcAttr(cmd *exec.Cmd) {}

func startOS(name string, args []string) error {
	cmd := exec.Command(name, args...)
	return cmd.Start()
}
