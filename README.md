# go-tombi

go-tombi is a distribution of [tombi][1], that can be built with Go. It does not actually reimplement any
functionality of tombi in Go, instead building it into a WebAssembly binary, and
executing with the pure Go Wasm runtime [wazero][2]. This means that `go install` or `go run`
can be used to execute it, with no need to rely on separate package managers such as cargo,
on any platform that Go supports.

## Installation

Precompiled binaries are available in the [releases](https://github.com/wasilibs/go-tombi/releases).
Alternatively, install the plugin you want using `go install`.

```bash
$ go install github.com/wasilibs/go-tombi/cmd/tombi@latest
```

To avoid installation entirely, it can be convenient to use `go run`

```bash
$ go run github.com/wasilibs/go-tombi/cmd/tombi@latest format .
```

Note that due to the sandboxing of the filesystem when using Wasm, currently only files that descend
from the current directory when executing the tool are accessible to it, i.e., `../other/my.toml` or
`/separate/root/my.toml` will not be found.

[1]: https://github.com/tombi-toml/tombi
[2]: https://wazero.io/
