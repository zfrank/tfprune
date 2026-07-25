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

### Flags

| Flag       | Default | Description                                                        |
|------------|---------|--------------------------------------------------------------------|
| `-dir`     | `.`     | Terraform working directory (contains `.terraform.lock.hcl`)       |
| `-dry-run` | `false` | Only print what would be deleted                                   |

Anything deleted by mistake is re-downloaded by the next `terraform init`, so
the worst case is a slower init.

## License

[MIT](LICENSE)
