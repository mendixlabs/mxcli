# Private PostgreSQL password input

Use `--db-password-stdin` with a private credential source or a protected file outside the project. Keep passwords out of tool commands, messages, and models.

The flag reads to EOF, so close the input pipe after supplying the password. Input is limited to 64 KiB, including one optional final LF or CRLF. That newline is removed; other spaces and tabs are preserved. Empty input, embedded line endings, and NUL bytes are rejected. Without the flag, stdin is not read and defaults stay unchanged.

Mendix 11.16.0 trims boundary whitespace from `DatabasePassword` and cannot authenticate passwords with leading or trailing whitespace. The CLI preserves the input. Other runtime versions are unverified.

The flag conflicts with explicit `--db-password`, including an empty flag, `--db-type hsqldb`, and `--detach`. Use a foreground run. `run restart` refuses before stopping the app. Stop explicitly and start again with fresh stdin. The watch loop can still restart the runtime with the password held in memory.

The existing private `.mxcli/run-local.json` boot configuration contains the password while the app runs. Keep that file out of version control.
