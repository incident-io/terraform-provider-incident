package provider

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// An acceptance test only runs if a family in families_test.go names it, and it only runs
// once if exactly one family does. Neither is enforced by the compiler: an acceptance test
// added as a top-level TestAccXxx runs alongside every other test rather than inside its
// family, which is how it ends up racing a test it shares an organisation with, and one
// left out of a family silently stops running at all.
//
// So check it here, by reading the package's own source. This needs no API credentials and
// runs as an ordinary unit test, which is the point: it fails on the pull request that adds
// the test, rather than in whichever unrelated acceptance test it later collides with.
func TestAcceptanceFamiliesCoverEveryTest(t *testing.T) {
	declared, referenced, topLevel := parseAcceptanceTests(t)

	for _, name := range topLevel {
		t.Errorf("%s is a top-level test, so it escapes its family and runs against "+
			"everything else at once: rename it to acc%s and add it to a family in "+
			"families_test.go", name, strings.TrimPrefix(name, "TestAcc"))
	}

	for _, name := range declared {
		switch count := referenced[name]; count {
		case 1:
		case 0:
			t.Errorf("%s is not in any family in families_test.go, so it never runs", name)
		default:
			t.Errorf("%s is in %d families, so it runs %d times", name, count, count)
		}
	}

	declaredSet := map[string]bool{}
	for _, name := range declared {
		declaredSet[name] = true
	}
	for name := range referenced {
		if !declaredSet[name] {
			t.Errorf("families_test.go names %s, which is not an acceptance test", name)
		}
	}
}

// parseAcceptanceTests reads the package's test files and returns the acceptance tests it
// declares, how many families name each one, and any that are still top-level tests.
func parseAcceptanceTests(t *testing.T) (declared []string, referenced map[string]int, topLevel []string) {
	t.Helper()

	paths, err := filepath.Glob("*_test.go")
	if err != nil {
		t.Fatalf("globbing test files: %s", err)
	}
	if len(paths) == 0 {
		t.Fatal("found no test files: this test reads the package source, so it has to run from the package directory")
	}

	referenced = map[string]int{}
	fset := token.NewFileSet()

	for _, path := range paths {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %s", path, err)
		}

		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv != nil || !isTestSignature(fn) {
				continue
			}

			switch name := fn.Name.Name; {
			case strings.HasPrefix(name, "acc"):
				declared = append(declared, name)
			// TestAcceptance* are the families themselves.
			case strings.HasPrefix(name, "TestAcc") && !strings.HasPrefix(name, "TestAcceptance"):
				topLevel = append(topLevel, name)
			}
		}

		// Every acceptance test a family names is an identifier in this file, so counting
		// the identifiers counts the memberships.
		if filepath.Base(path) == "families_test.go" {
			ast.Inspect(file, func(node ast.Node) bool {
				if ident, ok := node.(*ast.Ident); ok && strings.HasPrefix(ident.Name, "acc") {
					referenced[ident.Name]++
				}
				return true
			})
		}
	}

	sort.Strings(declared)
	sort.Strings(topLevel)

	return declared, referenced, topLevel
}

// isTestSignature reports whether fn takes a single *testing.T and returns nothing, which
// is what both an acceptance test and a family parent look like.
func isTestSignature(fn *ast.FuncDecl) bool {
	if fn.Type.Results != nil || fn.Type.Params == nil || len(fn.Type.Params.List) != 1 {
		return false
	}

	star, ok := fn.Type.Params.List[0].Type.(*ast.StarExpr)
	if !ok {
		return false
	}
	selector, ok := star.X.(*ast.SelectorExpr)
	if !ok {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)

	return ok && pkg.Name == "testing" && selector.Sel.Name == "T"
}
