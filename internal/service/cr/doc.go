// Package cr owns CR creation and lifecycle request shaping primitives that are
// independent of root service runtime orchestration.
//
// Ownership contract:
//   - Root service lifecycle flows consume this package for CR-domain helpers.
//   - This package defines validation/building rules for CR inputs and should
//     not take dependencies on sibling domain packages.
package cr
