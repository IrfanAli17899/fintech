// Package archtest enforces the dependency rules in docs/ARCHITECTURE.md.
// Go's internal/ rule already stops one module importing another's
// internals; these tests cover the rules the compiler cannot see.
package archtest

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/irfanali17899/fintech/"

// imports maps each package directory (relative to the repo root) to the
// module-internal and std imports of its non-test files.
func imports(t *testing.T) map[string][]string {
	t.Helper()
	out := map[string][]string{}
	err := filepath.WalkDir("../..", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		dir, _ := filepath.Rel("../..", filepath.Dir(path))
		dir = filepath.ToSlash(dir)
		out[dir] = append(out[dir], []string{}...) // record packages with no imports too
		for _, imp := range f.Imports {
			p, _ := strconv.Unquote(imp.Path.Value)
			out[dir] = append(out[dir], p)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func local(p string) (string, bool) { return strings.CutPrefix(p, module) }

func under(dir, prefix string) bool { return dir == prefix || strings.HasPrefix(dir, prefix+"/") }

type rule struct {
	name    string
	applies func(dir string) bool
	allowed func(imp string) bool // called for every import of an applying package
}

func TestDependencyRules(t *testing.T) {
	onlyLocal := func(allowed ...string) func(string) bool {
		return func(imp string) bool {
			l, ok := local(imp)
			if !ok {
				return true
			}
			for _, a := range allowed {
				if under(l, a) {
					return true
				}
			}
			return false
		}
	}
	rules := []rule{
		{"transport depends on the api, never on the application or domain",
			func(d string) bool { return under(d, "ledger/internal/transport") },
			onlyLocal("ledger/api", "ledger/codes", "platform")},
		{"use cases never see SQL",
			func(d string) bool { return d == "ledger/internal/application" },
			func(imp string) bool {
				l, _ := local(imp)
				return imp != "database/sql" && l != "platform/database" &&
					!under(l, "ledger/internal/transport") && !under(l, "ledger/internal/adapter")
			}},
		{"persistence knows nothing above it",
			func(d string) bool { return d == "ledger/internal/persistence" },
			onlyLocal("ledger/internal/domain", "platform")},
		{"domain depends only on codes and errdefs",
			func(d string) bool { return d == "ledger/internal/domain" },
			onlyLocal("ledger/codes", "platform/errdefs")},
		{"api depends only on codes",
			func(d string) bool { return d == "ledger/api" },
			onlyLocal("ledger/codes")},
		{"codes is a leaf",
			func(d string) bool { return d == "ledger/codes" },
			func(string) bool { return false }},
		{"ports depend on nothing in the module",
			func(d string) bool { return d == "ledger/ports" },
			onlyLocal()},
		{"rails sees the ledger only through its ports",
			func(d string) bool { return under(d, "rails") },
			func(imp string) bool {
				l, ok := local(imp)
				return !ok || !under(l, "ledger") || l == "ledger/ports"
			}},
		{"the ledger never depends on rails",
			func(d string) bool { return under(d, "ledger") },
			func(imp string) bool { l, _ := local(imp); return !under(l, "rails") }},
		{"platform is infrastructure with no module knowledge",
			func(d string) bool { return under(d, "platform") },
			func(imp string) bool { l, ok := local(imp); return !ok || under(l, "platform") }},
	}

	all := imports(t)
	for _, r := range rules {
		matched := false
		for dir, imps := range all {
			if !r.applies(dir) {
				continue
			}
			matched = true
			for _, imp := range imps {
				if !r.allowed(imp) {
					t.Errorf("%s: %s imports %s", r.name, dir, imp)
				}
			}
		}
		if !matched {
			t.Errorf("%s: matched no package; the rule or the layout has drifted", r.name)
		}
	}
}
