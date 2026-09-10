package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestScanClassifiesExportedDeclarations(t *testing.T) {
	dir := t.TempDir()
	source := `package engine

type ExportedType struct{}
type hiddenType struct{}
const ExportedConst = 1
const hiddenConst = 2
var ExportedVar = 3
var hiddenVar = 4
func ExportedFunc() {}
func hiddenFunc() {}
func (ExportedType) ExportedMethod() {}
`
	if err := os.WriteFile(filepath.Join(dir, "engine.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	// Test files must not affect the generated API.
	if err := os.WriteFile(filepath.Join(dir, "ignored_test.go"), []byte("package engine\nfunc LeakedFromTest() {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.types, []string{"ExportedType"}) {
		t.Fatalf("types = %#v", got.types)
	}
	if !reflect.DeepEqual(got.consts, []string{"ExportedConst"}) {
		t.Fatalf("consts = %#v", got.consts)
	}
	if !reflect.DeepEqual(got.vars, []string{"ExportedVar"}) {
		t.Fatalf("vars = %#v", got.vars)
	}
	if !reflect.DeepEqual(got.funcs, []string{"ExportedFunc"}) {
		t.Fatalf("funcs = %#v", got.funcs)
	}
}

func TestScanRejectsMissingEnginePackage(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "other.go"), []byte("package other\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := scan(dir); err == nil {
		t.Fatal("expected missing engine package error")
	}
}

func TestFindModuleRootWalksParents(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/gen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	nested := filepath.Join(root, "a", "b", "c")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(nested); err != nil {
		t.Fatal(err)
	}
	got, err := findModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	if got != root {
		t.Fatalf("root = %q, want %q", got, root)
	}
}

func TestMainGeneratesFacade(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.test/gen\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	engineDir := filepath.Join(root, "internal", "engine")
	if err := os.MkdirAll(engineDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := "package engine\ntype Widget struct{}\nconst Ready = 1\nfunc NewWidget() *Widget { return &Widget{} }\n"
	if err := os.WriteFile(filepath.Join(engineDir, "engine.go"), []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}

	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chdir(old) }()
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	main()
	data, err := os.ReadFile(filepath.Join(root, "api.go"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	for _, want := range []string{"type (\n\tWidget = engine.Widget", "Ready = engine.Ready", "NewWidget = engine.NewWidget"} {
		if !strings.Contains(text, want) {
			t.Fatalf("generated facade missing %q:\n%s", want, text)
		}
	}
}
