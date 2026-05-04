---
status: idea
---
# Migrate helm chart generation to library-olm

Move orb's `internal/helm/` chart generation logic into library-olm as a new rendering target alongside plain manifests (e.g., `bundle/registry/v1.ToHelmChart()`). The helm generation is tightly coupled to the same OLM-specific naming, hashing, and validation utilities that plain manifest conversion uses — these live in library-olm's `internal/` packages and should not be duplicated.

After this migration, orb's helm code becomes a thin caller of library-olm's chart generation API. The `internal/convert/` package can be fully deleted since both rendering paths (plain manifests and helm charts) will live in library-olm.

Depends on `migrate-convert-to-library-olm`.
