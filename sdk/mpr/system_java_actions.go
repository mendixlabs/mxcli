// SPDX-License-Identifier: Apache-2.0

package mpr

// The System module's built-in Java actions now live in modelsdk/meta, beside
// the virtual System module's entities and associations. This package keeps the
// two names it exported so its own callers are unaffected, and delegates — two
// copies of a hand-maintained platform list is exactly how the two readers
// would come to disagree about what the System module contains.

import (
	"github.com/mendixlabs/mxcli/mdl/types"
	"github.com/mendixlabs/mxcli/modelsdk/meta"
	"github.com/mendixlabs/mxcli/sdk/javaactions"
)

// BuildSystemJavaActions returns lightweight types.JavaAction entries for the System module.
func BuildSystemJavaActions() []*types.JavaAction { return meta.BuildSystemJavaActions() }

// BuildSystemJavaActionsFull returns fully-typed javaactions.JavaAction entries for the System module.
func BuildSystemJavaActionsFull() []*javaactions.JavaAction {
	return meta.BuildSystemJavaActionsFull()
}
