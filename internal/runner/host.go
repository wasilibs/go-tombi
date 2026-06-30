package runner

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/tetratelabs/wazero"
	"github.com/tetratelabs/wazero/api"
)

const maxFetchBytes = 32 << 20 // 32 MiB

var httpClient = &http.Client{Timeout: 30 * time.Second}

// instantiateHost wires up the "tombi" host module that the guest imports to
// fetch remote schemas.
func instantiateHost(ctx context.Context, rt wazero.Runtime) error {
	_, err := rt.NewHostModuleBuilder("tombi").
		NewFunctionBuilder().
		WithGoModuleFunction(api.GoModuleFunc(fetchSchema),
			[]api.ValueType{
				api.ValueTypeI32, api.ValueTypeI32, // url ptr, len
				api.ValueTypeI32, api.ValueTypeI32, // out body ptr addr, len addr
			},
			[]api.ValueType{api.ValueTypeI32}). // http status (0 = transport error)
		Export("fetch_schema").
		Instantiate(ctx)
	if err != nil {
		return fmt.Errorf("tombi host: instantiating module: %w", err)
	}
	return nil
}

// fetchSchema(url_ptr, url_len, →body_ptr, →body_len) -> http_status.
//
// On a 2xx response the body is copied into guest memory (allocated via the
// guest's exported allocator) and its pointer/length are written to the
// out-params. A transport-level failure returns status 0 with an empty body.
func fetchSchema(ctx context.Context, mod api.Module, stack []uint64) {
	url := readString(mod, uint32(stack[0]), uint32(stack[1]))
	outPtrAddr := uint32(stack[2])
	outLenAddr := uint32(stack[3])

	body, status := doFetch(ctx, url)
	writeOutput(ctx, mod, body, outPtrAddr, outLenAddr)
	stack[0] = uint64(uint32(status))
}

func doFetch(ctx context.Context, url string) (body []byte, status int) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, 0
	}
	req.Header.Set("User-Agent", "tombi-language-server")
	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, resp.StatusCode
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes))
	if err != nil {
		return nil, 0
	}
	return b, resp.StatusCode
}

func readString(mod api.Module, ptr, length uint32) string {
	return string(readBytes(mod, ptr, length))
}

func readBytes(mod api.Module, ptr, length uint32) []byte {
	if length == 0 {
		return nil
	}
	buf, ok := mod.Memory().Read(ptr, length)
	if !ok {
		panic("tombi host: memory read out of bounds")
	}
	// Read returns a view into linear memory; copy so later writes can't alias.
	out := make([]byte, length)
	copy(out, buf)
	return out
}

// writeOutput allocates guest memory via the exported allocator, copies data
// into it, and stores the resulting pointer and length at the out-param
// addresses. A zero-length payload writes a null pointer and zero length.
func writeOutput(ctx context.Context, mod api.Module, data []byte, outPtrAddr, outLenAddr uint32) {
	if len(data) == 0 {
		writeU32(mod, outPtrAddr, 0)
		writeU32(mod, outLenAddr, 0)
		return
	}
	res, err := mod.ExportedFunction("tombi_wasm_alloc").Call(ctx, uint64(len(data)))
	if err != nil {
		panic(err)
	}
	ptr := uint32(res[0])
	if !mod.Memory().Write(ptr, data) {
		panic("tombi host: memory write out of bounds")
	}
	writeU32(mod, outPtrAddr, ptr)
	writeU32(mod, outLenAddr, uint32(len(data)))
}

func writeU32(mod api.Module, addr, val uint32) {
	if !mod.Memory().WriteUint32Le(addr, val) {
		panic("tombi host: out-param write out of bounds")
	}
}
