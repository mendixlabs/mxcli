// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import "testing"

// An AutoNumber attribute's mapping value fell through to DataTypes$StringType
// and XmlPrimitiveType String; its schema element is an Integer, so mx check
// failed CE0227 + CE5015 (ako/mxcli#2603). It is an integer, as Long is.
func TestMappingValueType_AutoNumberIsAnInteger_2603(t *testing.T) {
	if got := mappingValueDataTypeToGen("AutoNumber").TypeName(); got != "DataTypes$IntegerType" {
		t.Errorf("AutoNumber Type = %s, want DataTypes$IntegerType", got)
	}
	if got := xmlPrimitiveTypeName("AutoNumber"); got != "Integer" {
		t.Errorf("AutoNumber XmlPrimitiveType = %s, want Integer", got)
	}
	// Control: a String stays a String.
	if got := mappingValueDataTypeToGen("String").TypeName(); got != "DataTypes$StringType" {
		t.Errorf("String Type = %s", got)
	}
}
