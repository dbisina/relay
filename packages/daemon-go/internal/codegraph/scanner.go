// internal/codegraph/scanner.go
//
// Walks a project directory, extracts top-level symbols from common languages,
// emits graph nodes + edges. Lightweight regex-based parser (no tree-sitter
// dependency) — good enough for context retrieval, not for refactoring.
//
// Node types:
//   "module"  — one source file
//   "symbol"  — a function, type, class, or const
//
// Edge types:
//   "defines"  module → symbol
//   "imports"  module → module (when a referenced import resolves to a local file)

package codegraph

import (
	"crypto/sha256"
	"encoding/hex"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// Symbol — one discovered top-level definition.
type Symbol struct {
	Kind string // "func" | "type" | "class" | "const" | "method"
	Name string
	File string // relative to repo root
	Line int
	Lang string
}

// Module — one source file.
type Module struct {
	Path    string // relative
	Lang    string
	Size    int64
	SHA     string // first 12 hex
	Imports []string
}

// Scan walks root, returning all discovered symbols and modules.
// Skips: vendor/, node_modules/, target/, .git/, dist/, build/, .relay/sessions/
// Skips files >256 KiB.
func Scan(root string) (mods []Module, syms []Symbol, err error) {
	skipDirs := map[string]bool{
		".git": true, "node_modules": true, "vendor": true, "target": true,
		"dist": true, "build": true, ".relay": true, ".idea": true, ".vscode": true,
	}

	werr := filepath.WalkDir(root, func(path string, d fs.DirEntry, e error) error {
		if e != nil {
			return nil
		}
		if d.IsDir() {
			if skipDirs[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		info, statErr := d.Info()
		if statErr != nil || info.Size() == 0 || info.Size() > 256*1024 {
			return nil
		}

		lang := langForExt(filepath.Ext(path))
		if lang == "" {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		rel = filepath.ToSlash(rel)

		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil
		}
		sum := sha256.Sum256(data)
		mod := Module{
			Path: rel, Lang: lang, Size: info.Size(),
			SHA: hex.EncodeToString(sum[:6]),
		}
		modSyms, imports := parseFile(lang, rel, string(data))
		mod.Imports = imports
		mods = append(mods, mod)
		syms = append(syms, modSyms...)
		return nil
	})
	if werr != nil && werr != io.EOF {
		return nil, nil, werr
	}
	sort.Slice(mods, func(i, j int) bool { return mods[i].Path < mods[j].Path })
	sort.Slice(syms, func(i, j int) bool {
		if syms[i].File == syms[j].File {
			return syms[i].Line < syms[j].Line
		}
		return syms[i].File < syms[j].File
	})
	return mods, syms, nil
}

// ─── language detection ──────────────────────────────────────────────────────

func langForExt(ext string) string {
	switch strings.ToLower(ext) {
	case ".go":
		return "go"
	case ".rs":
		return "rust"
	case ".ts", ".tsx":
		return "ts"
	case ".js", ".jsx", ".mjs":
		return "js"
	case ".py":
		return "py"
	case ".java", ".kt":
		return "jvm"
	case ".rb":
		return "rb"
	}
	return ""
}

// ─── per-language regex parsers ──────────────────────────────────────────────

var (
	goFunc   = regexp.MustCompile(`(?m)^func(?:\s+\([^)]*\))?\s+([A-Z][A-Za-z0-9_]*)\s*\(`)
	goFuncP  = regexp.MustCompile(`(?m)^func(?:\s+\([^)]*\))?\s+([a-z][A-Za-z0-9_]*)\s*\(`)
	goType   = regexp.MustCompile(`(?m)^type\s+([A-Z][A-Za-z0-9_]*)\s+(?:struct|interface|=)`)
	goImport = regexp.MustCompile(`(?m)^\s*"([^"]+)"`)

	rsFunc   = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?fn\s+([a-z_][A-Za-z0-9_]*)\s*[(<]`)
	rsStruct = regexp.MustCompile(`(?m)^\s*(?:pub\s+)?(?:struct|enum|trait)\s+([A-Z][A-Za-z0-9_]*)`)
	rsUse    = regexp.MustCompile(`(?m)^\s*use\s+(crate::[a-zA-Z0-9_::]+)`)

	tsFunc  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:async\s+)?function\s+([A-Za-z_][A-Za-z0-9_]*)\s*[(<]`)
	tsClass = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:abstract\s+)?class\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsType  = regexp.MustCompile(`(?m)^\s*(?:export\s+)?(?:type|interface)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	tsImp   = regexp.MustCompile(`(?m)^\s*import.+?from\s+['"]([^'"]+)['"]`)

	pyFunc  = regexp.MustCompile(`(?m)^def\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(`)
	pyClass = regexp.MustCompile(`(?m)^class\s+([A-Za-z_][A-Za-z0-9_]*)\s*[:(]`)
	pyImp   = regexp.MustCompile(`(?m)^(?:from\s+([\w.]+)\s+import|import\s+([\w.]+))`)
)

func parseFile(lang, rel, content string) (syms []Symbol, imports []string) {
	switch lang {
	case "go":
		syms = append(syms, matchAll(rel, "go", "func", goFunc, content)...)
		syms = append(syms, matchAll(rel, "go", "func", goFuncP, content)...)
		syms = append(syms, matchAll(rel, "go", "type", goType, content)...)
		imports = matchImports(goImport, content)
	case "rust":
		syms = append(syms, matchAll(rel, "rust", "func", rsFunc, content)...)
		syms = append(syms, matchAll(rel, "rust", "type", rsStruct, content)...)
		imports = matchImports(rsUse, content)
	case "ts", "js":
		syms = append(syms, matchAll(rel, lang, "func", tsFunc, content)...)
		syms = append(syms, matchAll(rel, lang, "class", tsClass, content)...)
		syms = append(syms, matchAll(rel, lang, "type", tsType, content)...)
		imports = matchImports(tsImp, content)
	case "py":
		syms = append(syms, matchAll(rel, "py", "func", pyFunc, content)...)
		syms = append(syms, matchAll(rel, "py", "class", pyClass, content)...)
		imports = matchPyImports(content)
	}
	return
}

func matchAll(rel, lang, kind string, re *regexp.Regexp, content string) []Symbol {
	matches := re.FindAllStringSubmatchIndex(content, -1)
	out := make([]Symbol, 0, len(matches))
	for _, m := range matches {
		if len(m) < 4 {
			continue
		}
		name := content[m[2]:m[3]]
		line := 1 + strings.Count(content[:m[0]], "\n")
		out = append(out, Symbol{
			Kind: kind, Name: name, File: rel, Line: line, Lang: lang,
		})
	}
	return out
}

func matchImports(re *regexp.Regexp, content string) []string {
	matches := re.FindAllStringSubmatch(content, -1)
	out := make([]string, 0, len(matches))
	seen := map[string]bool{}
	for _, m := range matches {
		if len(m) >= 2 && m[1] != "" && !seen[m[1]] {
			seen[m[1]] = true
			out = append(out, m[1])
		}
	}
	return out
}

func matchPyImports(content string) []string {
	matches := pyImp.FindAllStringSubmatch(content, -1)
	out := []string{}
	seen := map[string]bool{}
	for _, m := range matches {
		v := m[1]
		if v == "" {
			v = m[2]
		}
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}
