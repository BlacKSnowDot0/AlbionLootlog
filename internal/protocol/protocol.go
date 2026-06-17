// Package protocol contains Photon event/operation code definitions for
// Albion Online, mirrored from the upstream C# project
// Triky313/AlbionOnline-StatisticsAnalysis (GPL-3.0).
//
// The *_gen.go files in this package are produced by tools/codegen and must
// not be edited by hand. Run `go generate ./...` (or `go run ./tools/codegen`)
// to refresh them from upstream.
package protocol

import "strconv"

//go:generate go run ../../tools/codegen -root ../..

// itoa is a tiny wrapper used by the generated String() methods so the
// generated files do not need to import strconv directly.
func itoa(i int) string { return strconv.Itoa(i) }
