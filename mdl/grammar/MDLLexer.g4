/**
 * MDL (Mendix Definition Language) Lexer Grammar
 *
 * ANTLR4 lexer for parsing MDL syntax used by the Mendix REPL.
 * Converted from Chevrotain-based parser.
 *
 * Token ordering is critical - longer/more specific tokens must come before shorter ones.
 */
lexer grammar MDLLexer;

// =============================================================================
// WHITESPACE AND COMMENTS
// =============================================================================

WS: [ \t\r\n\u000B\u000C]+ -> skip;

DOC_COMMENT: '/**' .*? '*/';  // Not hidden - used for documentation
BLOCK_COMMENT: '/*' .*? '*/' -> skip;
LINE_COMMENT: '--' ~[\r\n]* -> skip;

// =============================================================================
// MULTI-WORD KEYWORDS (must come before single-word variants)
// =============================================================================

// These use fragment rules to handle optional whitespace
IS_NOT_NULL: I S WS+ N O T WS+ N U L L;
IS_NULL: I S WS+ N U L L;
NOT_NULL: N O T WS+ N U L L;
GROUP_BY: G R O U P WS+ B Y;
ORDER_BY: O R D E R WS+ B Y;
SORT_BY: S O R T WS+ B Y;
NON_PERSISTENT: N O N '-' P E R S I S T E N T;
REFERENCE_SET: R E F E R E N C E '_'? S E T;
LIST_OF: L I S T WS+ O F;

// Delete behavior compound keywords
DELETE_AND_REFERENCES: D E L E T E WS+ A N D WS+ R E F E R E N C E S
                     | D E L E T E '_' A N D '_' R E F E R E N C E S
                     | D E L E T E A N D R E F E R E N C E S;
DELETE_BUT_KEEP_REFERENCES: D E L E T E WS+ B U T WS+ K E E P WS+ R E F E R E N C E S
                          | D E L E T E '_' B U T '_' K E E P '_' R E F E R E N C E S
                          | D E L E T E B U T K E E P R E F E R E N C E S;
DELETE_IF_NO_REFERENCES: D E L E T E WS+ I F WS+ N O WS+ R E F E R E N C E S
                       | D E L E T E '_' I F '_' N O '_' R E F E R E N C E S
                       | D E L E T E I F N O R E F E R E N C E S;

// =============================================================================
// DDL KEYWORDS
// =============================================================================

CREATE: C R E A T E;
ALTER: A L T E R;
DROP: D R O P;
RENAME: R E N A M E;
MOVE: M O V E;
MODIFY: M O D I F Y;

ENTITY: E N T I T Y;
PERSISTENT: P E R S I S T E N T;
VIEW: V I E W;
EXTERNAL: E X T E R N A L;
ASSOCIATION: A S S O C I A T I O N;
ENUMERATION: E N U M E R A T I O N;
MODULE: M O D U L E;
MICROFLOW: M I C R O F L O W;
NANOFLOW: N A N O F L O W;
WORKFLOW: W O R K F L O W;
PAGE: P A G E;
SNIPPET: S N I P P E T;
LAYOUT: L A Y O U T;
NOTEBOOK: N O T E B O O K;
CONSTANT: C O N S T A N T;

ATTRIBUTE: A T T R I B U T E;
COLUMN: C O L U M N;
COLUMNS: C O L U M N S;
INDEX: I N D E X;
OWNER: O W N E R;
STORE: S T O R E;
REFERENCE: R E F E R E N C E;
GENERALIZATION: G E N E R A L I Z A T I O N;
EXTENDS: E X T E N D S;

ADD: A D D;
SET: S E T;
POSITION: P O S I T I O N;
DOCUMENTATION: D O C U M E N T A T I O N;
STORAGE: S T O R A G E;
TABLE: T A B L E;

// Delete behavior keywords
DELETE_BEHAVIOR: D E L E T E '_'? B E H A V I O R;
CASCADE: C A S C A D E;
PREVENT: P R E V E N T;

// =============================================================================
// CONNECTION KEYWORDS
// =============================================================================

CONNECT: C O N N E C T;
DISCONNECT: D I S C O N N E C T;
LOCAL: L O C A L;
PROJECT: P R O J E C T;
RUNTIME: R U N T I M E;
BRANCH: B R A N C H;
TOKEN: T O K E N;
HOST: H O S T;
PORT: P O R T;
SHOW: S H O W;
DESCRIBE: D E S C R I B E;
USE: U S E;
INTROSPECT: I N T R O S P E C T;
DEBUG: D E B U G;

// =============================================================================
// OQL / QUERY KEYWORDS
// =============================================================================

SELECT: S E L E C T;
FROM: F R O M;
WHERE: W H E R E;
HAVING: H A V I N G;
OFFSET: O F F S E T;
LIMIT: L I M I T;
AS: A S;
RETURNS: R E T U R N S;
RETURNING: R E T U R N I N G;

// CASE expression
CASE: C A S E;
WHEN: W H E N;
THEN: T H E N;
ELSE: E L S E;
END: E N D;

// DISTINCT/ALL
DISTINCT: D I S T I N C T;
ALL: A L L;

// JOIN keywords
JOIN: J O I N;
LEFT: L E F T;
RIGHT: R I G H T;
INNER: I N N E R;
OUTER: O U T E R;
FULL: F U L L;
CROSS: C R O S S;
ON: O N;

// Sort direction
ASC: A S C;
DESC: D E S C;

// =============================================================================
// MICROFLOW KEYWORDS
// =============================================================================

BEGIN: B E G I N;
DECLARE: D E C L A R E;
CHANGE: C H A N G E;
RETRIEVE: R E T R I E V E;
DELETE: D E L E T E;
COMMIT: C O M M I T;
ROLLBACK: R O L L B A C K;

LOOP: L O O P;
WHILE: W H I L E;
IF: I F;
ELSIF: E L S I F;
ELSEIF: E L S E I F;
CONTINUE: C O N T I N U E;
BREAK: B R E A K;
RETURN: R E T U R N;
THROW: T H R O W;
LOG: L O G;
CALL: C A L L;
JAVA: J A V A;
JAVASCRIPT: J A V A S C R I P T;
ACTION: A C T I O N;
ACTIONS: A C T I O N S;
CLOSE: C L O S E;

NODE: N O D E;
EVENTS: E V E N T S;

// List operations
HEAD: H E A D;
TAIL: T A I L;
FIND: F I N D;
SORT: S O R T;
UNION: U N I O N;
INTERSECT: I N T E R S E C T;
SUBTRACT: S U B T R A C T;
CONTAINS: C O N T A I N S;
AVERAGE: A V E R A G E;
MINIMUM: M I N I M U M;
MAXIMUM: M A X I M U M;
LIST: L I S T;
REMOVE: R E M O V E;
EQUALS_OP: E Q U A L S;

// Log levels
INFO: I N F O;
WARNING: W A R N I N G;
TRACE: T R A C E;
CRITICAL: C R I T I C A L;
WITH: W I T H;
EMPTY: E M P T Y;
OBJECT: O B J E C T;
OBJECTS: O B J E C T S;

// =============================================================================
// PAGE / WIDGET KEYWORDS
// =============================================================================

PAGES: P A G E S;
LAYOUTS: L A Y O U T S;
SNIPPETS: S N I P P E T S;
NOTEBOOKS: N O T E B O O K S;

PLACEHOLDER: P L A C E H O L D E R;
SNIPPETCALL: S N I P P E T C A L L;

// Grid/Layout widgets
LAYOUTGRID: L A Y O U T G R I D;
DATAGRID: D A T A G R I D;
DATAVIEW: D A T A V I E W;
LISTVIEW: L I S T V I E W;
GALLERY: G A L L E R Y;
CONTAINER: C O N T A I N E R;
ROW: R O W;
ITEM: I T E M;
CONTROLBAR: C O N T R O L B A R;
SEARCH: S E A R C H;
SEARCHBAR: S E A R C H B A R;
NAVIGATIONLIST: N A V I G A T I O N L I S T;

// Button widgets
ACTIONBUTTON: A C T I O N B U T T O N;
LINKBUTTON: L I N K B U T T O N;
BUTTON: B U T T O N;

// Text/Display widgets
TITLE: T I T L E;
DYNAMICTEXT: D Y N A M I C T E X T;
DYNAMIC: D Y N A M I C;
STATICTEXT: S T A T I C T E X T;
LABEL: L A B E L;

// Input widgets
TEXTBOX: T E X T B O X;
TEXTAREA: T E X T A R E A;
DATEPICKER: D A T E P I C K E R;
RADIOBUTTONS: R A D I O B U T T O N S;
DROPDOWN: D R O P D O W N;
COMBOBOX: C O M B O B O X;
CHECKBOX: C H E C K B O X;
REFERENCESELECTOR: R E F E R E N C E S E L E C T O R;
INPUTREFERENCESETSELECTOR: I N P U T R E F E R E N C E S E T S E L E C T O R;
FILEINPUT: F I L E I N P U T;
IMAGEINPUT: I M A G E I N P U T;

// Custom/Filter widgets
CUSTOMWIDGET: C U S T O M W I D G E T;
TEXTFILTER: T E X T F I L T E R;
NUMBERFILTER: N U M B E R F I L T E R;
DROPDOWNFILTER: D R O P D O W N F I L T E R;
DATEFILTER: D A T E F I L T E R;
FILTER: F I L T E R;

// Widget properties
WIDGET: W I D G E T;
WIDGETS: W I D G E T S;
CAPTION: C A P T I O N;
ICON: I C O N;
TOOLTIP: T O O L T I P;
DATASOURCE: D A T A S O U R C E;
SOURCE_KW: S O U R C E;
SELECTION: S E L E C T I O N;
FOOTER: F O O T E R;
HEADER: H E A D E R;
CONTENT: C O N T E N T;
RENDERMODE: R E N D E R M O D E;

// V3 property keywords
BINDS: B I N D S;
ATTR: A T T R;
CONTENTPARAMS: C O N T E N T P A R A M S;
CAPTIONPARAMS: C A P T I O N P A R A M S;
PARAMS: P A R A M S;
VARIABLES_KW: V A R I A B L E S;
DESKTOPWIDTH: D E S K T O P W I D T H;
TABLETWIDTH: T A B L E T W I D T H;
PHONEWIDTH: P H O N E W I D T H;
CLASS: C L A S S;
STYLE: S T Y L E;
BUTTONSTYLE: B U T T O N S T Y L E;
DESIGN: D E S I G N;
PROPERTIES: P R O P E R T I E S;
DESIGNPROPERTIES: D E S I G N P R O P E R T I E S;
STYLING: S T Y L I N G;
CLEAR: C L E A R;
WIDTH: W I D T H;
HEIGHT: H E I G H T;
AUTOFILL: A U T O F I L L;
URL: U R L;
FOLDER: F O L D E R;
PASSING: P A S S I N G;
CONTEXT: C O N T E X T;
EDITABLE: E D I T A B L E;
READONLY: R E A D O N L Y;
ATTRIBUTES: A T T R I B U T E S;
FILTERTYPE: F I L T E R T Y P E;
IMAGE: I M A G E;
COLLECTION: C O L L E C T I O N;
STATICIMAGE: S T A T I C I M A G E;
DYNAMICIMAGE: D Y N A M I C I M A G E;
CUSTOMCONTAINER: C U S T O M C O N T A I N E R;
GROUPBOX: G R O U P B O X;
VISIBLE: V I S I B L E;
SAVECHANGES: S A V E C H A N G E S;
SAVE_CHANGES: S A V E '_' C H A N G E S;
CANCEL_CHANGES: C A N C E L '_' C H A N G E S;
CLOSE_PAGE: C L O S E '_' P A G E;
SHOW_PAGE: S H O W '_' P A G E;
DELETE_ACTION: D E L E T E '_' A C T I O N;
DELETE_OBJECT: D E L E T E '_' O B J E C T;
CREATE_OBJECT: C R E A T E '_' O B J E C T;
CALL_MICROFLOW: C A L L '_' M I C R O F L O W;
CALL_NANOFLOW: C A L L '_' N A N O F L O W;
OPEN_LINK: O P E N '_' L I N K;
SIGN_OUT: S I G N '_' O U T;
CANCEL: C A N C E L;

// Button styles
PRIMARY: P R I M A R Y;
// DEFAULT is defined below (used for both button style and attribute default values)
SUCCESS: S U C C E S S;
DANGER: D A N G E R;
WARNING_STYLE: W A R N I N G;
INFO_STYLE: I N F O;
TEMPLATE: T E M P L A T E;
ONCLICK: O N C L I C K;
ONCHANGE: O N C H A N G E;
TABINDEX: T A B I N D E X;

// Render modes / heading levels
H1: H '1';
H2: H '2';
H3: H '3';
H4: H '4';
H5: H '5';
H6: H '6';
PARAGRAPH: P A R A G R A P H;

// =============================================================================
// DATA TYPE KEYWORDS
// =============================================================================

STRING_TYPE: S T R I N G;
INTEGER_TYPE: I N T E G E R;
LONG_TYPE: L O N G;
DECIMAL_TYPE: D E C I M A L;
BOOLEAN_TYPE: B O O L E A N;
DATETIME_TYPE: D A T E T I M E;
DATE_TYPE: D A T E;
AUTONUMBER_TYPE: A U T O N U M B E R;
BINARY_TYPE: B I N A R Y;
HASHEDSTRING_TYPE: H A S H E D S T R I N G;
CURRENCY_TYPE: C U R R E N C Y;
FLOAT_TYPE: F L O A T;
STRINGTEMPLATE_TYPE: S T R I N G T E M P L A T E;
ENUM_TYPE: E N U M;

// =============================================================================
// AGGREGATE FUNCTIONS
// =============================================================================

COUNT: C O U N T;
SUM: S U M;
AVG: A V G;
MIN: M I N;
MAX: M A X;
LENGTH: L E N G T H;
TRIM: T R I M;
COALESCE: C O A L E S C E;
CAST: C A S T;

// =============================================================================
// LOGICAL / COMPARISON KEYWORDS
// =============================================================================

AND: A N D;
OR: O R;
NOT: N O T;
NULL: N U L L;
IN: I N;
BETWEEN: B E T W E E N;
LIKE: L I K E;
MATCH: M A T C H;
EXISTS: E X I S T S;
UNIQUE: U N I Q U E;
DEFAULT: D E F A U L T;
TRUE: T R U E;
FALSE: F A L S E;

// =============================================================================
// VALIDATION / CONSTRAINT KEYWORDS
// =============================================================================

VALIDATION: V A L I D A T I O N;
FEEDBACK: F E E D B A C K;
RULE: R U L E;
REQUIRED: R E Q U I R E D;
ERROR: E R R O R;
RAISE: R A I S E;
RANGE: R A N G E;
REGEX: R E G E X;
PATTERN: P A T T E R N;
EXPRESSION: E X P R E S S I O N;
XPATH: X P A T H;
CONSTRAINT: C O N S T R A I N T;
CALCULATED: C A L C U L A T E D;

// =============================================================================
// REST CLIENT KEYWORDS
// =============================================================================

REST: R E S T;
SERVICE: S E R V I C E;
SERVICES: S E R V I C E S;
ODATA: O D A T A;
BASE: B A S E;
AUTH: A U T H;
AUTHENTICATION: A U T H E N T I C A T I O N;
BASIC: B A S I C;
NOTHING: N O T H I N G;
OAUTH: O A U T H;
OPERATION: O P E R A T I O N;
METHOD: M E T H O D;
PATH: P A T H;
TIMEOUT: T I M E O U T;
BODY: B O D Y;
RESPONSE: R E S P O N S E;
REQUEST: R E Q U E S T;
SEND: S E N D;
JSON: J S O N;
XML: X M L;
STATUS: S T A T U S;
FILE_KW: F I L E;
VERSION: V E R S I O N;
GET: G E T;
POST: P O S T;
PUT: P U T;
PATCH: P A T C H;
API: A P I;
CLIENT: C L I E N T;
CLIENTS: C L I E N T S;
PUBLISH: P U B L I S H;
PUBLISHED: P U B L I S H E D;
EXPOSE: E X P O S E;
CONTRACT: C O N T R A C T;
NAMESPACE_KW: N A M E S P A C E;
SESSION: S E S S I O N;
GUEST: G U E S T;
PAGING: P A G I N G;
NOT_SUPPORTED: N O T '_' S U P P O R T E D;
USERNAME: U S E R N A M E;
PASSWORD: P A S S W O R D;
CONNECTION: C O N N E C T I O N;
DATABASE: D A T A B A S E;
QUERY: Q U E R Y;
MAP: M A P;
MAPPING: M A P P I N G;
IMPORT: I M P O R T;
INTO: I N T O;
BATCH: B A T C H;
LINK: L I N K;
EXPORT: E X P O R T;
GENERATE: G E N E R A T E;
CONNECTOR: C O N N E C T O R;
EXEC: E X E C;
TABLES: T A B L E S;
VIEWS: V I E W S;
EXPOSED: E X P O S E D;
PARAMETER: P A R A M E T E R;
PARAMETERS: P A R A M E T E R S;
HEADERS: H E A D E R S;

// Navigation keywords
NAVIGATION: N A V I G A T I O N;
MENU_KW: M E N U;
HOMES: H O M E S;
HOME: H O M E;
LOGIN: L O G I N;
FOUND: F O U N D;

// =============================================================================
// UTILITY KEYWORDS
// =============================================================================

MODULES: M O D U L E S;
ENTITIES: E N T I T I E S;
ASSOCIATIONS: A S S O C I A T I O N S;
MICROFLOWS: M I C R O F L O W S;
NANOFLOWS: N A N O F L O W S;
WORKFLOWS: W O R K F L O W S;
ENUMERATIONS: E N U M E R A T I O N S;
CONSTANTS: C O N S T A N T S;
CONNECTIONS: C O N N E C T I O N S;

// Fragment keywords
DEFINE: D E F I N E;
FRAGMENT: F R A G M E N T;
FRAGMENTS: F R A G M E N T S;
LANGUAGES: L A N G U A G E S;

// ALTER PAGE keywords
INSERT: I N S E R T;
BEFORE: B E F O R E;
AFTER: A F T E R;

UPDATE: U P D A T E;
REFRESH: R E F R E S H;
CHECK: C H E C K;
BUILD: B U I L D;
EXECUTE: E X E C U T E;
SCRIPT: S C R I P T;
LINT: L I N T;
RULES: R U L E S;
TEXT: T E X T;
SARIF: S A R I F;
MESSAGE: M E S S A G E;
MESSAGES: M E S S A G E S;
CHANNELS: C H A N N E L S;
COMMENT: C O M M E N T;
CUSTOM_NAME_MAP: C U S T O M WS+ N A M E WS+ M A P;
CATALOG: C A T A L O G;
FORCE: F O R C E;
BACKGROUND: B A C K G R O U N D;
CALLERS: C A L L E R S;
CALLEES: C A L L E E S;
REFERENCES: R E F E R E N C E S;
TRANSITIVE: T R A N S I T I V E;
IMPACT: I M P A C T;
DEPTH: D E P T H;
STRUCTURE: S T R U C T U R E;
STRUCTURES: S T R U C T U R E S;
TYPE: T Y P E;
VALUE: V A L U E;
VALUES: V A L U E S;
SINGLE: S I N G L E;
MULTIPLE: M U L T I P L E;
NONE: N O N E;
BOTH: B O T H;
TO: T O;
OF: O F;
OVER: O V E R;
FOR: F O R;
REPLACE: R E P L A C E;
MEMBERS: M E M B E R S;
ATTRIBUTE_NAME: A T T R I B U T E N A M E;
FORMAT: F O R M A T;
SQL: S Q L;
WITHOUT: W I T H O U T;
DRY: D R Y;
RUN: R U N;
WIDGETTYPE: W I D G E T T Y P E;
V3: V '3';  // Syntax version V3

// =============================================================================
// BUSINESS EVENTS / SETTINGS KEYWORDS
// =============================================================================

BUSINESS: B U S I N E S S;
EVENT: E V E N T;
SUBSCRIBE: S U B S C R I B E;
SETTINGS: S E T T I N G S;
CONFIGURATION: C O N F I G U R A T I O N;

// =============================================================================
// VERSION FEATURE REGISTRY KEYWORDS
// =============================================================================

FEATURES: F E A T U R E S;
ADDED: A D D E D;
SINCE: S I N C E;

// =============================================================================
// SECURITY KEYWORDS
// =============================================================================

SECURITY: S E C U R I T Y;
ROLE: R O L E;
ROLES: R O L E S;
GRANT: G R A N T;
REVOKE: R E V O K E;
PRODUCTION: P R O D U C T I O N;
PROTOTYPE: P R O T O T Y P E;
MANAGE: M A N A G E;
DEMO: D E M O;
MATRIX: M A T R I X;
APPLY: A P P L Y;
ACCESS: A C C E S S;
LEVEL: L E V E L;
USER: U S E R;
TASK: T A S K;
DECISION: D E C I S I O N;
SPLIT: S P L I T;
OUTCOMES: O U T C O M E S;
TARGETING: T A R G E T I N G;
NOTIFICATION: N O T I F I C A T I O N;
TIMER: T I M E R;
JUMP: J U M P;
DUE: D U E;
OVERVIEW: O V E R V I E W;
DATE: D A T E;
PARALLEL: P A R A L L E L;
WAIT: W A I T;
ANNOTATION: A N N O T A T I O N;
BOUNDARY: B O U N D A R Y;
INTERRUPTING: I N T E R R U P T I N G;
NON: N O N;
MULTI: M U L T I;
BY: B Y;
READ: R E A D;
WRITE: W R I T E;
DESCRIPTION: D E S C R I P T I O N;
DISPLAY: D I S P L A Y;
OFF: O F F;
USERS: U S E R S;

// =============================================================================
// COMPARISON OPERATORS (multi-char before single-char)
// =============================================================================

NOT_EQUALS: '<>' | '!=';
LESS_THAN_OR_EQUAL: '<=';
GREATER_THAN_OR_EQUAL: '>=';
EQUALS: '=';
LESS_THAN: '<';
GREATER_THAN: '>';

// =============================================================================
// ARITHMETIC OPERATORS
// =============================================================================

PLUS: '+';
MINUS: '-';
STAR: '*';
SLASH: '/';
PERCENT: '%';
MOD: M O D;
DIV: D I V;

// =============================================================================
// PUNCTUATION
// =============================================================================

SEMICOLON: ';';
COMMA: ',';
DOT: '.';
LPAREN: '(';
RPAREN: ')';
LBRACE: '{';
RBRACE: '}';
LBRACKET: '[';
RBRACKET: ']';
COLON: ':';
AT: '@';
PIPE: '|';
DOUBLE_COLON: '::';
ARROW: '->';
QUESTION: '?';
HASH: '#';

// =============================================================================
// LITERALS
// =============================================================================

// Mendix token: [%TokenName%] or [%'literal'%]
MENDIX_TOKEN: '[%' .*? '%]';

// String literals (single-quoted, with escape support)
STRING_LITERAL
    : '\'' ( ~['\r\n\\] | '\\' . | '\'\'' )* '\''
    ;

// Dollar-quoted string literal (PostgreSQL style) for embedding code blocks
// Supports: $$ ... $$ for multi-line code blocks (Java, SQL, etc.)
DOLLAR_STRING
    : '$$' .*? '$$'
    ;

// Number literal (integer or decimal)
NUMBER_LITERAL
    : '-'? DIGIT+ ('.' DIGIT+)? ([eE] [+-]? DIGIT+)?
    ;

// =============================================================================
// IDENTIFIERS (must come last)
// =============================================================================

// Variable identifier: $name (allows multi-character names)
VARIABLE: '$' ID_BODY+;

// Standard identifier (must come before HYPHENATED_ID)
IDENTIFIER: ID_START ID_BODY*;

// Hyphenated identifier: starts-with, ends-with (for XPath functions)
// Must contain at least one hyphen
HYPHENATED_ID: ID_START (ID_BODY* '-')+ ID_BODY*;

// Quoted identifier: "double-quoted" (ANSI SQL) or `backtick` (MySQL) style
QUOTED_IDENTIFIER
    : '"' ~["\r\n]* '"'
    | '`' ~[`\r\n]* '`'
    ;

// =============================================================================
// FRAGMENTS (building blocks, not tokens)
// =============================================================================

fragment ID_START: [a-zA-Z_];
fragment ID_BODY: [a-zA-Z0-9_];
fragment DIGIT: [0-9];

// Case-insensitive letter fragments
fragment A: [aA];
fragment B: [bB];
fragment C: [cC];
fragment D: [dD];
fragment E: [eE];
fragment F: [fF];
fragment G: [gG];
fragment H: [hH];
fragment I: [iI];
fragment J: [jJ];
fragment K: [kK];
fragment L: [lL];
fragment M: [mM];
fragment N: [nN];
fragment O: [oO];
fragment P: [pP];
fragment Q: [qQ];
fragment R: [rR];
fragment S: [sS];
fragment T: [tT];
fragment U: [uU];
fragment V: [vV];
fragment W: [wW];
fragment X: [xX];
fragment Y: [yY];
fragment Z: [zZ];
