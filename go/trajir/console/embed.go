package console

import "embed"

// UI is the static Trajectory console shell (no build step).
//
//go:embed web/*
var UI embed.FS
