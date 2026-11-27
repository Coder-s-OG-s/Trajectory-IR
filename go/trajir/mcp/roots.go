package mcp

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/workdir"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// claudeProjectDirEnv is set by Claude Code in the environment of an MCP
// stdio server. It is the project directory, not the process working directory.
const claudeProjectDirEnv = "CLAUDE_PROJECT_DIR"

// workspaceRootsRequest is the input-request id used to ask the client for
// its workspace roots when no root was configured.
const workspaceRootsRequest = "trajir_workspace"

// resolvedRootKey carries a workspace root already chosen for this call.
type resolvedRootKey struct{}

func errWorkspaceRoot() error {
	return fmt.Errorf("mcp: %s must be set to an approved workspace root", EnvWorkspaceRoot)
}

func errWorkspaceRootFromClient() error {
	return fmt.Errorf("mcp: %s must be set to an approved workspace root. The client did not report exactly one workspace directory", EnvWorkspaceRoot)
}

// explicitEnvRoot returns TRAJIR_MCP_ROOT when it is a real value.
// An unexpanded ${...} placeholder is ignored so a host that also sets
// CLAUDE_PROJECT_DIR can still name the project.
func explicitEnvRoot() string {
	root := strings.TrimSpace(os.Getenv(EnvWorkspaceRoot))
	if root == "" || strings.Contains(root, "${") {
		return ""
	}
	return root
}

// claudeProjectDir returns an absolute CLAUDE_PROJECT_DIR.
// A relative value is ignored. Accepting it would resolve against the
// process working directory, which is not an approved root.
func claudeProjectDir() string {
	root := strings.TrimSpace(os.Getenv(claudeProjectDirEnv))
	if root == "" || !filepath.IsAbs(root) {
		return ""
	}
	return root
}

// bindWorkspace chooses the root for one tool call.
// A configured root wins. Otherwise the handler asks the client for its
// workspace roots and continues on the next round of the same call.
// The result is non-nil only when the client still has to answer.
func bindWorkspace(ctx context.Context, req *mcp.CallToolRequest) (context.Context, *mcp.CallToolResult, error) {
	if explicitEnvRoot() != "" || claudeProjectDir() != "" {
		return ctx, nil, nil
	}
	if req != nil && req.Params != nil && req.Params.InputResponses != nil {
		raw, ok := req.Params.InputResponses[workspaceRootsRequest]
		if ok {
			resp, ok := raw.(*mcp.ListRootsResult)
			if !ok || resp == nil {
				return ctx, nil, errWorkspaceRootFromClient()
			}
			root, err := singleWorkspaceRoot(resp.Roots)
			if err != nil {
				return ctx, nil, err
			}
			return context.WithValue(ctx, resolvedRootKey{}, root), nil, nil
		}
	}
	if req == nil {
		return ctx, nil, errWorkspaceRoot()
	}
	return ctx, &mcp.CallToolResult{
		InputRequests: mcp.InputRequestMap{
			workspaceRootsRequest: &mcp.ListRootsParams{},
		},
	}, nil
}

// singleWorkspaceRoot accepts exactly one file directory from the client.
// Zero, or more than one, fails closed. A prompt cannot add a root here.
// The list comes from the MCP client, which is the host the user opened.
func singleWorkspaceRoot(roots []*mcp.Root) (string, error) {
	var dirs []string
	for _, root := range roots {
		if root == nil {
			continue
		}
		path, err := pathFromFileURI(root.URI)
		if err != nil {
			continue
		}
		canonical, err := workdir.CanonicalizeDir(path)
		if err != nil {
			continue
		}
		dirs = append(dirs, canonical)
	}
	if len(dirs) != 1 {
		return "", errWorkspaceRootFromClient()
	}
	return dirs[0], nil
}

// pathFromFileURI converts a file URI to an absolute local path.
// A host other than empty or localhost is rejected.
func pathFromFileURI(uri string) (string, error) {
	parsed, err := url.Parse(uri)
	if err != nil {
		return "", err
	}
	if parsed.Scheme != "file" {
		return "", fmt.Errorf("mcp: not a file URI")
	}
	if parsed.Host != "" && !strings.EqualFold(parsed.Host, "localhost") {
		return "", fmt.Errorf("mcp: unsupported file host")
	}
	if parsed.Path == "" {
		return "", fmt.Errorf("mcp: empty file path")
	}
	path := filepath.FromSlash(parsed.Path)
	// A file URI for a Windows drive is /C:/Users. FromSlash keeps the
	// leading separator, which is not an absolute drive path.
	if len(path) >= 3 && (path[0] == '\\' || path[0] == '/') && path[2] == ':' {
		path = path[1:]
	}
	if !filepath.IsAbs(path) {
		return "", fmt.Errorf("mcp: file URI is not an absolute path")
	}
	return path, nil
}
