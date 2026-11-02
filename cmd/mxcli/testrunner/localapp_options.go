// SPDX-License-Identifier: Apache-2.0

package testrunner

import (
	"fmt"
	"io"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
)

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
	db := dbConfig(opts)
	return docker.LocalAppOptions{
		ProjectPath: opts.ProjectPath,
		AppPort:     opts.appPort(),
		AdminPort:   opts.adminPort(),
		ServePort:   opts.servePort(),
		// DeployDir is deliberately left at its default, <project dir>/deployment.
		// It is shared with a concurrent `mxcli run --local` — unlike the ports and
		// the database — because mxbuild writes the deployment there and has no
		// option to move it, so a scratch tree is one nothing populates
		// (mxcli-ledger §150). The damage that sharing used to do, a headless boot
		// deleting the browser bundle the running app serves, is undone by
		// StartLocalApp carrying the bundle across the boot (FINDINGS §62).
		DB: db,
		// Provisioning is PostgreSQL's; the file database is created by the runtime.
		EnsureDB: !db.IsFileBased(),
		// The database is this run's scratch, so it starts empty. Kept between
		// runs, data a previous run committed or seeded under an earlier model
		// made an idempotent init skip seeding, and the tests failed on stale
		// data (ako/mxcli#1853) — on PostgreSQL as on the file database.
		FreshDB:           true,
		SkipBuild:         opts.SkipBuild,
		Env:               env,
		ConstantOverrides: opts.ConstantOverrides,
		JVMArgs:           opts.JVMArgs,
		MxBuildPath:       opts.MxBuildPath,
		RuntimeLogPath:    logPath,
		Stdout:            w,
		Stderr:            w,
	}
}

// dbConfig is the scratch database a --local run boots against, so a
// `run --local` dev loop can keep serving the same project while the tests run.
//
// The suffix applies to the file database too: there the name selects the
// database files, and the unsuffixed ones are the dev loop's — sharing them
// would let test writes land in the developer's data and contend for HSQLDB's
// file lock with a running `run --local --db-type hsqldb`.
//
// Type is translated to the runtime spelling because DBConfig.IsFileBased keys
// on it. opts.DBType is assumed canonical (see ResolveTestDBType); anything
// that is not the file database boots on PostgreSQL, as before the flag.
func dbConfig(opts RunOptions) docker.DBConfig {
	kind := docker.DBTypePostgreSQL
	if docker.IsFileBasedDBType(opts.DBType) {
		kind = docker.DBTypeHSQLDB
	}
	return docker.DBConfig{
		Type: docker.RuntimeDatabaseType(kind),
		Name: docker.DeriveDBName(opts.ProjectPath) + localTestDBSuffix,
	}
}

// ResolveTestDBType validates `mxcli test --db-type` and returns the canonical
// type for RunOptions.DBType. Only a --local run that boots its own app has a
// database to choose; --attach uses the database of the app it attaches to and
// the Docker path configures the container, so the flag is refused there rather
// than accepted and ignored. A value outside postgresql/hsqldb is refused before
// anything is built.
func ResolveTestDBType(raw string, local, attach bool) (string, error) {
	if !local || attach {
		if raw != "" {
			return "", fmt.Errorf("--db-type applies only to a --local test run " +
				"(--attach uses the database of the app it attaches to; the Docker path configures the container's)")
		}
		return "", nil
	}
	return docker.NormalizeDBType(raw)
}
