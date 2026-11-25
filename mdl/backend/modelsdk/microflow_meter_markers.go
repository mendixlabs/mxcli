// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "github.com/mendixlabs/mxcli/modelsdk/codec"

func init() {
	// The metrics activities' Tags (Microflows$MeterTagMapping) list is marker 2.
	// Measured: Studio Pro 11.16.1, one Counter, one Gauge and one Increment
	// Counter activity, each with one tag (ako/TestApp, BusinessEventsClient.
	// MetricsMf) — all three [2, …]. Without it the encoder's default 3 differs
	// from every Studio Pro-authored activity.
	//
	// Mandatory: a new activity with no tags still writes `Tags: [2]`, so its
	// property set is Studio Pro's (CLAUDE.md "Write Studio Pro's exact property
	// set"); the empty form follows the measured marker of the non-empty one.
	for _, owner := range []string{
		"Microflows$CounterMeterAction",
		"Microflows$IncrementCounterMeterAction",
		"Microflows$GaugeMeterAction",
	} {
		codec.RegisterMandatoryPropertyList(owner, "Tags", 2)
	}
	codec.RegisterListMarker("Microflows$MeterTagMapping", 2)
}
