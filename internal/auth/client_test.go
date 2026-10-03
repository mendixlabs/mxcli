// SPDX-License-Identifier: Apache-2.0

package auth

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/mendixlabs/mxcli/internal/testutil"
)

// rewriteTransport routes all requests to a given test server's URL while
// preserving the request's Host header for assertions. Lets us test the
// authTransport's host-based scheme routing against a real httptest.Server.
type rewriteTransport struct {
	target  *url.URL
	inner   http.RoundTripper
	seenURL string
}

func (r *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	r.seenURL = req.URL.String()
	req.URL.Scheme = r.target.Scheme
	req.URL.Host = r.target.Host
	return r.inner.RoundTrip(req)
}

func TestAuthTransport_InjectsPATHeader(t *testing.T) {
	var gotAuth string
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	target, _ := url.Parse(ts.URL)
	cred := &Credential{Profile: "default", Scheme: SchemePAT, Token: "my-pat"}
	client := &http.Client{
		Transport: &authTransport{
			cred:  cred,
			inner: &rewriteTransport{target: target, inner: http.DefaultTransport},
		},
	}

	// Use a known Mendix host so the scheme lookup succeeds.
	resp, err := client.Get("https://marketplace-api.mendix.com/v1/content/2888")
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if gotAuth != "MxToken my-pat" {
		t.Errorf("expected Authorization=MxToken my-pat, got %q", gotAuth)
	}
}

func TestAuthTransport_UnknownHost(t *testing.T) {
	cred := &Credential{Profile: "default", Scheme: SchemePAT, Token: "tok"}
	client := NewClient(cred)
	req, _ := http.NewRequest("GET", "https://example.com/", nil)
	_, err := client.Transport.RoundTrip(req)
	var unknown *ErrUnknownHost
	if !errors.As(err, &unknown) {
		t.Errorf("expected ErrUnknownHost, got %v", err)
	}
}

func TestAuthTransport_UnauthorizedStatusesWrap(t *testing.T) {
	// Mendix returns 401 when no credential is valid and 403 for
	// invalid/expired PATs (per portal docs). Both wrap as
	// ErrUnauthenticated.
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(status)
		}))

		target, _ := url.Parse(ts.URL)
		cred := &Credential{Profile: "default", Scheme: SchemePAT, Token: "bad"}
		client := &http.Client{
			Transport: &authTransport{
				cred:  cred,
				inner: &rewriteTransport{target: target, inner: http.DefaultTransport},
			},
		}

		resp, err := client.Get("https://marketplace-api.mendix.com/foo")
		if resp != nil {
			resp.Body.Close()
		}
		var unauth *ErrUnauthenticated
		if !errors.As(err, &unauth) {
			t.Errorf("status %d: expected ErrUnauthenticated, got %v", status, err)
		}
		if unauth != nil && unauth.Profile != "default" {
			t.Errorf("status %d: expected profile=default, got %q", status, unauth.Profile)
		}
		ts.Close()
	}
}

func TestAuthTransport_DoesNotMutateCallerRequest(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	target, _ := url.Parse(ts.URL)
	cred := &Credential{Profile: "default", Scheme: SchemePAT, Token: "tok"}
	tr := &authTransport{
		cred:  cred,
		inner: &rewriteTransport{target: target, inner: http.DefaultTransport},
	}

	req, _ := http.NewRequest("GET", "https://marketplace-api.mendix.com/foo", nil)
	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatalf("roundtrip: %v", err)
	}
	if req.Header.Get("Authorization") != "" {
		t.Errorf("caller request was mutated: %q", req.Header.Get("Authorization"))
	}
}

func TestClientFor_ResolvesFromEnv(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "env-pat")
	t.Setenv(EnvProfile, "")

	client, err := ClientFor(t.Context(), ProfileDefault)
	if err != nil {
		t.Fatalf("ClientFor: %v", err)
	}
	if client == nil {
		t.Fatal("nil client")
	}
	if client.Timeout == 0 {
		t.Error("expected non-zero timeout")
	}
}

func TestClientFor_NoCredential(t *testing.T) {
	testutil.SetHome(t, t.TempDir())
	t.Setenv(EnvPAT, "")
	t.Setenv(EnvProfile, "")

	_, err := ClientFor(t.Context(), ProfileDefault)
	var noCred *ErrNoCredential
	if !errors.As(err, &noCred) {
		t.Errorf("expected ErrNoCredential, got %v", err)
	}
}

func TestErrorMessages_IncludeHints(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{&ErrUnauthenticated{Profile: "ci"}, "mxcli auth login --profile ci"},
		{&ErrUnauthenticated{}, "mxcli auth login"},
		{&ErrNoCredential{Profile: "default"}, "mxcli auth login --profile default"},
		{&ErrSchemeMismatch{Host: "x.y", Need: SchemePAT, Have: "apikey"}, "x.y"},
		{&ErrUnknownHost{Host: "evil.com"}, "evil.com"},
		{&ErrPermissionsTooOpen{Path: "/f", Mode: 0o644}, "chmod 0600 /f"},
	}
	for _, tc := range cases {
		msg := tc.err.Error()
		if msg == "" {
			t.Errorf("%T: empty error message", tc.err)
		}
		if tc.want != "" && !contains(msg, tc.want) {
			t.Errorf("%T: message %q does not contain %q", tc.err, msg, tc.want)
		}
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestSchemeForHost(t *testing.T) {
	if s, ok := SchemeForHost("marketplace-api.mendix.com"); !ok || s != SchemePAT {
		t.Errorf("marketplace-api host should map to PAT, got (%q, %v)", s, ok)
	}
	if s, ok := SchemeForHost("catalog.mendix.com"); !ok || s != SchemePAT {
		t.Errorf("catalog host should map to PAT, got (%q, %v)", s, ok)
	}
	if _, ok := SchemeForHost("evil.example.com"); ok {
		t.Errorf("unknown host should return false")
	}
}
