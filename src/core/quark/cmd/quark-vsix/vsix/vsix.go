// Package vsix writes a VS Code extension package (.vsix): a zip with an
// OPC content-types file, an extension.vsixmanifest, and the extension's
// files under extension/.
package vsix

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/template"
	"time"
)

// included lists what goes into the package, relative to the extension
// directory. Directories are included recursively.
var included = []string{
	"package.json",
	"extension.js",
	"language-configuration.json",
	"README.md",
	"CHANGELOG.md",
	"syntaxes",
	"snippets",
	"data",
}

type manifest struct {
	Name        string   `json:"name"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Version     string   `json:"version"`
	Publisher   string   `json:"publisher"`
	Categories  []string `json:"categories"`
	Keywords    []string `json:"keywords"`
	Engines     struct {
		VSCode string `json:"vscode"`
	} `json:"engines"`
}

// Build packages extDir into out (or the default path if out is empty) and
// returns the path written. licenseFile is added as extension/LICENSE.txt.
func Build(extDir, licenseFile, out string) (string, error) {
	raw, err := os.ReadFile(filepath.Join(extDir, "package.json"))
	if err != nil {
		return "", err
	}
	var m manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return "", fmt.Errorf("package.json: %w", err)
	}
	for field, v := range map[string]string{"name": m.Name, "version": m.Version, "publisher": m.Publisher, "engines.vscode": m.Engines.VSCode} {
		if v == "" {
			return "", fmt.Errorf("package.json: %s is required", field)
		}
	}
	if out == "" {
		out = filepath.Join(filepath.Dir(extDir), "dist", fmt.Sprintf("%s-%s.vsix", m.Name, m.Version))
	}

	files, err := collect(extDir)
	if err != nil {
		return "", err
	}
	// A broken JSON file would make the extension fail to load; catch it here.
	for _, rel := range files {
		if strings.HasSuffix(rel, ".json") {
			data, err := os.ReadFile(filepath.Join(extDir, rel))
			if err != nil {
				return "", err
			}
			if !json.Valid(data) {
				return "", fmt.Errorf("%s is not valid JSON", rel)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return "", err
	}
	tmp := out + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp)
	zw := zip.NewWriter(f)

	if err := writeTemplate(zw, "[Content_Types].xml", contentTypes, nil); err != nil {
		return "", err
	}
	if err := writeTemplate(zw, "extension.vsixmanifest", vsixManifest, m); err != nil {
		return "", err
	}
	for _, rel := range files {
		if err := copyInto(zw, filepath.Join(extDir, rel), "extension/"+filepath.ToSlash(rel)); err != nil {
			return "", err
		}
	}
	if err := copyInto(zw, licenseFile, "extension/LICENSE.txt"); err != nil {
		return "", err
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	if err := f.Close(); err != nil {
		return "", err
	}
	return out, os.Rename(tmp, out)
}

func collect(extDir string) ([]string, error) {
	var files []string
	for _, entry := range included {
		root := filepath.Join(extDir, entry)
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(extDir, path)
			if err != nil {
				return err
			}
			files = append(files, rel)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("extension file %s: %w", entry, err)
		}
	}
	sort.Strings(files)
	return files, nil
}

func copyInto(zw *zip.Writer, src, name string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	w, err := create(zw, name)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// create adds a deflated entry stamped with the current time.
func create(zw *zip.Writer, name string) (io.Writer, error) {
	return zw.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Deflate, Modified: time.Now()})
}

func writeTemplate(zw *zip.Writer, name, text string, data interface{}) error {
	t, err := template.New(name).Funcs(template.FuncMap{"join": strings.Join}).Parse(text)
	if err != nil {
		return err
	}
	w, err := create(zw, name)
	if err != nil {
		return err
	}
	return t.Execute(w, data)
}

const contentTypes = `<?xml version="1.0" encoding="utf-8"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension=".json" ContentType="application/json"/>
  <Default Extension=".js" ContentType="application/javascript"/>
  <Default Extension=".md" ContentType="text/markdown"/>
  <Default Extension=".txt" ContentType="text/plain"/>
  <Default Extension=".vsixmanifest" ContentType="text/xml"/>
</Types>
`

// Every value from package.json goes through text/template's html function,
// which escapes the XML special characters (& < > ' ").
const vsixManifest = `<?xml version="1.0" encoding="utf-8"?>
<PackageManifest Version="2.0.0" xmlns="http://schemas.microsoft.com/developer/vsx-schema/2011" xmlns:d="http://schemas.microsoft.com/developer/vsx-schema-design/2011">
  <Metadata>
    <Identity Language="en-US" Id="{{html .Name}}" Version="{{html .Version}}" Publisher="{{html .Publisher}}"/>
    <DisplayName>{{html .DisplayName}}</DisplayName>
    <Description xml:space="preserve">{{html .Description}}</Description>
    <Tags>{{html (join .Keywords ",")}}</Tags>
    <Categories>{{html (join .Categories ",")}}</Categories>
    <GalleryFlags>Public</GalleryFlags>
    <Properties>
      <Property Id="Microsoft.VisualStudio.Code.Engine" Value="{{html .Engines.VSCode}}"/>
      <Property Id="Microsoft.VisualStudio.Code.ExtensionDependencies" Value=""/>
      <Property Id="Microsoft.VisualStudio.Code.ExtensionPack" Value=""/>
      <Property Id="Microsoft.VisualStudio.Code.ExtensionKind" Value="workspace"/>
      <Property Id="Microsoft.VisualStudio.Code.LocalizedLanguages" Value=""/>
      <Property Id="Microsoft.VisualStudio.Services.GitHubFlavoredMarkdown" Value="true"/>
    </Properties>
    <License>extension/LICENSE.txt</License>
  </Metadata>
  <Installation>
    <InstallationTarget Id="Microsoft.VisualStudio.Code"/>
  </Installation>
  <Dependencies/>
  <Assets>
    <Asset Type="Microsoft.VisualStudio.Code.Manifest" Path="extension/package.json" Addressable="true"/>
    <Asset Type="Microsoft.VisualStudio.Services.Content.Details" Path="extension/README.md" Addressable="true"/>
    <Asset Type="Microsoft.VisualStudio.Services.Content.Changelog" Path="extension/CHANGELOG.md" Addressable="true"/>
    <Asset Type="Microsoft.VisualStudio.Services.Content.License" Path="extension/LICENSE.txt" Addressable="true"/>
  </Assets>
</PackageManifest>
`
