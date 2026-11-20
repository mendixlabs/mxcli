// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"testing"

	"github.com/mendixlabs/mxcli/model"
	genPg "github.com/mendixlabs/mxcli/modelsdk/gen/pages"
	genTexts "github.com/mendixlabs/mxcli/modelsdk/gen/texts"
	"github.com/mendixlabs/mxcli/sdk/pages"
)

// ako/mxcli#2448 (mendixlabs/mxcli#1902): `call microflow … with (ProgressBar:
// Blocking)` and no ProgressMessage stored ProgressMessage as null. Studio Pro
// always stores a Texts$Text when a progress bar is set (an empty one for a
// blank field: 33 of 33 on a real 10.24 project), and the null crashed its
// page export with a NullReferenceException in MicroflowSettings.WriteProperties.
func TestProgressBarWithoutMessageWritesAnEmptyText_Issue2448(t *testing.T) {
	for _, bar := range []string{"Blocking", "NonBlocking"} {
		el, err := clientActionToGen(&pages.MicroflowClientAction{
			BaseElement:      model.BaseElement{ID: "a"},
			MicroflowName:    "M.Do",
			FlowCallSettings: pages.FlowCallSettings{ProgressBar: bar},
		})
		if err != nil {
			t.Fatal(err)
		}
		s := el.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings)
		if _, ok := s.ProgressMessage().(*genTexts.Text); !ok {
			t.Errorf("microflow, %s: ProgressMessage = %T, want an empty Texts$Text", bar, s.ProgressMessage())
		}

		el, err = clientActionToGen(&pages.NanoflowClientAction{
			BaseElement:      model.BaseElement{ID: "b"},
			NanoflowName:     "M.NDo",
			FlowCallSettings: pages.FlowCallSettings{ProgressBar: bar},
		})
		if err != nil {
			t.Fatal(err)
		}
		if _, ok := el.(*genPg.CallNanoflowClientAction).ProgressMessage().(*genTexts.Text); !ok {
			t.Errorf("nanoflow, %s: ProgressMessage = %T, want an empty Texts$Text", bar, el.(*genPg.CallNanoflowClientAction).ProgressMessage())
		}
	}

	// CONTROL: no progress bar keeps the null Studio Pro stores (210 of 210).
	el, _ := clientActionToGen(&pages.MicroflowClientAction{BaseElement: model.BaseElement{ID: "c"}, MicroflowName: "M.Do"})
	s := el.(*genPg.MicroflowClientAction).MicroflowSettings().(*genPg.MicroflowSettings)
	if txt, ok := s.ProgressMessage().(*genTexts.Text); ok && txt != nil {
		t.Errorf("no progress bar: ProgressMessage = %v, want null", txt)
	}
}
