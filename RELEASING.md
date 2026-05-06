# Releasing

Releases are published to
[openrelik/openrelik-go-client](https://github.com/openrelik/openrelik-go-client/releases)
as GitHub Releases with auto-generated release notes.

## Prerequisites

- [GitHub CLI](https://cli.github.com) installed and authenticated (`gh auth login`)

## Using release.sh (recommended)

```bash
# Bump minor version (e.g. 0.1.0 → 0.2.0)
./release.sh

# Bump patch version (e.g. 0.1.0 → 0.1.1)
./release.sh patch
```

The script will:
1. Fetch the latest release tag from GitHub
2. Compute the next version and prompt for confirmation
3. Create and push the git tag
4. Create a GitHub Release with auto-generated notes

## Manual release

```bash
# 1. Ensure the working tree is clean
git status

# 2. Tag and push
git tag v0.2.0
git push origin v0.2.0

# 3. Create the GitHub Release
gh release create v0.2.0 --generate-notes
```
