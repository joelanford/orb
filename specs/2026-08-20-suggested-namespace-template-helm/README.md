---
status: idea
---
# suggested-namespace-template aware helm chart generation

Bring the helm chart generator to parity with the plain path's derived-namespace behavior so a generated chart can create and label its own install namespace. The primary driver is functional namespace metadata — most importantly Pod Security Admission (PSA) labels (e.g. `pod-security.kubernetes.io/enforce: privileged`) that the workload requires to run. Today the bespoke generator templates everything off `{{ .Release.Namespace }}` and never emits a `Namespace`, so those labels are silently the caller's problem.

## Design: single chart, value-controlled mode

One generated chart, one new boolean value `useSuggestedNamespace` (default `false`), switching modes at install time:

- **`useSuggestedNamespace: false` (default)** — today's behavior, unchanged. Resources are placed in `.Release.Namespace` (self-managed); no `Namespace` object is emitted.
- **`useSuggestedNamespace: true`** — emit a `Namespace` object whose name/labels/annotations are baked at chart-generation time from `operatorframework.io/suggested-namespace-template` (falling back to `operatorframework.io/suggested-namespace`, then `<package>-system`), and pin all operator resources to that derived namespace name. PSA labels ship as a first-class chart artifact.
- **Helper `orb.installNamespace`** resolves the effective namespace: the baked derived name when `useSuggestedNamespace` is true, else `.Release.Namespace`. Every namespaced resource, RBAC subject/binding, webhook service ref, and cert-manager `inject-ca-from` reference uses this helper instead of `.Release.Namespace` directly.
- **Guard:** when `useSuggestedNamespace` is true, the derived namespace must differ from the release namespace — an emitted `Namespace` matching the release namespace collides with a pre-existing namespace or `helm install --create-namespace`. Enforce via a template `fail` or document clearly.

## Deliverables

- Add the `useSuggestedNamespace` value (default `false`) + JSON schema entry and the `orb.installNamespace` helper in `internal/helm/`.
- Bake the derived namespace name and its labels/annotations into the chart at generation time, resolved in precedence order: `operatorframework.io/suggested-namespace-template` (authoritative — full `Namespace` including labels/annotations), then `operatorframework.io/suggested-namespace` (name only), then `<packageName>-system` (name only).
- Replace direct `{{ .Release.Namespace }}` placement references across `deployment.go`, `rbac.go`, `webhook.go`, `certprovider.go`, and `crd.go` with the helper.
- Emit the derived `Namespace` (with baked labels/annotations) guarded by the value; surface the suggested-namespace labels in the default self-managed mode via `NOTES.txt`/README so the requirement is never silent.
- Tests: `useSuggestedNamespace: true` emits a labeled `Namespace` and pins resources to it; default `false` is unchanged; guard rejects a derived namespace equal to the release namespace.

## Why it matters

Directly serves mission goals 2 (faithful bundle → Helm conversion) and 3 (seamless catalog-to-cluster via Helm). Without namespace-label support, charts for operators that need privileged PSA silently fail admission, breaking the catalog-to-cluster workflow.

## Notes

- Build in orb's current bespoke generator now for the capability. Per the thin-wrapper principle, this derivation logic should eventually move upstream so plain and helm share one implementation — fold into [[migrate-helm-to-library-olm]] when that migration happens.
- Assumes the library-olm bump that makes the install namespace optional (`WithSelfManagedInstallNamespace`, derive-and-emit on the plain path) has already landed.
