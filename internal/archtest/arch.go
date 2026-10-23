// Package archtest enforces the dependency rules in docs/ARCHITECTURE.md for
// every module, without naming any of them, so a new module is covered the
// day it is created. Go's internal/ rule already stops one module importing
// another's internals; these checks cover what the compiler cannot see.
//
// A module is a top-level directory with a module.go. Its layers:
//
//	<m>/                          wiring (module.go, migrate.go)
//	<m>/api, <m>/codes, <m>/ports contracts other modules may use
//	<m>/migrations                its own schema
//	<m>/internal/domain           rules, no I/O
//	<m>/internal/application      use cases, no SQL
//	<m>/internal/persistence      SQL, only for its own schema
//	<m>/internal/adapter          implements the module's api over application
//	<m>/internal/adapters         implements another module's ports
//	<m>/internal/transport        HTTP, through the api only
package archtest

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// mayImport is the layering inside one module: what each layer may import
// from its own module. A layer may always import itself.
var mayImport = map[string][]string{
	"root":        {"api", "codes", "ports", "domain", "application", "persistence", "adapter", "adapters", "transport"},
	"api":         {"codes"},
	"codes":       {},
	"ports":       {},
	"domain":      {"codes"},
	"application": {"domain", "persistence", "ports", "codes"},
	"persistence": {"domain"},
	"adapter":     {"api", "codes", "application", "domain"},
	"adapters":    {"domain", "persistence"},
	"transport":   {"api", "codes"},
}

// noDatabase are the layers that must never touch a database directly.
var noDatabase = map[string]bool{
	"api": true, "codes": true, "ports": true, "domain": true,
	"application": true, "adapter": true, "transport": true,
}

// contracts are the parts of a module another module may import.
var contracts = map[string]bool{"api": true, "codes": true, "ports": true}

type pkg struct {
	dir     string   // relative to the repo root, slash-separated
	imports []string // import paths of non-test files
	strs    []string // string literals of non-test files, for schema checks
}

// Check walks the repo at root, whose Go module path is module, and returns
// every rule broken, sorted. deps lists, for each module, the modules it may
// depend on (through their contracts only); everything else is forbidden.
func Check(root, module, platform string, deps map[string][]string) ([]string, error) {
	pkgs, err := load(root)
	if err != nil {
		return nil, err
	}
	mods := modules(root)
	var out []string
	add := func(format string, a ...any) { out = append(out, fmt.Sprintf(format, a...)) }
	allowed := func(from, to string) bool {
		for _, d := range deps[from] {
			if d == to {
				return true
			}
		}
		return false
	}

	for _, p := range pkgs {
		owner, layer := classify(p.dir, mods)
		if owner != "" && layer == "other" {
			add("%s: not a known layer of module %s", p.dir, owner)
		}
		for _, imp := range p.imports {
			if noDatabase[layer] && (imp == "database/sql" || imp == module+"/"+platform+"/database") {
				add("%s (%s) must not touch the database: imports %s", p.dir, layer, imp)
			}
			if layer == "domain" && imp == "net/http" {
				add("%s (domain) must not import net/http", p.dir)
			}
			l, local := strings.CutPrefix(imp, module+"/")
			if !local {
				continue
			}
			to, toLayer := classify(l, mods)
			switch {
			case under(p.dir, platform) && to != "":
				add("%s: %s must not depend on module %s", p.dir, platform, to)
			case to == "":
				// cmd, platform and repo-internal packages are fine to import
			case owner == "" && toLayer == "root" && !under(p.dir, "cmd"):
				add("%s: only cmd/ may import module wiring %s", p.dir, l)
			case owner != "" && to != owner && !contracts[toLayer]:
				add("%s: module %s may use module %s only through api, codes or ports, imports %s", p.dir, owner, to, l)
			case owner != "" && to != owner && !allowed(owner, to):
				add("%s: module %s does not declare a dependency on module %s", p.dir, owner, to)
			case owner != "" && to == owner && toLayer != layer && !contains(mayImport[layer], toLayer):
				add("%s: %s must not import %s (%s)", p.dir, layer, l, toLayer)
			}
		}
		if owner != "" {
			for _, s := range p.strs {
				for _, other := range mods {
					if other != owner && schemaRef(other).MatchString(s) {
						add("%s: SQL references schema %s, owned by another module", p.dir, other)
					}
				}
			}
		}
	}

	for _, m := range mods {
		_ = filepath.WalkDir(filepath.Join(root, m, "migrations"), func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sql") {
				return nil
			}
			b, _ := os.ReadFile(path)
			rel, _ := filepath.Rel(root, path)
			for _, other := range mods {
				if other != m && schemaRef(other).Match(b) {
					add("%s: migration references schema %s", filepath.ToSlash(rel), other)
				}
			}
			return nil
		})
	}
	sort.Strings(out)
	return dedupe(out), nil
}

// classify returns the module a slash-separated directory belongs to and its
// layer; module is "" outside any module.
func classify(dir string, mods []string) (module, layer string) {
	parts := strings.Split(dir, "/")
	if !contains(mods, parts[0]) {
		return "", ""
	}
	module = parts[0]
	switch {
	case len(parts) == 1:
		return module, "root"
	case parts[1] == "api" || parts[1] == "codes" || parts[1] == "ports":
		return module, parts[1]
	case parts[1] == "internal" && len(parts) >= 3:
		if _, known := mayImport[parts[2]]; known {
			return module, parts[2]
		}
	}
	return module, "other"
}

// modules are the top-level directories that contain a module.go.
func modules(root string) []string {
	entries, _ := os.ReadDir(root)
	var out []string
	for _, e := range entries {
		if e.IsDir() {
			if _, err := os.Stat(filepath.Join(root, e.Name(), "module.go")); err == nil {
				out = append(out, e.Name())
			}
		}
	}
	return out
}

func schemaRef(schema string) *regexp.Regexp {
	return regexp.MustCompile(`(?i)(^|[^a-z0-9_."])` + regexp.QuoteMeta(schema) + `\.[a-z_]`)
}

func load(root string) ([]pkg, error) {
	byDir := map[string]*pkg{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && (d.Name() == "testdata" || strings.HasPrefix(d.Name(), ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, filepath.Dir(path))
		rel = filepath.ToSlash(rel)
		p := byDir[rel]
		if p == nil {
			p = &pkg{dir: rel}
			byDir[rel] = p
		}
		for _, imp := range f.Imports {
			s, _ := strconv.Unquote(imp.Path.Value)
			p.imports = append(p.imports, s)
		}
		ast.Inspect(f, func(n ast.Node) bool {
			if _, isImport := n.(*ast.ImportSpec); isImport {
				return false
			}
			if lit, ok := n.(*ast.BasicLit); ok && lit.Kind == token.STRING {
				if s, err := strconv.Unquote(lit.Value); err == nil {
					p.strs = append(p.strs, s)
				}
			}
			return true
		})
		return nil
	})
	out := make([]pkg, 0, len(byDir))
	for _, p := range byDir {
		out = append(out, *p)
	}
	return out, err
}

func under(dir, prefix string) bool { return dir == prefix || strings.HasPrefix(dir, prefix+"/") }

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func dedupe(xs []string) []string {
	var out []string
	for i, x := range xs {
		if i == 0 || x != xs[i-1] {
			out = append(out, x)
		}
	}
	return out
}
