// SPDX-License-Identifier: Apache-2.0

package mpr

import (
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"

	"go.mongodb.org/mongo-driver/bson"
)

// serializeTextBox serializes a TextBox widget.
func serializeTextBox(tb *pages.TextBox) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(tb.ID))},
		{Key: "$Type", Value: "Forms$TextBox"},
		{Key: "Appearance", Value: serializeAppearance(tb.Class, tb.Style, tb.DynamicClasses, tb.DesignProperties)},
		{Key: "AriaRequired", Value: false},
		{Key: "AttributeRef", Value: serializeAttributeRef(tb.AttributePath)},
		{Key: "AutoFocus", Value: false},
		{Key: "Autocomplete", Value: true},
		{Key: "AutocompletePurpose", Value: "On"},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "FormattingInfo", Value: serializeFormattingInfo()},
		{Key: "InputMask", Value: ""},
		{Key: "IsPasswordBox", Value: tb.IsPassword},
		{Key: "KeyboardType", Value: "Default"},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(tb.Label)},
		{Key: "MaxLengthCode", Value: int64(-1)},
		{Key: "Name", Value: tb.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterKeyPressAction", Value: serializeClientAction(nil)},
		{Key: "OnLeaveAction", Value: serializeClientAction(nil)},
		{Key: "PlaceholderTemplate", Value: serializeEmptyPlaceholderTemplate()},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "SubmitBehaviour", Value: "OnEndEditing"},
		{Key: "SubmitOnInputDelay", Value: int64(300)},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}

// serializeTextArea serializes a TextArea widget.
func serializeTextArea(ta *pages.TextArea) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(ta.ID))},
		{Key: "$Type", Value: "Forms$TextArea"},
		{Key: "Appearance", Value: serializeAppearance(ta.Class, ta.Style, ta.DynamicClasses, ta.DesignProperties)},
		{Key: "AriaRequired", Value: false},
		{Key: "AttributeRef", Value: serializeAttributeRef(ta.AttributePath)},
		{Key: "AutoFocus", Value: false},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "CounterMessage", Value: serializeEmptyText()},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(ta.Label)},
		{Key: "MaxLengthCode", Value: int64(-1)},
		{Key: "Name", Value: ta.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "NumberOfLines", Value: int64(5)},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "OnLeaveAction", Value: serializeClientAction(nil)},
		{Key: "PlaceholderTemplate", Value: serializeEmptyPlaceholderTemplate()},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "SubmitBehaviour", Value: "OnEndEditing"},
		{Key: "SubmitOnInputDelay", Value: int64(300)},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}

// serializeDatePicker serializes a DatePicker widget.
func serializeDatePicker(dp *pages.DatePicker) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dp.ID))},
		{Key: "$Type", Value: "Forms$DatePicker"},
		{Key: "Appearance", Value: serializeAppearance(dp.Class, dp.Style, dp.DynamicClasses, dp.DesignProperties)},
		{Key: "AriaRequired", Value: false},
		{Key: "AttributeRef", Value: serializeAttributeRef(dp.AttributePath)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "DateFormat", Value: "Date"},
		{Key: "Editable", Value: "Always"},
		{Key: "FormattingInfo", Value: serializeFormattingInfo()},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(dp.Label)},
		{Key: "Name", Value: dp.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "PlaceholderTemplate", Value: serializeEmptyPlaceholderTemplate()},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}

// serializeCheckBox serializes a CheckBox widget.
func serializeCheckBox(cb *pages.CheckBox) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(cb.ID))},
		{Key: "$Type", Value: "Forms$CheckBox"},
		{Key: "Appearance", Value: serializeAppearance(cb.Class, cb.Style, cb.DynamicClasses, cb.DesignProperties)},
		{Key: "AttributeRef", Value: serializeAttributeRef(cb.AttributePath)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(cb.Label)},
		{Key: "Name", Value: cb.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}

// serializeRadioButtons serializes a RadioButtons widget.
func serializeRadioButtons(rb *pages.RadioButtons) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(rb.ID))},
		{Key: "$Type", Value: "Forms$RadioButtonGroup"},
		{Key: "Appearance", Value: serializeAppearance(rb.Class, rb.Style, rb.DynamicClasses, rb.DesignProperties)},
		{Key: "AriaRequired", Value: false},
		{Key: "AttributeRef", Value: serializeAttributeRef(rb.AttributePath)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(rb.Label)},
		{Key: "Name", Value: rb.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "Orientation", Value: "Horizontal"},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}

// serializeDropDown serializes a DropDown widget.
func serializeDropDown(dd *pages.DropDown) bson.D {
	return bson.D{
		{Key: "$ID", Value: idToBsonBinary(string(dd.ID))},
		{Key: "$Type", Value: "Forms$DropDown"},
		{Key: "Appearance", Value: serializeAppearance(dd.Class, dd.Style, dd.DynamicClasses, dd.DesignProperties)},
		{Key: "AriaRequired", Value: false},
		{Key: "AttributeRef", Value: serializeAttributeRef(dd.AttributePath)},
		{Key: "ConditionalEditabilitySettings", Value: nil},
		{Key: "ConditionalVisibilitySettings", Value: nil},
		{Key: "Editable", Value: "Always"},
		{Key: "EmptyOptionCaption", Value: bson.D{
			{Key: "$ID", Value: idToBsonBinary(generateUUID())},
			{Key: "$Type", Value: "Texts$Text"},
			{Key: "Items", Value: bson.A{int32(3)}},
		}},
		{Key: "LabelTemplate", Value: serializeLabelTemplate(dd.Label)},
		{Key: "Name", Value: dd.Name},
		{Key: "NativeAccessibilitySettings", Value: nil},
		{Key: "OnChangeAction", Value: serializeClientAction(nil)},
		{Key: "OnEnterAction", Value: serializeClientAction(nil)},
		{Key: "OnLeaveAction", Value: serializeClientAction(nil)},
		{Key: "ReadOnlyStyle", Value: "Inherit"},
		{Key: "ScreenReaderLabel", Value: nil},
		{Key: "SourceVariable", Value: nil},
		{Key: "TabIndex", Value: int64(0)},
		{Key: "Validation", Value: serializeWidgetValidation()},
	}
}
