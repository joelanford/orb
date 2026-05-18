# Verification

## Implementation Correctness

- [ ] Image scaling extracted into shared `scaleImage()` helper used by both backends
- [ ] Remaining rendering logic from `renderImage()` renamed to `renderHalfBlock()` — no logic change
- [ ] `Render()` dispatches to `renderKitty()` or `renderHalfBlock()` based on detection
- [ ] Detection caches result via `sync.Once` — no repeated detection per call
- [ ] Env var heuristic checks `ORB_NO_KITTY`, `KITTY_WINDOW_ID`, `TERM_PROGRAM`, `TERM` in correct priority order
- [ ] RGBA extraction produces correct pixel data from `image.Image` (including non-RGBA image types like `image.YCbCr` from JPEG)
- [ ] Kitty escape sequences use correct format: `\033_G` prefix, `;` before base64 data, `\033\\` terminator
- [ ] First chunk includes `a=T,f=32,s={width},v={height}` (no cell-based sizing params)
- [ ] Continuation chunks use `m=1`, final chunk uses `m=0`
- [ ] Base64 chunks are ≤4096 bytes each
- [ ] Kitty backend: pixel data capped at 512x512 via `kittyScaleImage()`, displayed at native resolution (no cell-based sizing)
- [ ] Half-block backend: scaled to fit `maxWidth` via `scaleImage()`, no upscale for raster
- [ ] SVGs rasterized at 512px for kitty, `maxWidth` for half-block
- [ ] Non-TTY writers get no output at all (`Render()` returns nil immediately)
- [ ] `ORB_NO_KITTY=1` always produces half-block output

## Project Conventions

- [ ] No changes to `Render()` function signature
- [ ] No changes to call sites in `internal/cmd/`
- [ ] All new code in `internal/termimage/` (no exported APIs)
- [ ] No new external dependencies
- [ ] `make test` passes with `-race -count=1`
- [ ] `make lint` passes with no warnings and no `//nolint` comments
- [ ] `make verify` passes (tidy + lint-fix produce no diff)
- [ ] Design follows mission principle #1 (client-side first) and #4 (progressive disclosure)
- [ ] Uses Go standard library `image`, `encoding/base64`, `os` — consistent with tech stack
