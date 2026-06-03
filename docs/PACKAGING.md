# Packaging

Uvoo-MiniCMS supports two local packaging paths:

- `scripts/package.sh` creates a tarball-style Linux archive.
- `scripts/package-linux.sh` creates distro packages with nFPM, usually `.deb` and `.rpm`.

## Build deb/rpm Packages

Install `nfpm`, then run:

```bash
make package-linux
```

By default this creates both `.deb` and `.rpm` packages in `dist/`.

To build only one package type:

```bash
FORMATS=deb make package-linux
FORMATS=rpm make package-linux
```

## Versioning

`scripts/package-linux.sh` reads the version from `VERSION` when provided. Otherwise it uses:

```bash
git describe --tags --always --dirty
```

Package versions for deb/rpm must start with a digit. The packaging script normalizes git-derived values before passing them to nFPM:

| Source version | Package version example |
|---|---|
| `v0.1.0` | `0.1.0` |
| `v0.1.0-4-ga607267` | `0.1.0.4.ga607267` |
| `a607267` | `0.0.35+git.a607267` |
| `a607267-dirty` | `0.0.35+git.a607267.dirty` |

The `0.0.<commit-count>+git.<hash>` fallback keeps untagged builds installable and gives package managers a version that can sort forward as commits are added.

## Dirty Builds

A dirty build is a package built from a working tree with uncommitted changes. You do not need a special flag:

```bash
# edit a file, but do not commit it
git status --short
make package-linux
```

If the working tree is dirty, `git describe --dirty` appends `-dirty`, and the package script converts it into a package-safe version. For example:

```text
packaging uvoo-minicms version 0.0.35+git.a607267.dirty from source version a607267-dirty
```

You can also force a snapshot version explicitly:

```bash
VERSION=0.0.35+git.a607267.dirty FORMATS=deb make package-linux
```

Use explicit `VERSION=...` values in CI when you want fully predictable package names.

## Release Builds

For a clean release, tag first:

```bash
git tag v0.1.0
make package-linux
```

This produces package versions like `0.1.0`, which are easier to reason about than hash-derived snapshot versions.

## Verify a Package

For Debian packages:

```bash
dpkg-deb -I dist/uvoo-minicms_*.deb | sed -n '1,80p'
```

Check that the `Version:` field starts with a digit.

For RPM packages:

```bash
rpm -qp --queryformat '%{NAME} %{VERSION}-%{RELEASE}\n' dist/uvoo-minicms_*.rpm
```

## Install or Upgrade

Install a local Debian package with:

```bash
sudo apt install ./dist/uvoo-minicms_<version>_amd64.deb
```

Install a local RPM package with:

```bash
sudo rpm -Uvh dist/uvoo-minicms_<version>_amd64.rpm
```

The package installs:

- `/usr/bin/uvoo-minicms`
- `/usr/share/uvoo-minicms/web/dist`
- `/etc/uvoo-minicms/uvoo-minicms.env`
- `/var/lib/uvoo-minicms`
- `uvoo-minicms.service`

