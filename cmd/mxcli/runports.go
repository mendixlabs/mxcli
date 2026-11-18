// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strings"
)

// runPorts is the app and admin ports of a run: each flag that was given,
// else the selected configuration's port, else 0 (the run's own default).
// Studio Pro starts an app on its configuration's ports, and `run --local`
// listened on 8080/8090 whatever the configuration said (ako/mxcli#2365).
func runPorts(w io.Writer, appFlag, adminFlag int, c constantChain) (app, admin int) {
	app, admin = appFlag, adminFlag
	var from []string
	if app == 0 && c.AppPort > 0 {
		app = c.AppPort
		from = append(from, fmt.Sprintf("app %d", app))
	}
	if admin == 0 && c.AdminPort > 0 {
		admin = c.AdminPort
		from = append(from, fmt.Sprintf("admin %d", admin))
	}
	if len(from) > 0 {
		fmt.Fprintf(w, "Ports from configuration %q: %s (override with --app-port / --admin-port)\n",
			c.Configuration, strings.Join(from, ", "))
	}
	return app, admin
}
