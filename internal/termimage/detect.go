package termimage

import (
	"os"
	"strings"
	"sync"

	"golang.org/x/term"
)

type backend int

const (
	backendNone      backend = iota // not a TTY — skip rendering
	backendHalfBlock                // ANSI half-block fallback
	backendKitty                    // kitty graphics protocol
)

var (
	detectedBackend backend
	detectOnce      sync.Once
	overrideBackend *backend // testing only
)

func detectBackend(w any) backend {
	if overrideBackend != nil {
		return *overrideBackend
	}
	detectOnce.Do(func() {
		detectedBackend = doDetectBackend(w)
	})
	return detectedBackend
}

func doDetectBackend(w any) backend {
	f, ok := w.(*os.File)
	if !ok || !term.IsTerminal(int(f.Fd())) {
		return backendNone
	}

	if os.Getenv("ORB_NO_KITTY") != "" {
		return backendHalfBlock
	}

	if detectKittyEnv() {
		return backendKitty
	}

	return backendHalfBlock
}

func detectKittyEnv() bool {
	if os.Getenv("KITTY_WINDOW_ID") != "" {
		return true
	}

	tp := os.Getenv("TERM_PROGRAM")
	switch strings.ToLower(tp) {
	case "kitty", "wezterm", "ghostty":
		return true
	}

	return strings.Contains(os.Getenv("TERM"), "kitty")
}
