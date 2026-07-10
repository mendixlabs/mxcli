// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/javaactions"
)

// JavaBackend provides Java and JavaScript action operations.
type JavaBackend interface {
	ListJavaActions() ([]*types.JavaAction, error)
	ListJavaActionsFull() ([]*javaactions.JavaAction, error)
	ListJavaScriptActions() ([]*types.JavaScriptAction, error)
	ReadJavaActionByName(qualifiedName string) (*javaactions.JavaAction, error)
	ReadJavaScriptActionByName(qualifiedName string) (*types.JavaScriptAction, error)
	CreateJavaAction(ja *javaactions.JavaAction) error
	UpdateJavaAction(ja *javaactions.JavaAction) error
	DeleteJavaAction(id model.ID) error
	WriteJavaSourceFile(moduleName, actionName string, javaCode string, params []*javaactions.JavaActionParameter, returnType javaactions.CodeActionReturnType, extraImports []string, extraCode string) error
	DeleteJavaSourceFile(moduleName, actionName string) error
	RenameJavaSourceFile(moduleName, oldName, newName string) error
	ReadJavaSourceFile(moduleName, actionName string) (string, error)

	CreateJavaScriptAction(jsa *types.JavaScriptAction) error
	UpdateJavaScriptAction(jsa *types.JavaScriptAction) error
	DeleteJavaScriptAction(id model.ID) error
	WriteJavaScriptSourceFile(moduleName, actionName string, jsCode string, params []*types.JavaActionParameter, returnType types.CodeActionReturnType) error
	DeleteJavaScriptSourceFile(moduleName, actionName string) error
	RenameJavaScriptSourceFile(moduleName, oldName, newName string) error
}
