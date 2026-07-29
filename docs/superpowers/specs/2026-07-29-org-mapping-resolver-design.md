# Design: Resolve accessible Org IDs from org_mapping + Groups claim

## Problem

Given the Grafana `org_mapping` string this provider builds (via
`grafana.BuildOrgMapping`, format `<group>:<orgId>:<role>[,...]`, entries
comma-separated, group field may contain glob wildcards `*`/`?`) and a set of
group names from an OIDC `Groups` claim, determine which org identifiers
(IDs or names — treated as opaque strings) the claim grants access to.

## Decision

Add a pure function to the existing `internal/grafana` package rather than a
new package or module. It operates on the exact string format
`BuildOrgMapping`/`OrgMappingContains` already define, so it belongs next to
them — no new parsing dialect, no new package boundary.

```go
// ResolveOrgIDs returns the sorted, de-duplicated list of Grafana org
// identifiers (as they appear in orgMapping) that any of the given claim
// groups grants access to, based on Grafana's org_mapping semantics.
func ResolveOrgIDs(orgMapping string, claimGroups []string) []string
```

File: `internal/grafana/resolve.go`.
Tests: `internal/grafana/resolve_test.go`.

## Behavior

- Parses `orgMapping` the same way `OrgMappingContains` conceptually does:
  split on `,` for entries, then split each entry on unescaped `:` into
  exactly 3 fields (group, orgID, role). `\:` unescapes to a literal `:`
  within a field.
- Entries that don't parse into exactly 3 fields are skipped silently (same
  posture as the existing code: malformed input doesn't error the whole
  call).
- The group field supports glob wildcards (`*` = any sequence, `?` = any
  single char), matched case-sensitively against each claim group
  independently. This mirrors Grafana's own org_mapping matching behavior.
- Role is intentionally ignored — this function answers "can this claim
  access this org at all," not "with which role."
- Result is deduplicated and sorted (`sort.Strings`) for deterministic
  output.
- Empty `orgMapping` or empty `claimGroups` → empty (non-nil vs nil is not
  significant; `nil` is acceptable).

## Non-goals

- No JMESPath library dependency — the "JMESPath style mapping" referred to
  in the original ask is actually Grafana's own `org_mapping` syntax, not a
  JMESPath expression evaluated against JSON. Confirmed with the user.
- No role resolution / highest-role-wins logic — flat org ID list only.
- Not wired into the Tenant controller reconcile loop in this change; it's a
  standalone, independently testable function callers can adopt later.

## Testing

Table-driven tests in `internal/grafana/resolve_test.go` covering: exact
match, glob wildcard match (`*`, `?`), no match, one claim group matching
entries across multiple orgs, multiple claim groups, escaped-colon group
names, malformed entries, empty `orgMapping`, empty `claimGroups`.

## Feasibility

This is fully achievable as plain Go using only the standard library (string
splitting + a small glob matcher, e.g. `path.Match` semantics implemented
inline or via `path/filepath.Match` adapted for non-path strings). No
external dependency is required.
