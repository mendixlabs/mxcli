// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"
)

// ako/mxcli#2528 (mendixlabs/mxcli#1923): an aggregate by an attribute that is
// not a number passed check and exec, and mxbuild then failed it:
//
//	[CE0016] "Attribute must be of type Autonumber, Decimal, Integer, or Long."
//	         at Aggregate list activity 'Take minimum of attribute 'DueDate' in Items'
//
// Measured on 11.12.6, one microflow each: minimum, maximum, sum and average
// over a DateTime and over a String attribute — all eight CE0016; the same
// four over an Integer build clean.
func TestAggregateByNonNumericAttribute_Refused_2528(t *testing.T) {
	for _, stmt := range []string{
		"$m = minimum $l by Placed;",
		"$m = maximum $l by Placed;",
		"$m = sum $l by Number;",
		"$m = average $l by Number;",
	} {
		t.Run(stmt, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			src := aggregateTypeHead + fmt.Sprintf(listMemberFlow, stmt)
			assertAgree(t, exec, dir, src, "CE0016")
		})
	}
}

// Controls: every numeric kind builds, and an attribute whose type is not
// known here (inherited) is not judged.
func TestAggregateByNumericAttribute_Accepted_2528(t *testing.T) {
	for _, stmt := range []string{
		"$m = minimum $l by Total;",
		"$m = sum $l by Qty;",
		"$m = maximum $l by Big;",
		"$m = average $l by Seq;",
		"$m = sum $sl by Total;",
	} {
		t.Run(stmt, func(t *testing.T) {
			exec, _, dir := openPedAppCopy(t)
			assertAgree(t, exec, dir, aggregateTypeHead+fmt.Sprintf(listMemberFlow, stmt), "")
		})
	}
}

const aggregateTypeHead = `mdl 1;
create module B1646;
create module O1646;
create persistent entity B1646.Cust (Name: String(50));
create persistent entity B1646.Ord (Number: String(20), Total: Decimal, Qty: Integer, Big: Long, Seq: AutoNumber default 1, Placed: DateTime);
create persistent entity B1646.SubOrd extends B1646.Ord (Extra: String(20));
create persistent entity O1646.Base (BaseName: String(20));
create persistent entity O1646.Thing (ThingName: String(20));
create persistent entity B1646.Spec extends O1646.Base (SpecName: String(20));
`
