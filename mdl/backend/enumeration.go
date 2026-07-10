// SPDX-License-Identifier: Apache-2.0

package backend

import (
	"github.com/JordtenBulte-OLC/mxcli/model"
)

// EnumerationBackend provides enumeration operations.
type EnumerationBackend interface {
	ListEnumerations() ([]*model.Enumeration, error)
	GetEnumeration(id model.ID) (*model.Enumeration, error)
	CreateEnumeration(enum *model.Enumeration) error
	UpdateEnumeration(enum *model.Enumeration) error
	MoveEnumeration(enum *model.Enumeration) error
	DeleteEnumeration(id model.ID) error
}

// ConstantBackend provides constant operations.
type ConstantBackend interface {
	ListConstants() ([]*model.Constant, error)
	GetConstant(id model.ID) (*model.Constant, error)
	CreateConstant(constant *model.Constant) error
	UpdateConstant(constant *model.Constant) error
	MoveConstant(constant *model.Constant) error
	DeleteConstant(id model.ID) error
}
