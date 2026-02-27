// Package tasks owns task contract/checkpoint normalization and transition
// helpers used by task lifecycle orchestration.
//
// Ownership contract:
//   - Root service task lifecycle flows consume this package.
//   - This package defines task-domain contract/checkpoint primitives and
//     should not depend on sibling domain packages.
package tasks
