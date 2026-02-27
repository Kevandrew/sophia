// Package diff owns change-summary and base-anchor resolution helpers used by
// diff/rangediff surfaces.
//
// Ownership contract:
//   - Root service diff orchestration depends on this package.
//   - This package defines deterministic diff-domain derivations and should not
//     depend on sibling domain packages.
package diff
