// SPDX-License-Identifier: Apache-2.0

package modelsdkbackend

import (
	"fmt"

	"github.com/JordtenBulte-OLC/mxcli/model"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
	genDT "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/datatypes"
	genPg "github.com/JordtenBulte-OLC/mxcli/modelsdk/gen/pages"
	"github.com/JordtenBulte-OLC/mxcli/modelsdk/element"
	mmpr "github.com/JordtenBulte-OLC/mxcli/modelsdk/mpr"
	"github.com/JordtenBulte-OLC/mxcli/sdk/pages"
)

func init() {
	// A snippet always emits its Parameters and Variables arrays (empty = marker 3).
	codec.RegisterTypeDefaults("Forms$Snippet", codec.TypeDefaults{
		MandatoryLists: []string{"Parameters", "Variables"},
	})
}

// CreateSnippet inserts a new Forms$Snippet document — a reusable widget tree with
// its own parameters (entity-typed) and a flat Widgets list (no layout call).
func (b *Backend) CreateSnippet(snippet *pages.Snippet) error {
	if snippet == nil {
		return fmt.Errorf("CreateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("CreateSnippet: not connected for writing")
	}
	if snippet.ID == "" {
		snippet.ID = model.ID(mmpr.GenerateID())
	}
	g, err := snippetToGen(snippet)
	if err != nil {
		return err
	}
	g.SetID(element.ID(snippet.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("CreateSnippet: encode: %w", err)
	}
	if err := b.writer.InsertUnit(string(snippet.ID), string(snippet.ContainerID), "Documents", "Forms$Snippet", contents); err != nil {
		return fmt.Errorf("CreateSnippet: insert: %w", err)
	}
	return nil
}

// UpdateSnippet rewrites a snippet document (CREATE OR REPLACE).
func (b *Backend) UpdateSnippet(snippet *pages.Snippet) error {
	if snippet == nil {
		return fmt.Errorf("UpdateSnippet: nil snippet")
	}
	if b.writer == nil {
		return fmt.Errorf("UpdateSnippet: not connected for writing")
	}
	g, err := snippetToGen(snippet)
	if err != nil {
		return err
	}
	g.SetID(element.ID(snippet.ID))
	contents, err := (&codec.Encoder{}).Encode(g)
	if err != nil {
		return fmt.Errorf("UpdateSnippet: encode: %w", err)
	}
	if err := b.writer.UpdateRawUnit(string(snippet.ID), contents); err != nil {
		return fmt.Errorf("UpdateSnippet: update: %w", err)
	}
	return nil
}

// DeleteSnippet removes the snippet unit.
func (b *Backend) DeleteSnippet(id model.ID) error {
	if b.writer == nil {
		return fmt.Errorf("DeleteSnippet: not connected for writing")
	}
	return b.writer.DeleteUnit(string(id))
}

// snippetToGen builds the gen Snippet (header + parameters + widget tree).
func snippetToGen(s *pages.Snippet) (*genPg.Snippet, error) {
	out := genPg.NewSnippet()
	out.SetName(s.Name)
	out.SetDocumentation(s.Documentation)
	out.SetExcluded(false)
	out.SetExportLevel("Hidden")
	out.SetCanvasWidth(800)
	out.SetCanvasHeight(600)
	out.SetType("")

	for _, p := range s.Parameters {
		out.AddParameters(snippetParameterToGen(p))
	}
	for _, w := range s.Widgets {
		wg, err := widgetToGen(w)
		if err != nil {
			return nil, err
		}
		out.AddWidgets(wg)
	}
	return out, nil
}

// snippetParameterToGen builds a Forms$SnippetParameter (entity-typed).
func snippetParameterToGen(p *pages.SnippetParameter) *genPg.SnippetParameter {
	gp := genPg.NewSnippetParameter()
	if p.ID != "" {
		gp.SetID(element.ID(p.ID))
	}
	assignID(gp)
	gp.SetName(p.Name)
	t := genDT.NewObjectType()
	assignID(t)
	t.SetEntityQualifiedName(p.EntityName)
	gp.SetParameterType(t)
	return gp
}
