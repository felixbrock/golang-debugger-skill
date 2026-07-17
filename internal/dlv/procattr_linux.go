package dlv

import "syscall"

// sysProcAttr puts dlv in its own process group so Kill can reap its whole
// tree, and asks the kernel to SIGKILL dlv if the daemon dies — even on the
// daemon's own SIGKILL/OOM/crash, where no cleanup code runs. Without this an
// orphaned dlv keeps the target process alive and its build locked.
// (Pdeathsig fires when the spawning thread dies, which for the daemon's
// lifetime is close enough; non-Linux has no equivalent and relies on the
// daemon reaping on relaunch/`gdbg down`.)
func sysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{Setpgid: true, Pdeathsig: syscall.SIGKILL}
}
