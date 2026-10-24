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
	never  []string
	reason string
}

var rules = []rule{
	{"internal/build/", []string{"internal/site", "internal/driver"}, "build depends on pkg/driver, never on the site or on a driver"},
	{"internal/site/", []string{"internal/build", "internal/driver"}, "the site depends on pkg/driver, never on the build or on a driver"},
	{"internal/driver/", []string{"internal/build", "internal/site"}, "a driver knows pkg/, oci and report, never a side"},
	{"pkg/", []string{"internal/build", "internal/site", "internal/driver"}, "a public contract never names an implementation"},
}

func TestImportsKeepTheTwoSidesApart(t *testing.T) {
	for file, imports := range sources(t) {
		for _, rule := range rules {
			if !strings.HasPrefix(file, rule.from) {
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
		return nil
	})
	if err != nil {
		t.Fatalf("read the sources: %v", err)
	}
	return files
}
