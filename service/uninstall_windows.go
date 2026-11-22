//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
)

// removeInstallDir takes the folder off the user PATH and deletes it. Windows will not let a running
// program delete its own .exe and .dll files, so a detached helper does it a few seconds after clumyap exits.
func removeInstallDir(dir string) error {
	ps := fmt.Sprintf(`$d = '%s'; $p = [Environment]::GetEnvironmentVariable('Path', 'User'); `+
		`if ($p) { [Environment]::SetEnvironmentVariable('Path', ((($p -split ';') | Where-Object { $_ -and $_ -ne $d }) -join ';'), 'User') }`,
		strings.ReplaceAll(dir, "'", "''"))
	if err := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", ps).Run(); err != nil {
		return fmt.Errorf("remove %s from PATH: %w", dir, err)
	}
	cmd := exec.Command("cmd")
	cmd.Dir = os.TempDir() // not inside the folder being deleted
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine:       fmt.Sprintf(`cmd /c ping -n 4 127.0.0.1 >nul & rmdir /s /q "%s"`, dir),
		CreationFlags: 0x00000008 | 0x00000200, // DETACHED_PROCESS | CREATE_NEW_PROCESS_GROUP
	}
	return cmd.Start()
}
