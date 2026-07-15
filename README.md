# ys

[![CI](https://github.com/greeddj/ys/actions/workflows/ci.yml/badge.svg)](https://github.com/greeddj/ys/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/greeddj/ys?sort=semver)](https://github.com/greeddj/ys/releases)
[![Go Report Card](https://goreportcard.com/badge/github.com/greeddj/ys)](https://goreportcard.com/report/github.com/greeddj/ys)
[![License: MIT](https://img.shields.io/github/license/greeddj/ys)](LICENSE)

`ys` searches YAML files for a key and groups identical results together, printing
each unique `path: value` block once with the list of files it was found in.

It is built for auditing config drift across many near-identical deployment value
files: instead of diffing everything by hand, you ask "what is `dhcp` set to
everywhere?" and immediately see the consensus value, the outliers, and exactly
which files disagree.

```console
$ ys dhcp ./envs
# 3 file(s):
#   envs/dev/values.yaml
#   envs/prod/values.yaml
#   envs/staging/values.yaml
secrets.crm.settings.dhcp: enabled
---
# 1 file(s):
#   envs/legacy/values.yaml
secrets.crm.settings.dhcp: disabled
```

The most common value is printed first, so the single `legacy` outlier is obvious.

## Features

- Three matching modes: exact leaf, path suffix, or full-path regexp.
- Groups identical `path: value` blocks and lists every file each block came from.
- Comment-insensitive: values that differ only by an incidental `# comment` are
  treated as equal.
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
docker run --rm -v "$PWD:/work" -w /work ghcr.io/greeddj/ys:latest dhcp .
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
| `-p`, `--path` | match `KEY` as a path suffix, e.g. `settings.dhcp` |
| `-r`, `--regexp` | match `KEY` as a regexp over the full dotted path |
| `-C`, `--colors` | force colorized output (default: only on a terminal) |
| `-M`, `--no-colors` | disable colorized output |
| `-h`, `--help` | show help |
| `-v`, `--version` | print the version |

### Matching modes

- **Default** matches `KEY` exactly against the **last** segment of a dotted path.
  `ys dhcp` matches `secrets.crm.settings.dhcp` and `db.dhcp`, but not `dhcp_lease`.
- **`-p` (path suffix)** matches `KEY` against a trailing run of segments.
  `ys -p settings.dhcp` matches `secrets.crm.settings.dhcp` but not `other.dhcp`.
- **`-r` (regexp)** matches `KEY` as a [regular expression](https://pkg.go.dev/regexp/syntax)
  against the whole dotted path. `ys -r 'settings\.(dhcp|dns)$'`.

When both `-r` and `-p` are given, `-r` wins.

### Examples

```sh
# Every "replicas" value across a tree of Helm value files.
ys replicas ./charts

# A specific nested key, several roots at once.
ys -p resources.limits.cpu ./prod ./staging

# Anything under settings that ends in dhcp or dns.
ys -r 'settings\.(dhcp|dns)$' ./envs

# Force color through a pager.
ys -C image ./envs | less -R
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
valid YAML-ish text you can page or grep. Ordering is stable: groups with the same
path are kept together, the most common value within a path comes first, and value
text breaks any remaining ties.

### Color

Output is colorized in the style of `yq` (keys, strings, numbers, booleans, and
`null` each get their own color) only when writing to a terminal. Piping to a file
or another program yields plain text. Set `-C` to force color on (for example
through `less -R`), `-M` to force it off, or the [`NO_COLOR`](https://no-color.org)
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
