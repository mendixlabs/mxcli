// SPDX-License-Identifier: Apache-2.0

package main

import (
	"strings"
	"testing"

	"github.com/mendixlabs/mxcli/model"
)

// ako/mxcli#2365 (mendixlabs/mxcli#1880): with the selected configuration
// saying HttpPortNumber 8085 and ServerPortNumber 8095, `run --local
// --configuration Default` listened on 8080 and 8090; only --app-port and
// --admin-port moved it. The configuration's ports are the run's ports, and
// the flags override them.

func portsCfg(name string, http, admin int) *model.ServerConfiguration {
	return &model.ServerConfiguration{Name: name, HttpPortNumber: http, ServerPortNumber: admin}
}

func TestResolveConstantChain_CarriesTheConfigurationsPorts_Issue2365(t *testing.T) {
	ps := settingsWith(portsCfg("Default", 8085, 8095), portsCfg("Other", 9000, 9001))
	chain, err := resolveConstantChain(ps, "Default", nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if chain.AppPort != 8085 || chain.AdminPort != 8095 {
		t.Errorf("ports = %d/%d, want Default's 8085/8095", chain.AppPort, chain.AdminPort)
	}
}

func TestRunPorts_Issue2365(t *testing.T) {
	chain := constantChain{Configuration: "Default", AppPort: 8085, AdminPort: 8095}
	var out strings.Builder

	app, admin := runPorts(&out, 0, 0, chain)
	if app != 8085 || admin != 8095 {
		t.Errorf("no flags: ports = %d/%d, want the configuration's 8085/8095", app, admin)
	}
	if !strings.Contains(out.String(), `"Default"`) {
		t.Errorf("the run does not say where its ports came from: %q", out.String())
	}

	// CONTROLS: flags win, each on its own; a configuration with no ports
	// leaves the defaults (0 = the run's own default) alone.
	if app, admin := runPorts(&out, 8100, 0, chain); app != 8100 || admin != 8095 {
		t.Errorf("--app-port 8100: ports = %d/%d, want 8100/8095", app, admin)
	}
	if app, admin := runPorts(&out, 0, 8200, chain); app != 8085 || admin != 8200 {
		t.Errorf("--admin-port 8200: ports = %d/%d, want 8085/8200", app, admin)
	}
	out.Reset()
	if app, admin := runPorts(&out, 0, 0, constantChain{Configuration: "Default"}); app != 0 || admin != 0 || out.Len() != 0 {
		t.Errorf("no configured ports: %d/%d %q, want 0/0 and nothing said", app, admin, out.String())
	}
}
