// SPDX-License-Identifier: Apache-2.0

//go:build !unix

package main

import "os"

// maxRSSMB is not measured on this platform.
func maxRSSMB(*os.ProcessState) float64 { return 0 }
