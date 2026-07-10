// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/sdk/microflows"
)

// unknownMicroflowObject satisfies MicroflowObject but is not in the type switch.
type unknownMicroflowObject struct{}

func (u *unknownMicroflowObject) GetID() model.ID          { return "" }
func (u *unknownMicroflowObject) GetPosition() model.Point { return model.Point{} }
func (u *unknownMicroflowObject) SetPosition(model.Point)  {}

func TestGetMicroflowObjectType(t *testing.T) {
	tests := []struct {
		name string
		obj  microflows.MicroflowObject
		want string
	}{
		{"ActionActivity", &microflows.ActionActivity{}, "ActionActivity"},
		{"StartEvent", &microflows.StartEvent{}, "StartEvent"},
		{"EndEvent", &microflows.EndEvent{}, "EndEvent"},
		{"ExclusiveSplit", &microflows.ExclusiveSplit{}, "ExclusiveSplit"},
		{"InheritanceSplit", &microflows.InheritanceSplit{}, "InheritanceSplit"},
		{"ExclusiveMerge", &microflows.ExclusiveMerge{}, "ExclusiveMerge"},
		{"LoopedActivity", &microflows.LoopedActivity{}, "LoopedActivity"},
		{"Annotation", &microflows.Annotation{}, "Annotation"},
		{"BreakEvent", &microflows.BreakEvent{}, "BreakEvent"},
		{"ContinueEvent", &microflows.ContinueEvent{}, "ContinueEvent"},
		{"ErrorEvent", &microflows.ErrorEvent{}, "ErrorEvent"},
		{"unknown object falls to default", &unknownMicroflowObject{}, "MicroflowObject"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getMicroflowObjectType(tt.obj); got != tt.want {
				t.Errorf("getMicroflowObjectType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetMicroflowActionType(t *testing.T) {
	tests := []struct {
		name   string
		action microflows.MicroflowAction
		want   string
	}{
		{"CreateObjectAction", &microflows.CreateObjectAction{}, "CreateObjectAction"},
		{"ChangeObjectAction", &microflows.ChangeObjectAction{}, "ChangeObjectAction"},
		{"RetrieveAction", &microflows.RetrieveAction{}, "RetrieveAction"},
		{"MicroflowCallAction", &microflows.MicroflowCallAction{}, "MicroflowCallAction"},
		{"JavaActionCallAction", &microflows.JavaActionCallAction{}, "JavaActionCallAction"},
		{"ShowMessageAction", &microflows.ShowMessageAction{}, "ShowMessageAction"},
		{"LogMessageAction", &microflows.LogMessageAction{}, "LogMessageAction"},
		{"ValidationFeedbackAction", &microflows.ValidationFeedbackAction{}, "ValidationFeedbackAction"},
		{"ChangeVariableAction", &microflows.ChangeVariableAction{}, "ChangeVariableAction"},
		{"CreateVariableAction", &microflows.CreateVariableAction{}, "CreateVariableAction"},
		{"AggregateListAction", &microflows.AggregateListAction{}, "AggregateListAction"},
		{"ListOperationAction", &microflows.ListOperationAction{}, "ListOperationAction"},
		{"CastAction", &microflows.CastAction{}, "CastAction"},
		{"DownloadFileAction", &microflows.DownloadFileAction{}, "DownloadFileAction"},
		{"ClosePageAction", &microflows.ClosePageAction{}, "ClosePageAction"},
		{"ShowPageAction", &microflows.ShowPageAction{}, "ShowPageAction"},
		{"CallExternalAction", &microflows.CallExternalAction{}, "CallExternalAction"},
		// Previously these fell through to the generic "MicroflowAction" label
		// because the switch lacked a case for them; the type-derived label now
		// names them correctly.
		{"RestCallAction", &microflows.RestCallAction{}, "RestCallAction"},
		{"RestOperationCallAction", &microflows.RestOperationCallAction{}, "RestOperationCallAction"},
		{"WebServiceCallAction", &microflows.WebServiceCallAction{}, "WebServiceCallAction"},
		{"NanoflowCallAction", &microflows.NanoflowCallAction{}, "NanoflowCallAction"},
		{"JavaScriptActionCallAction", &microflows.JavaScriptActionCallAction{}, "JavaScriptActionCallAction"},
		{"ExecuteDatabaseQueryAction", &microflows.ExecuteDatabaseQueryAction{}, "ExecuteDatabaseQueryAction"},
		{"TransformJsonAction", &microflows.TransformJsonAction{}, "TransformJsonAction"},
		{"ImportXmlAction", &microflows.ImportXmlAction{}, "ImportXmlAction"},
		{"ExportXmlAction", &microflows.ExportXmlAction{}, "ExportXmlAction"},
		{"ShowHomePageAction", &microflows.ShowHomePageAction{}, "ShowHomePageAction"},
		// UnknownAction surfaces its real Mendix storage name (prefix stripped)
		// rather than a generic label.
		{"UnknownAction with prefixed storage name", &microflows.UnknownAction{TypeName: "Microflows$SomeFutureAction"}, "SomeFutureAction"},
		{"UnknownAction with bare type name", &microflows.UnknownAction{TypeName: "CustomThing"}, "CustomThing"},
		{"UnknownAction without type name", &microflows.UnknownAction{}, "UnknownAction"},
		{"nil action", nil, "MicroflowAction"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getMicroflowActionType(tt.action); got != tt.want {
				t.Errorf("getMicroflowActionType() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetDataTypeName(t *testing.T) {
	tests := []struct {
		name string
		dt   microflows.DataType
		want string
	}{
		{"nil", nil, ""},
		{"Boolean", &microflows.BooleanType{}, "Boolean"},
		{"Integer", &microflows.IntegerType{}, "Integer"},
		{"Long", &microflows.LongType{}, "Long"},
		{"Decimal", &microflows.DecimalType{}, "Decimal"},
		{"String", &microflows.StringType{}, "String"},
		{"DateTime", &microflows.DateTimeType{}, "DateTime"},
		{"Date", &microflows.DateType{}, "Date"},
		{"Void", &microflows.VoidType{}, "Void"},
		{"Object with entity", &microflows.ObjectType{EntityQualifiedName: "Module.Entity"}, "Object:Module.Entity"},
		{"List with entity", &microflows.ListType{EntityQualifiedName: "Module.Entity"}, "List:Module.Entity"},
		{"Enumeration", &microflows.EnumerationType{EnumerationQualifiedName: "Module.Color"}, "Enumeration:Module.Color"},
		{"Binary falls to Unknown", &microflows.BinaryType{}, "Unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := getDataTypeName(tt.dt); got != tt.want {
				t.Errorf("getDataTypeName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestCountMicroflowActivities(t *testing.T) {
	tests := []struct {
		name string
		mf   *microflows.Microflow
		want int
	}{
		{
			name: "nil object collection",
			mf:   &microflows.Microflow{},
			want: 0,
		},
		{
			name: "empty objects",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{},
			},
			want: 0,
		},
		{
			name: "excludes start/end/merge",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.StartEvent{},
						&microflows.ActionActivity{},
						&microflows.ExclusiveSplit{},
						&microflows.EndEvent{},
						&microflows.ExclusiveMerge{},
					},
				},
			},
			want: 2, // ActionActivity + ExclusiveSplit
		},
		{
			name: "counts loops and annotations",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.LoopedActivity{},
						&microflows.Annotation{},
						&microflows.ErrorEvent{},
					},
				},
			},
			want: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countMicroflowActivities(tt.mf); got != tt.want {
				t.Errorf("countMicroflowActivities() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateMcCabeComplexity(t *testing.T) {
	tests := []struct {
		name string
		mf   *microflows.Microflow
		want int
	}{
		{
			name: "nil object collection — base complexity",
			mf:   &microflows.Microflow{},
			want: 1,
		},
		{
			name: "no decision points",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.StartEvent{},
						&microflows.ActionActivity{},
						&microflows.EndEvent{},
					},
				},
			},
			want: 1,
		},
		{
			name: "exclusive split adds 1",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.ExclusiveSplit{},
					},
				},
			},
			want: 2,
		},
		{
			name: "inheritance split adds 1",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.InheritanceSplit{},
					},
				},
			},
			want: 2,
		},
		{
			name: "loop adds 1 plus nested decisions",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.LoopedActivity{
							ObjectCollection: &microflows.MicroflowObjectCollection{
								Objects: []microflows.MicroflowObject{
									&microflows.ExclusiveSplit{},
								},
							},
						},
					},
				},
			},
			want: 3, // 1 base + 1 loop + 1 nested split
		},
		{
			name: "error event adds 1",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.ErrorEvent{},
					},
				},
			},
			want: 2,
		},
		{
			name: "complex flow",
			mf: &microflows.Microflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.ExclusiveSplit{},
						&microflows.ExclusiveSplit{},
						&microflows.InheritanceSplit{},
						&microflows.LoopedActivity{},
						&microflows.ErrorEvent{},
					},
				},
			},
			want: 6, // 1 + 2 splits + 1 inheritance + 1 loop + 1 error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := calculateMcCabeComplexity(tt.mf); got != tt.want {
				t.Errorf("calculateMcCabeComplexity() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCountNanoflowActivities(t *testing.T) {
	tests := []struct {
		name string
		nf   *microflows.Nanoflow
		want int
	}{
		{
			name: "nil object collection",
			nf:   &microflows.Nanoflow{},
			want: 0,
		},
		{
			name: "excludes structural elements",
			nf: &microflows.Nanoflow{
				ObjectCollection: &microflows.MicroflowObjectCollection{
					Objects: []microflows.MicroflowObject{
						&microflows.StartEvent{},
						&microflows.ActionActivity{},
						&microflows.EndEvent{},
						&microflows.ExclusiveMerge{},
					},
				},
			},
			want: 1, // only ActionActivity
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := countNanoflowActivities(tt.nf); got != tt.want {
				t.Errorf("countNanoflowActivities() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestCalculateNanoflowComplexity(t *testing.T) {
	nf := &microflows.Nanoflow{
		ObjectCollection: &microflows.MicroflowObjectCollection{
			Objects: []microflows.MicroflowObject{
				&microflows.ExclusiveSplit{},
				&microflows.LoopedActivity{
					ObjectCollection: &microflows.MicroflowObjectCollection{
						Objects: []microflows.MicroflowObject{
							&microflows.InheritanceSplit{},
						},
					},
				},
			},
		},
	}

	got := calculateNanoflowComplexity(nf)
	want := 4 // 1 base + 1 split + 1 loop + 1 nested inheritance
	if got != want {
		t.Errorf("calculateNanoflowComplexity() = %d, want %d", got, want)
	}
}
