package archtest

import (
	"strings"
	"testing"
)

// deps is the module dependency graph from docs/ARCHITECTURE.md: a module
// may depend on the listed modules, through their contracts only. A new edge
// is an architecture decision; record it there and here.
var deps = map[string][]string{
	"rails": {"ledger"},
}

func TestRepoFollowsTheRules(t *testing.T) {
	broken, err := Check("../..", "github.com/irfanali17899/fintech", "platform", deps)
	if err != nil {
		t.Fatal(err)
	}
	for _, b := range broken {
		t.Error(b)
	}
	if mods := modules("../.."); len(mods) < 2 {
		t.Fatalf("found modules %v; the module detection has drifted", mods)
	}
}

func TestCheckerAcceptsAGoodRepo(t *testing.T) {
	broken, err := Check("testdata/good", "example.com/shop", "platform", map[string][]string{"orders": {"catalog"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(broken) != 0 {
		t.Fatalf("good fixture reported:\n%s", strings.Join(broken, "\n"))
	}
}

// Every rule is broken exactly once in the bad fixture; the checker must
// report exactly these, nothing missed and nothing extra.
func TestCheckerCatchesEveryRule(t *testing.T) {
	broken, err := Check("testdata/bad", "example.com/shop", "platform", map[string][]string{"orders": {"catalog"}})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"catalog/api: api must not import catalog/internal/domain (domain)",
		"catalog/helpers: not a known layer of module catalog",
		"catalog/internal/application (application) must not touch the database: imports example.com/shop/platform/database",
		"catalog/internal/domain (domain) must not import net/http",
		"catalog/internal/domain (domain) must not touch the database: imports database/sql",
		"catalog/internal/transport: transport must not import catalog/internal/application (application)",
		"orders/internal/application: module orders may use module catalog only through api, codes or ports, imports catalog/internal/domain",
		"orders/internal/persistence: SQL references schema catalog, owned by another module",
		"orders/migrations/001.up.sql: migration references schema catalog",
		"payments/internal/application: module payments does not declare a dependency on module catalog",
		"platform/database: platform must not depend on module catalog",
		"tools/seed: only cmd/ may import module wiring catalog",
	}
	if strings.Join(broken, "\n") != strings.Join(want, "\n") {
		t.Fatalf("got:\n%s\n\nwant:\n%s", strings.Join(broken, "\n"), strings.Join(want, "\n"))
	}
}
