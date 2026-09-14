# MaaFramework payload

`tools/packmaafw` writes the MaaFramework runtime that bundled maactl builds
carry inside their executable into this directory:

| file          | content                                                       |
| ------------- | ------------------------------------------------------------- |
| `bin.zip`     | the runtime libraries taken from a MaaFramework release `bin/` |
| `version.txt` | the MaaFramework version those libraries come from             |

Both files are generated and untracked; this README only keeps the directory
embeddable, so that builds behave the same before and after a payload exists.

Populate the payload and build the two supported executables with:

```powershell
go run ./tools/packmaafw

# self-contained: carries MaaFramework inside the executable
go build -tags bundled -o maactl.exe ./cmd/maactl

# lite: loads MaaFramework from ./maafw/bin or --lib-dir
go build -o maactl-lite.exe ./cmd/maactl
```

See the "自带 MaaFramework" section of the project README for details.
