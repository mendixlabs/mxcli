// SPDX-License-Identifier: Apache-2.0

package main

import (
	"fmt"
	"io"
	"strconv"
	"strings"

	"github.com/mendixlabs/mxcli/cmd/mxcli/docker"
	"github.com/spf13/cobra"
)

const maxDBPasswordInput = 64 * 1024

func runDBPassword(cmd *cobra.Command) (string, error) {
	password, _ := cmd.Flags().GetString("db-password")
	fromStdin, _ := cmd.Flags().GetBool("db-password-stdin")
	if !fromStdin {
		return password, nil
	}
	if cmd.Flags().Changed("db-password") {
		return "", fmt.Errorf("--db-password-stdin cannot be combined with --db-password")
	}
	local, _ := cmd.Flags().GetBool("local")
	hub, _ := cmd.Flags().GetString("hub")
	if !local && hub == "" {
		return "", fmt.Errorf("--db-password-stdin requires --local (or --hub)")
	}
	if detach, _ := cmd.Flags().GetBool("detach"); detach {
		return "", fmt.Errorf("--db-password-stdin cannot be combined with --detach; start a foreground run instead")
	}
	dbType, _ := cmd.Flags().GetString("db-type")
	kind, err := docker.NormalizeDBType(dbType)
	if err != nil || docker.IsFileBasedDBType(kind) {
		return "", fmt.Errorf("--db-password-stdin requires a PostgreSQL database")
	}
	input, err := io.ReadAll(io.LimitReader(cmd.InOrStdin(), maxDBPasswordInput+1))
	if err != nil {
		// Reader errors can contain input bytes, so do not include the cause.
		return "", fmt.Errorf("could not read --db-password-stdin")
	}
	if len(input) > maxDBPasswordInput {
		return "", fmt.Errorf("--db-password-stdin input exceeds 64 KiB")
	}
	password = string(input)
	if strings.HasSuffix(password, "\n") {
		password = strings.TrimSuffix(strings.TrimSuffix(password, "\n"), "\r")
	}
	if password == "" {
		return "", fmt.Errorf("--db-password-stdin requires a non-empty password")
	}
	if strings.ContainsAny(password, "\r\n\x00") {
		return "", fmt.Errorf("--db-password-stdin requires one password without embedded line endings or NUL bytes")
	}
	return password, nil
}

func runArgsNeedPasswordStdin(args []string) bool {
	enabled := false
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			break
		}
		if arg == "--db-password-stdin" {
			enabled = true
		} else if value, ok := strings.CutPrefix(arg, "--db-password-stdin="); ok {
			enabled, _ = strconv.ParseBool(value)
		} else if name, ok := strings.CutPrefix(arg, "--"); ok {
			// A string flag's value can itself look like the stdin flag.
			if flag := runCmd.Flag(name); flag != nil && flag.NoOptDefVal == "" {
				i++
			}
		}
	}
	return enabled
}
