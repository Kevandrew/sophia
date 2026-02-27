// Package trust owns trust scoring, threshold, and evidence interpretation
// helper primitives used by trust-domain orchestration.
//
// Ownership contract:
//   - Root service trust orchestration depends on this package.
//   - This package provides trust-domain computations and should avoid
//     dependencies on sibling internal/service domain packages.
package trust
