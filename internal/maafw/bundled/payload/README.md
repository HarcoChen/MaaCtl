# MaaFramework payload

`tools/packmaafw` writes the MaaFramework runtime that bundled maactl builds
carry inside their executable into this directory:

| file           | content                                                        |
| -------------- | -------------------------------------------------------------- |
| `bin.zip`      | the runtime libraries packed from `maafw/bin`                   |
| `version.txt`  | the MaaFramework version those libraries come from              |
| `platform.txt` | the platform they were built for, e.g. `win-x86_64`             |

All three files are generated and untracked; this README only keeps the
directory embeddable, so that builds behave the same before and after a payload
exists.

The runtime has to be on disk first: `tools/packmaafw` never downloads anything,
it refuses to pack a directory that does not hold the libraries of the platform
being built for. Unpack the matching `MAA-<platform>-<version>.zip` release
archive into `maafw/` (CI does this per platform), then:

```bash
python3 .github/scripts/fetch_maafw.py --platform linux-x86_64

# self-contained: carries MaaFramework inside the executable
go run ./tools/packmaafw
go build -tags bundled -o maactl ./cmd/maactl

# lite: loads MaaFramework from ./maafw/bin or --lib-dir
go build -o maactl-lite ./cmd/maactl
```

`platform.txt` is what keeps a wrong payload from being used silently: the
runtime refuses to unpack libraries that were built for another platform.

See the "自带 MaaFramework" section of docs/build.md for details.
