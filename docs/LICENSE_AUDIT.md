# License Audit

This project is licensed under Apache-2.0. Dependencies keep their own licenses.

Run the local helper:

```bash
make license-check
```

By default this checks production/distribution dependencies. To include dev-only
packages from `web/package-lock.json`, run:

```bash
INCLUDE_DEV_LICENSES=1 make license-check
```

The helper uses these scanners when installed:

- Go modules: `go-licenses`
- npm packages: `license-checker-rseidelsohn` or `license-checker`

Install the scanners with:

```bash
go install github.com/google/go-licenses@latest
cd web && npm install --save-dev license-checker-rseidelsohn
```

The default policy allows common permissive licenses that are typically
compatible with Apache-2.0 distribution: Apache-2.0, MIT, BSD-2-Clause,
BSD-3-Clause, ISC, BlueOak-1.0.0, 0BSD, Python-2.0, CC-BY-4.0, and
W3C-20150513.

## Current Findings

The previous MDXEditor dependency path was removed:

```text
@mdxeditor/editor -> @codesandbox/sandpack-react -> @codesandbox/sandpack-client -> @codesandbox/nodebox
```

The admin Markdown editor now uses TOAST UI Editor. `make license-check`
should remain free of `@codesandbox/nodebox`, Sandpack, and other
Sustainable Use License packages.

Syntax highlighting uses the TOAST UI code syntax plugin and PrismJS in the
admin editor, plus goldmark-highlighting and Chroma in the public renderer.
These dependencies are currently permissively licensed and should remain covered
by the normal license check before release.

Before adding copied third-party files, generated assets, examples, or templates,
record their source and license. Do not add GPL, AGPL, LGPL, proprietary, or
unknown-license code unless the licensing implications have been reviewed.
