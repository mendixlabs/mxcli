// SPDX-License-Identifier: Apache-2.0

package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
)

func main() {
	var dir, mdOut, jsonOut, alertsOut string
	t := thresholds{}
	flag.StringVar(&dir, "records", "", "directory holding the records, searched recursively for .json files (required)")
	flag.StringVar(&mdOut, "md", "", "write the trend as Markdown here")
	flag.StringVar(&jsonOut, "json", "", "write the series as JSON here")
	flag.StringVar(&alertsOut, "alerts", "", "write the alerts as a Markdown issue body here; the file is left empty when there are none")
	flag.Float64Var(&t.slower, "slower", 1.20, "a step is slower on a night when its wall ratio against the baseline is above this")
	flag.Float64Var(&t.drift, "drift", 1.30, "a step has drifted when the median of its last 3 runs is this many times the median of the 7 before")
	flag.Float64Var(&t.minMS, "drift-min-ms", 100, "steps shorter than this are not judged for drift")
	flag.Parse()
	if dir == "" {
		fatal(fmt.Errorf("-records is required"))
	}

	records, skipped, err := loadRecords(dir)
	if err != nil {
		fatal(err)
	}
	alerts := findAlerts(records, t)
	md := trendMarkdown(records, skipped, alerts)

	if mdOut != "" {
		if err := os.WriteFile(mdOut, []byte(md), 0o644); err != nil {
			fatal(err)
		}
	} else {
		fmt.Print(md)
	}
	if jsonOut != "" {
		data, err := json.MarshalIndent(struct {
			Runs   int      `json:"runs"`
			Alerts []alert  `json:"alerts"`
			Series []series `json:"series"`
		}{len(records), alerts, buildSeries(records)}, "", " ")
		if err != nil {
			fatal(err)
		}
		if err := os.WriteFile(jsonOut, append(data, '\n'), 0o644); err != nil {
			fatal(err)
		}
	}
	if alertsOut != "" {
		if err := os.WriteFile(alertsOut, []byte(alertsMarkdown(records, alerts)), 0o644); err != nil {
			fatal(err)
		}
	}
	fmt.Fprintf(os.Stderr, "agentbench trend: %d runs, %d alert(s)\n", len(records), len(alerts))
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "agentbench trend:", err)
	os.Exit(1)
}
