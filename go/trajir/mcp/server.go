// Package mcp implements a Model Context Protocol server for Trajectory IR.
//
// It is a thin adapter over trajir/client and trajir/tir. Hosts such as Claude
// Code and Cursor talk MCP; this package never reimplements durable execution
// or invents node kinds. See docs/INTEGRATIONS.md at the repository root.
package mcp

import (
	"context"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// Version is the MCP server implementation version (not the IR package format).
const Version = "0.1.0"

// ServerName is the MCP implementation name advertised to hosts.
const ServerName = "trajectory-ir"

// NewServer builds a configured MCP server with Trajectory IR tools registered.
func NewServer() *mcp.Server {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    ServerName,
		Version: Version,
	}, nil)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "trajectory_status",
		Description: "Summarize a trajectory NodeLog: node counts by kind, seal count, and paths.",
	}, toolStatus)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "trajectory_export_tir",
		Description: "Export a trajectory to a portable .tir package (thin mode by default).",
	}, toolExportTIR)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "trajectory_import_tir",
		Description: "Load and hash-verify a .tir package (does not write into a NodeLog).",
	}, toolImportTIR)

	mcp.AddTool(server, &mcp.Tool{
		Name:        "trajectory_verify_signature",
		Description: "Verify an optional trajir-pkg-sig-v1 package signature (unsigned packages are OK unless require_signature is true).",
	}, toolVerifySignature)

	return server
}

// RunStdio runs the server until the MCP client disconnects.
func RunStdio(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}
	return NewServer().Run(ctx, &mcp.StdioTransport{})
}
