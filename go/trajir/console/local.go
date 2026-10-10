package console

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// startLocal launches a local process and does not wait. Tests replace it.
var startLocal = func(name string, args ...string) error {
	return startOS(name, args)
}

func isLoopback(r *http.Request) bool {
	if r == nil {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

type localAction struct {
	Root         bool   `json:"root"`
	TrajectoryID string `json:"trajectory_id"`
	Name         string `json:"name"`
}

// isLoopbackHost reports whether a Host or Origin host names this machine.
func isLoopbackHost(hostport string) bool {
	host := hostport
	if h, _, err := net.SplitHostPort(hostport); err == nil {
		host = h
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// isSameOriginLocal blocks browser requests from other sites. A page on any
// origin can POST to 127.0.0.1, so the loopback peer check alone is not enough.
// Host must be a loopback name (no DNS rebinding) and Origin, when sent, must
// be this console.
func isSameOriginLocal(r *http.Request) bool {
	if !isLoopbackHost(r.Host) {
		return false
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "same-origin" && site != "none" {
		return false
	}
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Host, r.Host)
}

func (s *Server) requireLoopback(w http.ResponseWriter, r *http.Request) bool {
	if !isLoopback(r) {
		writeErr(w, http.StatusForbidden, "local actions are loopback-only")
		return false
	}
	if !isSameOriginLocal(r) {
		writeErr(w, http.StatusForbidden, "local actions are same-origin only")
		return false
	}
	return true
}

func (s *Server) resolveLocalPath(act localAction) (string, error) {
	if act.Root || (strings.TrimSpace(act.TrajectoryID) == "" && strings.TrimSpace(act.Name) == "") {
		return s.Store.Root(), nil
	}
	if strings.TrimSpace(act.Name) == "" {
		if err := validateTrajectoryID(act.TrajectoryID); err != nil {
			return "", err
		}
		return filepath.Join(s.Store.Root(), packagesDir, act.TrajectoryID), nil
	}
	return s.Store.PackagePath(act.TrajectoryID, act.Name)
}

func parseLocalAction(r *http.Request) (localAction, error) {
	var act localAction
	if r.Body == nil {
		return localAction{Root: true}, nil
	}
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(&act); err != nil {
		if err == io.EOF {
			return localAction{Root: true}, nil
		}
		return act, err
	}
	return act, nil
}

func (s *Server) handleReveal(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if !s.requireLoopback(w, r) {
		return
	}
	act, err := parseLocalAction(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	path, err := s.resolveLocalPath(act)
	if err != nil {
		writeErr(w, statusForRead(err), err.Error())
		return
	}
	name, args := revealArgs(path)
	if err := startLocal(name, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not open folder")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "opened", "path": path})
}

func (s *Server) handleOpenShell(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if !s.requireLoopback(w, r) {
		return
	}
	act, err := parseLocalAction(r)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "invalid json")
		return
	}
	if strings.TrimSpace(act.Name) != "" {
		// Shell opens a directory, never a file path as a command.
		act.Name = ""
	}
	path, err := s.resolveLocalPath(act)
	if err != nil {
		writeErr(w, statusForRead(err), err.Error())
		return
	}
	name, args := openShellArgs(path)
	if err := startLocal(name, args...); err != nil {
		writeErr(w, http.StatusInternalServerError, "could not open shell")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "opened", "path": path})
}

// handleRunLangGraphDemo runs the evidence-worktree demo_stub into this console.
// Loopback-only. Fixed script path. No user-supplied command.
func (s *Server) handleRunLangGraphDemo(w http.ResponseWriter, r *http.Request) {
	if !s.authorize(w, r) {
		return
	}
	if !s.requireLoopback(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		writeErr(w, http.StatusMethodNotAllowed, "POST required")
		return
	}
	root, err := resolveEvidenceRoot(s.Store.Root())
	if err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	demo := filepath.Join(root, "integrations", "langgraph", "demo_stub.py")
	if _, err := os.Stat(demo); err != nil {
		writeErr(w, http.StatusBadRequest, "langgraph demo_stub.py not found under "+root+"; set TRAJIR_EVIDENCE_ROOT")
		return
	}
	py := resolvePython(root)
	host := r.Host
	if host == "" {
		host = "127.0.0.1:8787"
	}
	consoleURL := "http://" + host
	cmd := exec.Command(py, demo) // #nosec G204 -- fixed demo path, not request input
	cmd.Dir = root
	sep := string(os.PathListSeparator)
	cmd.Env = append(os.Environ(),
		"TRAJIR_CONSOLE_SINK=http",
		"TRAJIR_CONSOLE_URL="+consoleURL,
		"PYTHONPATH="+root+sep+filepath.Join(root, "pkg"),
	)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if len(trimmed) > 4000 {
		trimmed = trimmed[len(trimmed)-4000:]
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{
			"status":        "failed",
			"trajectory_id": "langgraph-demo",
			"error":         err.Error(),
			"output":        trimmed,
			"evidence_root": root,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":        "ok",
		"trajectory_id": "langgraph-demo",
		"output":        trimmed,
		"evidence_root": root,
	})
}

func resolveEvidenceRoot(consoleDataRoot string) (string, error) {
	if v := strings.TrimSpace(os.Getenv("TRAJIR_EVIDENCE_ROOT")); v != "" {
		if !isDir(v) {
			return "", fmt.Errorf("TRAJIR_EVIDENCE_ROOT is not a directory: %s", v)
		}
		return filepath.Clean(v), nil
	}
	// Sibling worktree: Trajectory-IR-wt-evidence next to this console checkout.
	consoleRoot := filepath.Dir(consoleDataRoot)
	sibling := filepath.Clean(filepath.Join(consoleRoot, "..", "Trajectory-IR-wt-evidence"))
	if isDir(sibling) {
		return sibling, nil
	}
	return "", fmt.Errorf("set TRAJIR_EVIDENCE_ROOT to the evidence worktree (looked for %s)", sibling)
}

func resolvePython(evidenceRoot string) string {
	if v := strings.TrimSpace(os.Getenv("TRAJIR_PYTHON")); v != "" {
		return v
	}
	candidates := []string{
		filepath.Join(evidenceRoot, ".venv", "Scripts", "python.exe"),
		filepath.Join(evidenceRoot, ".venv", "bin", "python"),
		filepath.Join(filepath.Dir(evidenceRoot), "Trajectory-IR", ".venv", "Scripts", "python.exe"),
		filepath.Join(filepath.Dir(evidenceRoot), ".venv", "Scripts", "python.exe"),
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			return c
		}
	}
	if p, err := exec.LookPath("python"); err == nil {
		return p
	}
	return "python"
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func windowsSystemRoot() string {
	root := os.Getenv("SystemRoot")
	if root == "" {
		root = os.Getenv("WINDIR")
	}
	return root
}

func windowsCmd() string {
	root := windowsSystemRoot()
	if root == "" {
		return "cmd.exe"
	}
	return filepath.Join(root, "System32", "cmd.exe")
}

func windowsExplorer() string {
	root := windowsSystemRoot()
	if root == "" {
		return "explorer.exe"
	}
	return filepath.Join(root, "explorer.exe")
}

func windowsPowerShell() string {
	if p, err := exec.LookPath("pwsh"); err == nil {
		return p
	}
	root := windowsSystemRoot()
	if root == "" {
		return "powershell.exe"
	}
	return filepath.Join(root, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
}

func revealArgs(path string) (string, []string) {
	switch runtime.GOOS {
	case "windows":
		if isDir(path) {
			// cmd start opens the folder in this session. explorer /select, on a
			// dotted directory only highlights it in the parent and looks like a no-op.
			return windowsCmd(), []string{"/c", "start", "", path}
		}
		return windowsExplorer(), []string{"/select," + path}
	case "darwin":
		if isDir(path) {
			return "open", []string{path}
		}
		return "open", []string{"-R", path}
	default:
		return "xdg-open", []string{path}
	}
}

func openShellArgs(dir string) (string, []string) {
	switch runtime.GOOS {
	case "windows":
		return windowsCmd(), []string{
			"/c", "start", "", "/D", dir, windowsPowerShell(), "-NoLogo", "-NoExit",
		}
	case "darwin":
		return "open", []string{"-a", "Terminal", dir}
	default:
		return "xdg-open", []string{dir}
	}
}
