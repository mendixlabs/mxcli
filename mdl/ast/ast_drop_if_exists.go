// SPDX-License-Identifier: Apache-2.0

package ast

// DropIfExists is embedded in the document DROP statements that accept
// IF EXISTS. A missing document is then skipped rather than an error, so a
// script that removes one can run twice (mendixlabs/mxcli#1190).
type DropIfExists struct {
	IfExists bool
}

// SkipsMissing reports whether the statement was written with IF EXISTS.
func (d DropIfExists) SkipsMissing() bool { return d.IfExists }

// MissingSkipper is implemented by every statement that embeds DropIfExists.
type MissingSkipper interface {
	SkipsMissing() bool
}
