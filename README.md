# ys

[![CI](https://github.com/greeddj/ys/actions/workflows/ci.yml/badge.svg)](https://github.com/greeddj/ys/actions/workflows/ci.yml)
[![codecov](https://codecov.io/gh/greeddj/ys/graph/badge.svg)](https://codecov.io/gh/greeddj/ys)
[![Release](https://img.shields.io/github/v/release/greeddj/ys?sort=semver)](https://github.com/greeddj/ys/releases)

`ys` searches YAML files for a key and groups identical results together, printing
each unique `path: value` block once with the list of files it was found in.

It is built for auditing config drift across many near-identical deployment value
files: instead of diffing everything by hand, you ask "what is the log `level`
everywhere?" and immediately see the consensus value, the outliers, and exactly
which files disagree.

```console
$ ys level ./envs
# 3 file(s):
#   envs/dev/values.yaml
#   envs/prod/values.yaml
#   envs/staging/values.yaml
observability.logging.level: info
---
# 1 file(s):
#   envs/legacy/values.yaml
observability.logging.level: debug
```

The most common value is printed first, so the single `legacy` outlier is obvious.

## Features

- Three matching modes: exact leaf, path suffix, or full-path regexp.
- Groups identical `path: value` blocks and lists every file each block came from.
- In regexp mode, sibling paths with an identical value are merged into a single
  block whose path lists the alternatives: `services.(api|worker).resources.limits`.
- Notation-insensitive: values that differ only by an incidental `# comment`, by
  quoting (`"x"`, `'x'`, `x`), by flow or block style, or by the order of a
  block's keys and entries are treated as equal.
- Type-preserving all the same: quoting that carries a type is kept, so the
  string `"123"` never merges with the number `123`.
- Handles multi-document files (`---`), nested maps, and sequences.
- Deterministic, sorted output that is easy to diff and pipe.
- yq-style colorized output on a terminal, plain text when piped.
- Single static binary, no runtime dependencies.

## Install

### go install

```sh
go install github.com/greeddj/ys/cmd/ys@latest
```

### Homebrew

```sh
brew install greeddj/tap/ys
```

### Docker

Images are published to GitHub Container Registry for `linux/amd64` and
`linux/arm64`. Mount the directory you want to scan:

```sh
docker run --rm -v "$PWD:/work" -w /work ghcr.io/greeddj/ys:latest level .
```

### Binaries

Prebuilt archives for Linux and macOS (amd64/arm64) are attached to every
[release](https://github.com/greeddj/ys/releases).

### From source

```sh
git clone https://github.com/greeddj/ys
cd ys
go build -o ys ./cmd/ys
```

## Usage

```text
ys [-r|-p] KEY PATH...
```

`KEY` is the key to search for; each `PATH` is a file or a directory. Directories
are walked recursively and every `.yml`/`.yaml` file is scanned. Unreadable roots
or files are reported to stderr and skipped, so one bad file never aborts a run.

### Options

| Flag | Description |
| --- | --- |
| `-p`, `--path` | match `KEY` as a path suffix, e.g. `http.timeout` |
| `-r`, `--regexp` | match `KEY` as a regexp over the full dotted path |
| `-c`, `--color` | force colorized output (default: only on a terminal) |
| `-n`, `--no-color` | disable colorized output |
| `-h`, `--help` | show help |
| `-v`, `--version` | print the version |

### Matching modes

- **Default** matches `KEY` exactly against the **last** segment of a dotted path.
  `ys timeout` matches `services.api.http.timeout` and `database.timeout`, but not
  `probes.readiness.timeoutSeconds`.
- **`-p` (path suffix)** matches `KEY` against a trailing run of segments.
  `ys -p http.timeout` matches `services.api.http.timeout` but not `database.timeout`.
- **`-r` (regexp)** matches `KEY` as a [regular expression](https://pkg.go.dev/regexp/syntax)
  against the whole dotted path. `ys -r 'logging\.(level|format)$'`.

When both `-r` and `-p` are given, `-r` wins.

### Merging in regexp mode

When a regexp matches several sibling paths that carry an identical value, the
groups are merged into one block. Paths of equal depth that differ in exactly
one segment are merged greedily into an alternation:

```console
$ ys -r 'resources\.limits$' ./envs
# 4 file(s):
#   envs/dev/values.yaml
#   envs/legacy/values.yaml
#   envs/prod/values.yaml
#   envs/staging/values.yaml
services.(api|worker).resources.limits:
  cpu: 500m
  memory: 512Mi
---
# 1 file(s):
#   envs/legacy/values.yaml
services.api.resources.limits:
  cpu: 250m
  memory: 256Mi
```

Both services agree on one limits block everywhere except `legacy`, which is the
only file listed twice: its `worker` still carries the shared value, while its
`api` has drifted to a smaller one.

Only combinations that were actually found are merged: paths that differ in
real depth (a key literally named `a.b` is one segment, not two) or in more
than one segment stay separate, and a segment containing `(`, `|`, or `)`
never joins an alternation, so the display never implies a path that does not
exist.

### Examples

```sh
# Every "replicas" value across a tree of Helm value files.
ys replicas ./charts

# A specific nested key, several roots at once.
ys -p resources.limits.cpu ./prod ./staging

# Anything under logging that ends in level or format.
ys -r 'logging\.(level|format)$' ./envs

# Force color through a pager.
ys -c image ./envs | less -R
```

## Output

Each group is printed as a header comment listing the files, followed by the
shared block:

```text
# <N> file(s):
#   <file>
#   ...
<dotted.path>: <value>
```

Consecutive groups are separated by a `---` marker, so the whole output is itself
valid YAML-ish text you can page or grep. Ordering is stable: files and groups are
sorted naturally (`env1` before `env10`), related paths are kept together
(a merged group sorts by its smallest constituent path), the most common value
comes first, and block text breaks any remaining ties.

### Canonical blocks

A block is printed canonically rather than verbatim, which is what lets files
that agree on the config but not on how they spell it collapse into one group:
comments are dropped, quoting is reduced to what the value's type actually
requires, flow collections (`{a: 1}`) are printed as blocks, and the keys and
entries nested inside the block are sorted naturally.

```console
$ ys limits ./envs           # dev writes {memory: 512Mi, cpu: 500m} inline
# 2 file(s):
#   envs/dev.yaml
#   envs/prod.yaml
limits:
  cpu: 500m
  memory: 512Mi
```

Only the block content is canonicalized, never the paths: a sequence index still
names the element the file really holds there, so `ys -r 'args\.0$'` reports each
file's own first argument.

### Color

Output is colorized in the style of `yq` (keys, strings, numbers, booleans, and
`null` each get their own color) only when writing to a terminal. Piping to a file
or another program yields plain text. Set `-c` to force color on (for example
through `less -R`), `-n` to force it off, or the [`NO_COLOR`](https://no-color.org)
environment variable to opt out globally.

### Exit codes

| Code | Meaning |
| --- | --- |
| `0` | success |
| `2` | usage error: missing arguments or an invalid regexp |
| `130` | interrupted (Ctrl+C) |

## Development

Requires Go 1.26 and [`just`](https://github.com/casey/just). Dependencies are
vendored.

```sh
just deps    # go mod tidy + vendor
just fix     # gofmt, go fix, fieldalignment -fix
just lint    # golangci-lint
just test    # go test ./...
just check   # vet, staticcheck, govulncheck, fieldalignment
just build   # static binary into ./dist
just run     # run with the race detector
```

## License

[MIT](LICENSE) (c) Dmitrij Shishkin
