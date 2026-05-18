# Implementation Plan

1. **Refactor render.go: extract half-block renderer and shared scaling**
   - Extract image scaling (nearest-neighbor downscale, no upscale for raster) from `renderImage()` into a shared `scaleImage()` helper so both backends receive a pre-scaled `image.Image`
   - Rename the remaining rendering logic in `renderImage()` to `renderHalfBlock()`
   - Keep `Render()` as the entry point: decode → scale → dispatch to backend
   - No behavior change — all tests should still pass after this step

2. **Add detect.go: terminal capability detection**
   - Implement `detectKittySupport(w io.Writer) bool` with `sync.Once` caching
   - Env var heuristic: check `ORB_NO_KITTY`, `KITTY_WINDOW_ID`, `TERM_PROGRAM`, `TERM`
   - TTY check: if writer is not a TTY file descriptor, `Render()` returns nil immediately (no output at all — both kitty and half-block are meaningless outside a terminal)
   - Use `golang.org/x/term` `IsTerminal()` (transitive dependency via termenv)
   - Unit tests for env var detection logic (mock env vars via `t.Setenv`)

3. **Add kitty.go: kitty protocol rendering**
   - Implement `extractRGBA(img image.Image) ([]byte, int, int)` to get raw RGBA pixel data
   - Implement `renderKitty(w io.Writer, img image.Image, maxWidth int) error`
   - Base64-encode RGBA data, chunk into ≤4096 byte pieces
   - Write escape sequences: first chunk with `a=T,f=32,s={w},v={h}`, continuation chunks with `m=1`, final with `m=0`
   - Image arrives pre-scaled from the shared pipeline (step 1)
   - Unit tests: verify escape sequence format, chunking, RGBA extraction

4. **Wire up Render() to dispatch**
   - Update `Render()` to call `detectBackend(w)` and dispatch to `renderKitty()` or `renderHalfBlock()`
   - Image decoding and SVG rasterization happen before dispatch (shared pipeline)

5. **Verify**
   - Run `make test` — all existing and new tests pass
   - Run `make lint` — no linter warnings
   - Run `make verify` — no uncommitted changes after tidy and lint-fix
   - Manual test in kitty/WezTerm/Ghostty: `orb catalog info <package>` shows crisp icon
   - Manual test in non-kitty terminal: output unchanged from before
   - Manual test with `ORB_NO_KITTY=1`: falls back to half-block
   - Manual test with pipe: `orb catalog info <package> | cat` produces no image output
