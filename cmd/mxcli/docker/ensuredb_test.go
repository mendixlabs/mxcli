// SPDX-License-Identifier: Apache-2.0

package docker

import (
	"io"
	"testing"
)

func TestSplitHostPort(t *testing.T) {
	cases := []struct{ in, host, port string }{
		{"127.0.0.1:5432", "127.0.0.1", "5432"},
		{"db.example.com:6000", "db.example.com", "6000"},
		{"localhost", "localhost", "5432"},
	}
	for _, c := range cases {
		h, p := splitHostPort(c.in)
		if h != c.host || p != c.port {
			t.Errorf("splitHostPort(%q) = (%q,%q), want (%q,%q)", c.in, h, p, c.host, c.port)
		}
	}
}

func TestIsLocalHost(t *testing.T) {
	for _, h := range []string{"127.0.0.1", "localhost", "::1", ""} {
		if !isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = false, want true", h)
		}
	}
	for _, h := range []string{"db.example.com", "10.0.0.5", "postgres"} {
		if isLocalHost(h) {
			t.Errorf("isLocalHost(%q) = true, want false", h)
		}
	}
}

func TestQuoteSQLString(t *testing.T) {
	if got := quoteSQLString("mendix"); got != "'mendix'" {
		t.Errorf("quoteSQLString = %q", got)
	}
	if got := quoteSQLString("it's"); got != "'it''s'" {
		t.Errorf("quoteSQLString(apostrophe) = %q, want doubled", got)
	}
}

func TestEnsureDatabase_Validation(t *testing.T) {
	// Non-Postgres type is rejected.
	if err := EnsureDatabase(DBConfig{Type: "HSQLDB", Name: "a", User: "u"}, io.Discard); err == nil {
		t.Error("expected error for non-PostgreSQL type")
	}
	// Unsafe identifiers are rejected (before any exec).
	if err := EnsureDatabase(DBConfig{Type: "PostgreSQL", Name: "bad-name;DROP", User: "u", Host: "127.0.0.1:5432"}, io.Discard); err == nil {
		t.Error("expected error for unsafe database name")
	}
	if err := EnsureDatabase(DBConfig{Type: "PostgreSQL", Name: "ok", User: "bad user", Host: "127.0.0.1:5432"}, io.Discard); err == nil {
		t.Error("expected error for unsafe database user")
	}
}
