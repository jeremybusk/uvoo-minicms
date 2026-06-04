# License Audit

This project is licensed under Apache-2.0. Dependencies keep their own licenses.

Run the local helper:

```bash
make license-check
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

The frontend dependency tree currently includes:

```text
@mdxeditor/editor -> @codesandbox/sandpack-react -> @codesandbox/sandpack-client -> @codesandbox/nodebox
```

`@codesandbox/nodebox` uses the Sustainable Use License. That license restricts
commercial and redistribution use and should be treated as incompatible with a
general Apache-2.0 open source release unless legal counsel confirms otherwise.

Before publishing a public release, remove or replace the dependency path, avoid
shipping the affected package, or document a reviewed legal decision.

Before adding copied third-party files, generated assets, examples, or templates,
record their source and license. Do not add GPL, AGPL, LGPL, proprietary, or
unknown-license code unless the licensing implications have been reviewed.
