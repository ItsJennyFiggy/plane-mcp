// Package tools — documents the method signatures expected from the parallel agent.
//
// The tools.go file defines planeClient and planeResolver interfaces. The
// production Register() function (in register.go) expects *plane.Client and
// *plane.Resolver to satisfy these interfaces.
//
// The following methods are NOT yet present in the plane package and will be
// added by a parallel agent (AGENT-10 client layer):
//
//   - (*plane.Client).ListWorkItems(ctx, projectID, params) ([]WorkItem, error)
//   - (*plane.Client).CreateWorkItem(ctx, projectID, body) (*WorkItem, error)
//   - (*plane.Client).CreateWorkItemComment(ctx, projectID, itemID, comment) error
//   - (*plane.Client).UpdateWorkItem(ctx, projectID, itemID, body) (*WorkItem, error)
//   - (*plane.Client).CreateWorkItemLink(ctx, projectID, itemID, linkURL, title) error
//   - (*plane.Resolver).GetCallerID(ctx) (string, error)
//
// Once those methods are merged, register.go will compile without the
// //go:build !skip_register constraint.
//
// Test compilation notes:
//   - Build the package for testing with: go test -tags skip_register ./internal/tools/...
//   - This excludes register.go (which has the interface satisfaction assertions)
//     and allows all other package code to compile and be tested independently.
package tools
