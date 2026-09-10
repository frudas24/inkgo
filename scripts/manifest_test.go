package scripts

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestInventory(t *testing.T) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash is required for manifest scripts")
	}
	for _, checkout := range []bool{false, true} {
		name := "archive"
		if checkout {
			name = "checkout"
		}
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			if err := os.Mkdir(filepath.Join(dir, "scripts"), 0755); err != nil {
				t.Fatal(err)
			}
			write := func(name, content string) {
				t.Helper()
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
					t.Fatal(err)
				}
			}
			for _, name := range []string{"manifest-common.sh", "update-manifest.sh", "check-manifest.sh"} {
				data, err := os.ReadFile(name)
				if err != nil {
					t.Fatal(err)
				}
				write("scripts/"+name, string(data))
			}
			run := func(script string, wantOK bool) {
				t.Helper()
				cmd := exec.Command(bash, "scripts/"+script)
				cmd.Dir = dir
				out, err := cmd.CombinedOutput()
				if (err == nil) != wantOK {
					t.Fatalf("%s: error=%v output=%s", script, err, out)
				}
			}
			if checkout {
				cmd := exec.Command("git", "init", "-q")
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git init: %v %s", err, out)
				}
				write(".gitignore", "ignored.out\n")
				write("ignored.out", "ignore me")
			}
			write("source.go", "package source\n")
			if checkout {
				cmd := exec.Command("git", "add", "source.go")
				cmd.Dir = dir
				if out, err := cmd.CombinedOutput(); err != nil {
					t.Fatalf("git add: %v %s", err, out)
				}
			}
			if err := os.Mkdir(filepath.Join(dir, ".agent"), 0755); err != nil {
				t.Fatal(err)
			}
			write(".agent/workspace_map_state.json", "{}")
			write("coverage.out", "generated")
			write("cpu.prof", "generated")
			run("update-manifest.sh", true)
			run("check-manifest.sh", true)
			original, err := os.ReadFile(filepath.Join(dir, "MANIFEST.sha256"))
			if err != nil {
				t.Fatal(err)
			}
			manifest := string(original)
			if !strings.Contains(manifest, "./source.go") {
				t.Fatal("new source omitted by generator")
			}
			for _, excluded := range []string{"coverage.out", "cpu.prof", "ignored.out", ".git/", ".agent/"} {
				if strings.Contains(manifest, excluded) {
					t.Fatalf("manifest includes excluded file: %s", excluded)
				}
			}
			lines := strings.Split(strings.TrimSpace(manifest), "\n")
			for name, content := range map[string]string{
				"empty":             "",
				"omitted":           strings.Join(lines[1:], "\n") + "\n",
				"duplicate":         manifest + lines[0] + "\n",
				"malformed":         "bad entry\n",
				"unterminated junk": manifest + "junk",
			} {
				t.Log("reject", name)
				write("MANIFEST.sha256", content)
				run("check-manifest.sh", false)
			}
			write("MANIFEST.sha256", manifest)
			write("source.go", "changed")
			run("check-manifest.sh", false)
			run("update-manifest.sh", true)
			run("check-manifest.sh", true)
			write("new.go", "package source\n")
			run("check-manifest.sh", false)
			run("update-manifest.sh", true)
			run("check-manifest.sh", true)
		})
	}
}
