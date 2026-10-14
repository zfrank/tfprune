package main

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const lockfile = `
provider "registry.terraform.io/auth0/auth0" {
  version     = "1.53.0"
  constraints = "~> 1.0"
  hashes = [
    "h1:abc123=",
  ]
}

provider "registry.terraform.io/hashicorp/aws" {
  version = "5.60.0"
}
`

// setup writes the lockfile and a providers cache with the given
// "namespace/name/version" entries into a temp dir, returning its path.
func setup(t *testing.T, cached ...string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(lockfile), 0o644); err != nil {
		t.Fatal(err)
	}
	populate(t, filepath.Join(dir, ".terraform", "providers"), cached...)
	return dir
}

// populate creates the given "namespace/name/version" entries in a providers
// directory laid out like .terraform/providers or TF_PLUGIN_CACHE_DIR.
func populate(t *testing.T, providersDir string, cached ...string) {
	t.Helper()
	for _, c := range cached {
		if err := os.MkdirAll(filepath.Join(providersDir, "registry.terraform.io", c, "linux_amd64"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
}

// module creates a module directory in a temp dir whose lockfile locks the
// given "namespace/name/version" entries, returning its path.
func module(t *testing.T, locked ...string) string {
	t.Helper()
	dir := t.TempDir()
	var b strings.Builder
	for _, l := range locked {
		i := strings.LastIndex(l, "/")
		fmt.Fprintf(&b, "provider %q {\n  version = %q\n}\n\n", "registry.terraform.io/"+l[:i], l[i+1:])
	}
	if err := os.WriteFile(filepath.Join(dir, ".terraform.lock.hcl"), []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// remaining returns the version directories still present in a module's
// .terraform/providers, as "namespace/name/version".
func remaining(t *testing.T, dir string) map[string]bool {
	t.Helper()
	return remainingIn(t, filepath.Join(dir, ".terraform", "providers"))
}

// remainingIn returns the version directories still present in a providers
// directory, as "namespace/name/version".
func remainingIn(t *testing.T, providersDir string) map[string]bool {
	t.Helper()
	got := make(map[string]bool)
	root := filepath.Join(providersDir, "registry.terraform.io")
	matches, err := filepath.Glob(filepath.Join(root, "*", "*", "*"))
	if err != nil {
		t.Fatal(err)
	}
	for _, m := range matches {
		rel, _ := filepath.Rel(root, m)
		got[filepath.ToSlash(rel)] = true
	}
	return got
}

func TestParseLockfile(t *testing.T) {
	dir := setup(t)
	locked, err := parseLockfile(filepath.Join(dir, ".terraform.lock.hcl"))
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{
		"registry.terraform.io/auth0/auth0":   "1.53.0",
		"registry.terraform.io/hashicorp/aws": "5.60.0",
	}
	if len(locked) != len(want) {
		t.Fatalf("got %v, want %v", locked, want)
	}
	for source, version := range want {
		if locked[source] != version {
			t.Errorf("locked[%q] = %q, want %q", source, locked[source], version)
		}
	}
}

func TestParseLockfileInvalid(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".terraform.lock.hcl")
	if err := os.WriteFile(path, []byte(`provider "x" {`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := parseLockfile(path); err == nil {
		t.Fatal("expected error for malformed lockfile")
	}
}

func TestCleanProviders(t *testing.T) {
	dir := setup(t,
		"auth0/auth0/1.49.0",
		"auth0/auth0/1.53.0",
		"hashicorp/aws/5.60.0",
		"hashicorp/random/3.6.0",
	)
	locked, err := collectLocked([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanProviders(filepath.Join(dir, ".terraform", "providers"), locked, false); err != nil {
		t.Fatal(err)
	}
	got := remaining(t, dir)
	if len(got) != 2 || !got["auth0/auth0/1.53.0"] || !got["hashicorp/aws/5.60.0"] {
		t.Errorf("remaining = %v, want only locked versions", got)
	}
}

func TestCleanProvidersDryRun(t *testing.T) {
	dir := setup(t, "auth0/auth0/1.49.0", "auth0/auth0/1.53.0")
	locked, err := collectLocked([]string{dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := cleanProviders(filepath.Join(dir, ".terraform", "providers"), locked, true); err != nil {
		t.Fatal(err)
	}
	if got := remaining(t, dir); len(got) != 2 {
		t.Errorf("dry run deleted directories, remaining = %v", got)
	}
}

func TestCleanProvidersWrongDir(t *testing.T) {
	// Pointing at the parent of the cache shifts every level by one, so
	// provider names land where versions are expected and must not be deleted.
	root := t.TempDir()
	cache := filepath.Join(root, "plugin-cache")
	populate(t, cache, "hashicorp/aws/5.60.0", "hashicorp/random/3.6.0")
	locked := map[string]map[string]bool{"registry.terraform.io/hashicorp/aws": {"5.60.0": true}}
	if err := cleanProviders(root, locked, false); err != nil {
		t.Fatal(err)
	}
	if got := remainingIn(t, cache); len(got) != 2 {
		t.Errorf("remaining = %v, want cache untouched", got)
	}
}

func TestCollectLocked(t *testing.T) {
	a := module(t, "auth0/auth0/1.53.0", "hashicorp/aws/5.60.0")
	b := module(t, "auth0/auth0/1.49.0", "hashicorp/aws/5.60.0")
	locked, err := collectLocked([]string{a, b})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{
		"registry.terraform.io/auth0/auth0":   {"1.49.0", "1.53.0"},
		"registry.terraform.io/hashicorp/aws": {"5.60.0"},
	}
	if len(locked) != len(want) {
		t.Fatalf("got %v, want %v", locked, want)
	}
	for source, versions := range want {
		if len(locked[source]) != len(versions) {
			t.Errorf("locked[%q] = %v, want %v", source, locked[source], versions)
		}
		for _, v := range versions {
			if !locked[source][v] {
				t.Errorf("locked[%q] missing %q", source, v)
			}
		}
	}
}

func TestRunSingleDir(t *testing.T) {
	dir := setup(t, "auth0/auth0/1.49.0", "auth0/auth0/1.53.0", "hashicorp/aws/5.60.0")
	if err := run([]string{dir}, "", false); err != nil {
		t.Fatal(err)
	}
	got := remaining(t, dir)
	if len(got) != 2 || !got["auth0/auth0/1.53.0"] || !got["hashicorp/aws/5.60.0"] {
		t.Errorf("remaining = %v, want only locked versions", got)
	}
}

func TestRunSharedCache(t *testing.T) {
	a := module(t, "auth0/auth0/1.53.0", "hashicorp/aws/5.60.0")
	b := module(t, "auth0/auth0/1.49.0")
	c := module(t, "hashicorp/random/3.6.0")
	// The modules' own .terraform/providers are not touched in cache mode.
	populate(t, filepath.Join(a, ".terraform", "providers"), "hashicorp/aws/5.50.0")
	cache := t.TempDir()
	populate(t, cache,
		"auth0/auth0/1.40.0",
		"auth0/auth0/1.49.0",
		"auth0/auth0/1.53.0",
		"hashicorp/aws/5.50.0",
		"hashicorp/aws/5.60.0",
		"hashicorp/random/3.6.0",
		"hashicorp/null/3.2.0",
	)
	if err := run([]string{a, b, c}, cache, false); err != nil {
		t.Fatal(err)
	}
	got := remainingIn(t, cache)
	want := []string{"auth0/auth0/1.49.0", "auth0/auth0/1.53.0", "hashicorp/aws/5.60.0", "hashicorp/random/3.6.0"}
	if len(got) != len(want) {
		t.Errorf("remaining = %v, want %v", got, want)
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("remaining = %v, missing %q", got, w)
		}
	}
	if got := remaining(t, a); !got["hashicorp/aws/5.50.0"] {
		t.Errorf("module providers dir was pruned in cache mode, remaining = %v", got)
	}
}

func TestRunSharedCacheDryRun(t *testing.T) {
	a := module(t, "auth0/auth0/1.53.0")
	b := module(t, "auth0/auth0/1.49.0")
	cache := t.TempDir()
	populate(t, cache, "auth0/auth0/1.40.0", "auth0/auth0/1.49.0", "auth0/auth0/1.53.0")
	if err := run([]string{a, b}, cache, true); err != nil {
		t.Fatal(err)
	}
	if got := remainingIn(t, cache); len(got) != 3 {
		t.Errorf("dry run deleted directories, remaining = %v", got)
	}
}

func TestRunSharedCacheMissingLockfile(t *testing.T) {
	a := module(t, "auth0/auth0/1.53.0")
	missing := t.TempDir()
	cache := t.TempDir()
	populate(t, cache, "auth0/auth0/1.49.0", "auth0/auth0/1.53.0")
	if err := run([]string{a, missing}, cache, false); err == nil {
		t.Fatal("expected error for module without lockfile")
	}
	if got := remainingIn(t, cache); len(got) != 2 {
		t.Errorf("cache pruned despite unreadable lockfile, remaining = %v", got)
	}
}

func TestRunMultipleDirsWithoutCache(t *testing.T) {
	a := setup(t, "auth0/auth0/1.49.0")
	b := setup(t, "auth0/auth0/1.49.0")
	if err := run([]string{a, b}, "", false); err == nil {
		t.Fatal("expected error for multiple dirs without -cache-dir")
	}
	if got := remaining(t, a); len(got) != 1 {
		t.Errorf("remaining = %v, want untouched", got)
	}
}

func TestRunFlagAfterDirs(t *testing.T) {
	a := module(t, "auth0/auth0/1.53.0")
	cache := t.TempDir()
	populate(t, cache, "auth0/auth0/1.49.0")
	if err := run([]string{a, "-dry-run"}, cache, false); err == nil {
		t.Fatal("expected error for flag after module directories")
	}
	if got := remainingIn(t, cache); len(got) != 1 {
		t.Errorf("remaining = %v, want untouched", got)
	}
}
