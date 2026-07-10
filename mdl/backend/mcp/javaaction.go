// SPDX-License-Identifier: Apache-2.0

package mcp

import (
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// Java actions cannot be authored over MCP. PED refuses to create the
// JavaActions$JavaAction document type outright ("Document type
// 'JavaActions$JavaAction' cannot be created."), because a Java action is backed
// by a .java source file Studio Pro generates and manages — the model document
// can't be conjured standalone. CREATE / CREATE OR MODIFY are gated on the capability
// model (capabilities.yaml / javaaction_create); they reject today and would light
// up only if a future server lifts the limit and a create path is built.
//
// Calling a Java action from a microflow is unaffected — that is supported (see
// JavaActionCallAction in microflow.go); only authoring the action document is not.

func (b *Backend) CreateJavaAction(ja *javaactions.JavaAction) error {
	if !b.canAuthor(capJavaActionCreate) {
		return b.notAuthorable("java action", ja.Name, capJavaActionCreate)
	}
	return errCreatePathUnbuilt("java action", ja.Name)
}

func (b *Backend) UpdateJavaAction(ja *javaactions.JavaAction) error {
	if !b.canAuthor(capJavaActionCreate) {
		return b.notAuthorable("java action", ja.Name, capJavaActionCreate)
	}
	return errCreatePathUnbuilt("java action", ja.Name)
}
