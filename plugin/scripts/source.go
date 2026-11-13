package main

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// module is one go.mod found in the tree.
type module struct {
	Path   string // module path from the module line
	Dir    string // directory relative to the tree root, "." for the root
	Nested bool   // another module's directory contains this one
}

// testFunc is one test that go test runs: a Test or Fuzz function, or an
// Example with an output comment.
type testFunc struct {
	Name       string
	Package    string // import path
	File       string // relative to the tree root, slash separated
	Line       int
	Constraint string // the file's //go:build expression, if any
	FileOS     string // GOOS or GOARCH limit from the file name, if any
	CallsRun   bool   // the body calls t.Run on its own *testing.T
	Module     *module
}

// tree is everything neverran found in the source directory.
type tree struct {
	Root     string
	Modules  []*module
	Tests    []*testFunc
	Packages map[string]*module // import path of each package with tests
	Outside  int                // test files under no go.mod
}

// loadTree walks dir the way the go command does: it skips testdata, vendor,
// and directories whose names start with "." or "_", and treats each go.mod
// as the root of its own module.
func loadTree(dir string) (*tree, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}
	t := &tree{Root: dir, Packages: map[string]*module{}}
	mods := map[string]*module{} // by relative dir
	type testFile struct{ rel, abs string }
	var files []testFile
	err = filepath.WalkDir(dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		rel = filepath.ToSlash(rel)
		name := d.Name()
		if d.IsDir() {
			if rel != "." && (strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") || name == "testdata" || name == "vendor") {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			return nil
		}
		if name == "go.mod" {
			path, err := readModulePath(p)
			if err != nil {
				return fmt.Errorf("%s: %v", filepath.Join(dir, filepath.FromSlash(rel)), err)
			}
			mods[pathDir(rel)] = &module{Path: path, Dir: pathDir(rel)}
		}
		if strings.HasSuffix(name, "_test.go") && d.Type().IsRegular() {
			files = append(files, testFile{rel, p})
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, m := range mods {
		t.Modules = append(t.Modules, m)
		for d := parentDir(m.Dir); d != ""; d = parentDir(d) {
			if _, ok := mods[d]; ok {
				m.Nested = true
				break
			}
		}
	}
	sort.Slice(t.Modules, func(i, j int) bool { return t.Modules[i].Dir < t.Modules[j].Dir })
	if len(t.Modules) == 0 {
		return nil, fmt.Errorf("no go.mod in %s or below it", dir)
	}
	fset := token.NewFileSet()
	for _, f := range files {
		m, pkgDir := owner(mods, pathDir(f.rel))
		if m == nil {
			t.Outside++
			continue
		}
		importPath := m.Path
		if pkgDir != "" {
			importPath += "/" + pkgDir
		}
		tests, err := readTestFile(fset, f.abs, f.rel)
		if err != nil {
			return nil, err
		}
		for _, tf := range tests {
			tf.Package = importPath
			tf.Module = m
			t.Tests = append(t.Tests, tf)
			t.Packages[importPath] = m
		}
	}
	sort.Slice(t.Tests, func(i, j int) bool {
		a, b := t.Tests[i], t.Tests[j]
		if a.Package != b.Package {
			return a.Package < b.Package
		}
		if a.File != b.File {
			return a.File < b.File
		}
		return a.Line < b.Line
	})
	return t, nil
}

// pathDir returns the slash-separated directory of a relative file path, "."
// for the root.
func pathDir(rel string) string {
	i := strings.LastIndex(rel, "/")
	if i < 0 {
		return "."
	}
	return rel[:i]
}

func parentDir(d string) string {
	if d == "." {
		return ""
	}
	return pathDir(d)
}

// owner finds the module whose directory contains dir, and dir's path inside it.
func owner(mods map[string]*module, dir string) (*module, string) {
	for d := dir; d != ""; d = parentDir(d) {
		if m, ok := mods[d]; ok {
			if d == "." {
				if dir == "." {
					return m, ""
				}
				return m, dir
			}
			return m, strings.TrimPrefix(strings.TrimPrefix(dir, d), "/")
		}
	}
	return nil, ""
}

var goModVerbs = map[string]bool{
	"module": true, "go": true, "toolchain": true, "godebug": true, "require": true,
	"replace": true, "exclude": true, "retract": true, "tool": true, "ignore": true,
}

// readModulePath reads the module path from a go.mod file, and refuses a file
// that isn't a whole go.mod: unknown directives, an unclosed block, or no
// module line.
func readModulePath(p string) (string, error) {
	data, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	if !utf8.Valid(data) {
		return "", fmt.Errorf("not a go.mod file: it isn't UTF-8 text")
	}
	path := ""
	inBlock := false
	for i, line := range strings.Split(string(data), "\n") {
		if j := strings.Index(line, "//"); j >= 0 {
			line = line[:j]
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if inBlock {
			if line == ")" {
				inBlock = false
			}
			continue
		}
		fields := strings.Fields(line)
		if !goModVerbs[fields[0]] {
			return "", fmt.Errorf("line %d: %q is not a go.mod directive; this doesn't look like a go.mod file", i+1, clip(line, 40))
		}
		if len(fields) == 1 {
			return "", fmt.Errorf("line %d: %s has no value", i+1, fields[0])
		}
		if fields[1] == "(" && len(fields) == 2 {
			inBlock = true
			continue
		}
		if fields[0] == "module" {
			if path != "" {
				return "", fmt.Errorf("line %d: a second module line", i+1)
			}
			v := fields[1]
			if uq, err := strconv.Unquote(v); err == nil {
				v = uq
			}
			if len(fields) != 2 || !validModulePath(v) {
				return "", fmt.Errorf("line %d: %q is not a module path", i+1, clip(line, 60))
			}
			path = v
		}
	}
	if inBlock {
		return "", fmt.Errorf("a ( block is never closed; the file looks cut off")
	}
	if path == "" {
		return "", fmt.Errorf("no module line")
	}
	return path, nil
}

var modulePathRx = regexp.MustCompile(`^[A-Za-z0-9._~+\-/]+$`)

func validModulePath(p string) bool {
	return modulePathRx.MatchString(p) && !strings.HasPrefix(p, "/") && !strings.HasSuffix(p, "/") && !strings.Contains(p, "//")
}

var outputRx = regexp.MustCompile(`(?i)^[[:space:]]*(unordered )?output:`)

// readTestFile parses one _test.go file and returns the tests go test would
// run from it. A file that doesn't parse is an error: its package can't have
// built, so no run of it can be trusted.
func readTestFile(fset *token.FileSet, abs, rel string) ([]*testFunc, error) {
	src, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	f, err := parser.ParseFile(fset, abs, src, parser.ParseComments)
	if err != nil {
		return nil, fmt.Errorf("can't parse %s: %v", rel, firstLine(err.Error()))
	}
	expr := buildConstraint(f)
	fileOS := fileNameLimit(filepath.Base(rel))
	var out []*testFunc
	for _, d := range f.Decls {
		fn, ok := d.(*ast.FuncDecl)
		if !ok || fn.Recv != nil || fn.Body == nil || fn.Type.TypeParams != nil {
			continue
		}
		name := fn.Name.Name
		run := false
		param := ""
		switch {
		case isTestName(name, "Test"):
			param, run = testingParam(fn.Type, "T")
		case isTestName(name, "Fuzz"):
			param, run = testingParam(fn.Type, "F")
		case isTestName(name, "Example"):
			run = len(fn.Type.Params.List) == 0 && fn.Type.Results == nil && hasOutputComment(fn.Body, f.Comments)
		}
		if !run || name == "TestMain" {
			continue
		}
		tf := &testFunc{
			Name:       name,
			File:       rel,
			Line:       fset.Position(fn.Pos()).Line,
			Constraint: expr,
			FileOS:     fileOS,
		}
		if strings.HasPrefix(name, "Test") && param != "" && param != "_" {
			tf.CallsRun = callsRun(fn.Body, param)
		}
		out = append(out, tf)
	}
	return out, nil
}

// isTestName follows the go command's rule: the prefix alone, or the prefix
// followed by a character that isn't a lower-case letter.
func isTestName(name, prefix string) bool {
	if !strings.HasPrefix(name, prefix) {
		return false
	}
	if len(name) == len(prefix) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(name[len(prefix):])
	return !unicode.IsLower(r)
}

// testingParam reports whether the function takes exactly one *testing.T (or
// *testing.F) and nothing else, and returns the parameter's name.
func testingParam(ft *ast.FuncType, typ string) (string, bool) {
	if ft.Results != nil && len(ft.Results.List) > 0 {
		return "", false
	}
	if ft.Params == nil || len(ft.Params.List) != 1 || len(ft.Params.List[0].Names) > 1 {
		return "", false
	}
	field := ft.Params.List[0]
	star, ok := field.Type.(*ast.StarExpr)
	if !ok {
		return "", false
	}
	switch x := star.X.(type) {
	case *ast.SelectorExpr:
		if x.Sel.Name != typ {
			return "", false
		}
	case *ast.Ident:
		if x.Name != typ {
			return "", false
		}
	default:
		return "", false
	}
	if len(field.Names) == 0 {
		return "", true
	}
	return field.Names[0].Name, true
}

// hasOutputComment follows go/doc: an example runs only when the last comment
// in its body starts with "Output:" or "Unordered output:".
func hasOutputComment(body *ast.BlockStmt, comments []*ast.CommentGroup) bool {
	var last *ast.CommentGroup
	for _, cg := range comments {
		if cg.Pos() > body.Lbrace && cg.End() < body.Rbrace {
			last = cg
		}
	}
	return last != nil && outputRx.MatchString(last.Text())
}

// callsRun reports whether body calls param.Run anywhere, as a table test does.
func callsRun(body *ast.BlockStmt, param string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		if found {
			return false
		}
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != "Run" {
			return true
		}
		if id, ok := sel.X.(*ast.Ident); ok && id.Name == param {
			found = true
		}
		return true
	})
	return found
}

// buildConstraint returns the file's //go:build expression, or its // +build
// lines joined, or "".
func buildConstraint(f *ast.File) string {
	var plus []string
	for _, cg := range f.Comments {
		if cg.Pos() >= f.Package {
			break
		}
		for _, c := range cg.List {
			if constraint.IsGoBuild(c.Text) {
				if x, err := constraint.Parse(c.Text); err == nil {
					return x.String()
				}
				return strings.TrimSpace(strings.TrimPrefix(c.Text, "//go:build"))
			}
			if constraint.IsPlusBuild(c.Text) {
				plus = append(plus, strings.TrimSpace(strings.TrimPrefix(c.Text, "// +build")))
			}
		}
	}
	return strings.Join(plus, " && ")
}

var knownOS = strings.Fields("aix android darwin dragonfly freebsd hurd illumos ios js linux nacl netbsd openbsd plan9 solaris wasip1 windows zos")
var knownArch = strings.Fields("386 amd64 amd64p32 arm armbe arm64 arm64be loong64 mips mipsle mips64 mips64le mips64p32 mips64p32le ppc ppc64 ppc64le riscv riscv64 s390 s390x sparc sparc64 wasm")

// fileNameLimit returns the GOOS and GOARCH a file name like x_windows_test.go
// or x_linux_arm64_test.go limits the file to.
func fileNameLimit(name string) string {
	parts := strings.Split(strings.TrimSuffix(name, "_test.go"), "_")
	if len(parts) < 2 {
		return ""
	}
	isOS := func(s string) bool { return contains(knownOS, s) }
	isArch := func(s string) bool { return contains(knownArch, s) }
	n := len(parts)
	if n >= 3 && isOS(parts[n-2]) && isArch(parts[n-1]) {
		return parts[n-2] + "/" + parts[n-1]
	}
	if isOS(parts[n-1]) || isArch(parts[n-1]) {
		return parts[n-1]
	}
	return ""
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
