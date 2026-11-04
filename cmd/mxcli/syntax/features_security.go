// SPDX-License-Identifier: Apache-2.0

package syntax

func init() {
	Register(SyntaxFeature{
		Path:    "security",
		Summary: "Application security: roles, access control, demo users",
		Keywords: []string{
			"security", "access control", "roles", "permissions",
			"grant", "revoke", "authentication", "authorization",
		},
		Syntax:  "DESCRIBE APP SECURITY;\nLIST MODULE ROLES [IN <module>];\nLIST USER ROLES;\nDESCRIBE SECURITY MATRIX [IN <module>];",
		Example: "DESCRIBE APP SECURITY;\nDESCRIBE SECURITY MATRIX IN Shop;",
		SeeAlso: []string{"security.module-role", "security.entity-access", "security.user-role"},
	})

	Register(SyntaxFeature{
		Path:     "security.module-role",
		Features: []string{"security/module_roles"},
		Summary:  "Create and manage module-level security roles",
		Keywords: []string{
			"module role", "create role", "drop role",
		},
		Syntax:  "CREATE [OR MODIFY] MODULE ROLE <module>.<role> [DESCRIPTION '<text>'];\nDROP MODULE ROLE <module>.<role>;",
		Example: "mdl 1;\nCREATE MODULE ROLE Shop.Admin DESCRIPTION 'Full access';\n-- OR MODIFY makes a security script re-runnable:\nCREATE OR MODIFY MODULE ROLE Shop.User DESCRIPTION 'Read-only access';\n-- Without DESCRIPTION it keeps the stored one; DESCRIPTION '' clears it:\nCREATE OR MODIFY MODULE ROLE Shop.Admin;",
		SeeAlso: []string{"security.user-role", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:     "security.entity-access",
		Features: []string{"security/entity_access"},
		Summary:  "Grant or revoke entity-level access (CRUD, attribute-level, XPath rules)",
		Keywords: []string{
			"entity access", "grant", "revoke", "read", "write",
			"create", "delete", "xpath", "row-level security",
		},
		Syntax: "GRANT <rights> ON ENTITY <module>.<entity> TO <module>.<role> [, ...] [WHERE [<xpath>]];\n" +
			"REVOKE ALL ON ENTITY <module>.<entity> FROM <module>.<role> [, ...];       -- removes the rule\n" +
			"REVOKE <rights> ON ENTITY <module>.<entity> FROM <module>.<role> [, ...];  -- takes rights away\n\n" +
			"Rights: CREATE, DELETE, READ *, READ (<attr>,...), WRITE *, WRITE (<attr>,...)\n\n" +
			"The XPath is written in [ ], as everywhere else, so quotes inside it are\n" +
			"not doubled; sibling groups ([a][b]) are one constraint. The old order,\n" +
			"GRANT <role> ON <entity> (<rights>) WHERE '<xpath>', still parses and\n" +
			"warns MDL-DEPR030; `mxcli fmt --upgrade` rewrites it. The revoke mirrors\n" +
			"the grant; its old order, REVOKE <role> ON <entity> [(<rights>)], warns\n" +
			"MDL-DEPR082 and is rewritten the same way.\n\n" +
			"A module role is always Module.Role. A bare role name parses but is\n" +
			"refused (MDL-GRANT02) — mxcli cannot tell which module it belongs to.\n\n" +
			"Members added later:\n" +
			"  A rule also carries a default for members added AFTER it was written,\n" +
			"  derived from the grant: WRITE * gives ReadWrite, READ * gives ReadOnly,\n" +
			"  and member lists alone leave it None. So an attribute added later is\n" +
			"  granted None on a member-listed rule — a clean build in which the field\n" +
			"  renders blank for that role. ALTER ENTITY ... ADD ATTRIBUTE warns and\n" +
			"  prints the GRANT that widens it. What decides this is the rule's\n" +
			"  default, not how narrow its member list is: READ *, WRITE (Email) is\n" +
			"  narrower than READ *, WRITE * and still picks up new members.\n\n" +
			"Inherited members:\n" +
			"  Mendix inheritance is multi-table — a child adds attributes to its\n" +
			"  parent's, and ALL the parent's members belong to the child. Name them\n" +
			"  in a GRANT exactly like the entity's own; READ */WRITE * covers them\n" +
			"  too. A name that matches no member is an error, not a silent skip.\n\n" +
			"  Exception: entities extending System.User are user entities, whose\n" +
			"  platform members (Name, Password, Blocked, ...) Mendix manages. Do not\n" +
			"  grant those; mxcli leaves them out of the rule automatically.",
		Example: "mdl 1;\n" +
			"GRANT CREATE, DELETE, READ *, WRITE * ON ENTITY Shop.Customer TO Shop.Admin;\n" +
			"GRANT READ * ON ENTITY Shop.Customer TO Shop.User WHERE [Active = true()];\n" +
			"GRANT READ *, WRITE * ON ENTITY Shop.Order TO Shop.User WHERE [Status = 'Open'];\n\n" +
			"-- Contract extends DocumentBase: DocName is inherited, ContractNumber is own\n" +
			"GRANT READ (DocName, ContractNumber) ON ENTITY Docs.Contract TO Docs.Viewer;\n\n" +
			"-- Attachment extends System.FileDocument: Name and Size are inherited\n" +
			"GRANT READ (Category, \"Name\", Size) ON ENTITY Docs.Attachment TO Docs.Viewer;\n\n" +
			"REVOKE WRITE (Email) ON ENTITY Shop.Customer FROM Shop.User;\n" +
			"REVOKE ALL ON ENTITY Shop.Order FROM Shop.User;",
		SeeAlso: []string{"security.module-role", "security.microflow-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.update-security",
		Summary: "Repair entity access rules that no longer match their domain model (CE0066)",
		Keywords: []string{
			"update security", "CE0066", "entity access out of date",
			"reconcile", "member access", "update security button",
		},
		Syntax: "UPDATE SECURITY;\n" +
			"UPDATE SECURITY <module>;\n" +
			"UPDATE SECURITY IN <module>;\n\n" +
			"This is the headless equivalent of Studio Pro's 'Update security'\n" +
			"button in the domain model editor. It adds the member entries an\n" +
			"access rule is missing, removes entries for members that no longer\n" +
			"exist, and reports how many rules it changed.\n\n" +
			"You rarely need it for models mxcli writes — every write path\n" +
			"reconciles as it writes. It is for a model that arrived from\n" +
			"somewhere else: a module imported or updated outside Studio Pro,\n" +
			"whose rules do not cover every member of their entities. Mendix\n" +
			"rejects that with CE0066 'Entity access is out of date'.\n\n" +
			"A project whose rules are already complete is not written to; the\n" +
			"command says 'All entity access rules are up to date'. System is\n" +
			"skipped — its access rules are the platform's.",
		Example: "mdl 1;\n" +
			"-- After a headless module install or update:\n" +
			"UPDATE SECURITY UserCommons;\n\n" +
			"-- Every module in the project:\n" +
			"UPDATE SECURITY;",
		SeeAlso: []string{"security.entity-access", "security.module-role"},
	})

	Register(SyntaxFeature{
		Path:    "security.microflow-access",
		Summary: "Grant or revoke execution rights on microflows",
		Keywords: []string{
			"microflow access", "execute", "grant microflow",
			"revoke microflow",
		},
		Syntax:  "GRANT EXECUTE ON MICROFLOW <module>.<name> TO <role> [, <role>...];\nREVOKE EXECUTE ON MICROFLOW <module>.<name> FROM <role> [, <role>...];",
		Example: "mdl 1;\nGRANT EXECUTE ON MICROFLOW Shop.ProcessOrder TO Shop.Admin, Shop.User;\nREVOKE EXECUTE ON MICROFLOW Shop.ProcessOrder FROM Shop.User;",
		SeeAlso: []string{"security.page-access", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.page-access",
		Summary: "Grant or revoke view rights on pages",
		Keywords: []string{
			"page access", "view", "grant page", "revoke page",
		},
		Syntax:  "GRANT VIEW ON PAGE <module>.<name> TO <role> [, <role>...];\nREVOKE VIEW ON PAGE <module>.<name> FROM <role> [, <role>...];",
		Example: "GRANT VIEW ON PAGE Shop.OrderOverview TO Shop.Admin, Shop.User;",
		SeeAlso: []string{"security.microflow-access", "security.entity-access"},
	})

	Register(SyntaxFeature{
		Path:     "security.user-role",
		Features: []string{"security/user_roles"},
		Summary:  "Create and manage application-level user roles that bundle module roles",
		Keywords: []string{
			"user role", "application role", "manage roles",
			"add module roles", "remove module roles",
		},
		Syntax:  "CREATE USER ROLE <name> [( ModuleRoles: (<role> [, ...]), Description: '<text>', ManageAllRoles: true|false, ManageableRoles: (<user role> [, ...]), ManageUsersWithoutRoles: true|false, CheckSecurity: true|false )];\nALTER USER ROLE <name> ADD MODULE ROLES (<role> [, ...]);\nALTER USER ROLE <name> DROP MODULE ROLES (<role> [, ...]);\nDROP USER ROLE [IF EXISTS] <name>;",
		Example: "mdl 1;\nCREATE USER ROLE AppAdmin ( ModuleRoles: (Shop.Admin, HR.Admin), ManageAllRoles: true );\nALTER USER ROLE AppAdmin ADD MODULE ROLES (Reporting.Viewer);",
		SeeAlso: []string{"security.module-role", "security.demo-user"},
	})

	Register(SyntaxFeature{
		Path:    "security.project-security",
		Summary: "Set project security level, strict mode, demo user and guest access toggles, admin user name and password",
		Keywords: []string{
			"project security", "app security", "alter app security", "security level", "prototype",
			"production", "off", "strict mode", "SEC005", "admin user", "AdminUserName", "MxAdmin",
			"admin password", "AdminPassword", "CE0129",
		},
		Syntax: "ALTER APP SECURITY (\n" +
			"  [SecurityLevel: OFF|PROTOTYPE|PRODUCTION,]\n" +
			"  [EnableDemoUsers: TRUE|FALSE,]\n" +
			"  [EnableGuestAccess: TRUE|FALSE,]\n" +
			"  [GuestUserRole: <UserRole>,]\n" +
			"  [StrictMode: TRUE|FALSE,]         -- clears lint rule SEC005\n" +
			"  [AdminUserName: '<name>',]        -- the built-in administrator (default MxAdmin)\n" +
			"  [AdminPassword: '<password>']     -- its password; empty is CE0129\n" +
			");\n\n" +
			"-- Set any subset of the properties, in create's ( Key: value ) list. The\n" +
			"-- clause forms (LEVEL …, DEMO USERS ON|OFF, GUEST ACCESS ON [ROLE r]|OFF,\n" +
			"-- STRICT MODE ON|OFF) still run and warn MDL-DEPR133.\n" +
			"-- AdminPassword is stored in the model as written, as Studio Pro stores it;\n" +
			"-- '' is refused (CE0129 \"Administrator password has not been set\" at\n" +
			"-- prototype and production level). mxbuild does not check it against the\n" +
			"-- password policy. `describe app security` says whether one is set.",
		Example: "mdl 1;\n" +
			"ALTER APP SECURITY ( SecurityLevel: PRODUCTION );\n" +
			"ALTER APP SECURITY ( EnableDemoUsers: FALSE );\n" +
			"ALTER APP SECURITY ( StrictMode: TRUE );\n" +
			"ALTER APP SECURITY ( AdminUserName: 'appadmin' );\n" +
			"ALTER APP SECURITY ( AdminPassword: 'N3w-Admin-Passw0rd' );  -- resolves CE0129",
		SeeAlso: []string{"security.demo-user", "security.guest-access"},
	})

	Register(SyntaxFeature{
		Path:    "security.guest-access",
		Summary: "Enable anonymous (guest) access and pick the role anonymous visitors get",
		Keywords: []string{
			"guest access", "anonymous", "anonymous users", "public",
			"unauthenticated", "guest user role", "CE0133",
		},
		Syntax: "ALTER APP SECURITY ( EnableGuestAccess: TRUE, GuestUserRole: <UserRole> );\n" +
			"ALTER APP SECURITY ( EnableGuestAccess: TRUE );   -- only when a role is already configured\n" +
			"ALTER APP SECURITY ( EnableGuestAccess: FALSE );  -- keeps the stored role\n" +
			"\n" +
			"-- The role is what anonymous visitors get, so its entity access IS the app's\n" +
			"-- public surface. Mendix requires one: guest access with no role fails the\n" +
			"-- build (CE0133), so TRUE is refused unless a role is given or already stored.\n" +
			"-- GuestUserRole alone changes the role and keeps guest access on or off.\n" +
			"-- Mendix does not check the role exists, so mxcli does — an unknown role\n" +
			"-- would build cleanly and leave visitors with nothing.",
		Example: "mdl 1;\n" +
			"CREATE USER ROLE Anonymous ( ModuleRoles: (Shop.Viewer, System.User) );\n" +
			"ALTER APP SECURITY ( EnableGuestAccess: TRUE, GuestUserRole: Anonymous );\n" +
			"GRANT READ * ON ENTITY Shop.Product TO Shop.Viewer;",
		SeeAlso: []string{"security.user-role", "security.project-security"},
	})

	Register(SyntaxFeature{
		Path:     "security.demo-user",
		Features: []string{"security/demo_users"},
		Summary:  "Create and manage demo users for testing",
		Keywords: []string{
			"demo user", "test user", "demo account",
			"password", "login",
		},
		Syntax:  "CREATE [OR MODIFY] DEMO USER '<name>' (\n  Password: '<pass>',\n  [Entity: Module.Entity,]\n  UserRoles: (<userrole> [, ...])\n);\nDROP DEMO USER [IF EXISTS] '<name>';\n\n-- The keys are Studio Pro's property names. Password is required; without\n-- Entity the user entity is detected from the project. The clause form\n-- PASSWORD 'p' [ENTITY E] (roles) is its deprecated alias (MDL-DEPR137).",
		Example: "mdl 1;\nCREATE DEMO USER 'admin' ( Password: 'Admin1!', UserRoles: (AppAdmin) );\nCREATE DEMO USER 'user' ( Password: 'User1!', UserRoles: (AppUser) );",
		SeeAlso: []string{"security.user-role", "security.project-security"},
	})
}
