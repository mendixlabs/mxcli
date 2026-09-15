// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"strings"

	modelsdk "github.com/mendixlabs/mxcli"
	"github.com/mendixlabs/mxcli/internal/marketplace"
)

// mendixVersionOf reports the project's Mendix version, or "" when the project
// cannot be opened or records none. Callers treat "" as "cannot evaluate".
func mendixVersionOf(mprPath string) string {
	reader, err := modelsdk.Open(mprPath)
	if err != nil {
		return ""
	}
	defer reader.Disconnect()
	v, _ := reader.GetMendixVersion()
	return v
}

// checkMendixCompatibility refuses a marketplace version the project's Mendix
// version cannot import, and names the newest version that it can.
//
// Every version the API returns carries `minSupportedMendixVersion`, and the
// marketplace publishes new releases against the newest Studio Pro patch within
// days of it shipping — so on any project that is not on the very latest patch,
// the *default* (latest) version is routinely the one that cannot be installed.
// Measured 2026-08-12 on a project at 11.12.1: the latest release of all six
// agent-stack modules required 11.12.2, published five days earlier.
//
// Without this check the refusal still happens, but three layers down and in a
// form that reads like an mxcli failure: the package is downloaded, a reference
// project is built, and `mx module-import` exits 117 with "the package ... was
// created with a newer version of Mendix Studio Pro". Checking here turns that
// into one line naming the version to pass instead.
//
// A version whose minimum cannot be parsed is allowed through rather than
// refused — the check exists to give a better error, never to block an install
// it cannot evaluate.
func checkMendixCompatibility(v *marketplace.Version, all []marketplace.Version, projectVersion, contentName string) error {
	if v == nil || v.MinSupportedMendixVersion == "" || projectVersion == "" {
		return nil
	}
	if compareSemverLike(v.MinSupportedMendixVersion, projectVersion) <= 0 {
		return nil
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s %s requires Mendix %s, and the project is %s",
		contentName, v.VersionNumber, v.MinSupportedMendixVersion, projectVersion)
	if best := newestCompatibleVersion(all, projectVersion); best != "" {
		fmt.Fprintf(&b, "\n  hint: install --version %s (the newest release built for %s or older)",
			best, projectVersion)
	} else {
		fmt.Fprintf(&b, "\n  hint: no published version supports Mendix %s; upgrade the project first",
			projectVersion)
	}
	return fmt.Errorf("%s", b.String())
}

// newestCompatibleVersion returns the highest version number whose minimum
// Mendix version the project satisfies, or "" when none does. The API returns
// versions newest-first, so the first match is the answer; it is not re-sorted
// here, because version numbers are publisher-controlled strings and the
// publication order is the only ranking the API actually guarantees.
func newestCompatibleVersion(all []marketplace.Version, projectVersion string) string {
	for i := range all {
		if all[i].MinSupportedMendixVersion == "" {
			continue
		}
		if compareSemverLike(all[i].MinSupportedMendixVersion, projectVersion) <= 0 {
			return all[i].VersionNumber
		}
	}
	return ""
}
