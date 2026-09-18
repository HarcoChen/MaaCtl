# MaaFramework payload

`tools/packmaafw` writes the MaaFramework runtime that bundled maactl builds
carry inside their executable into this directory:

| file      | content                                    |
| --------- | ------------------------------------------ |
| `bin.zip` | the runtime packed from `maafw/bin`, as is |

`bin.zip` is generated and untracked; this README only keeps the directory
embeddable, so builds behave the same before and after a payload exists.

The payload is a plain copy of a runtime directory: no version, no platform, no
metadata. maactl reports the version the loaded libraries report about
themselves (`maactl -v` loads them to answer, `maactl selfcheck` shows where
they came from), and the runtime files are what decides which platform the build
works on.

The runtime has to be on disk first: `tools/packmaafw` never downloads anything.

```bash
# which release to unpack is decided here, and only here
python3 .github/scripts/fetch_maafw.py --platform linux-x86_64   # current stable
python3 .github/scripts/fetch_maafw.py --platform linux-x86_64 --version v5.13.1

# self-contained: carries MaaFramework inside the executable
go run ./tools/packmaafw
go build -tags bundled -o maactl ./cmd/maactl

# lite: loads MaaFramework from ./maafw/bin or --lib-dir
go build -o maactl-lite ./cmd/maactl
```

Unpacking a release of the wrong platform is the one mistake the packer cannot
notice, so `fetch_maafw.py` verifies the libraries of the platform it was asked
for, and CI runs `maactl selfcheck` on every platform to prove the packaged
runtime really loads.

See the "自带 MaaFramework" section of docs/build.md for details.
