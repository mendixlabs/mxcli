// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/mdl/types"
)

// Type aliases — all AsyncAPI types now live in mdl/types.
type AsyncAPIDocument = types.AsyncAPIDocument
type AsyncAPIChannel = types.AsyncAPIChannel
type AsyncAPIMessage = types.AsyncAPIMessage
type AsyncAPIProperty = types.AsyncAPIProperty

// ParseAsyncAPI delegates to types.ParseAsyncAPI.
func ParseAsyncAPI(yamlStr string) (*AsyncAPIDocument, error) {
	return types.ParseAsyncAPI(yamlStr)
}
