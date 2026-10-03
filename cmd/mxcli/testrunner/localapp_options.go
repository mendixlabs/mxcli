// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"io"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

// ResolveTestDBType validates a `--db-type` for `mxcli test` and returns the value
// the runtime's DatabaseType parameter wants.
//
// It exists so the command layer can refuse a typo BEFORE it boots a runtime,
// rather than leaving the failure to surface as a connection error to a database
// the user never asked for. `run --local` refuses the same values at the same
// point (docker.NormalizeDBType), so one spelling works on both commands.
func ResolveTestDBType(raw string) (string, error) {
	kind, err := docker.NormalizeDBType(raw)
	if err != nil {
		return "", err
	}
	return docker.RuntimeDatabaseType(kind), nil
}

// localAppOptions builds the headless boot for a `--local` test run, shared by
// the endpoint runner and the legacy after-startup runner.
//
// The two runners differ only in what reaches the runtime's environment and in
// which log they read, so everything else — ports, the scratch database, and
// the constant values the app runs with — is decided in one place. It was the
// constants that made this worth sharing: they were absent from both runners,
// so a suite saw a different value under `--local` than the same suite saw
// under `--attach` (which runs against an app `run --local` booted, with the
// configuration's values applied). Nothing errored; the assertion just ran
// against the wrong constant. See docs/11-proposals/PROPOSAL_constant_values.md.
func localAppOptions(opts RunOptions, logPath string, env []string, w io.Writer) docker.LocalAppOptions {
	return docker.LocalAppOptions{
		ProjectPath: opts.ProjectPath,
		AppPort:     localTestAppPort,
		AdminPort:   localTestAdminPort,
		ServePort:   localTestServePort,
		// DeployDir is deliberately left at its default, <project dir>/deployment.
		// It is shared with a concurrent `mxcli run --local` — unlike the ports and
		// the database — because mxbuild writes the deployment there and has no
		// option to move it, so a scratch tree is one nothing populates
		// (mxcli-ledger §150). The damage that sharing used to do, a headless boot
		// deleting the browser bundle the running app serves, is undone by
		// StartLocalApp carrying the bundle across the boot (FINDINGS §62).
		DB:                dbConfig(opts),
		EnsureDB:          !dbConfig(opts).IsFileBased(),
		SkipBuild:         opts.SkipBuild,
		Env:               env,
		ConstantOverrides: opts.ConstantOverrides,
		MxBuildPath:       opts.MxBuildPath,
		RuntimeLogPath:    logPath,
		Stdout:            w,
		Stderr:            w,
	}
}

// dbConfig is the database the headless boot runs against.
//
// The scratch database is what lets a `run --local` dev loop keep serving the same
// project while tests run, and it is only meaningful for PostgreSQL. The built-in
// file database is a file under the project's own deployment directory, so there
// is nothing to keep separate and nothing to provision — a scratch NAME would aim
// the runtime at a database that does not exist, and EnsureDB would ask for a role
// and a database the file database does not have.
//
// The type is normalised here rather than trusted, so a RunOptions built by hand
// (or by a future caller that skipped ResolveTestDBType) still reaches the runtime
// in the spelling its DatabaseType parameter wants. An unknown value falls back to
// PostgreSQL, the historical default; the command layer refuses one outright via
// ResolveTestDBType, so this only ever sees a value that was already accepted.
func dbConfig(opts RunOptions) docker.DBConfig {
	kind, err := docker.NormalizeDBType(opts.DBType)
	if err != nil {
		kind = docker.DBTypePostgreSQL
	}
	cfg := docker.DBConfig{Type: docker.RuntimeDatabaseType(kind)}
	if cfg.IsFileBased() {
		return cfg
	}
	cfg.Name = docker.DeriveDBName(opts.ProjectPath) + localTestDBSuffix
	return cfg
}
