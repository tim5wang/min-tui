//go:build !windows

package minitui

import (
	"os"
	"os/signal"
	"syscall"
)

func setupSignals(sigCh chan os.Signal) {
	signal.Notify(sigCh, syscall.SIGWINCH)
}

func teardownSignals(sigCh chan os.Signal) {
	signal.Stop(sigCh)
}
