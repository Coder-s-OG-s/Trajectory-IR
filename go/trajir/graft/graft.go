// Package graft transfers artifact refs between trajectories (R07).
// Never copies THOUGHT (or other private kinds) as nodes.
package graft

import (
	"fmt"
	"sort"

	nodelog "github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/log"
	"github.com/Coder-s-OG-s/Trajectory-IR/go/trajir/nodes"
)

// PrivateKinds must never cross a graft boundary as full nodes.
var PrivateKinds = map[string]struct{}{
	"THOUGHT": {},
}

var artifactKinds = map[string]struct{}{
	"ARTIFACT_PUT": {},
	"ARTIFACT_REF": {},
}

// Error is a graft failure.
type Error struct {
	Msg string
}

func (e *Error) Error() string { return e.Msg }

func payloadContentHash(payload map[string]any) string {
	if payload == nil {
		return ""
	}
	for _, key := range []string{"content_hash", "hash", "sha256"} {
		if v, ok := payload[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// FindArtifactRef returns source ARTIFACT_* metadata for contentHash.
func FindArtifactRef(sourceNodes []map[string]any, contentHash string) (map[string]any, error) {
	if contentHash == "" {
		return nil, &Error{Msg: "content_hash is required"}
	}
	var matches []map[string]any
	for _, n := range sourceNodes {
		kind, _ := n["kind"].(string)
		if _, priv := PrivateKinds[kind]; priv {
			continue
		}
		if _, ok := artifactKinds[kind]; !ok {
			continue
		}
		payload, _ := n["payload"].(map[string]any)
		if payloadContentHash(payload) == contentHash {
			matches = append(matches, n)
		}
	}
	if len(matches) == 0 {
		return nil, &Error{Msg: fmt.Sprintf("no ARTIFACT_PUT/ARTIFACT_REF with content_hash=%q", contentHash)}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		ki, _ := matches[i]["kind"].(string)
		kj, _ := matches[j]["kind"].(string)
		pi, pj := 1, 1
		if ki == "ARTIFACT_REF" {
			pi = 0
		}
		if kj == "ARTIFACT_REF" {
			pj = 0
		}
		if pi != pj {
			return pi < pj
		}
		si, sj := 0, 0
		if v, ok := matches[i]["seq"].(int); ok {
			si = v
		} else if v, ok := matches[i]["seq"].(float64); ok {
			si = int(v)
		}
		if v, ok := matches[j]["seq"].(int); ok {
			sj = v
		} else if v, ok := matches[j]["seq"].(float64); ok {
			sj = int(v)
		}
		return si < sj
	})
	return matches[0], nil
}

// GraftOptions configure GraftArtifactRef.
type GraftOptions struct {
	ContentHash         string
	TargetTrajectoryID  string
	TargetTenantID      string
	Seq                 int
	StepN               *int
	SourceNodes         []map[string]any
	LogicalPath         string
	URI                 string
	SourceTrajectoryID  string
	HasLogicalPath      bool
	HasURI              bool
	HasSourceTrajectory bool
}

// GraftArtifactRef appends ARTIFACT_REF on target. Never copies THOUGHT nodes.
func GraftArtifactRef(target *nodelog.NodeLog, opts GraftOptions) (*nodes.Node, error) {
	if target == nil {
		return nil, &Error{Msg: "nil NodeLog"}
	}
	logicalPath := opts.LogicalPath
	uri := opts.URI
	srcTraj := opts.SourceTrajectoryID
	if opts.SourceNodes != nil {
		for _, n := range opts.SourceNodes {
			kind, _ := n["kind"].(string)
			if _, priv := PrivateKinds[kind]; priv {
				payload, _ := n["payload"].(map[string]any)
				if payloadContentHash(payload) == opts.ContentHash {
					return nil, &Error{Msg: "refusing to graft from a THOUGHT/private node"}
				}
			}
		}
		src, err := FindArtifactRef(opts.SourceNodes, opts.ContentHash)
		if err != nil {
			return nil, err
		}
		payload, _ := src["payload"].(map[string]any)
		if !opts.HasLogicalPath {
			if v, ok := payload["logical_path"].(string); ok {
				logicalPath = v
			}
		}
		if !opts.HasURI {
			if v, ok := payload["uri"].(string); ok {
				uri = v
			}
		}
		if !opts.HasSourceTrajectory {
			if v, ok := src["trajectory_id"].(string); ok {
				srcTraj = v
			}
		}
	}
	payload := map[string]any{
		"content_hash": opts.ContentHash,
		"grafted":      true,
	}
	if logicalPath != "" || opts.HasLogicalPath {
		payload["logical_path"] = logicalPath
	}
	if uri != "" || opts.HasURI {
		payload["uri"] = uri
	}
	if srcTraj != "" || opts.HasSourceTrajectory {
		payload["source_trajectory_id"] = srcTraj
	}
	return target.Append(
		"ARTIFACT_REF",
		opts.StepN,
		payload,
		opts.TargetTrajectoryID,
		opts.TargetTenantID,
		opts.Seq,
	)
}
