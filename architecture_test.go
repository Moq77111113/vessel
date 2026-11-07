package vessel_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

const module = "github.com/Moq77111113/vessel/"

type rule struct {
	from   string
	except []string
	never  []string
	reason string
}

var rules = []rule{
	{"internal/build/", nil, []string{"internal/site"}, "build depends on pkg/driver, never on the site"},
	{"internal/site/", nil, []string{"internal/build"}, "the site depends on pkg/driver, never on the build"},
	{"internal/driver/", nil, []string{"internal/build", "internal/site"}, "a driver knows pkg/, oci and report, never a side"},
	{"pkg/", nil, []string{"internal/build", "internal/site"}, "a public contract never names an implementation"},
	{"", []string{"cmd/vessel/root.go", "internal/driver/"}, []string{"internal/driver"}, "only cmd/vessel/root.go names a driver"},
}

func TestImportsKeepTheTwoSidesApart(t *testing.T) {
	for file, imports := range sources(t) {
		for _, rule := range rules {
			if !rule.covers(file) {
				continue
			}
			for _, path := range imports {
				for _, never := range rule.never {
					if path == never || strings.HasPrefix(path, never+"/") {
						t.Errorf("%s imports %s: %s", file, path, rule.reason)
					}
				}
			}
		}
	}
}

func (r rule) covers(file string) bool {
	if !strings.HasPrefix(file, r.from) {
		return false
	}
	for _, skip := range r.except {
		if strings.HasPrefix(file, skip) {
			return false
		}
	}
	return true
}

func sources(t *testing.T) map[string][]string {
	t.Helper()
	files := map[string][]string{}
	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() && (entry.Name() == "testdata" || strings.HasPrefix(entry.Name(), ".")) && path != "." {
			return filepath.SkipDir
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		parsed, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, spec := range parsed.Imports {
			name, err := strconv.Unquote(spec.Path.Value)
			if err != nil {
				return err
			}
			if local, ok := strings.CutPrefix(name, module); ok {
				files[filepath.ToSlash(path)] = append(files[filepath.ToSlash(path)], local)
			}
		}
		if _, ok := files[filepath.ToSlash(path)]; !ok {
			files[filepath.ToSlash(path)] = nil
		}
		return nil
	})
	if err != nil {
		t.Fatalf("read the sources: %v", err)
	}
	return files
}
