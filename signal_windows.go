//go:build windows

package minitui

import "os"

func setupSignals(sigCh chan os.Signal) {
	// SIGWINCH is not available on Windows.
}

func teardownSignals(sigCh chan os.Signal) {
	// no-op on Windows.
}
