//go:build tools

// Package tools tracks build-time tool dependencies so they are recorded in
// go.mod and can be run reproducibly via `go run`.
package tools

import (
	_ "github.com/hashicorp/terraform-plugin-docs/cmd/tfplugindocs"
)
