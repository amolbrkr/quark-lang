package vsix

import (
	"archive/zip"
	"encoding/xml"
	"io"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestBuild_PackagesTheExtension(t *testing.T) {
	_, file, _, _ := runtime.Caller(0)
	repo := filepath.Clean(filepath.Join(filepath.Dir(file), "..", "..", "..", "..", "..", ".."))
	extDir := filepath.Join(repo, "src", "devtools", "vscode-extension", "quark")
	out := filepath.Join(t.TempDir(), "quark.vsix")

	path, err := Build(extDir, filepath.Join(repo, "LICENSE"), out)
	if err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(path)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()

	names := map[string]*zip.File{}
	for _, f := range zr.File {
		names[f.Name] = f
	}
	for _, want := range []string{
		"[Content_Types].xml",
		"extension.vsixmanifest",
		"extension/package.json",
		"extension/extension.js",
		"extension/language-configuration.json",
		"extension/syntaxes/quark.tmLanguage.json",
		"extension/snippets/quark.json",
		"extension/data/quark.json",
		"extension/README.md",
		"extension/LICENSE.txt",
	} {
		if names[want] == nil {
			t.Errorf("missing %s", want)
		}
	}
	for name := range names {
		if strings.HasPrefix(name, "extension/test/") {
			t.Errorf("tests should not be packaged: %s", name)
		}
	}

	// The manifest must be well-formed XML with the package identity.
	rc, err := names["extension.vsixmanifest"].Open()
	if err != nil {
		t.Fatal(err)
	}
	data, _ := io.ReadAll(rc)
	rc.Close()
	var pm struct {
		Metadata struct {
			Identity struct {
				ID        string `xml:"Id,attr"`
				Version   string `xml:"Version,attr"`
				Publisher string `xml:"Publisher,attr"`
			} `xml:"Identity"`
		} `xml:"Metadata"`
	}
	if err := xml.Unmarshal(data, &pm); err != nil {
		t.Fatalf("manifest is not valid XML: %v\n%s", err, data)
	}
	id := pm.Metadata.Identity
	if id.ID != "quark-language" || id.Publisher != "quark-lang" || id.Version == "" {
		t.Fatalf("unexpected identity %+v", id)
	}
}
