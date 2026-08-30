// Package parity provides black-box behavior comparison between this Go
// rewrite and the original TypeScript Claude Code binary (the "oracle").
//
// It is the landing of retrospective recommendation 3
// (docs/project/discussions/2026-08-29-process-retrospective.md): acceptance
// signals must include an external anchor, not only internal metrics such as
// coverage or self-authored tests.
//
// The oracle is injected via the CCG_PARITY_ORACLE environment variable and
// the Go build via CCG_PARITY_TARGET (default: bin/claude). Without an
// oracle every comparison test SKIPs, so ordinary `go test ./...` and CI stay
// green; `make parity` builds the target and runs the suite.
//
// Helpers live in helpers_test.go; the runnable cases in parity_test.go;
// the case ledger (including not-yet-covered gaps) in cases.md.
package parity
