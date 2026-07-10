// SPDX-License-Identifier: Apache-2.0

// Supplement for code-action parameter value types whose gen stubs are
// empty (no decoded properties wired by the generator). Registers the
// factory so the codec returns the typed struct on load, and exposes a
// typed getter that reads the Expression field from raw BSON.

package microflows

import (
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
)

func init() {
	codec.DefaultRegistry.Register(
		"Microflows$ExpressionBasedCodeActionParameterValue",
		func() element.Element { return &ExpressionBasedCodeActionParameterValue{} },
	)
}

// Expression returns the Mendix expression string stored in this parameter value.
func (o *ExpressionBasedCodeActionParameterValue) Expression() string {
	v, _ := codec.ReadBSONFieldString(o.Raw(), "Expression")
	return v
}
