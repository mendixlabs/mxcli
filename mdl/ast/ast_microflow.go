// SPDX-License-Identifier: Apache-2.0

package ast

// ============================================================================
// Microflow Statements
// ============================================================================

// MicroflowStatement represents a statement inside a microflow body.
type MicroflowStatement interface {
	isMicroflowStatement()
}

// ============================================================================
// Error Handling
// ============================================================================

// ErrorHandlingType represents how errors are handled on a microflow activity.
type ErrorHandlingType string

const (
	ErrorHandlingContinue              ErrorHandlingType = "Continue"
	ErrorHandlingRollback              ErrorHandlingType = "Rollback"
	ErrorHandlingCustom                ErrorHandlingType = "Custom"
	ErrorHandlingCustomWithoutRollback ErrorHandlingType = "CustomWithoutRollBack"
)

// ErrorHandlingClause represents an ON ERROR clause on a microflow statement.
type ErrorHandlingClause struct {
	Type ErrorHandlingType
	Body []MicroflowStatement // non-nil for Custom/CustomWithoutRollback
}

// MicroflowParam represents a microflow parameter.
type MicroflowParam struct {
	Name     string    // Parameter name (without $ prefix)
	Type     DataType  // Parameter type
	Position *Position // @position(x, y) on the parameter; nil to let the layout place it
	// UnknownAnnotations holds annotation names written on the parameter that
	// mxcli does not implement there. Collected rather than dropped so MDL059
	// can refuse them: an annotation that parses and does nothing loses whatever
	// it was meant to express, silently (#884, the same reasoning one node
	// family over).
	UnknownAnnotations []string
}

// MicroflowReturnType represents a microflow return type.
type MicroflowReturnType struct {
	Type     DataType // Return type
	Variable string   // Variable name for AS $Var clause
}

// CreateMicroflowStmt represents: CREATE MICROFLOW Module.Name (params) RETURNS type BEGIN body END
type CreateMicroflowStmt struct {
	Name          QualifiedName
	Parameters    []MicroflowParam
	ReturnType    *MicroflowReturnType
	Body          []MicroflowStatement
	Documentation string
	// DocumentationSet records whether the statement carried a `/** … */`
	// comment at all, as opposed to carrying an empty one. A rewrite that did
	// not mention documentation preserves the stored value; an explicitly empty
	// comment clears it (mendixlabs/mxcli#1018).
	DocumentationSet bool
	Folder           string // Folder path within module (e.g., "Resources/Images")
	CreateOrModify   bool
	Excluded         bool // @excluded — document excluded from project
	// ApplyEntityAccess is Studio Pro's "apply entity access" checkbox, set by
	// `@applyentityaccess` / `@applyentityaccess(false)`.
	//
	// A POINTER because absent and false are different answers: an absent
	// annotation preserves what is stored (the setting is model state, like
	// @excluded), while an explicit false clears it. A plain bool would make
	// every rewrite that did not mention it turn the setting off, which is the
	// bug this field exists to fix.
	ApplyEntityAccess *bool
	// Expose holds the EXPOSED AS … ACTION clauses. A microflow has two toolbox
	// entries — one for the microflow editor, one for the workflow editor — so
	// there can be one of each.
	Expose []ExposeActionClause

	// URL is the deep link (Mendix 10.6+), e.g. `item/{Key}`. A POINTER for the
	// same reason as ApplyEntityAccess: absent means "the script does not say",
	// which preserves what is stored, while DROP URL sets an explicit empty.
	URL *string
	// URLSearchParameters are the parameter names named by `URL SEARCH
	// PARAMETERS (...)`, without the `$`. Nil means the clause was absent;
	// non-nil and empty means it was stated with an empty list, which clears.
	URLSearchParameters *[]string
	// ExportLevel is "API" or "Hidden"; nil preserves.
	ExportLevel *string
	// Concurrency is the DISALLOW/ALLOW CONCURRENT EXECUTION clause; nil
	// preserves what is stored.
	Concurrency *ConcurrencyClause
}

// ConcurrencyClause is one DISALLOW/ALLOW CONCURRENT EXECUTION clause.
//
// Mendix requires an error message or an error microflow when execution is
// disallowed (CE4899). The grammar accepts the bare DISALLOW so that the
// omission is reported by name rather than as a parse error; the check is
// types.CheckMicroflowConcurrency.
type ConcurrencyClause struct {
	// Allow is true for ALLOW CONCURRENT EXECUTION.
	Allow bool
	// ErrorMessage is the text shown to the second caller. Only one of
	// ErrorMessage / ErrorMicroflow is set.
	ErrorMessage string
	// ErrorMessageSet distinguishes an omitted message from an empty one, so a
	// stored message with translations is not silently replaced by "".
	ErrorMessageSet bool
	// ErrorMicroflow is the qualified name of the microflow that handles it.
	ErrorMicroflow string
}

// ExposeActionClause is one EXPOSED AS <kind> ACTION clause, or its NOT form.
//
// An absent clause is neither: it preserves what is stored, including the icon
// and image bitmaps MDL cannot express. Removal is explicit.
type ExposeActionClause struct {
	// Workflow selects WorkflowActionInfo over MicroflowActionInfo.
	Workflow bool
	Caption  string
	Category string
	Remove   bool // NOT EXPOSED AS … ACTION
	Bitmaps  []ExposeBitmap
}

// ExposeBitmap is one ICON/IMAGE clause on an exposed clause.
//
// An omitted bitmap is preserved rather than cleared, so Clear (DROP ICON) is
// how a script asks for one to go away — the same shape as the clause itself.
type ExposeBitmap struct {
	// Image selects the 256x192 toolbox image over the 64x64 icon.
	Image bool
	// Dark selects the dark-mode variant.
	Dark bool
	// Path is the PNG file to read, resolved against the working directory.
	// Empty when Clear is set.
	Path  string
	Clear bool
}

func (s *CreateMicroflowStmt) isStatement() {}

// DropMicroflowStmt represents: DROP MICROFLOW Module.Name
type DropMicroflowStmt struct {
	Name QualifiedName
	DropIfExists
}

func (s *DropMicroflowStmt) isStatement() {}

// CreateNanoflowStmt represents: CREATE NANOFLOW Module.Name (params) RETURNS type BEGIN body END
type CreateNanoflowStmt struct {
	Name             QualifiedName
	Parameters       []MicroflowParam
	ReturnType       *MicroflowReturnType
	Body             []MicroflowStatement
	Documentation    string
	DocumentationSet bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Folder           string // Folder path within module
	CreateOrModify   bool
	Excluded         bool // @excluded — document excluded from project
	// No ApplyEntityAccess: a nanoflow runs in the CLIENT and Mendix stores no
	// such property on Nanoflows$Nanoflow (modelsdk/gen declares the accessor
	// on Microflow and Rule only). Carrying it here would give the annotation
	// somewhere to parse and nothing to do.
	//
	// Expose is parsed but refused: only Microflows$Microflow carries the toolbox
	// properties. Accepting it in the grammar and explaining the refusal beats a
	// parse error that says only "no viable alternative".
	Expose []ExposeActionClause
}

func (s *CreateNanoflowStmt) isStatement() {}

// CreateRuleStmt represents: CREATE RULE Module.Name (params) RETURNS type BEGIN body END
//
// Mirrors CreateNanoflowStmt: a rule shares a microflow's body, so the fields
// are the same minus the ones a rule document has no property for (a rule stores
// no AllowedModuleRoles, so there is nothing to grant).
type CreateRuleStmt struct {
	Name             QualifiedName
	Parameters       []MicroflowParam
	ReturnType       *MicroflowReturnType
	Body             []MicroflowStatement
	Documentation    string
	DocumentationSet bool   // see mendixlabs/mxcli#1018: absent preserves, empty clears
	Folder           string // Folder path within module
	CreateOrModify   bool
	Excluded         bool // @excluded — document excluded from project
	// ApplyEntityAccess is Studio Pro's "apply entity access" checkbox, set by
	// `@applyentityaccess` / `@applyentityaccess(false)`.
	//
	// A POINTER because absent and false are different answers: an absent
	// annotation preserves what is stored (the setting is model state, like
	// @excluded), while an explicit false clears it. A plain bool would make
	// every rewrite that did not mention it turn the setting off, which is the
	// bug this field exists to fix.
	ApplyEntityAccess *bool
	// Expose is parsed but refused — see CreateNanoflowStmt.Expose.
	Expose []ExposeActionClause
}

func (s *CreateRuleStmt) isStatement() {}

// DropRuleStmt represents: DROP RULE Module.Name
type DropRuleStmt struct {
	Name QualifiedName
}

func (s *DropRuleStmt) isStatement() {}

// DropNanoflowStmt represents: DROP NANOFLOW Module.Name
type DropNanoflowStmt struct {
	Name QualifiedName
	DropIfExists
}

func (s *DropNanoflowStmt) isStatement() {}

// ============================================================================
// Microflow Body Statements
// ============================================================================

// DeclareStmt represents: DECLARE $Var Type = expr
type DeclareStmt struct {
	Variable      string               // Variable name (without $ prefix)
	Type          DataType             // Variable type
	InitialValue  Expression           // Optional initial value
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *DeclareStmt) isMicroflowStatement() {}

// EnumSplitCase represents one enumeration branch in an EnumSplit.
type EnumSplitCase struct {
	Value  string // First enumeration value, or "(empty)" for Mendix's empty enum case.
	Values []string
	Body   []MicroflowStatement
}

// EnumSplitStmt represents: CASE $Var WHEN value THEN body ... END CASE
type EnumSplitStmt struct {
	Variable    string // Variable or attribute path without $ prefix (e.g. EventType or Event/EventType)
	Cases       []EnumSplitCase
	ElseBody    []MicroflowStatement
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

// InheritanceSplitCase represents one typed branch in an inheritance split.
type InheritanceSplitCase struct {
	Entity QualifiedName
	Body   []MicroflowStatement
}

// InheritanceSplitStmt represents: SPLIT TYPE $Var ... END SPLIT
//
// ElseBody is Mendix's `(empty)` outgoing flow — the branch taken when the
// object is NULL. It is NOT a default for unmatched types: mxbuild demands a
// flow for every subtype and for the base entity regardless (CE0090), and
// omitting this one is CE0089. It is spelled `when (empty) then`; `else` is
// the legacy spelling that reads as a default and is not one (mxcli #913).
type InheritanceSplitStmt struct {
	Variable    string // Variable name without $ prefix
	Cases       []InheritanceSplitCase
	ElseBody    []MicroflowStatement
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation

	// Which spelling the source used, for the MDL065 deprecation warning only.
	// Both build the identical flow, so nothing downstream of the validator
	// may branch on these.
	LegacyCaseKeyword bool // at least one branch used `case X` instead of `when X then`
	LegacyElseKeyword bool // the empty branch used `else` instead of `when (empty) then`
}

func (s *EnumSplitStmt) isMicroflowStatement()        {}
func (s *InheritanceSplitStmt) isMicroflowStatement() {}

// CastObjectStmt represents: $Output = CAST $Object
type CastObjectStmt struct {
	OutputVariable string               // Output variable name without $ prefix
	ObjectVariable string               // Source object variable name without $ prefix
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CastObjectStmt) isMicroflowStatement() {}

// MfSetStmt represents: SET $Var = expr or SET $Var/Attr = expr
// (Named MfSetStmt to avoid conflict with existing SetStmt for SET key = value)
type MfSetStmt struct {
	Target        string               // Variable name or attribute path
	Value         Expression           // Value to assign
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *MfSetStmt) isMicroflowStatement() {}

// ReturnStmt represents: RETURN [expr]
type ReturnStmt struct {
	Value       Expression           // Optional return value
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *ReturnStmt) isMicroflowStatement() {}

// RaiseErrorStmt represents: RAISE ERROR
// Used in custom error handlers to terminate with an ErrorEvent instead of merging back.
type RaiseErrorStmt struct {
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *RaiseErrorStmt) isMicroflowStatement() {}

// AnchorSide identifies one of the four sides of an activity's visual box
// that a SequenceFlow attaches to.
type AnchorSide int

const (
	AnchorSideUnset  AnchorSide = -1
	AnchorSideTop    AnchorSide = 0
	AnchorSideRight  AnchorSide = 1
	AnchorSideBottom AnchorSide = 2
	AnchorSideLeft   AnchorSide = 3
)

// FlowAnchors captures the origin/destination anchors for a single
// SequenceFlow. Each side is independently optional: Unset means the builder
// should derive the anchor from the visual direction.
type FlowAnchors struct {
	From AnchorSide // OriginConnectionIndex on the outgoing SequenceFlow
	To   AnchorSide // DestinationConnectionIndex on the outgoing SequenceFlow
}

// ActivityAnnotations holds metadata annotations for microflow activities.
// These are emitted as @position, @caption, @color, @annotation, @excluded, @anchor lines in MDL.
type ActivityAnnotations struct {
	Position *Position // @position(x, y)
	Caption  string    // @caption 'text'
	Color    string    // @color Green
	// Notes are the @annotation lines attached to this statement, in source
	// order. A SLICE, not one string: see MicroflowAnnotation.
	Notes []MicroflowAnnotation

	// FreeNotes are @annotation lines that stand on their own — a note on the
	// canvas wired to nothing.
	FreeNotes []MicroflowAnnotation
	Excluded  bool         // @excluded
	Anchor    *FlowAnchors // @anchor(from: X, to: Y) — anchors of the flow leaving this statement

	// Split-specific anchors for IF statements. When the statement is not an
	// IF these remain nil. The grammar accepts them on IfStmt only:
	//   @anchor(true: (from: right, to: left), false: (from: bottom, to: left))
	TrueBranchAnchor  *FlowAnchors
	FalseBranchAnchor *FlowAnchors

	// Loop body anchors for LOOP/WHILE. IteratorAnchor is the flow that
	// enters the loop body from the iterator; BodyTailAnchor is the flow
	// from the last body statement back to the loop boundary. Both are only
	// populated on LoopStmt/WhileStmt.
	IteratorAnchor *FlowAnchors
	BodyTailAnchor *FlowAnchors

	// Curve is the bezier geometry of the flow LEAVING this statement:
	// @curve(from: (40, -90), to: (-40, 90)).
	//
	// Mendix stores no waypoints. A sequence flow's shape is two control
	// vectors on its Microflows$BezierCurve line — the tangent handles at each
	// end — so a hand-curved edge is a pair of (x, y) offsets, not a polyline.
	// Both writers already emit them; before #884 nothing could set them, so
	// they defaulted to "0;0" and any rewrite flattened a curve drawn in Studio
	// Pro. (upstream #884)
	Curve *FlowCurve

	// Merge positions the implicit merge node that closes a split — the end-if
	// join, or an enum/inheritance split's rejoin: @merge(x, y).
	//
	// The statement's own @position belongs to the SPLIT, so the merge needs its
	// own annotation. Before #884 it was placed by the layout pass alone and was
	// unaddressable, and it routinely landed on top of a neighbouring activity.
	Merge *Position

	// Start positions the StartEvent — the implicit node every flow begins at:
	// @start(x, y), written on the FIRST statement, the one the start flows into.
	//
	// Same shape and same reason as Merge: the node has no statement of its own,
	// so it is annotated on the statement it belongs to. Without it the start's
	// placement was inferred rather than stated, and the two things an inference
	// has to serve pull apart — a start a person dragged somewhere must survive a
	// rebuild (#884), while one mxcli derived must follow the activities when
	// they move (#951). An explicit position settles both by not guessing.
	//
	// DESCRIBE emits it only for a start that is not where the layout would have
	// put it, so a described flow round-trips exactly without every description
	// growing a line that just restates the arithmetic. (upstream #951)
	Start *Position

	// InvalidCurves holds the raw text of any @curve parameter whose coordinates
	// were not a whole-number (x, y) pair, so validation can refuse it rather
	// than silently straightening the edge.
	InvalidCurves []string

	// InvalidNotes holds the raw text of any `@annotation(...)` parameter
	// the visitor could not use — an unknown key, or a malformed `position:`/`size:`
	// pair — so validation can refuse it. Dropping it would lose the note
	// itself, not just the parameter.
	InvalidNotes []string

	// UnknownNames holds annotation names the visitor did not recognise, in
	// source order, so validation can refuse them.
	//
	// The visitor's switch has no default: an unrecognised name used to be
	// dropped in silence, which is benign for an annotation mxcli does not
	// implement (@size) and NOT benign for a typo of one it does — `@postion(10,
	// 20)` passed `check` and silently discarded the layout the author asked
	// for. Layout is the whole point of these annotations, so a name that does
	// nothing has to say so. (upstream #884)
	UnknownNames []string
}

// MicroflowAnnotation is one `@annotation` line — the yellow note Studio Pro
// draws beside an activity.
//
// In Mendix's model a note is a NODE with edges (`Microflows$Annotation` joined
// to activities by `Microflows$AnnotationFlow`), not a property of the activity
// it documents: one note can be wired to several activities, and several notes
// to one activity. MDL modelled it as a single string per activity, which lost
// both directions — a shared note came back copied once per target, and a
// second note on one activity overwrote the first, silently
// (mendixlabs/mxcli#1077). Hence a slice, and hence Label.
type MicroflowAnnotation struct {
	// Label is the `id:` in `@annotation(id: n1, text: '…')`. It exists only so
	// a later `@annotation(id: n1)` can attach the SAME note to another
	// activity instead of creating a second one. It is scoped to the flow being
	// authored and is NOT stored in the model — the describer re-derives labels
	// from scratch, so they are stable across a round trip by construction
	// rather than by being remembered.
	Label string

	// Text is the note's caption. Empty on a pure reference
	// (`@annotation(id: n1)`), which attaches a note already declared above.
	Text string

	// Position and Size are the note's own canvas geometry, which Mendix stores per
	// annotation and MDL had no way to spell. Nil means "let the writer place
	// it" — see defaultAnnotationGeometry in mdl/executor, which the builder and
	// the describer both consult so a round trip need not spell out a position
	// that can be re-derived.
	Position *Position
	Size     *BoxSize
}

// BoxSize is a width/height pair in canvas pixels.
type BoxSize struct {
	Width  int
	Height int
}

// FlowCurve is the pair of bezier control vectors on a sequence flow. Either end
// may be nil, which leaves that end straight.
type FlowCurve struct {
	From *Position // control vector at the origin end
	To   *Position // control vector at the destination end
}

// ChangeItem represents a single assignment in CREATE/CHANGE: Attr = expr
type ChangeItem struct {
	Attribute string     // Attribute name
	Value     Expression // Value expression
}

// CommitFlag is the Commit setting on a create/change activity, matching Mendix's
// Microflows$Commit enum. The zero value is CommitNo, which is Mendix's default and
// is therefore omitted from DESCRIBE output.
type CommitFlag int

const (
	CommitNo               CommitFlag = iota // no COMMIT clause
	CommitYes                                // COMMIT
	CommitYesWithoutEvents                   // COMMIT WITHOUT EVENTS
)

// CreateObjectStmt represents: $Var = CREATE Entity (assignments) [COMMIT [WITHOUT EVENTS]] [REFRESH] [ON ERROR ...]
type CreateObjectStmt struct {
	Variable        string               // Variable name (without $ prefix)
	EntityType      QualifiedName        // Entity type
	Changes         []ChangeItem         // SET assignments
	Commit          CommitFlag           // Commit setting (default CommitNo)
	RefreshInClient bool                 // Whether to refresh in client
	ErrorHandling   *ErrorHandlingClause // Optional ON ERROR clause
	Annotations     *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CreateObjectStmt) isMicroflowStatement() {}

// ChangeObjectStmt represents: CHANGE $Var (assignments) [COMMIT [WITHOUT EVENTS]] [REFRESH]
type ChangeObjectStmt struct {
	Variable        string               // Variable name
	Changes         []ChangeItem         // SET assignments
	Commit          CommitFlag           // Commit setting (default CommitNo)
	RefreshInClient bool                 // Whether to refresh in client
	Annotations     *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling   *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *ChangeObjectStmt) isMicroflowStatement() {}

// MfCommitStmt represents: COMMIT $Var [WITHOUT EVENTS] [REFRESH] [ON ERROR ...]
//
// The flag is WithoutEvents, not WithEvents, so that the zero value is what a
// bare `commit $Var;` means — Mendix's default, which is events ON (#895). Every
// other modifier on every other activity holds to that same invariant (absent
// modifier = zero value = Mendix default), and inverting this one field is what
// keeps it true here: a WithEvents bool would default to the one value Studio
// Pro never writes for a fresh Commit activity.
type MfCommitStmt struct {
	Variable      string // Variable to commit
	WithoutEvents bool   // WITHOUT EVENTS was written (absent = events on)
	// ExplicitWithEvents records that the redundant `WITH EVENTS` was written.
	// It changes nothing about the stored activity — both spellings mean events
	// on — and exists only so MDL067 can tell "the author said what they wanted"
	// from "the author said nothing", which is the whole question that note asks.
	ExplicitWithEvents bool
	RefreshInClient    bool                 // Whether to refresh in client
	ErrorHandling      *ErrorHandlingClause // Optional ON ERROR clause
	Annotations        *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *MfCommitStmt) isMicroflowStatement() {}

// DeleteObjectStmt represents: DELETE $Var [REFRESH] [ON ERROR ...]
type DeleteObjectStmt struct {
	Variable        string               // Variable to delete
	RefreshInClient bool                 // Whether to refresh in client
	ErrorHandling   *ErrorHandlingClause // Optional ON ERROR clause
	Annotations     *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *DeleteObjectStmt) isMicroflowStatement() {}

// RollbackStmt represents: ROLLBACK $Var [REFRESH]
type RollbackStmt struct {
	Variable        string               // Variable to rollback
	RefreshInClient bool                 // Whether to refresh in client
	Annotations     *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *RollbackStmt) isMicroflowStatement() {}

// RetrieveStmt represents: RETRIEVE $Var FROM Entity [WHERE condition] [SORT BY ...] [LIMIT n] [OFFSET n] [ON ERROR ...]
// or: RETRIEVE $Var FROM $Parent/Module.Association (association retrieve)
type RetrieveStmt struct {
	Variable      string               // Output variable
	Source        QualifiedName        // Entity (database) or Association (association retrieve)
	StartVariable string               // Non-empty for association retrieve: the starting variable name
	Where         Expression           // Optional WHERE condition
	SortColumns   []SortColumnDef      // Optional SORT BY columns
	Limit         string               // Optional LIMIT expression (empty = no limit)
	Offset        string               // Optional OFFSET expression (empty = no offset)
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *RetrieveStmt) isMicroflowStatement() {}

// IfStmt represents: IF expr THEN body [ELSE body] END IF
type IfStmt struct {
	Condition   Expression           // IF condition
	ThenBody    []MicroflowStatement // THEN branch
	ElseBody    []MicroflowStatement // ELSE branch (optional)
	HasElse     bool                 // true when the source contained ELSE, even if the body is empty
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *IfStmt) isMicroflowStatement() {}

// LoopStmt represents: LOOP $Var IN $List BEGIN body END LOOP
type LoopStmt struct {
	LoopVariable string               // Iterator variable name
	ListVariable string               // List variable name
	Body         []MicroflowStatement // Loop body
	Annotations  *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *LoopStmt) isMicroflowStatement() {}

// WhileStmt represents: WHILE expr BEGIN body END WHILE
type WhileStmt struct {
	Condition   Expression           // WHILE condition expression
	Body        []MicroflowStatement // Loop body
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *WhileStmt) isMicroflowStatement() {}

// LogLevel represents the severity level for LOG statements.
type LogLevel int

const (
	LogTrace LogLevel = iota
	LogDebug
	LogInfo
	LogWarning
	LogError
	LogCritical
)

func (l LogLevel) String() string {
	switch l {
	case LogTrace:
		return "Trace"
	case LogDebug:
		return "Debug"
	case LogInfo:
		return "Info"
	case LogWarning:
		return "Warning"
	case LogError:
		return "Error"
	case LogCritical:
		return "Critical"
	default:
		return "Info"
	}
}

// TemplateParam represents a parameter in string template WITH clause: {1} = expr
// Used by LOG statements (microflows) and CONTENT/captions (pages).
// Supports both simple expressions and data source attribute references ($Widget.Attr).
type TemplateParam struct {
	Index          int        // Placeholder index (1, 2, 3, ...)
	Value          Expression // Value expression (for general expressions)
	DataSourceName string     // Widget name for $WidgetName.Attribute syntax (empty if not a DS ref)
	AttributeName  string     // Attribute name for $WidgetName.Attribute syntax
}

// IsDataSourceRef returns true if this is a data source attribute reference.
func (p *TemplateParam) IsDataSourceRef() bool {
	return p.DataSourceName != ""
}

// LogStmt represents: LOG LEVEL [NODE expr] message [WITH params]
type LogStmt struct {
	Level         LogLevel             // Log level (INFO, WARNING, etc.)
	Node          Expression           // Optional log node expression
	Message       Expression           // Message expression
	Template      []TemplateParam      // Optional WITH template params
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *LogStmt) isMicroflowStatement() {}

// CallArgument represents a parameter in CALL: name = expr
type CallArgument struct {
	Name  string     // Parameter name
	Value Expression // Value expression
}

// CallMicroflowStmt represents: [$Result =] CALL MICROFLOW Name (args) [ON ERROR ...]
type CallMicroflowStmt struct {
	OutputVariable string               // Optional output variable
	MicroflowName  QualifiedName        // Microflow to call
	Arguments      []CallArgument       // Arguments
	Queue          *QualifiedName       // Optional IN QUEUE clause (task queue to run the call on)
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallMicroflowStmt) isMicroflowStatement() {}

// CallNanoflowStmt represents: [$Result =] CALL NANOFLOW Name (args) [ON ERROR ...]
type CallNanoflowStmt struct {
	OutputVariable string               // Optional output variable
	NanoflowName   QualifiedName        // Nanoflow to call
	Arguments      []CallArgument       // Arguments
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallNanoflowStmt) isMicroflowStatement() {}

// CallJavaActionStmt represents: CALL JAVA ACTION Name (args) [ON ERROR ...]
type CallJavaActionStmt struct {
	OutputVariable string               // Optional output variable
	ActionName     QualifiedName        // Java action name
	Arguments      []CallArgument       // Arguments
	Queue          *QualifiedName       // Optional IN QUEUE clause (task queue to run the call on)
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallJavaActionStmt) isMicroflowStatement() {}

// CallJavaScriptActionStmt represents: CALL JAVASCRIPT ACTION Name (args) [ON ERROR ...]
type CallJavaScriptActionStmt struct {
	OutputVariable string               // Optional output variable
	ActionName     QualifiedName        // JavaScript action name
	Arguments      []CallArgument       // Arguments
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallJavaScriptActionStmt) isMicroflowStatement() {}

// CallWebServiceStmt represents a legacy SOAP web service call.
type CallWebServiceStmt struct {
	OutputVariable string         // Optional output variable
	RawBSONBase64  string         // Raw Microflows$CallWebServiceAction BSON for lossless roundtrip
	ServiceID      string         // Consumed web service ID or qualified name
	OperationName  string         // Operation name
	Arguments      []CallArgument // Optional operation arguments — Microflows$SimpleRequestHandling
	SendMappingID  string         // Optional export mapping ID or qualified name
	// SendMappingVariable is the variable the export mapping maps FROM. An
	// export mapping always maps an object, so a send mapping without one is
	// incomplete — Mendix stores it as MappingRequestHandling.MappingVariableName.
	SendMappingVariable string
	ReceiveMappingID    string               // Optional import mapping ID or qualified name
	Timeout             Expression           // Optional timeout expression
	ErrorHandling       *ErrorHandlingClause // Optional ON ERROR clause
	Annotations         *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallWebServiceStmt) isMicroflowStatement() {}

// ExecuteDatabaseQueryStmt represents: EXECUTE DATABASE QUERY Module.Connection.QueryName ...
type ExecuteDatabaseQueryStmt struct {
	OutputVariable string // Optional output variable
	QueryName      string // Full 3-part identifier: Module.Connection.QueryName
	DynamicQuery   string // Optional dynamic SQL override
	// DynamicQueryIsExpression distinguishes `dynamic $Sql` from `dynamic 'SELECT …'`.
	// Both reach the executor as a bare string, and the builder has to quote one
	// and not the other: quoting an expression sends the literal text `$Sql` to
	// the database, which is a syntax error at the far end, not a Mendix one.
	DynamicQueryIsExpression bool
	Arguments                []CallArgument       // Parameter mappings (query parameters)
	ConnectionArguments      []CallArgument       // Connection parameter mappings (runtime connection override)
	ErrorHandling            *ErrorHandlingClause // Optional ON ERROR clause
	Annotations              *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *ExecuteDatabaseQueryStmt) isMicroflowStatement() {}

// CallExternalActionStmt represents: CALL EXTERNAL ACTION Service.ActionName (args) [ON ERROR ...]
type CallExternalActionStmt struct {
	OutputVariable string               // Optional output variable
	ServiceName    QualifiedName        // Consumed OData service qualified name
	ActionName     string               // External action name
	Arguments      []CallArgument       // Arguments
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CallExternalActionStmt) isMicroflowStatement() {}

// BreakStmt represents: BREAK
type BreakStmt struct {
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *BreakStmt) isMicroflowStatement() {}

// ContinueStmt represents: CONTINUE
type ContinueStmt struct {
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *ContinueStmt) isMicroflowStatement() {}

// MergeStmt represents: MERGE <label>
//
// Declares an ExclusiveMerge that other paths reach with JoinStmt. The label is
// an MDL-only handle — Mendix stores no name on a merge — so it lives no longer
// than one build or one describe.
type MergeStmt struct {
	Label       string
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *MergeStmt) isMicroflowStatement() {}

// JoinStmt represents: JOIN <label>
//
// Ends this path at the merge declared under Label. Forward and backward
// references both resolve, so a join may precede its merge (crossed branches)
// or follow it (a retry loop).
type JoinStmt struct {
	Label       string
	Annotations *ActivityAnnotations
}

func (s *JoinStmt) isMicroflowStatement() {}

// ============================================================================
// List Operations
// ============================================================================

// ListOperationType represents the type of list operation.
type ListOperationType int

const (
	ListOpHead ListOperationType = iota
	ListOpTail
	ListOpFind
	ListOpFilter
	ListOpSort
	ListOpUnion
	ListOpIntersect
	ListOpSubtract
	ListOpContains
	ListOpEquals
	ListOpRange
)

func (t ListOperationType) String() string {
	switch t {
	case ListOpHead:
		return "HEAD"
	case ListOpTail:
		return "TAIL"
	case ListOpFind:
		return "FIND"
	case ListOpFilter:
		return "FILTER"
	case ListOpSort:
		return "SORT"
	case ListOpUnion:
		return "UNION"
	case ListOpIntersect:
		return "INTERSECT"
	case ListOpSubtract:
		return "SUBTRACT"
	case ListOpContains:
		return "CONTAINS"
	case ListOpEquals:
		return "EQUALS"
	case ListOpRange:
		return "RANGE"
	default:
		return "UNKNOWN"
	}
}

// SortSpec represents a sort specification: attr ASC/DESC
type SortSpec struct {
	Attribute string // Attribute name
	Ascending bool   // True for ASC, false for DESC
}

// ListOperationStmt represents list operations like HEAD, TAIL, FIND, etc.
// $Var = HEAD($List)
// $Var = FIND($List, condition)
// $Var = SORT($List, attr ASC)
// $Var = UNION($List1, $List2)
type ListOperationStmt struct {
	OutputVariable string               // Output variable name
	Operation      ListOperationType    // Operation type
	InputVariable  string               // Input list variable (first operand)
	SecondVariable string               // Second operand for UNION, INTERSECT, SUBTRACT, CONTAINS, EQUALS
	Condition      Expression           // Condition for FIND/FILTER
	SortSpecs      []SortSpec           // Sort specifications for SORT
	OffsetExpr     Expression           // Offset expression for RANGE
	LimitExpr      Expression           // Limit expression for RANGE
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	// ErrorHandling is recorded only so the clause can be REFUSED. Mendix's
	// ListOperationsAction has no ErrorHandlingType, so an ON ERROR here has
	// nowhere to go; parsing it and reporting it beats dropping it silently.
	ErrorHandling *ErrorHandlingClause
	// UnresolvedOperands holds the list operands that did not reduce to a
	// variable name — see UnresolvedOperand. Empty on every well-formed statement.
	UnresolvedOperands []UnresolvedOperand
}

func (s *ListOperationStmt) isMicroflowStatement() {}

// UnresolvedOperand is a list operand that the visitor could not reduce to a
// variable name.
//
// A Mendix list-operation or aggregate activity stores its list as a VARIABLE
// REFERENCE — there is no slot for a nested computation. So MDL's expression
// grammar accepts `count(filter($l, …))`, which looks composable, but the model
// has nowhere to put the inner call. The conversion used to drop it silently and
// write the activity with an empty List, which passes `check`, execs with a
// success message, and fails the build with CE0012 / CE0096 (mendixlabs/mxcli#1101).
//
// Recording what was dropped — rather than leaving an empty InputVariable behind
// — is what lets the validator name the operand and print the two-statement
// rewrite. Expr is nil when the operand was absent altogether.
type UnresolvedOperand struct {
	Index int        // 0 = the list; 1 = the second list of UNION/INTERSECT/SUBTRACT/CONTAINS/EQUALS
	Expr  Expression // what was written there, for the diagnostic
}

// AggregateListOperationType represents the type of aggregate operation.
type AggregateListOperationType int

const (
	AggregateCount AggregateListOperationType = iota
	AggregateSum
	AggregateAverage
	AggregateMinimum
	AggregateMaximum
	AggregateReduce
	AggregateAll
	AggregateAny
)

func (t AggregateListOperationType) String() string {
	switch t {
	case AggregateCount:
		return "COUNT"
	case AggregateSum:
		return "SUM"
	case AggregateAverage:
		return "AVERAGE"
	case AggregateMinimum:
		return "MINIMUM"
	case AggregateMaximum:
		return "MAXIMUM"
	case AggregateReduce:
		return "REDUCE"
	case AggregateAll:
		return "ALL"
	case AggregateAny:
		return "ANY"
	default:
		return "UNKNOWN"
	}
}

// AggregateListStmt represents aggregate operations: COUNT, SUM, AVERAGE, etc.
// $Count = COUNT($List)
// $Sum = SUM($List/Attr)
// $Sum = SUM($List, $currentObject/Price * 2)  // expression form
type AggregateListStmt struct {
	OutputVariable string                     // Output variable name
	Operation      AggregateListOperationType // Operation type
	InputVariable  string                     // Input list variable
	Attribute      string                     // Attribute name for SUM/AVG/MIN/MAX (empty for COUNT or expression form)
	IsExpression   bool                       // true when Expression is used instead of Attribute
	Expression     Expression                 // Mendix expression (when IsExpression=true)

	// REDUCE only. InitialValue seeds $currentResult; ReturnType is the type the
	// fold produces. Mendix requires both and neither can be inferred from the
	// expression, so REDUCE names them and the other functions leave them zero.
	InitialValue Expression
	ReturnType   *DataType

	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	// ErrorHandling is recorded only so the clause can be REFUSED — Mendix's
	// AggregateAction has no ErrorHandlingType. See ListOperationStmt.
	ErrorHandling *ErrorHandlingClause
	// UnresolvedOperands holds the list operand that did not reduce to a variable
	// name — see UnresolvedOperand. Empty on every well-formed statement.
	UnresolvedOperands []UnresolvedOperand
}

func (s *AggregateListStmt) isMicroflowStatement() {}

// CreateListStmt represents: $Var = CREATE LIST OF Entity
type CreateListStmt struct {
	Variable    string               // Output variable name
	EntityType  QualifiedName        // Entity type for the list
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *CreateListStmt) isMicroflowStatement() {}

// AddToListStmt represents: ADD expr TO $List
type AddToListStmt struct {
	Item        string               // Item variable to add, kept for simple $Var compatibility
	Value       Expression           // Item expression to add
	List        string               // Target list variable
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *AddToListStmt) isMicroflowStatement() {}

// RemoveFromListStmt represents: REMOVE $Item FROM $List
type RemoveFromListStmt struct {
	Item        string               // Item variable to remove
	List        string               // Source list variable
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *RemoveFromListStmt) isMicroflowStatement() {}

// ============================================================================
// Page Actions
// ============================================================================

// ShowPageArg represents a page parameter argument: $Param = $Value
type ShowPageArg struct {
	ParamName string     // Parameter name (without $ prefix)
	Value     Expression // Value expression
}

// ShowPageStmt represents: SHOW PAGE Module.Page($param = $value) [FOR $obj] [WITH (settings)]
type ShowPageStmt struct {
	PageName      QualifiedName        // Page to show
	Arguments     []ShowPageArg        // Page parameter arguments
	ForObject     string               // Optional FOR variable (without $ prefix)
	Title         string               // Optional title override
	Location      string               // Optional location: Content, Popup, Modal (default: Content)
	ModalForm     bool                 // Whether to show as modal
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *ShowPageStmt) isMicroflowStatement() {}

// ClosePageStmt represents: CLOSE PAGE
type ClosePageStmt struct {
	NumberOfPages int                  // Number of pages to close (default 1)
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *ClosePageStmt) isMicroflowStatement() {}

// ShowHomePageStmt represents: SHOW HOME PAGE
type ShowHomePageStmt struct {
	Annotations *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *ShowHomePageStmt) isMicroflowStatement() {}

// ShowMessageStmt represents: SHOW MESSAGE 'text' TYPE Information OBJECTS [$Var1, $Var2];
type ShowMessageStmt struct {
	Message       Expression           // The message text (string template)
	Type          string               // Information, Warning, Error (default: Information)
	TemplateArgs  []Expression         // Template arguments for message placeholders {1}, {2}, etc.
	Blocking      bool                 // BLOCKING — the message halts the client until dismissed
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *ShowMessageStmt) isMicroflowStatement() {}

// DownloadFileStmt represents: DOWNLOAD FILE $FileDocument [SHOW IN BROWSER]
type DownloadFileStmt struct {
	FileDocument  string               // File document variable without $ prefix
	ShowInBrowser bool                 // Whether the file opens in the browser
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *DownloadFileStmt) isMicroflowStatement() {}

// SynchronizeStmt represents: SYNCHRONIZE ALL | UNSYNCHRONIZED | $Var[, $Var...]
//
// Nanoflow-only — Mendix rejects the activity in a (server-side) microflow.
type SynchronizeStmt struct {
	// SyncType is the platform enum value: All, Unsynchronized or Specific.
	SyncType string
	// Variables holds the object/list variable names (no $) for Specific mode.
	Variables     []string
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *SynchronizeStmt) isMicroflowStatement() {}

// ValidationFeedbackStmt represents: VALIDATION FEEDBACK $Var/Attr MESSAGE 'message' OBJECTS [$Var1, $Var2];
type ValidationFeedbackStmt struct {
	AttributePath *AttributePathExpr   // The attribute to associate with the feedback
	Message       Expression           // The feedback message (string template)
	TemplateArgs  []Expression         // Template arguments for message placeholders
	Annotations   *ActivityAnnotations // Optional @position, @caption, @color, @annotation
	ErrorHandling *ErrorHandlingClause // Optional ON ERROR clause
}

func (s *ValidationFeedbackStmt) isMicroflowStatement() {}

// ============================================================================
// REST Call Statements
// ============================================================================

// HttpMethod represents an HTTP method for REST calls.
type HttpMethod string

const (
	HttpMethodGet    HttpMethod = "Get"
	HttpMethodPost   HttpMethod = "Post"
	HttpMethodPut    HttpMethod = "Put"
	HttpMethodPatch  HttpMethod = "Patch"
	HttpMethodDelete HttpMethod = "Delete"
)

// RestHeader represents a custom HTTP header: HEADER name = value
type RestHeader struct {
	Name  string     // Header name (e.g., "Accept", "Content-Type")
	Value Expression // Header value expression
}

// RestAuth represents HTTP authentication configuration.
type RestAuth struct {
	Username Expression // Username expression
	Password Expression // Password expression
}

// RestBodyType represents the type of request body handling.
type RestBodyType int

const (
	RestBodyNone    RestBodyType = iota // No body
	RestBodyCustom                      // Custom body template
	RestBodyMapping                     // Export mapping
	RestBodyBinary                      // Binary body: the raw bytes of an expression
)

// RestBody represents the request body configuration.
type RestBody struct {
	Type RestBodyType // Body type
	// Template is the body template for Custom, and for Binary the expression
	// yielding the bytes to send — Studio Pro stores a FileDocument's Contents
	// member there, e.g. `$Doc/Contents`.
	Template       Expression      // Body template (for Custom type)
	TemplateParams []TemplateParam // Template parameters for placeholders
	MappingName    QualifiedName   // Export mapping name (for Mapping type)
	SourceVariable string          // Source variable for mapping
}

// RestResultType represents how the response should be handled.
type RestResultType int

const (
	RestResultString   RestResultType = iota // Return as string
	RestResultResponse                       // Return HttpResponse object
	RestResultMapping                        // Use import mapping
	RestResultNone                           // Ignore response
	// RestResultFileDocument stores the response in a file document. Mendix
	// requires a SPECIALIZATION here: `System.FileDocument` itself is rejected
	// as a return type with CE0362, so ResultEntity always names a subclass.
	RestResultFileDocument
)

// RestResult represents the response handling configuration.
type RestResult struct {
	Type         RestResultType // Result type
	MappingName  QualifiedName  // Import mapping name (for Mapping type)
	ResultEntity QualifiedName  // Result entity type (for Mapping and FileDocument types)
	// IsList distinguishes `as Module.Entity` (single object) from
	// `as list of Module.Entity` (list). Studio Pro stores this on the
	// microflow's ImportMappingCall (Range.SingleObject /
	// ForceSingleOccurrence), independently of whether the underlying
	// import mapping is list-typed: the same mapping can yield either a
	// single object or a list depending on this flag.
	IsList bool
}

// RestCallStmt represents: $Var = REST CALL METHOD url [HEADER ...] [AUTH ...] [BODY ...] [TIMEOUT ...] RETURNS ...
type RestCallStmt struct {
	OutputVariable string               // Optional output variable
	Method         HttpMethod           // HTTP method (GET, POST, PUT, PATCH, DELETE)
	URL            Expression           // URL expression (string literal or expression)
	URLParams      []TemplateParam      // URL template parameters
	Headers        []RestHeader         // Custom HTTP headers
	Auth           *RestAuth            // Optional authentication
	Body           *RestBody            // Optional request body
	Timeout        Expression           // Optional timeout expression (seconds)
	Result         RestResult           // Response handling
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *RestCallStmt) isMicroflowStatement() {}

// SendRestRequestStmt represents: [$Var =] SEND REST REQUEST Module.Service.Operation [BODY $var] [ON ERROR ...]
// Calls a consumed REST service operation defined via CREATE REST CLIENT.
type SendRestRequestStmt struct {
	OutputVariable string               // Optional output variable (without $)
	Operation      QualifiedName        // Consumed REST service operation (Module.Service.Operation)
	Parameters     []SendRestParamDef   // Parameter bindings from WITH clause
	BodyVariable   string               // Optional body variable name (without $)
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

// SendRestParamDef represents a parameter binding: $paramName = expression
type SendRestParamDef struct {
	Name       string // parameter name (without $)
	Expression string // Mendix expression
}

func (s *SendRestRequestStmt) isMicroflowStatement() {}

// ImportFromMappingStmt represents: [$Var =] IMPORT FROM MAPPING Module.IMM($SourceVar)
type ImportFromMappingStmt struct {
	OutputVariable string               // Optional result variable (without $)
	Mapping        QualifiedName        // Import mapping qualified name
	SourceVariable string               // Input string variable (without $)
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation

	// Range — how much of the mapping's result to bind. Mendix stores this on
	// the ImportMappingCall as ConstantRange{SingleObject} or
	// CustomRange{LimitExpression, OffsetExpression}; before #881 MDL could say
	// none of it, so all three settings described identically and a
	// describe→edit→exec cycle silently changed the activity's meaning.
	//
	// All fields unset = the range was not authored, and the builder keeps
	// inferring cardinality from the mapping's own root shape, as it always has.
	// DESCRIBE always emits one of All/First/Limit so a round trip cannot fall
	// back on that inference and change the activity's meaning.
	All        bool       // ALL   — bind the whole list, explicitly
	First      bool       // FIRST — bind ONE object rather than a list
	LimitExpr  Expression // LIMIT <expr>  — Custom range
	OffsetExpr Expression // OFFSET <expr> — Custom range
}

func (s *ImportFromMappingStmt) isMicroflowStatement() {}

// ExportToMappingStmt represents: $Var = EXPORT TO MAPPING Module.EMM($SourceVar)
type ExportToMappingStmt struct {
	OutputVariable string               // Result string variable (without $)
	Mapping        QualifiedName        // Export mapping qualified name
	SourceVariable string               // Input entity variable (without $)
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *ExportToMappingStmt) isMicroflowStatement() {}

// TransformJsonStmt represents: $Result = TRANSFORM $Input WITH Module.Transformer
type TransformJsonStmt struct {
	OutputVariable string               // Result string variable (without $)
	InputVariable  string               // Source JSON string variable (without $)
	Transformation QualifiedName        // Data transformer qualified name
	ErrorHandling  *ErrorHandlingClause // Optional ON ERROR clause
	Annotations    *ActivityAnnotations // Optional @position, @caption, @color, @annotation
}

func (s *TransformJsonStmt) isMicroflowStatement() {}
