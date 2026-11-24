// SPDX-License-Identifier: Apache-2.0

package dmlayout

import "github.com/mendixlabs/mxcli/model"

// copyLayers copies an ordering of the layers, so a candidate swap does not
// change the ordering it was made from.
func copyLayers(layers [][]model.ID) [][]model.ID {
	out := make([][]model.ID, len(layers))
	for i, l := range layers {
		out[i] = append([]model.ID(nil), l...)
	}
	return out
}
