// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"context"
	"errors"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

func TestCredentialFromEnv_PAT(t *testing.T) {
	t.Setenv(EnvPAT, "env-pat")
	t.Setenv(EnvProfile, "")

	cred := credentialFromEnv(ProfileDefault)
	if cred == nil {
		t.Fatal("expected credential from env, got nil")
	}
	if cred.Token != "env-pat" || cred.Scheme != SchemePAT {
		t.Errorf("unexpected credential: %+v", cred)
	}
}

func TestCredentialFromEnv_ProfileMismatch(t *testing.T) {
	t.Setenv(EnvPAT, "env-pat")
	t.Setenv(EnvProfile, "other")

	if cred := credentialFromEnv(ProfileDefault); cred != nil {
		t.Errorf("expected nil (profile mismatch), got %+v", cred)
	}
	if cred := credentialFromEnv("other"); cred == nil {
		t.Errorf("expected credential for matching profile, got nil")
	}
}

func TestCredentialFromEnv_Unset(t *testing.T) {
	t.Setenv(EnvPAT, "")
	t.Setenv(EnvProfile, "")

	if cred := credentialFromEnv(ProfileDefault); cred != nil {
		t.Errorf("expected nil when env unset, got %+v", cred)
	}
}

func TestCredentialFromEnv_WhitespaceTrimmed(t *testing.T) {
	t.Setenv(EnvPAT, "  tok-with-ws  \n")
	t.Setenv(EnvProfile, "")

	cred := credentialFromEnv(ProfileDefault)
	if cred == nil || cred.Token != "tok-with-ws" {
		t.Errorf("expected trimmed token, got %+v", cred)
	}
}

func TestResolve_EnvWinsOverStore(t *testing.T) {
	// Point DefaultFileStore at a tempdir to avoid touching the real
	// ~/.mxcli/auth.json.
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "env-pat")
	t.Setenv(EnvProfile, "")

	// Store has a different token — env should still win.
	store, err := DefaultFileStore()
	if err != nil {
		t.Fatal(err)
	}
	_ = store.Put(ProfileDefault, &Credential{Scheme: SchemePAT, Token: "store-pat"})

	cred, err := Resolve(context.Background(), ProfileDefault)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "env-pat" {
		t.Errorf("expected env to win, got token=%q", cred.Token)
	}
}

func TestResolve_FallsBackToStore(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "")
	t.Setenv(EnvProfile, "")

	store, _ := DefaultFileStore()
	_ = store.Put(ProfileDefault, &Credential{Scheme: SchemePAT, Token: "store-pat"})

	cred, err := Resolve(context.Background(), ProfileDefault)
	if err != nil {
		t.Fatal(err)
	}
	if cred.Token != "store-pat" {
		t.Errorf("expected store token, got %q", cred.Token)
	}
}

func TestResolve_NoCredential(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "")
	t.Setenv(EnvProfile, "")

	_, err := Resolve(context.Background(), ProfileDefault)
	var noCred *ErrNoCredential
	if !errors.As(err, &noCred) {
		t.Errorf("expected ErrNoCredential, got %v", err)
	}
}

func TestResolve_EmptyProfileDefaultsToDefault(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "env-pat")
	t.Setenv(EnvProfile, "")

	cred, err := Resolve(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if cred.Profile != ProfileDefault {
		t.Errorf("expected profile=%q, got %q", ProfileDefault, cred.Profile)
	}
}
