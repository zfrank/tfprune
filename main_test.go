package main

import (
	"os"
	"path/filepath"
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
	for _, c := range cached {
		if err := os.MkdirAll(filepath.Join(dir, ".terraform", "providers", "registry.terraform.io", c, "linux_amd64"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// remaining returns the version directories still present, as "namespace/name/version".
func remaining(t *testing.T, dir string) map[string]bool {
	t.Helper()
	got := make(map[string]bool)
	root := filepath.Join(dir, ".terraform", "providers", "registry.terraform.io")
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
	locked, err := parseLockfile(filepath.Join(dir, ".terraform.lock.hcl"))
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
	locked, err := parseLockfile(filepath.Join(dir, ".terraform.lock.hcl"))
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
