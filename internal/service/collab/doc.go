// Package collab owns CR collaboration/transport helpers, field decoding, and
// normalization logic used by collab sync flows.
//
// Ownership contract:
//   - Root service orchestration calls into this package.
//   - This package provides deterministic helper behavior and should not depend
//     on sibling domain packages under internal/service.
package collab
