// SPDX-License-Identifier: Apache-2.0

package types

// SnippetParameterTypeRule answers the one question both `mxcli check` and the
// snippet writer have to answer about a snippet parameter's declared type:
// is it a primitive, and if so what does mxbuild call it?
//
// A primitive snippet parameter is valid from Mendix 11.17 and refused below.
// Measured on Mendix 11.13.0 against a project whose only content was the
// snippets below (so nothing else could be the cause):
//
//	create snippet S.Label   ( params: { $Label: string } )
//	  → [error] [CE0046] "Invalid data type 'String'." at Snippet 'S.Label'
//	create snippet S.All     ( params: { $Label: String, $Count: Long,
//	                                     $Rank: Integer, $Amount: Decimal,
//	                                     $Active: Boolean, $Due: DateTime } )
//	  → one CE0046 per parameter, naming Studio Pro's caption for each
//	    ('String', 'Integer/Long' twice, 'Decimal', 'Boolean', 'Date and time')
//	create snippet S.Order   ( params: { $Order: S.Order } )
//	  → 0 errors
//
// And in the same run, the control that makes this a rule about SNIPPET
// parameters rather than about primitives: a PAGE declaring all six of those
// primitives as parameters builds at 0 errors.
//
// Re-measured across the 11.17 boundary with the same bytes: a snippet with
// five primitive parameters, inputs and a text template bound to them, and a
// page passing its own primitive parameters into the call. mx 11.16.0 reports
// one CE0046 per parameter; mx 11.17.0 reports 0 errors — and so does Studio
// Pro 11.17's own primitive snippet (ako/TestApp Snippet.SnippetPrimitiveParameter).
// The floor is registered as pages/snippet_parameter_primitive
// (sdk/versions/mendix-11.yaml); this function only names the type.
//
// Storage does not encode the restriction — Forms$SnippetParameter's
// ParameterType is the polymorphic DataTypes$DataType, exactly as
// Forms$PageParameter's is (generated/metamodel: PagesSnippetParameter
// .ParameterType *DataTypesDataType), so a primitive serializes perfectly well.
// Only mxbuild's validator says no, and the metamodel release notes record no
// change at 11.17. That is why the floor is a measurement, not a type gate.
//
// Returned string is the Studio Pro caption mxbuild quotes in CE0046 below
// 11.17, so the message can be checked against a build log verbatim; "" means
// not a primitive (an entity parameter).
func SnippetParameterTypeRule(bsonType string) (ce0046Caption string) {
	switch bsonType {
	case "DataTypes$StringType":
		return "String"
	case "DataTypes$IntegerType":
		// Studio Pro's single type for Integer and Long; storage has no LongType.
		return "Integer/Long"
	case "DataTypes$DecimalType":
		return "Decimal"
	case "DataTypes$BooleanType":
		return "Boolean"
	case "DataTypes$DateTimeType":
		return "Date and time"
	default:
		// "" is the entity case (no primitive $Type was resolved). An unmeasured
		// primitive is not guessed at — it falls through to mxbuild, which is
		// where the rule is actually enforced.
		return ""
	}
}
