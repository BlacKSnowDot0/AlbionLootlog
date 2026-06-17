// Command codegen fetches the upstream C# enum files (EventCodes.cs,
// OperationCodes.cs) from Triky313/AlbionOnline-StatisticsAnalysis and
// generates equivalent Go constant files.
//
// The upstream enums are POSITIONAL: only `Unused = 0` is given an explicit
// value, every following member auto-increments. That means the integer value
// of each event is simply its line position in the enum. When Albion patches
// and the protocol codes shift, the upstream maintainer reorders/inserts
// entries in these files, and re-running this generator reproduces the new
// values automatically.
//
// This is the safe, automatable half of "auto-update": syncing constants.
// Handler *logic* changes (field layouts, parsing) are NOT covered here and
// must be reviewed by a human.
//
// Usage:
//
//	go run ./tools/codegen            # fetch from upstream main branch
//	go run ./tools/codegen -ref vX.Y  # fetch from a specific tag/branch
package main

import (
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const rawBase = "https://raw.githubusercontent.com/Triky313/AlbionOnline-StatisticsAnalysis"

// source describes one upstream enum file to mirror into Go.
type source struct {
	upstreamPath string // path within the upstream repo
	enumName     string // C# enum name to locate
	goFile       string // output Go file (relative to repo root)
	goType       string // Go type name to emit
	docLine      string // short doc comment for the generated type
}

var sources = []source{
	{
		upstreamPath: "src/StatisticsAnalysisTool/Network/EventCodes.cs",
		enumName:     "EventCodes",
		goFile:       "internal/protocol/eventcodes_gen.go",
		goType:       "EventCode",
		docLine:      "EventCode identifies a Photon event. Values mirror the upstream positional enum.",
	},
	{
		upstreamPath: "src/StatisticsAnalysisTool/Network/OperationCodes.cs",
		enumName:     "OperationCodes",
		goFile:       "internal/protocol/opcodes_gen.go",
		goType:       "OperationCode",
		docLine:      "OperationCode identifies a Photon operation. Values mirror the upstream positional enum.",
	},
}

// memberRe matches a single enum member line, capturing the identifier and an
// optional explicit "= N" assignment. Trailing comments and commas are ignored.
var memberRe = regexp.MustCompile(`^\s*([A-Za-z_]\w*)\s*(?:=\s*(\d+))?\s*,?\s*(?://.*)?$`)

func main() {
	ref := flag.String("ref", "main", "upstream git ref (branch or tag) to fetch from")
	repoRoot := flag.String("root", ".", "repository root to write generated files into")
	flag.Parse()

	client := &http.Client{Timeout: 30 * time.Second}

	var failed bool
	for _, s := range sources {
		if err := generate(client, *ref, *repoRoot, s); err != nil {
			fmt.Fprintf(os.Stderr, "error generating %s: %v\n", s.goType, err)
			failed = true
			continue
		}
		fmt.Printf("generated %s\n", s.goFile)
	}
	if failed {
		os.Exit(1)
	}
}

func generate(client *http.Client, ref, root string, s source) error {
	url := fmt.Sprintf("%s/%s/%s", rawBase, ref, s.upstreamPath)
	raw, err := fetch(client, url)
	if err != nil {
		return err
	}

	members, err := parseEnum(raw, s.enumName)
	if err != nil {
		return err
	}
	if len(members) == 0 {
		return fmt.Errorf("no members parsed from enum %s", s.enumName)
	}

	out := renderGo(s, ref, members)

	dst := filepath.Join(root, filepath.FromSlash(s.goFile))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	return os.WriteFile(dst, []byte(out), 0o644)
}

func fetch(client *http.Client, url string) (string, error) {
	resp, err := client.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GET %s: %s", url, resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	return string(b), nil
}

type member struct {
	Name    string
	Value   int
	Comment string
}

// parseEnum extracts the ordered members of a C# enum, assigning auto-increment
// values where no explicit "= N" is present (mirroring C# enum semantics).
func parseEnum(src, enumName string) ([]member, error) {
	// Locate "enum <name>" then the opening brace.
	idx := strings.Index(src, "enum "+enumName)
	if idx < 0 {
		return nil, fmt.Errorf("enum %s not found", enumName)
	}
	open := strings.IndexByte(src[idx:], '{')
	if open < 0 {
		return nil, fmt.Errorf("opening brace for enum %s not found", enumName)
	}
	start := idx + open + 1
	close := strings.IndexByte(src[start:], '}')
	if close < 0 {
		return nil, fmt.Errorf("closing brace for enum %s not found", enumName)
	}
	body := src[start : start+close]

	var members []member
	next := 0 // next auto-increment value
	for _, line := range strings.Split(body, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "//") {
			continue
		}
		m := memberRe.FindStringSubmatch(line)
		if m == nil {
			// Not a recognizable member line (could be a block comment, etc.).
			continue
		}
		name := m[1]
		val := next
		if m[2] != "" {
			// Explicit assignment, e.g. "Unused = 0".
			fmt.Sscanf(m[2], "%d", &val)
		}
		members = append(members, member{
			Name:    name,
			Value:   val,
			Comment: extractComment(line),
		})
		next = val + 1
	}
	return members, nil
}

func extractComment(line string) string {
	if i := strings.Index(line, "//"); i >= 0 {
		c := strings.TrimSpace(line[i+2:])
		// Keep comments short; the upstream ones can be very long field maps.
		if len(c) > 160 {
			c = c[:157] + "..."
		}
		return c
	}
	return ""
}

func renderGo(s source, ref string, members []member) string {
	var b strings.Builder
	b.WriteString("// Code generated by tools/codegen; DO NOT EDIT.\n")
	fmt.Fprintf(&b, "// Source: %s (ref: %s)\n", s.upstreamPath, ref)
	b.WriteString("// Upstream: Triky313/AlbionOnline-StatisticsAnalysis (GPL-3.0)\n\n")
	b.WriteString("package protocol\n\n")

	fmt.Fprintf(&b, "// %s\n", s.docLine)
	fmt.Fprintf(&b, "type %s int\n\n", s.goType)

	b.WriteString("const (\n")
	for i, m := range members {
		line := fmt.Sprintf("\t%s%s %s = %d", s.goType, m.Name, s.goType, m.Value)
		// On the first line, anchor the type; subsequent lines can rely on iota-free
		// explicit values for clarity and stability across reorders.
		if i == 0 {
			line = fmt.Sprintf("\t%s%s %s = %d", s.goType, m.Name, s.goType, m.Value)
		}
		if m.Comment != "" {
			line += " // " + m.Comment
		}
		b.WriteString(line + "\n")
	}
	b.WriteString(")\n\n")

	// Name lookup table for human-readable logging/diagnostics.
	fmt.Fprintf(&b, "// %sNames maps a %s to its upstream identifier.\n", s.goType, s.goType)
	fmt.Fprintf(&b, "var %sNames = map[%s]string{\n", s.goType, s.goType)
	for _, m := range members {
		fmt.Fprintf(&b, "\t%s%s: %q,\n", s.goType, m.Name, m.Name)
	}
	b.WriteString("}\n\n")

	fmt.Fprintf(&b, "// String returns the upstream identifier for the %s.\n", s.goType)
	fmt.Fprintf(&b, "func (c %s) String() string {\n", s.goType)
	fmt.Fprintf(&b, "\tif n, ok := %sNames[c]; ok {\n", s.goType)
	b.WriteString("\t\treturn n\n")
	b.WriteString("\t}\n")
	fmt.Fprintf(&b, "\treturn \"%s(\" + itoa(int(c)) + \")\"\n", s.goType)
	b.WriteString("}\n")

	return b.String()
}
