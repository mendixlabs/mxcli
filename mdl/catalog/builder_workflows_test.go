// SPDX-License-Identifier: Apache-2.0

package catalog

import (
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/sdk/workflows"
)

func TestCountWorkflowActivityTypes(t *testing.T) {
	tests := []struct {
		name                               string
		wf                                 *workflows.Workflow
		wantTotal, wantUT, wantMF, wantDec int
	}{
		{
			name:      "nil flow",
			wf:        &workflows.Workflow{},
			wantTotal: 0,
		},
		{
			name: "empty flow",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{},
			},
			wantTotal: 0,
		},
		{
			name: "user task",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.UserTask{},
					},
				},
			},
			wantTotal: 1,
			wantUT:    1,
		},
		{
			name: "call microflow task",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.CallMicroflowTask{},
					},
				},
			},
			wantTotal: 1,
			wantMF:    1,
		},
		{
			name: "system task counts as microflow call",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.SystemTask{},
					},
				},
			},
			wantTotal: 1,
			wantMF:    1,
		},
		{
			name: "exclusive split counts as decision",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.ExclusiveSplitActivity{},
					},
				},
			},
			wantTotal: 1,
			wantDec:   1,
		},
		{
			name: "nested activities in user task outcomes",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.UserTask{
							Outcomes: []*workflows.UserTaskOutcome{
								{
									Flow: &workflows.Flow{
										Activities: []workflows.WorkflowActivity{
											&workflows.CallMicroflowTask{},
										},
									},
								},
							},
						},
					},
				},
			},
			wantTotal: 2,
			wantUT:    1,
			wantMF:    1,
		},
		{
			name: "parallel split recurses into outcomes",
			wf: &workflows.Workflow{
				Flow: &workflows.Flow{
					Activities: []workflows.WorkflowActivity{
						&workflows.ParallelSplitActivity{
							Outcomes: []*workflows.ParallelSplitOutcome{
								{
									Flow: &workflows.Flow{
										Activities: []workflows.WorkflowActivity{
											&workflows.UserTask{},
										},
									},
								},
								{
									Flow: &workflows.Flow{
										Activities: []workflows.WorkflowActivity{
											&workflows.ExclusiveSplitActivity{},
										},
									},
								},
							},
						},
					},
				},
			},
			wantTotal: 3, // parallel split + user task + decision
			wantUT:    1,
			wantDec:   1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			total, ut, mf, dec := countWorkflowActivityTypes(tt.wf)
			if total != tt.wantTotal {
				t.Errorf("total = %d, want %d", total, tt.wantTotal)
			}
			if ut != tt.wantUT {
				t.Errorf("userTasks = %d, want %d", ut, tt.wantUT)
			}
			if mf != tt.wantMF {
				t.Errorf("microflowCalls = %d, want %d", mf, tt.wantMF)
			}
			if dec != tt.wantDec {
				t.Errorf("decisions = %d, want %d", dec, tt.wantDec)
			}
		})
	}
}
