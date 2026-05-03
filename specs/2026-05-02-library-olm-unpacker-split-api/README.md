---
status: idea
---
# library-olm: split Unpacker into resolve + unpack

library-olm's `Unpacker` combines resolve + handler matching + unpack into a single `Unpack` call. This prevents callers from inserting logic between matching and unpacking — for example, computing total download size for progress bars, or inspecting the resolved manifest for logging.

A split API (e.g., `Unpacker.Resolve` returning a resolved result with a bound `Unpack` method) would let callers use the handler dispatch logic while still controlling the flow between match and unpack. This would eliminate the need for callers to drive handlers directly and duplicate the resolve + fetch manifest step that `Unpacker` does internally.

This is a library-olm change, tracked here because orb is the motivating consumer.
