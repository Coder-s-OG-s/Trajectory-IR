package console

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
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

func (s *Server) requireLoopback(w http.ResponseWriter, r *http.Request) bool {
	if isLoopback(r) {
		return true
	}
	writeErr(w, http.StatusForbidden, "local actions are loopback-only")
	return false
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
