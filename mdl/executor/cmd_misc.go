// SPDX-License-Identifier: Apache-2.0

// Package executor - Miscellaneous commands (UPDATE, REFRESH, SET, HELP, EXIT, EXECUTE SCRIPT)
package executor

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/JordtenBulte-OLC/mxcli/cmd/mxcli/syntax"
	"github.com/JordtenBulte-OLC/mxcli/mdl/ast"
	mdlerrors "github.com/JordtenBulte-OLC/mxcli/mdl/errors"
	"github.com/JordtenBulte-OLC/mxcli/mdl/visitor"
)

// ErrExit is a sentinel error indicating clean script/session termination.
// Use errors.Is(err, ErrExit) to detect exit requests.
var ErrExit = mdlerrors.ErrExit

// execUpdate handles UPDATE statements (refresh from disk).
func execUpdate(ctx *ExecContext) error {
	if ctx.MprPath == "" {
		return mdlerrors.NewNotConnected()
	}

	// Reconnect to refresh
	path := ctx.MprPath
	execDisconnect(ctx)
	return execConnect(ctx, &ast.ConnectStmt{Path: path})
}

// execRefresh handles REFRESH statements (alias for UPDATE).
func execRefresh(ctx *ExecContext) error {
	return execUpdate(ctx)
}

// execSet handles SET statements.
func execSet(ctx *ExecContext, s *ast.SetStmt) error {
	if ctx.Settings == nil {
		ctx.Settings = make(map[string]any)
	}
	ctx.Settings[s.Key] = s.Value

	// Apply recognized session keys to the live context immediately.
	// syncBack will persist ctx.Format back to e.format so subsequent
	// statements in the same session also pick up the change.
	switch strings.ToLower(s.Key) {
	case "format":
		if v, ok := s.Value.(string); ok {
			ctx.Format = OutputFormat(strings.ToLower(v))
		}
	}

	fmt.Fprintf(ctx.Output, "Set %s = %v\n", s.Key, s.Value)
	return nil
}

// execHelp handles HELP statements. With topic words, queries the syntax registry.
func execHelp(ctx *ExecContext, s *ast.HelpStmt) error {
	if len(s.Topic) > 0 {
		path := syntax.ResolveAlias(resolveHelpPath(s.Topic))
		features := syntax.ByPrefix(path)
		if len(features) == 0 {
			fmt.Fprintf(ctx.Output, "No syntax help found for: %s\n", path)
			fmt.Fprintln(ctx.Output, "Use HELP; for a command overview.")
			return nil
		}
		if ctx.Format == FormatJSON {
			return syntax.WriteJSON(ctx.Output, features)
		}
		syntax.WriteText(ctx.Output, features)
		return nil
	}
	help := `MDL Commands:

Connection:
  connect local '<path>'      Connect to local .mpr file
  disconnect                  Disconnect from project
  status                      Show connection status

Domain Model - Enumerations:
  /** Documentation */
  create enumeration Module.Name (
    VALUE1 'Caption1',
    VALUE2 'Caption2'
  );

  drop enumeration Module.Name;
  show enumerations [in Module];
  describe enumeration Module.Name;

Domain Model - Entities:
  /** Entity documentation */
  @Position(x, y)
  create [or modify] persistent|non-persistent entity Module.Name (
    /** Attribute documentation */
    AttrName: Type [not null [error 'msg']] [unique [error 'msg']] [default value]
  )
  [index (col1, col2 desc)];
  /

  create view entity Module.Name (
    AttrName: Type
  ) as
    select ... from ... where ...;
  /

  drop entity Module.Name;
  show entities [in Module];
  describe entity Module.Name;

Domain Model - Associations:
  create association Module.Name
    from Module.Parent
    to Module.Child
    type Reference|ReferenceSet
    [owner Default|Both|Parent|Child]
    [delete_behavior DELETE_BUT_KEEP_REFERENCES|DELETE_CASCADE];
  /

  drop association Module.Name;
  show associations [in Module];
  describe association Module.Name;

Microflows:
  /** Documentation */
  create microflow Module.Name (
    $Param1: Type,
    $Param2: Module.Entity
  )
  returns ReturnType as $ReturnVar
  [folder 'folder/path']
  begin
    declare $Var Type = value;           -- Declare primitive variable
    declare $Entity as Module.Entity;    -- Declare entity variable
    set $Var = expression;               -- Change variable (must be declared)
    if condition then ... end if;        -- Conditional
    loop $Item in $List begin ... end loop;
    $Result = create Module.Entity (attr = value);
    change $Object (attr = value);
    commit $Object [with events] [refresh];
    retrieve $List from Module.Entity where condition;
    $Var = call microflow Module.Name($param = value);
    $Var = call java action Module.Name($param = value);
    validation feedback $Var/Attr message 'Error';  -- Show validation error
    close page [n];                      -- Close page(s)
    log info|warning|error [node 'name'] 'message';
    @annotation 'text'                   -- Visual annotation on next activity
    @caption 'text'                      -- Custom caption for activity
    @color Green                         -- Background color for activity
    @position(100, 200)                  -- Canvas position for activity
    return $ReturnVar;
  end;
  /

  drop microflow Module.Name;
  show microflows [in Module];
  show nanoflows [in Module];
  describe microflow Module.Name;

Pages, Snippets, Layouts, Java Actions:
  create [or replace] page Module.Name (...) { ... };
  drop page Module.Name;
  create [or replace] snippet Module.Name (...) { ... };
  drop snippet Module.Name;
  show pages [in Module];
  show snippets [in Module];
  show layouts [in Module];
  show java actions [in Module];
  describe page Module.Name;
  describe snippet Module.Name;

Widget Discovery and Bulk Updates (requires refresh catalog full):
  *** EXPERIMENTAL: Untested proof-of-concept. Use dry run first! ***

  show widgets [where condition] [in Module];
    where conditions: WidgetType like '%pattern%', Name = 'value'

  update widgets
    set 'property' = value [, 'property' = value]
    where condition [and condition]
    [in Module]
    [dry run];

  Examples:
    show widgets where WidgetType like '%combobox%';
    update widgets set 'showLabel' = false where WidgetType like '%DataGrid%' dry run;

  Always backup your project before applying changes without dry RUN.

Catalog Queries:
  show catalog tables;
  show catalog status;             Show cache information
  describe CATALOG.tablename;      Show table columns and required mode
  refresh catalog;                 Rebuild catalog (uses cache if valid)
  refresh catalog full;            Full mode with activities/widgets/refs
  refresh catalog full source;     Full + MDL source for full-text search
  refresh catalog [full] force;    Force rebuild (ignore cache)
  refresh catalog [full] background; Build in background
  select columns from CATALOG.tablename
    [where condition]
    [GROUP by column [having condition]]
    [ORDER by column [asc|desc]]
    [limit n] [offset n];

  Tables: modules, entities, attributes, microflows, nanoflows, pages,
          snippets, layouts, enumerations, JAVA_ACTIONS, ACTIVITIES*,
          widgets*, XPATH_EXPRESSIONS, REFS*, PROJECTS, SNAPSHOTS,
          objects, ODATA_CLIENTS, ODATA_SERVICES,
          BUSINESS_EVENT_SERVICES, STRINGS*, source**
  (* only populated with refresh catalog full)
  (** only populated with refresh catalog full source)

  Cache is stored in .mxcli/catalog.db next to the .mpr file.

Code Search (requires refresh catalog full):
  show callers of Module.Microflow [transitive];
  show callees of Module.Microflow [transitive];
  show references to Module.Element;
  show impact of Module.Element;
  show context of Module.Element [depth n];  -- Assemble context for LLM

Security - Roles:
  create module role Module.Role [description 'text'];
  drop module role Module.Role;
  create user role Name (Module.Role [, ...]) [manage all roles];
  alter user role Name add module roles (Module.Role [, ...]);
  alter user role Name remove module roles (Module.Role [, ...]);
  drop user role Name;

Security - Access Control:
  grant execute on microflow Module.Name to Role [, Role...];
  revoke execute on microflow Module.Name from Role [, Role...];
  grant view on page Module.Name to Role [, Role...];
  revoke view on page Module.Name from Role [, Role...];
  grant Role on Module.Entity (create, delete, read *, write *) [where 'xpath'];
  revoke Role on Module.Entity;

Security - Project Settings:
  alter project security level off|prototype|production;
  alter project security demo users on|off;
  create demo user 'name' password 'pass' (UserRole [, ...]);
  drop demo user 'name';

Security - Queries:
  show project security;
  show module roles [in Module];
  show user roles;
  show demo users;
  show access on microflow Module.Name;
  show access on page Module.Name;
  show access on Module.Entity;
  show security matrix [in Module];
  describe module role Module.Role;
  describe user role Name;
  describe demo user 'name';

Navigation:
  show navigation;
  show navigation menu [Profile];
  show navigation homes;
  describe navigation Profile;
  create or replace navigation Profile
    home page Module.Page
    [home page Module.Page for Module.Role]
    [login page Module.Page]
    [not found page Module.Page]
    [menu (
      menu item 'Caption' page Module.Page;
      menu 'SubMenu' (
        menu item 'Child' microflow Module.Flow;
      );
    )];

Data Types:
  String[(length)]  Integer  Long  Decimal[(p,s)]
  Boolean  DateTime  Date  AutoNumber  Binary
  Enumeration(Module.EnumName)

Scripts:
  execute script 'path/to/script.mdl';

Modules:
  create module Name;
  drop module Name;                -- Cascade-deletes all contents
  show modules;

External sql:
  sql connect <driver> '<dsn>' as <alias>;
  sql disconnect <alias>;
  sql connections;
  sql <alias> show tables;
  sql <alias> show views;
  sql <alias> show FUNCTIONS;
  sql <alias> describe <table>;
  sql <alias> select * from users limit 10;
  sql <alias> generate connector into <module> [tables (...)] [views (...)] [exec];

  Supported drivers: postgres (pg), oracle (ora), sqlserver (mssql)
  DSN examples:
    'postgres://user:pass@localhost:5432/dbname'
    'oracle://user:pass@localhost:1521/service'
    'sqlserver://sa:pass@localhost:1433?database=mydb&encrypt=disable'

Import from External DB into Mendix App DB:
  import from <alias> query '<sql>'
    into Module.Entity
    map (source_col as TargetAttr [, ...])
    [link (source_col to AssocName on ChildAttr [, ...])]
    [batch n]
    [limit n];

  link maps source columns to associations (Reference type only):
    on ChildAttr  — lookup child entity by attribute value
    (no on)       — source value is a raw Mendix object ID

  Reads rows from an external database and inserts them into the
  Mendix app's PostgreSQL database. Auto-connects using project settings.
  Handles both Column and Table association storage automatically.
  Default batch size: 1000.

  Env var overrides (for devcontainers/Docker):
    MXCLI_DB_TYPE, MXCLI_DB_HOST, MXCLI_DB_PORT,
    MXCLI_DB_NAME, MXCLI_DB_USER, MXCLI_DB_PASSWORD

Image Collections:
  create image collection Module.Name
    [export level 'Hidden'|'Public']
    [comment 'description']
    [(image Name from file 'path', ...)];
  /

  drop image collection Module.Name;
  show image collection [in Module];
  describe image collection Module.Name;

Other:
  commit [message 'message'];
  set key = value;
  HELP or ?
  EXIT or QUIT

Statement Terminator:
  Use ; or / to end statements
`
	fmt.Fprint(ctx.Output, help)
	return nil
}

// resolveHelpPath converts space-separated words like ["workflow", "user", "task"]
// into a registry path like "workflow.user-task" by greedily merging adjacent words
// with hyphens to find the longest matching prefix at each level.
func resolveHelpPath(words []string) string {
	var segments []string
	i := 0
	for i < len(words) {
		matched := false
		for j := len(words); j > i; j-- {
			candidate := strings.Join(words[i:j], "-")
			testPath := strings.Join(append(segments, candidate), ".")
			if syntax.HasPrefix(testPath) {
				segments = append(segments, candidate)
				i = j
				matched = true
				break
			}
		}
		if !matched {
			segments = append(segments, words[i])
			i++
		}
	}
	return strings.Join(segments, ".")
}

// listVersion displays Mendix project version information.
func listVersion(ctx *ExecContext) error {
	if !ctx.Connected() {
		return mdlerrors.NewNotConnected()
	}

	pv := ctx.Backend.ProjectVersion()
	fmt.Fprintf(ctx.Output, "Mendix Version: %s\n", pv.ProductVersion)
	fmt.Fprintf(ctx.Output, "Build Version:  %s\n", pv.BuildVersion)
	fmt.Fprintf(ctx.Output, "MPR Format:     v%d\n", pv.FormatVersion)
	if pv.SchemaHash != "" {
		fmt.Fprintf(ctx.Output, "Schema Hash:    %s\n", pv.SchemaHash)
	}
	return nil
}

// execExit handles EXIT statements.
// Note: This just signals exit intent via ErrExit. The actual cleanup
// is done by the caller (CLI/REPL) when they handle ErrExit at the top level.
// This allows exit within nested scripts to stop just that script without
// closing the database connection.
func execExit(ctx *ExecContext) error {
	return ErrExit
}

// maxScriptDepth is the maximum allowed EXECUTE SCRIPT nesting level.
const maxScriptDepth = 16

// execExecuteScript handles EXECUTE SCRIPT statements.
func execExecuteScript(ctx *ExecContext, s *ast.ExecuteScriptStmt) error {
	if ctx.ScriptDepth >= maxScriptDepth {
		return mdlerrors.NewValidationf("maximum script nesting depth (%d) exceeded — possible recursive EXECUTE SCRIPT", maxScriptDepth)
	}

	// Resolve path relative to current working directory
	scriptPath := s.Path
	if !filepath.IsAbs(scriptPath) {
		cwd, err := os.Getwd()
		if err != nil {
			return mdlerrors.NewBackend("get current directory", err)
		}
		scriptPath = filepath.Join(cwd, scriptPath)
	}

	// Read the script file
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		return mdlerrors.NewBackend("read script file '"+s.Path+"'", err)
	}

	// Pre-process: remove "/" statement separators (SQL*Plus style)
	// The "/" allows scripts to have multi-statement blocks when statements contain ";" internally
	processedContent := stripSlashSeparators(string(content))

	// Parse the script
	prog, errs := visitor.Build(processedContent)
	if len(errs) > 0 {
		for _, err := range errs {
			fmt.Fprintf(ctx.Output, "Parse error in %s: %v\n", s.Path, err)
		}
		return mdlerrors.NewValidationf("script '%s' has parse errors", s.Path)
	}

	// Execute all statements in the script
	fmt.Fprintf(ctx.Output, "Executing script: %s\n", s.Path)
	if ctx.ExecuteFn == nil {
		return mdlerrors.NewBackend("execute script", errors.New("ExecuteFn not set — ExecContext was not created via Executor dispatch"))
	}
	ctx.ScriptDepth++
	defer func() { ctx.ScriptDepth-- }()
	for _, stmt := range prog.Statements {
		if err := ctx.ExecuteFn(stmt); err != nil {
			// Exit within a script just stops the script, doesn't exit mxcli
			if errors.Is(err, ErrExit) {
				fmt.Fprintf(ctx.Output, "Script exited: %s\n", s.Path)
				return nil
			}
			return fmt.Errorf("error in script '%s': %w", s.Path, err)
		}
	}
	fmt.Fprintf(ctx.Output, "Script completed: %s\n", s.Path)

	return nil
}

// stripSlashSeparators removes lines that contain only "/" from the script content.
// This allows "/" to be used as a statement separator (SQL*Plus style) without
// requiring the grammar to support it.
func stripSlashSeparators(content string) string {
	lines := strings.Split(content, "\n")
	var result []string
	for _, line := range lines {
		// Skip lines that are just "/" (with optional whitespace)
		if strings.TrimSpace(line) == "/" {
			continue
		}
		result = append(result, line)
	}
	return strings.Join(result, "\n")
}
