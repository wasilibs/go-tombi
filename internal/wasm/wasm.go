package wasm

import _ "embed"

//go:embed memory.wasm
var Memory []byte

//go:embed tombi.wasm
var Tombi []byte
