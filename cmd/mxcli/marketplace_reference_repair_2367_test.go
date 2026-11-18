// SPDX-License-Identifier: Apache-2.0

package main

import (
	"errors"
	"io"
	"slices"
	"testing"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// ako/mxcli#2367: `marketplace install` repairs the app it writes into (mx
// update-widgets, rename-design-properties), but the reference `diff` compares
// against was built without them, so snippets whose stored widget props the
// repair rewrites (`popupmenu … clippingStrategy: 'absolute'`) read as edits.
// The reference gets the same repairs, in the same order.
func TestRepairReference_RunsTheInstallRepairs_Issue2367(t *testing.T) {
	var ran []string
	defer func(r func(string) (string, error), t func(string, string, string, io.Writer, io.Writer) (*docker.HarvestResult, error)) {
		postInstallResolveMx, postInstallRunTool = r, t
	}(postInstallResolveMx, postInstallRunTool)
	postInstallResolveMx = func(string) (string, error) { return "/mx", nil }
	postInstallRunTool = func(_, mpr, sub string, _, _ io.Writer) (*docker.HarvestResult, error) {
		ran = append(ran, sub+" "+mpr)
		return nil, nil
	}

	if err := repairReference("/ref/PackageRef.mpr", "11.12.6"); err != nil {
		t.Fatal(err)
	}
	want := []string{"update-widgets /ref/PackageRef.mpr", "rename-design-properties /ref/PackageRef.mpr"}
	if !slices.Equal(ran, want) {
		t.Errorf("ran %v, want %v (what install runs on the app)", ran, want)
	}

	// A repair that fails leaves a reference that would report false edits:
	// it is an error, so the reference is not cached.
	postInstallRunTool = func(_, _, _ string, _, _ io.Writer) (*docker.HarvestResult, error) {
		return nil, errors.New("boom")
	}
	if err := repairReference("/ref/PackageRef.mpr", "11.12.6"); err == nil {
		t.Error("a failed repair was not reported")
	}
}
