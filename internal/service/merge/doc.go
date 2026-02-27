// Package merge owns merge-domain helper logic such as advice and override
// evidence shaping.
//
// Ownership contract:
//   - Root service merge orchestration consumes this package.
//   - This package remains focused on merge-domain derivations and should avoid
//     coupling to sibling internal/service domain packages.
package merge
