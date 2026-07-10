// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"fmt"
	"testing"

	"github.com/JordtenBulte-OLC/mxcli/mdl/backend/mock"
	"github.com/JordtenBulte-OLC/mxcli/model"
)

func TestShowSettings_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, listSettings(ctx))

	out := buf.String()
	assertContainsStr(t, out, "Section")
	assertContainsStr(t, out, "Key Values")
}

func TestDescribeSettings_Mock(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
					RoundingMode:  "HalfUp",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb))
	assertNoError(t, describeSettings(ctx))
	assertContainsStr(t, buf.String(), "alter settings")
}

func TestShowSettings_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listSettings(ctx))
}

func TestDescribeSettings_NotConnected(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return false },
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, describeSettings(ctx))
}

func TestShowSettings_BackendError(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return nil, fmt.Errorf("connection lost")
		},
	}
	ctx, _ := newMockCtx(t, withBackend(mb))
	assertError(t, listSettings(ctx))
}

func TestShowSettings_JSON(t *testing.T) {
	mb := &mock.MockBackend{
		IsConnectedFunc: func() bool { return true },
		GetProjectSettingsFunc: func() (*model.ProjectSettings, error) {
			return &model.ProjectSettings{
				Model: &model.ModelSettings{
					HashAlgorithm: "BCrypt",
					JavaVersion:   "17",
				},
			}, nil
		},
	}
	ctx, buf := newMockCtx(t, withBackend(mb), withFormat(FormatJSON))
	assertNoError(t, listSettings(ctx))
	assertValidJSON(t, buf.String())
}
