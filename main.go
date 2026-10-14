package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclparse"
)

// parseLockfile returns a map of provider source address to locked version,
// e.g. "registry.terraform.io/auth0/auth0" -> "1.53.0".
func parseLockfile(path string) (map[string]string, error) {
	file, diags := hclparse.NewParser().ParseHCLFile(path)
	if diags.HasErrors() {
		return nil, diags
	}

	content, diags := file.Body.Content(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{
			{Type: "provider", LabelNames: []string{"source"}},
		},
	})
	if diags.HasErrors() {
		return nil, diags
	}

	locked := make(map[string]string)
	for _, block := range content.Blocks {
		blockContent, _, diags := block.Body.PartialContent(&hcl.BodySchema{
			Attributes: []hcl.AttributeSchema{
				{Name: "version", Required: true},
			},
		})
		if diags.HasErrors() {
			return nil, diags
		}
		version, diags := blockContent.Attributes["version"].Expr.Value(nil)
		if diags.HasErrors() {
			return nil, diags
		}
		locked[block.Labels[0]] = version.AsString()
	}
	return locked, nil
}

// collectLocked parses the lockfile of every module directory and returns a map
// of provider source address to the set of versions locked by at least one of
// them, e.g. "registry.terraform.io/auth0/auth0" -> {"1.49.0", "1.53.0"}.
func collectLocked(dirs []string) (map[string]map[string]bool, error) {
	locked := make(map[string]map[string]bool)
	for _, dir := range dirs {
		versions, err := parseLockfile(filepath.Join(dir, ".terraform.lock.hcl"))
		if err != nil {
			return nil, err
		}
		for source, version := range versions {
			if locked[source] == nil {
				locked[source] = make(map[string]bool)
			}
			locked[source][version] = true
		}
	}
	return locked, nil
}

// versionPattern matches provider version directory names such as "1.53.0" or
// "5.0.0-beta1". Anything else is left alone, so pointing -cache-dir at the
// wrong directory doesn't delete unrelated files.
var versionPattern = regexp.MustCompile(`^\d+\.\d+\.\d+([-+].*)?$`)

// cleanProviders walks <providersDir>/<registry>/<namespace>/<name>/<version>
// and removes every version directory not present in locked. providersDir is
// either a module's .terraform/providers or a shared TF_PLUGIN_CACHE_DIR, which
// use the same layout.
func cleanProviders(providersDir string, locked map[string]map[string]bool, dryRun bool) error {
	registries, err := os.ReadDir(providersDir)
	if err != nil {
		return err
	}
	for _, registry := range registries {
		if !registry.IsDir() {
			continue
		}
		namespaces, err := os.ReadDir(filepath.Join(providersDir, registry.Name()))
		if err != nil {
			return err
		}
		for _, namespace := range namespaces {
			if !namespace.IsDir() {
				continue
			}
			names, err := os.ReadDir(filepath.Join(providersDir, registry.Name(), namespace.Name()))
			if err != nil {
				return err
			}
			for _, name := range names {
				if !name.IsDir() {
					continue
				}
				source := registry.Name() + "/" + namespace.Name() + "/" + name.Name()
				versions, err := os.ReadDir(filepath.Join(providersDir, registry.Name(), namespace.Name(), name.Name()))
				if err != nil {
					return err
				}
				for _, version := range versions {
					if !version.IsDir() || !versionPattern.MatchString(version.Name()) {
						continue
					}
					if locked[source][version.Name()] {
						fmt.Printf("keeping  %s %s\n", source, version.Name())
						continue
					}
					dir := filepath.Join(providersDir, registry.Name(), namespace.Name(), name.Name(), version.Name())
					if dryRun {
						fmt.Printf("would delete %s %s (%s)\n", source, version.Name(), dir)
						continue
					}
					fmt.Printf("deleting %s %s\n", source, version.Name())
					if err := os.RemoveAll(dir); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// run prunes provider versions not locked by any of the module dirs. Without
// cacheDir, dirs must hold a single module and its own .terraform/providers is
// pruned. With cacheDir, the shared plugin cache is pruned instead.
func run(dirs []string, cacheDir string, dryRun bool) error {
	for _, dir := range dirs {
		// The flag package stops at the first positional argument, so a
		// trailing "-dry-run" would otherwise be silently taken as a directory.
		if strings.HasPrefix(dir, "-") {
			return fmt.Errorf("flags must come before module directories, got %q", dir)
		}
	}
	if cacheDir == "" && len(dirs) > 1 {
		return errors.New("multiple module directories require -cache-dir")
	}

	// Every lockfile is read before anything is deleted: skipping an
	// unreadable one would delete the providers that module depends on.
	locked, err := collectLocked(dirs)
	if err != nil {
		return fmt.Errorf("error reading lockfile: %w", err)
	}
	if len(locked) == 0 {
		fmt.Fprintln(os.Stderr, "no providers found in lockfile, nothing to do")
		return nil
	}

	providersDir := cacheDir
	if providersDir == "" {
		providersDir = filepath.Join(dirs[0], ".terraform", "providers")
	}
	if err := cleanProviders(providersDir, locked, dryRun); err != nil {
		return fmt.Errorf("error cleaning providers: %w", err)
	}
	return nil
}

func main() {
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "usage: %s [flags] [module-dir ...]\n", os.Args[0])
		flag.PrintDefaults()
	}
	dir := flag.String("dir", ".", "terraform working directory (contains .terraform.lock.hcl and .terraform/)")
	cacheDir := flag.String("cache-dir", "", "shared plugin cache directory (TF_PLUGIN_CACHE_DIR) to prune using the lockfiles of all module dirs")
	dryRun := flag.Bool("dry-run", false, "only print what would be deleted")
	flag.Parse()

	// Module directories come from positional arguments and/or -dir, which
	// defaults to the current directory when neither is given.
	dirs := flag.Args()
	dirSet := false
	flag.Visit(func(f *flag.Flag) { dirSet = dirSet || f.Name == "dir" })
	if dirSet || len(dirs) == 0 {
		dirs = append([]string{*dir}, dirs...)
	}

	if err := run(dirs, *cacheDir, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
