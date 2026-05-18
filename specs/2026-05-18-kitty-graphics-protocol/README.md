---
status: done
pr: https://github.com/joelanford/orb/pull/44
---
# Kitty terminal graphics protocol support

## Summary

Add kitty terminal graphics protocol support to `internal/termimage/` for higher-fidelity icon rendering. The kitty protocol transmits actual pixel data to capable terminals, producing crisp full-resolution images instead of the half-block Unicode approximation used today. Terminals that don't support the protocol continue to see the existing half-block output — no changes to the call site or public API.

## Design

### Backend selection

`Render()` keeps its current signature: `Render(w io.Writer, imgData []byte, mediaType string, maxWidth int) error`. Internally, it selects a rendering backend based on terminal capability detection. Detection is lazy and cached (run once per process via `sync.Once`).

The detection fallback chain:

1. **Env var heuristic** — check `KITTY_WINDOW_ID` (set by kitty), `TERM_PROGRAM` (set by WezTerm, Ghostty, etc.), or `TERM` containing `kitty`. If any match a known kitty-capable terminal, use kitty protocol.
2. **Half-block fallback** — if no env var match, use the existing half-block renderer.

Query-based detection (sending a kitty graphics query escape sequence and reading the response) was considered but rejected — terminals that don't understand the sequence display it as garbage.

An env var `ORB_NO_KITTY=1` force-disables kitty protocol (useful for terminals that claim support but render poorly).

### Non-TTY detection

If the writer is not a TTY (e.g., output piped to a file or another process), `Render()` returns `nil` immediately without writing anything. Both kitty escape sequences and half-block ANSI output are meaningless outside a terminal, so skipping entirely is the correct behavior. The caller already handles this gracefully — `catalog.go` only acts on `Render()` success (`if err := termimage.Render(...); err == nil`).

Detection: type-assert the writer to `*os.File`, then check `term.IsTerminal(fd)` using `golang.org/x/term` (already a transitive dependency via termenv).

### Kitty graphics protocol transmission

The protocol transmits image data via escape sequences. For orb's use case (small icon-sized images), the implementation is straightforward:

- **Format:** raw RGBA pixels (`f=32`). All image sources (PNG, JPEG, GIF, rasterized SVG) are already decoded to `image.Image` in memory. Extract raw RGBA pixel data directly — no re-encoding step needed.
- **Transmission:** stream mode (`t=d`, direct data). Base64-encode the pixel data and split into chunks ≤4096 bytes. Each chunk is wrapped in `\033_G...;{data}\033\\`. The first chunk includes `a=T,f=32,s={width},v={height}`. Continuation chunks use `m=1` (more data) until the final chunk uses `m=0`.
- **Pixel data resolution:** capped at 512x512 pixels per side (no upscale). Large source images are scaled down to fit; small images are sent at native resolution. This limits payload size while preserving quality. SVGs are rasterized at 512px wide (instead of `maxWidth`) so the terminal receives high-resolution pixel data.
- **Display sizing:** images are displayed at their native pixel resolution — no cell-based scaling. This keeps images sharp; small icons appear at their natural size rather than being stretched across terminal columns.
- **Cleanup:** kitty protocol images persist in the terminal's memory. `a=d,d=A` (delete all) isn't needed since we're using direct transmission with no placement IDs — images scroll naturally with terminal content.

### Image decoding pipeline

`Render()` detects the backend first, then decodes the image accordingly:

1. Detect backend (none → return nil, kitty, or half-block)
2. If `mediaType` starts with `image/svg`, rasterize via `rasterizeSVG()` — at `kittyMaxPixels` (512) for kitty, `maxWidth` for half-block
3. Otherwise, decode via `image.Decode()` to `image.Image`
4. **Kitty path:** cap pixel data at 512x512 via `kittyScaleImage()`, extract raw RGBA, transmit
5. **Half-block path:** scale to fit `maxWidth` via `scaleImage()`, render with ANSI half-blocks

### Code organization

All new code lives in `internal/termimage/`:

- `render.go` — update `Render()` to dispatch to kitty or half-block based on detected capability. Extract the current half-block logic into a `renderHalfBlock()` function.
- `kitty.go` — kitty protocol transmission: `renderKitty(w io.Writer, img image.Image) error`, RGBA pixel extraction, and `kittyScaleImage()` (cap at 512px).
- `detect.go` — terminal capability detection: `detectBackend(w any) backend` with TTY check, env var heuristic, and `ORB_NO_KITTY` override. Cached via `sync.Once`.
