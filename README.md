# tfprune

Prunes unused provider versions from Terraform's local cache.

When you upgrade providers, Terraform downloads the new version into
`.terraform/providers/` but leaves the old ones behind. Over time these stale
copies add up (a single AWS provider version is several hundred MB). `tfprune`
reads `.terraform.lock.hcl` to find the versions actually in use and deletes
every other cached version.

## Install

```sh
go install github.com/zfrank/tfprune@latest
```

Or download a prebuilt binary from the
[releases page](https://github.com/zfrank/tfprune/releases).

## Usage

Run it in a Terraform working directory (or point it at one with `-dir`).
Use `-dry-run` first to see what it would remove:

```sh
$ tfprune -dry-run
would delete registry.terraform.io/auth0/auth0 1.49.0 (.terraform/providers/registry.terraform.io/auth0/auth0/1.49.0)
keeping  registry.terraform.io/auth0/auth0 1.53.0
keeping  registry.terraform.io/hashicorp/aws 5.60.0

$ tfprune
deleting registry.terraform.io/auth0/auth0 1.49.0
keeping  registry.terraform.io/auth0/auth0 1.53.0
keeping  registry.terraform.io/hashicorp/aws 5.60.0
```

### Shared plugin cache

If several modules share a provider cache through
[`TF_PLUGIN_CACHE_DIR`](https://developer.hashicorp.com/terraform/cli/config/config-file#provider-plugin-cache),
pass the cache with `-cache-dir` and list every module that uses it. `tfprune`
reads all of their lockfiles and only deletes versions that no module locks:

```sh
$ tfprune -cache-dir "$TF_PLUGIN_CACHE_DIR" -dry-run envs/prod envs/staging
would delete registry.terraform.io/hashicorp/aws 5.40.0 (/home/me/.terraform.d/plugin-cache/registry.terraform.io/hashicorp/aws/5.40.0)
keeping  registry.terraform.io/hashicorp/aws 5.50.0
keeping  registry.terraform.io/hashicorp/aws 5.60.0
```

Any version locked only by a module you leave off the list is deleted, so
make sure to list them all. Shell globs work (`tfprune -cache-dir ... envs/*/`).
Flags must come before the module directories. If any lockfile can't be read,
nothing is deleted. In this mode only the shared cache is pruned; the modules'
own `.terraform/providers` directories are left alone.

`-cache-dir` is never read from `TF_PLUGIN_CACHE_DIR` implicitly: pruning a
shared cache based on a single module would delete every other module's
providers.

### Flags

| Flag         | Default | Description                                                              |
|--------------|---------|--------------------------------------------------------------------------|
| `-dir`       | `.`     | Terraform working directory (contains `.terraform.lock.hcl`)             |
| `-cache-dir` |         | Shared plugin cache to prune, using the lockfiles of all module dirs     |
| `-dry-run`   | `false` | Only print what would be deleted                                         |

Module directories can also be given as positional arguments. Without
`-cache-dir`, only one module directory is allowed.

Anything deleted by mistake is re-downloaded by the next `terraform init`, so
the worst case is a slower init.

## License

[MIT](LICENSE)
