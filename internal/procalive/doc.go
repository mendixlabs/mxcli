// SPDX-License-Identifier: Apache-2.0

// Package procalive answers one question: is the process with this pid still
// running?
//
// It exists because the portable-looking answer, os.FindProcess(pid) followed by
// Signal(0), is wrong on Windows: Go supports no signal but Kill there, so every
// pid reads as dead (mendixlabs/mxcli#1284). `mxcli test --attach` and the dev
// loop's "already serving" check both read a pid out of a handshake file, so on
// Windows they refused a live host. The per-OS files hold the one check that
// works on each platform; callers that only have a pid use Alive.
package procalive
