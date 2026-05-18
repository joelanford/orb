# Requirements

- Kitty graphics protocol rendering for all image types currently supported (PNG, JPEG, GIF, SVG)
- Terminal capability detection with fallback chain: env var heuristic → half-block
- Detection result cached per process (no repeated detection on each Render call)
- `ORB_NO_KITTY=1` env var to force-disable kitty protocol
- Existing half-block renderer preserved as universal fallback
- No changes to the `Render()` function signature
- No changes to call sites in `internal/cmd/catalog.go`
- Raw RGBA pixel format (`f=32`) for kitty transmission
- Half-block backend scales to fit `maxWidth`; kitty backend caps pixel data at 512x512 and displays at native resolution
- Non-TTY writers (pipes, files) skip image rendering entirely (return nil)

## Acceptance Criteria

- `Render()` produces kitty graphics escape sequences when running in a kitty-capable terminal
- `Render()` produces half-block output when running in a non-kitty terminal (same output as today)
- `Render()` produces half-block output when `ORB_NO_KITTY=1` is set, regardless of terminal
- `Render()` returns nil and writes nothing when the writer is not a TTY (e.g., piped to a file)
- SVG images are rasterized and transmitted via kitty protocol at the correct dimensions
- PNG/JPEG/GIF images are decoded and transmitted via kitty protocol at the correct dimensions
- Kitty escape sequences are well-formed: correct `f=32` format, chunked base64 with `m=0/1`, valid `s=` and `v=` dimensions
- Detection query does not hang or block for more than 100ms on non-responsive terminals
- All existing `termimage` tests continue to pass
- New tests cover kitty protocol encoding, RGBA extraction, chunked transmission, and detection logic
- `make test`, `make lint`, and `make verify` pass
