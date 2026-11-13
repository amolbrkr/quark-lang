// Package names maps the internal names the loader gives definitions back to
// the names users write (modules v0.2 spec, §5.1).
//
// The loader renames every definition of an imported file, and every module
// member, to a unique internal name (q2_fmt__show_vec, mathx__square), so the
// analyzer and the back end work on unique strings. Everything user-facing
// (diagnostics, QFnInfo names, struct display names) goes through Display.
package names

import (
	"strings"

	"quark/ast"
)

// Kind is what a name defines.
type Kind int

const (
	Func Kind = iota
	Struct
	Extern
	Value
	Module
	Local // a local of an imported file renamed to avoid the importer's names
)

// File is one loaded source file.
type File struct {
	ID      int    // 0 for the main script, then imports in load order
	Path    string // absolute path
	Stem    string // last path segment without .qrk; the default binding
	Display string // how diagnostics name the file (std/fmt.qrk, lib/geo.qrk)
	Main    bool
}

// Origin is where an internal name was defined.
type Origin struct {
	File   *File
	Module string // "" for a top-level name
	Name   string // as written: "show_vec"
	Kind   Kind
	Def    *ast.TreeNode
}

// Qualified is the name as users write it from outside its file:
// geo::Point, fmt::show_vec, formats::csv::read; main-script names without
// the file (Point, mathx::square).
func (o *Origin) Qualified() string {
	if o.Kind == Local {
		return o.Name
	}
	parts := make([]string, 0, 3)
	if !o.File.Main {
		parts = append(parts, o.File.Stem)
	}
	if o.Module != "" && !(o.File != nil && !o.File.Main && o.Module == o.File.Stem) {
		parts = append(parts, o.Module)
	}
	parts = append(parts, o.Name)
	return strings.Join(parts, "::")
}

// Table maps internal names to their origin.
type Table map[string]*Origin

// Display is the user-facing name for internal, or internal itself when it
// isn't a renamed definition.
func (t Table) Display(internal string) string {
	if o, ok := t[internal]; ok {
		return o.Qualified()
	}
	return internal
}

// DisplayText replaces every internal name appearing as a word in msg with
// its display name, so messages built from internal names read as written.
func (t Table) DisplayText(msg string) string {
	if len(t) == 0 {
		return msg
	}
	var b strings.Builder
	i := 0
	for i < len(msg) {
		if !isWordStart(msg[i]) || (i > 0 && isWordChar(msg[i-1])) {
			b.WriteByte(msg[i])
			i++
			continue
		}
		j := i
		for j < len(msg) && isWordChar(msg[j]) {
			j++
		}
		b.WriteString(t.Display(msg[i:j]))
		i = j
	}
	return b.String()
}

func isWordStart(c byte) bool {
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z')
}

func isWordChar(c byte) bool {
	return isWordStart(c) || (c >= '0' && c <= '9')
}
