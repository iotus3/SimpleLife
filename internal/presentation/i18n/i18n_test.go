package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unicode"

	"github.com/iotus3/simplelife/app/internal/domain/ledger"
)

func TestCatalogsHaveSameKeys(t *testing.T) {
	for id, text := range catalogZh {
		if _, ok := catalogEn[id]; !ok {
			t.Errorf("catalogEn is missing %q", id)
		}
		if text == "" {
			t.Errorf("catalogZh has an empty entry for %q", id)
		}
	}
	for id, text := range catalogEn {
		if _, ok := catalogZh[id]; !ok {
			t.Errorf("catalogZh is missing %q", id)
		}
		if text == "" {
			t.Errorf("catalogEn has an empty entry for %q", id)
		}
	}
}

func TestVerbCountsAgree(t *testing.T) {
	count := func(format string) int {
		n := 0
		for i := 0; i < len(format)-1; i++ {
			if format[i] == '%' && format[i+1] != '%' {
				n++
			}
		}
		return n
	}
	for id, zh := range catalogZh {
		en, ok := catalogEn[id]
		if !ok {
			continue
		}
		if count(zh) != count(en) {
			t.Errorf("%q: zh has %d verbs, en has %d", id, count(zh), count(en))
		}
	}
}

func TestBuiltinKeysExistInBothCatalogs(t *testing.T) {
	for _, key := range ledger.AllBuiltinKeys {
		id := MessageID(key)
		if !Has(ZhCN, id) {
			t.Errorf("catalogZh is missing %q", key)
		}
		if !Has(En, id) {
			t.Errorf("catalogEn is missing %q", key)
		}
	}
}

func TestT(t *testing.T) {
	if got, want := T(ZhCN, MsgNavHome), "今日"; got != want {
		t.Errorf("T(zh, nav.home) = %q, want %q", got, want)
	}
	if got, want := T(En, MsgNavHome), "Today"; got != want {
		t.Errorf("T(en, nav.home) = %q, want %q", got, want)
	}
	if got, want := T(En, MsgTaskOverdueCount, 3), "3 overdue tasks"; got != want {
		t.Errorf("T(plural) = %q, want %q", got, want)
	}
	// An unknown key falls back to another language and then to the key itself,
	// never to an empty label.
	if got, want := T(En, MessageID("nope.nothing")), "nope.nothing"; got != want {
		t.Errorf("T(unknown) = %q, want %q", got, want)
	}
	if ZhCN.Plural(1, MsgTaskCountOne, MsgTaskCount) != MsgTaskCount {
		t.Error("Chinese must always use the plural form")
	}
	if En.Plural(1, MsgTaskCountOne, MsgTaskCount) != MsgTaskCountOne {
		t.Error("English must use the singular for one")
	}
	if En.Plural(0, MsgTaskCountOne, MsgTaskCount) != MsgTaskCount {
		t.Error("English must use the plural for zero")
	}
}

func TestDisplayName(t *testing.T) {
	if got, want := DisplayName(En, "现金", ledger.BuiltinAccountCash), "Cash"; got != want {
		t.Errorf("builtin name = %q, want %q", got, want)
	}
	if got, want := DisplayName(En, "午餐", ""), "午餐"; got != want {
		t.Errorf("user name = %q, want %q", got, want)
	}
}

func TestResolve(t *testing.T) {
	cases := []struct {
		name   string
		pref   string
		system []string
		want   Lang
	}{
		{"explicit zh", PrefZhCN, []string{"en-US"}, ZhCN},
		{"explicit en", PrefEn, []string{"zh-CN"}, En},
		{"auto follows system english", PrefAuto, []string{"en-GB"}, En},
		{"auto matches zh variants", PrefAuto, []string{"zh-Hans-CN"}, ZhCN},
		{"auto skips unknown", PrefAuto, []string{"fr-FR", "en-US"}, En},
		{"auto without system", PrefAuto, nil, ZhCN},
		{"unknown preference falls through", "de-DE", []string{"en-US"}, En},
		{"underscore tag", PrefAuto, []string{"zh_TW"}, ZhCN},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Resolve(c.pref, c.system); got != c.want {
				t.Errorf("Resolve(%q, %v) = %q, want %q", c.pref, c.system, got, c.want)
			}
		})
	}
}

func TestValidPreference(t *testing.T) {
	for _, pref := range []string{PrefAuto, PrefZhCN, PrefEn} {
		if !ValidPreference(pref) {
			t.Errorf("ValidPreference(%q) = false", pref)
		}
	}
	for _, pref := range []string{"", "en-US", "zh", "fr"} {
		if ValidPreference(pref) {
			t.Errorf("ValidPreference(%q) = true", pref)
		}
	}
}

// TestEveryDomainCodeHasMessage scans the domain and application layers for the
// error codes they build and fails when one of them has no sentence: an English
// user must never meet the generic failure for a cause the program knows.
func TestEveryDomainCodeHasMessage(t *testing.T) {
	root := filepath.Join("..", "..")
	var files []string
	// Infrastructure is scanned too: a failure that reaches the user from a file
	// write or a network read is a message like any other.
	for _, dir := range []string{"domain", "application", "infrastructure"} {
		err := filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if !info.IsDir() && strings.HasSuffix(path, ".go") && !strings.HasSuffix(path, "_test.go") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if len(files) == 0 {
		t.Fatal("found no source files to scan")
	}
	builders := map[string]bool{"Invalid": true, "NotFound": true, "Conflict": true, "Unsupported": true}
	codes := map[string]string{}
	fset := token.NewFileSet()
	for _, path := range files {
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) == 0 {
				return true
			}
			// Both shared.Invalid(...) and the package-local Invalid(...) of the
			// shared package itself count.
			name := ""
			switch fun := call.Fun.(type) {
			case *ast.SelectorExpr:
				name = fun.Sel.Name
			case *ast.Ident:
				name = fun.Name
			}
			if !builders[name] {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			code, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Errorf("%s: unquote %s: %v", fset.Position(lit.Pos()), lit.Value, err)
				return true
			}
			codes[code] = path
			return true
		})
	}
	if len(codes) < 50 {
		t.Fatalf("found only %d codes, the scan is probably broken", len(codes))
	}
	for code, path := range codes {
		if _, ok := codeMessages[code]; !ok {
			t.Errorf("no message for code %q (built in %s)", code, path)
		}
	}
	for code := range codeMessages {
		if strings.HasPrefix(code, "ui.") {
			continue // presentation-owned codes have no domain builder
		}
		if _, ok := codes[code]; !ok {
			t.Errorf("codeMessages has %q, which no domain code builds", code)
		}
	}
}

// TestNoHardcodedUserText keeps user-visible text out of the pages: every
// sentence comes from this package, so switching language switches all of it.
func TestNoHardcodedUserText(t *testing.T) {
	root := filepath.Join("..")
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == "i18n" && filepath.Dir(path) == root {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for i, line := range strings.Split(string(data), "\n") {
			for _, r := range line {
				if unicode.Is(unicode.Han, r) {
					t.Errorf("%s:%d has hardcoded text %q; add a message id instead", path, i+1, strings.TrimSpace(line))
					return nil
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}
