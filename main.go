package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

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

// cleanProviders walks .terraform/providers/<registry>/<namespace>/<name>/<version>
// and removes every version directory not present in the lockfile.
func cleanProviders(providersDir string, locked map[string]string, dryRun bool) error {
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
					if !version.IsDir() {
						continue
					}
					if locked[source] == version.Name() {
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

func main() {
	dir := flag.String("dir", ".", "terraform working directory (contains .terraform.lock.hcl and .terraform/)")
	dryRun := flag.Bool("dry-run", false, "only print what would be deleted")
	flag.Parse()

	locked, err := parseLockfile(filepath.Join(*dir, ".terraform.lock.hcl"))
	if err != nil {
		fmt.Fprintln(os.Stderr, "error reading lockfile:", err)
		os.Exit(1)
	}
	if len(locked) == 0 {
		fmt.Fprintln(os.Stderr, "no providers found in lockfile, nothing to do")
		return
	}

	providersDir := filepath.Join(*dir, ".terraform", "providers")
	if err := cleanProviders(providersDir, locked, *dryRun); err != nil {
		fmt.Fprintln(os.Stderr, "error cleaning providers:", err)
		os.Exit(1)
	}
}
