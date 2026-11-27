package hosthook

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/emit"
)

// DefaultConsoleDir is where hook runs send dashboard events unless told
// otherwise: TRAJIR_CONSOLE_DATA, else ~/.trajir/console-data. One dir for
// every project and agent, so one dashboard shows them all.
func DefaultConsoleDir(getenv func(string) string) string {
	if v := strings.TrimSpace(getenv("TRAJIR_CONSOLE_DATA")); v != "" {
		return v
	}
	home := strings.TrimSpace(getenv("HOME"))
	if home == "" {
		home = strings.TrimSpace(getenv("USERPROFILE"))
	}
	if home == "" {
		home, _ = os.UserHomeDir()
	}
	return filepath.Join(home, ".trajir", "console-data")
}

// DefaultSink returns the dashboard sink for hook runs and its data dir.
// TRAJIR_CONSOLE_SINK=off (or none, 0) sends nothing. http posts to
// TRAJIR_CONSOLE_URL. Anything else, including unset, is a file sink in
// DefaultConsoleDir. The console re-reads those files on every request, so a
// running dashboard updates live.
func DefaultSink(getenv func(string) string) (emit.Sink, string) {
	switch strings.ToLower(strings.TrimSpace(getenv("TRAJIR_CONSOLE_SINK"))) {
	case "off", "none", "0", "false":
		return nil, ""
	case "http":
		url := strings.TrimSpace(getenv("TRAJIR_CONSOLE_URL"))
		if url == "" {
			return nil, ""
		}
		return &emit.HTTPSink{BaseURL: url, Token: getenv("TRAJIR_CONSOLE_TOKEN")}, ""
	}
	dir := DefaultConsoleDir(getenv)
	return &emit.FileSink{Dir: dir}, dir
}
