// SPDX-License-Identifier: Apache-2.0

// Supplement getters for executor DESCRIBE WORKFLOW formatting.
//
// The gen-typed Workflow describe code in mdl/executor reads BSON string
// fields that aren't surfaced through narrow gen getters, either because
// the receiver is an *element.Base fallback (the legacy $Type wasn't
// registered to a narrow gen struct) or because the gen stub for the
// type doesn't expose the field. Centralising the codec access here
// keeps mdl/executor free of direct modelsdk/codec imports (enforced by
// TestNoDirectBSONImportInExecutor).

package workflows

import "github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"

func readField(raw []byte, key string) string {
	v, _ := codec.ReadBSONFieldString(raw, key)
	return v
}

// RawFieldString reads a BSON string field directly from raw element
// bytes. Used by the describe formatter when the receiver decoded to
// *element.Base (unregistered $Type) or when the typed receiver has
// no narrow getter for the field being read.
func RawFieldString(raw []byte, key string) string {
	return readField(raw, key)
}

// AnnotationDescription reads the Description BSON field from an
// Annotation, mirroring the typed (*Annotation).Description() getter
// so the executor can stay codec-free.
func AnnotationDescription(o *Annotation) string {
	if o == nil {
		return ""
	}
	return readField(o.Raw(), "Description")
}

// AnnotationText reads the Text BSON field from an Annotation; used
// as a fallback when Description is empty (legacy storage).
func AnnotationText(o *Annotation) string {
	if o == nil {
		return ""
	}
	return readField(o.Raw(), "Text")
}

// AnnotationTextTranslation reads the Translation BSON field from a
// Texts$Text wrapper element bytes (the localised translation table).
func AnnotationTextTranslation(raw []byte) string {
	return readField(raw, "Translation")
}

// AnnotationTextValue reads the Value BSON field from a Texts$Text
// wrapper element bytes (the literal value).
func AnnotationTextValue(raw []byte) string {
	return readField(raw, "Value")
}

// WorkflowParameterEntity reads the Entity BSON field from a workflow
// Parameter Part when the narrow EntityQualifiedName() getter is
// unavailable (e.g. the Part decoded to *element.Base).
func WorkflowParameterEntity(o *Parameter) string {
	if o == nil {
		return ""
	}
	return readField(o.Raw(), "Entity")
}

// ExclusiveSplitOutcomeValue reads the Value BSON field from a generic
// ExclusiveSplitOutcome whose stored value is not one of the specialised
// outcome subtypes (boolean / enum / void).
func ExclusiveSplitOutcomeValue(o *ExclusiveSplitOutcome) string {
	if o == nil {
		return ""
	}
	return readField(o.Raw(), "Value")
}

// SystemTaskAnnotation reads the Annotation BSON field from a SystemTask
// element bytes (gen has no narrow SystemTask struct).
func SystemTaskAnnotation(raw []byte) string {
	return readField(raw, "Annotation")
}

// SystemTaskName reads the Name BSON field from a SystemTask element.
func SystemTaskName(raw []byte) string {
	return readField(raw, "Name")
}

// SystemTaskCaption reads the Caption BSON field from a SystemTask element.
func SystemTaskCaption(raw []byte) string {
	return readField(raw, "Caption")
}

// SystemTaskMicroflow reads the Microflow BSON field from a SystemTask element.
func SystemTaskMicroflow(raw []byte) string {
	return readField(raw, "Microflow")
}

// TimerActivityDelay reads the Delay BSON field from a timer boundary event.
func TimerActivityDelay(raw []byte) string {
	return readField(raw, "Delay")
}

// TimerActivityFirstExecutionTime reads the FirstExecutionTime BSON
// field from a timer boundary event (alter-workflow write path).
func TimerActivityFirstExecutionTime(raw []byte) string {
	return readField(raw, "FirstExecutionTime")
}
