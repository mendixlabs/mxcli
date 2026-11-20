// SPDX-License-Identifier: Apache-2.0

package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
	"github.com/mendixlabs/mxcli/mdl/visitor"
)

// flowLayoutFixture connects an executor to a copy of the shared fixture. Its
// flows were drawn in Studio Pro; MyFirstModule is the project's own, while
// Administration and FeedbackModule come from the Marketplace.
func flowLayoutFixture(t *testing.T) (*bytes.Buffer, func(args ...string) error) {
	t.Helper()
	dst := t.TempDir()
	if err := testutil.CopyFixture(dst, "../../testdata/expr-checker"); err != nil {
		t.Fatalf("copy fixture: %v", err)
	}
	exec, logger := newLoggedExecutor("test")
	t.Cleanup(func() { logger.Close(); exec.Close() })
	exec.SetQuiet(true)
	prog, _ := visitor.Build("CONNECT LOCAL '" + visitor.QuoteString(filepath.Join(dst, "minimal.mpr")) + "'")
	for _, stmt := range prog.Statements {
		if err := exec.Execute(stmt); err != nil {
			t.Fatal(err)
		}
	}
	out := &bytes.Buffer{}
	return out, func(args ...string) error {
		targets, err := flowLayoutTargets(exec, args)
		if err != nil {
			return err
		}
		return layoutFlowTargets(out, exec, targets)
	}
}

func withLayoutFlags(t *testing.T, modules []string, dryRun, marketplace bool) {
	t.Helper()
	prevM, prevD, prevI := layoutModules, layoutDryRun, layoutIncludeMarketplace
	t.Cleanup(func() { layoutModules, layoutDryRun, layoutIncludeMarketplace = prevM, prevD, prevI })
	layoutModules, layoutDryRun, layoutIncludeMarketplace = modules, dryRun, marketplace
}

func TestLayoutFlows_NamedFlowIsResolvedByKind(t *testing.T) {
	withLayoutFlags(t, nil, true, true)
	out, layout := flowLayoutFixture(t)
	// One nanoflow and one microflow, the module spelled in the wrong case.
	if err := layout("feedbackmodule.ACT_SubmitFeedback", "FeedbackModule.SUB_Feedback_Sanitize"); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"FeedbackModule.ACT_SubmitFeedback:", "FeedbackModule.SUB_Feedback_Sanitize:", "Dry run: 2 flows would change"} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
}

// A typo must not report success having done nothing.
func TestLayoutFlows_UnknownFlowIsAnError(t *testing.T) {
	withLayoutFlags(t, nil, true, false)
	_, layout := flowLayoutFixture(t)
	err := layout("MyFirstModule.NoSuchFlow")
	if err == nil || !strings.Contains(err.Error(), "MyFirstModule.NoSuchFlow") {
		t.Errorf("err = %v, want one naming the missing flow", err)
	}
}

// A flow in a Marketplace module is refused as the module itself is by
// `mxcli layout`, unless --include-marketplace says otherwise.
func TestLayoutFlows_MarketplaceFlowNeedsTheFlag(t *testing.T) {
	withLayoutFlags(t, nil, true, false)
	out, layout := flowLayoutFixture(t)
	// Control: the project's own module needs no flag.
	if err := layout("MyFirstModule.MyFirstLogic"); err != nil {
		t.Fatalf("own module: %v", err)
	}
	if err := layout("Administration.ChangeMyPassword"); err == nil || !strings.Contains(err.Error(), "Marketplace") {
		t.Errorf("err = %v, want a Marketplace refusal", err)
	}

	withLayoutFlags(t, nil, true, true)
	out.Reset()
	if err := layout("Administration.ChangeMyPassword"); err != nil {
		t.Fatalf("with --include-marketplace: %v", err)
	}
	if !strings.Contains(out.String(), "would move") {
		t.Errorf("output: %s", out.String())
	}
}

// A refused flow is reported and the batch carries on.
func TestLayoutFlows_ModuleBatchSkipsWhatDoesNotRoundTrip(t *testing.T) {
	withLayoutFlags(t, []string{"FeedbackModule"}, true, true)
	out, layout := flowLayoutFixture(t)
	if err := layout(); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	if !strings.Contains(got, "FeedbackModule.VAL_Feedback: skipped") {
		t.Errorf("VAL_Feedback not reported as skipped:\n%s", got)
	}
	if !strings.Contains(got, "FeedbackModule.ACT_SubmitFeedback:") || !strings.Contains(got, "would move") {
		t.Errorf("the batch did not carry on past the refusal:\n%s", got)
	}
}
