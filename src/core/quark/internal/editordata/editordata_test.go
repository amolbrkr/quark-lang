// Package editordata keeps the VS Code / Positron extension in sync with the
// compiler. The extension's completion data (data/quark.json) is generated
// from the builtin catalog, the lexer's keyword table and the stdlib modules;
// its grammar and snippets are checked against the same sources.
//
// After changing builtins, keywords or the stdlib, regenerate with:
//
//	go test ./internal/editordata -update
package editordata

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"

	"quark/ast"
	"quark/builtins"
	"quark/lexer"
	"quark/parser"
	"quark/token"
)

var update = flag.Bool("update", false, "regenerate the extension's data/quark.json")

func repoRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate test file")
	}
	// <repo>/src/core/quark/internal/editordata/editordata_test.go
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", ".."))
}

func extensionDir(t *testing.T) string {
	return filepath.Join(repoRoot(t), "src", "devtools", "vscode-extension", "quark")
}

// Function is a callable shown in completion.
type Function struct {
	Name      string `json:"name"`
	Signature string `json:"signature"`     // e.g. "len(any) int" or "dot(x: vec, y: vec) float"
	Doc       string `json:"doc,omitempty"` // first comment line above a stdlib function
}

// Module is a stdlib module importable as `incl 'std/<name>'`.
type Module struct {
	Name      string     `json:"name"`
	Functions []Function `json:"functions"`
}

// Data is the content of data/quark.json.
type Data struct {
	Comment   string                `json:"_comment"`
	Keywords  []string              `json:"keywords"`
	Functions []Function            `json:"functions"`
	Methods   map[string][]Function `json:"methods"` // receiver type -> methods
	Stdlib    []Module              `json:"stdlib"`
}

// displayType turns a catalog type key into the name a user writes.
func displayType(k builtins.TypeKey) string {
	s := string(k)
	switch {
	case s == "void" || s == "":
		return ""
	case strings.HasPrefix(s, "list"):
		return "list"
	case strings.HasPrefix(s, "dict"):
		return "dict"
	case strings.HasPrefix(s, "vec"):
		return "vec"
	case strings.HasPrefix(s, "result"):
		return "result"
	}
	return s
}

var receiverNames = map[builtins.TypeKey]string{
	builtins.TypeString:    "str",
	builtins.TypeListAny:   "list",
	builtins.TypeDictAny:   "dict",
	builtins.TypeVectorAny: "vec",
}

// catalogSignature renders a builtin as name(a: T, ...) R. The catalog has
// no parameter names, so parameters are named by type; optional ones get '?'.
func catalogSignature(s builtins.Spec) string {
	params := s.ParamTypes
	if s.ReceiverType != "" && len(params) > 0 {
		params = params[1:] // the receiver is written before the dot
	}
	min := s.MinArgs
	if s.ReceiverType != "" && min > 0 {
		min--
	}
	parts := make([]string, len(params))
	for i, p := range params {
		name := displayType(p)
		if name == "" {
			name = "any"
		}
		if i >= min {
			name += "?"
		}
		parts[i] = name
	}
	sig := fmt.Sprintf("%s(%s)", s.Name, strings.Join(parts, ", "))
	if r := displayType(s.ReturnType); r != "" {
		sig += " " + r
	}
	return sig
}

// stdlibModules parses every src/core/stdlib/*.qrk with the real parser and
// lists the functions of each module block.
func stdlibModules(t *testing.T) []Module {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot(t), "src", "core", "stdlib", "*.qrk"))
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(files)
	var mods []Module
	for _, f := range files {
		src, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		lines := strings.Split(string(src), "\n")
		p := parser.New(lexer.New(string(src)).Tokenize())
		tree := p.Parse()
		if len(p.Errors()) > 0 {
			t.Fatalf("parse %s: %v", f, p.Errors())
		}
		for _, n := range tree.Children {
			if n.NodeType != ast.ModuleNode || len(n.Children) < 2 {
				continue
			}
			mod := Module{Name: strings.TrimSuffix(filepath.Base(f), ".qrk")}
			for _, stmt := range n.Children[1].Children {
				if fn, ok := moduleFunction(stmt, lines); ok {
					mod.Functions = append(mod.Functions, fn)
				}
			}
			mods = append(mods, mod)
		}
	}
	return mods
}

// moduleFunction recognizes `name = fn(params) T -> ...` (what `fn name(...)`
// desugars to) inside a module body.
func moduleFunction(stmt *ast.TreeNode, lines []string) (Function, bool) {
	if stmt.NodeType != ast.OperatorNode || len(stmt.Children) != 2 ||
		stmt.Children[0].NodeType != ast.IdentifierNode || stmt.Children[1].NodeType != ast.LambdaNode {
		return Function{}, false
	}
	name := stmt.Children[0].TokenLiteral()
	lambda := stmt.Children[1]
	var params []string
	if len(lambda.Children) > 0 {
		for _, p := range lambda.Children[0].Children {
			params = append(params, paramText(p))
		}
	}
	sig := fmt.Sprintf("%s(%s)", name, strings.Join(params, ", "))
	if lambda.ReturnType != nil {
		sig += " " + lambda.ReturnType.TokenLiteral()
	}
	fn := Function{Name: name, Signature: sig}
	// A `//` comment on the line above documents the function.
	if tok := stmt.Children[0].Token; tok != nil && tok.Line >= 2 {
		above := strings.TrimSpace(lines[tok.Line-2])
		if strings.HasPrefix(above, "//") {
			fn.Doc = strings.TrimSpace(strings.TrimPrefix(above, "//"))
		}
	}
	return fn, true
}

func paramText(p *ast.TreeNode) string {
	s := p.TokenLiteral()
	for _, c := range p.Children {
		if c.NodeType == ast.TypeNode {
			s += ": " + c.TokenLiteral()
		}
	}
	if p.DefaultValue != nil {
		lit := p.DefaultValue.TokenLiteral()
		if p.DefaultValue.Token != nil && p.DefaultValue.Token.Type == token.STRING {
			lit = "'" + lit + "'"
		}
		s += " = " + lit
	}
	return s
}

func buildData(t *testing.T) Data {
	d := Data{
		Comment:  "Generated by src/core/quark/internal/editordata; do not edit. Regenerate: cd src/core/quark && go test ./internal/editordata -update",
		Keywords: token.Keywords(),
		Methods:  map[string][]Function{},
		Stdlib:   stdlibModules(t),
	}
	seen := map[string]bool{}
	for _, s := range builtins.Catalog() {
		fn := Function{Name: s.Name, Signature: catalogSignature(s)}
		if s.ReceiverType == "" {
			if !seen[s.Name] {
				seen[s.Name] = true
				d.Functions = append(d.Functions, fn)
			}
			continue
		}
		recv, ok := receiverNames[s.ReceiverType]
		if !ok {
			t.Fatalf("method %s has receiver type %s with no display name; add it to receiverNames", s.Name, s.ReceiverType)
		}
		d.Methods[recv] = append(d.Methods[recv], fn)
	}
	return d
}

func TestExtensionData_UpToDate(t *testing.T) {
	path := filepath.Join(extensionDir(t), "data", "quark.json")
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	if err := enc.Encode(buildData(t)); err != nil {
		t.Fatal(err)
	}
	if *update {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	got, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(got, buf.Bytes()) {
		t.Fatalf("%s is out of date with the compiler; run: cd src/core/quark && go test ./internal/editordata -update", path)
	}
}

// grammarWords returns every alternative inside \b(...)\b groups of the
// grammar's match patterns under the given repository keys.
func grammarWords(t *testing.T, keys ...string) map[string]bool {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(extensionDir(t), "syntaxes", "quark.tmLanguage.json"))
	if err != nil {
		t.Fatal(err)
	}
	var g struct {
		Repository map[string]struct {
			Patterns []struct {
				Match string `json:"match"`
			} `json:"patterns"`
		} `json:"repository"`
	}
	if err := json.Unmarshal(raw, &g); err != nil {
		t.Fatalf("grammar is not valid JSON: %v", err)
	}
	group := regexp.MustCompile(`\(\??:?([A-Za-z_|]+)\)`)
	words := map[string]bool{}
	for _, key := range keys {
		entry, ok := g.Repository[key]
		if !ok {
			t.Fatalf("grammar has no repository entry %q", key)
		}
		for _, p := range entry.Patterns {
			for _, m := range group.FindAllStringSubmatch(p.Match, -1) {
				for _, w := range strings.Split(m[1], "|") {
					words[w] = true
				}
			}
		}
	}
	return words
}

func TestGrammar_CoversKeywords(t *testing.T) {
	words := grammarWords(t, "keywords")
	for _, kw := range token.Keywords() {
		if !words[kw] {
			t.Errorf("keyword %q is not highlighted; add it to the \"keywords\" patterns in syntaxes/quark.tmLanguage.json", kw)
		}
	}
}

func TestGrammar_CoversBuiltins(t *testing.T) {
	words := grammarWords(t, "builtin-functions")
	for _, s := range builtins.Catalog() {
		if !words[s.Name] {
			t.Errorf("builtin %q is not highlighted; add it to the \"builtin-functions\" patterns in syntaxes/quark.tmLanguage.json", s.Name)
		}
	}
}

// TestSnippets_Parse expands every snippet with its placeholder defaults and
// checks the result is valid Quark, so snippets never teach wrong syntax.
func TestSnippets_Parse(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join(extensionDir(t), "snippets", "quark.json"))
	if err != nil {
		t.Fatal(err)
	}
	var snippets map[string]struct {
		Prefix string   `json:"prefix"`
		Body   []string `json:"body"`
	}
	if err := json.Unmarshal(raw, &snippets); err != nil {
		t.Fatalf("snippets are not valid JSON: %v", err)
	}
	withDefault := regexp.MustCompile(`\$\{\d+:([^}]*)\}`)
	choice := regexp.MustCompile(`\$\{\d+\|([^,|]*)[^}]*\}`)
	bare := regexp.MustCompile(`\$\{?\d+\}?`)
	for name, s := range snippets {
		body := strings.Join(s.Body, "\n")
		body = withDefault.ReplaceAllString(body, "$1")
		body = choice.ReplaceAllString(body, "$1")
		body = bare.ReplaceAllString(body, "null")
		body = strings.ReplaceAll(body, "\t", "    ")
		errs := parseErrors(body)
		if len(errs) > 0 {
			// Expression snippets (lambdas, struct literals) are only valid
			// as values: parse them as the right side of an assignment, with
			// the struct they use declared first.
			if len(parseErrors(snippetContext+"_value = "+body)) == 0 {
				continue
			}
			t.Errorf("snippet %q (prefix %q) does not parse:\n%s\nerrors: %v", name, s.Prefix, body, errs)
		}
	}
}

const snippetContext = "struct Point:\n    x: float\n    y: float\n"

func parseErrors(src string) []string {
	p := parser.New(lexer.New(src + "\n").Tokenize())
	p.Parse()
	return p.Errors()
}

// TestExtensionJS runs the extension's own tests (test/extension.test.js)
// with node from PATH, or from QUARK_NODE (Positron and VS Code bundle a node
// binary that works). Skipped when no node is available.
func TestExtensionJS(t *testing.T) {
	node := os.Getenv("QUARK_NODE")
	if node == "" {
		var err error
		if node, err = exec.LookPath("node"); err != nil {
			t.Skip("node not found; set QUARK_NODE to run the extension tests")
		}
	}
	cmd := exec.Command(node, filepath.Join("test", "extension.test.js"))
	cmd.Dir = extensionDir(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("extension tests failed: %v\n%s", err, out)
	}
}
