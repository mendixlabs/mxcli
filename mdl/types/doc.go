// SPDX-License-Identifier: Apache-2.0

// Package types defines shared value types used in backend interfaces.
// These types are decoupled from sdk/mpr to avoid pulling in CGO
// dependencies, keeping the mdl/ subtree dependency-light.
// Conversion functions between these types and their sdk/mpr
// counterparts live in mdl/backend/mpr/.
package types
