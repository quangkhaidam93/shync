# Update Strategy

## Updating shync

### Homebrew

```sh
brew upgrade shync
```

### Self-update

```sh
shync update
```

### Re-run install script

```sh
curl -fsSL https://raw.githubusercontent.com/quangkhaidam93/shync/master/install.sh | sh
```

## Versioning

shync follows [Semantic Versioning](https://semver.org/). Versions are managed via git tags and automated releases.

### Release Workflow

1. Tag the release:
   ```sh
   git tag v0.1.0
   git push origin v0.1.0
   ```
2. GitHub Actions runs [GoReleaser](https://goreleaser.com/) which:
   - Cross-compiles for Linux and macOS (amd64 + arm64)
   - Embeds the version, commit hash, and build date into the binary
   - Creates a GitHub release with archives and checksums
   - Updates the Homebrew formula in [quangkhaidam93/homebrew-tap](https://github.com/quangkhaidam93/homebrew-tap)

### Build from source with version info

```sh
go build -ldflags "-X github.com/quangkhaidam93/shync/cmd.Version=v0.1.0" -o shync .
```

## Setup for Maintainers

To enable automated Homebrew formula updates:

1. Create the [quangkhaidam93/homebrew-tap](https://github.com/quangkhaidam93/homebrew-tap) repo (public, with a `Formula/` directory)
2. Create a Personal Access Token with write access to that repo
3. Add it as `HOMEBREW_TAP_GITHUB_TOKEN` in the shync repo's Actions secrets
