// Package policy owns SOPHIA.yaml policy parsing/normalization helpers and
// canonical validation primitives for policy-driven behavior.
//
// Ownership contract:
//   - Root service policy-loading paths depend on this package.
//   - This package defines policy-domain normalization and should stay
//     independent from sibling domain packages.
package policy
