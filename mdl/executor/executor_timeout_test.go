// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"testing"
	"time"

	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
)

func TestConfiguredExecuteTimeoutUsesDurationEnv(t *testing.T) {
	t.Setenv("MXCLI_EXEC_TIMEOUT", "12m")

	if got := configuredExecuteTimeout(); got != 12*time.Minute {
		t.Fatalf("configured timeout = %v, want 12m", got)
	}
}

func TestConfiguredExecuteTimeoutUsesSecondEnv(t *testing.T) {
	t.Setenv("MXCLI_EXEC_TIMEOUT", "900")

	if got := configuredExecuteTimeout(); got != 15*time.Minute {
		t.Fatalf("configured timeout = %v, want 15m", got)
	}
}

func TestConfiguredExecuteTimeoutFallsBackForInvalidEnv(t *testing.T) {
	t.Setenv("MXCLI_EXEC_TIMEOUT", "invalid")

	if got := configuredExecuteTimeout(); got != defaultExecuteTimeout {
		t.Fatalf("configured timeout = %v, want default %v", got, defaultExecuteTimeout)
	}
}

// TestEffectiveExecuteTimeout covers the #651 fix: REFRESH CATALOG is exempt
// from the default wall-clock guard, an explicit MXCLI_EXEC_TIMEOUT always wins,
// and ordinary statements keep the default.
func TestEffectiveExecuteTimeout(t *testing.T) {
	t.Run("refresh catalog is exempt when no override set", func(t *testing.T) {
		t.Setenv("MXCLI_EXEC_TIMEOUT", "") // treated as unset
		if d := effectiveExecuteTimeout(&ast.RefreshCatalogStmt{Source: true}); d != 0 {
			t.Errorf("got %v, want 0 (no wall-clock guard)", d)
		}
	})

	t.Run("ordinary statement keeps the default", func(t *testing.T) {
		t.Setenv("MXCLI_EXEC_TIMEOUT", "")
		if d := effectiveExecuteTimeout(&ast.StatusStmt{}); d != defaultExecuteTimeout {
			t.Errorf("got %v, want %v", d, defaultExecuteTimeout)
		}
	})

	t.Run("explicit override wins even for exempt statements", func(t *testing.T) {
		t.Setenv("MXCLI_EXEC_TIMEOUT", "90s")
		if d := effectiveExecuteTimeout(&ast.RefreshCatalogStmt{}); d != 90*time.Second {
			t.Errorf("got %v, want 90s", d)
		}
	})

	t.Run("explicit override applies to ordinary statements too", func(t *testing.T) {
		t.Setenv("MXCLI_EXEC_TIMEOUT", "12m")
		if d := effectiveExecuteTimeout(&ast.StatusStmt{}); d != 12*time.Minute {
			t.Errorf("got %v, want 12m", d)
		}
	})
}

func TestIsExemptFromExecuteTimeout(t *testing.T) {
	if !isExemptFromExecuteTimeout(&ast.RefreshCatalogStmt{}) {
		t.Error("RefreshCatalogStmt should be exempt")
	}
	if isExemptFromExecuteTimeout(&ast.StatusStmt{}) {
		t.Error("StatusStmt should not be exempt")
	}
}
