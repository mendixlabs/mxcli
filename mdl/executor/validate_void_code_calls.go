// SPDX-License-Identifier: Apache-2.0

package executor

import (
	"sync"
	"time"

	"github.com/mendixlabs/mxcli/mdl/ast"
	"github.com/mendixlabs/mxcli/mdl/backend"
	"github.com/mendixlabs/mxcli/mdl/linter"
	"github.com/mendixlabs/mxcli/model"
	"github.com/mendixlabs/mxcli/sdk/microflows"
)

// voidCodeActions answers one question for the duplicate-variable checks: does
// a Java or JavaScript action call target an action that returns nothing?
//
// A call to a VOID action declares no variable, whatever output name it
// carries (ako/mxcli#953). Studio Pro keeps an output name on such a call — it
// names a JavaScript action's after the action (`$RefreshEntity`) and stores
// UseReturnVariable=true on some — so a model holding two of them is ordinary.
// Measured on mxbuild 11.13.0:
//
//	two void calls with the same output name            0 errors
//	void call `$V = …` then `declare $V String`          0 errors
//	void call `$V = …` then a log message using `$V`     CE0109 "Undefined variable 'V'"
//	void JS call `$W = …` then a non-void JS call `$W`   0 errors
//
// The name is inert: it neither collides with a later variable nor defines one.
//
// An action the resolver cannot find (no project, a runtime-provided System
// action) is NOT treated as void by `check`: the author wrote `$X =`, which
// normally asks for a value, and guessing void would silence a real CE0111.
// The editor makes the opposite trade (unknownIsVoid, ako/mxcli#962): a
// squiggle on a call it merely cannot resolve is a false refusal while typing,
// and `check` still runs before anything is written.
//
// Whatever that policy, only a call KNOWN to be void counts for the CE0109
// rule (MDL093): "possibly void" must never turn a use of the output into an
// error.
//
// Calls to a void MICROFLOW or NANOFLOW are the same case. Studio Pro stores
// them with an output name and UseReturnVariable=true (Evora Factory
// Management's OIDC.webCallback calls the void ACT_ShowCusomExceptionMessage
// three times as `$Variable_1`), and mxbuild treats the name as inert. Measured
// on mxbuild 10.24.15 and 11.13.0: two void microflow calls of one name, a void
// call then a declare of it, and two void nanoflow calls are 0 errors; reading
// a void microflow call's output is CE0109; two String microflow calls are
// CE0111 (control). Without the flows in here, check refused those stored
// microflows' own describe output with MDL063.
type voidCodeActions struct {
	// script holds the actions the script itself creates, keyed by
	// codeActionKey, valued true when the action returns Void.
	script map[string]bool
	// open returns the project to read stored actions from, or nil.
	open   func() backend.FullBackend
	opened bool
	b      backend.FullBackend
	cache  map[string]voidness
	// unknownIsVoid makes callIsVoid answer true for an action it cannot
	// resolve. Set by the editor (NewFlowRules); `check` leaves it false.
	unknownIsVoid bool
	// shared, when set, carries project resolutions across runs (the editor).
	shared *CodeActionCache
	// microflowNames, when set, names every stored microflow by ID, so a
	// called microflow is read by ID rather than found by a name lookup that
	// reads every microflow — once per call, per describe (#1789).
	microflowNames func() map[model.ID]string
	microflowIDs   map[string]model.ID

	// declared types: which qualified names are enumerations and which are
	// entities, for MDL043's wording (validate_declare_enum.go). The script's
	// own come from the program; the project's are read once, on first ask.
	scriptKinds  map[string]declaredTypeKind
	projectKinds map[string]declaredTypeKind
}

// voidness is what the resolver knows about one action.
type voidness struct {
	void  bool // the action returns Void
	known bool // the action was found, so void is an answer and not a default
}

// flowKey keys a microflow or nanoflow in the same maps as the code actions.
func flowKey(nanoflow bool, qn string) string {
	if nanoflow {
		return "nf:" + qn
	}
	return "mf:" + qn
}

// flowReturnsVoid reports whether a stored flow's return type is Void: no
// return type at all, or an explicit VoidType.
func flowReturnsVoid(rt microflows.DataType) bool {
	return rt == nil || rt.GetTypeName() == "Void"
}

func codeActionKey(javaScript bool, qn string) string {
	if javaScript {
		return "js:" + qn
	}
	return "java:" + qn
}

// newVoidCodeActions collects the script's own action declarations; open,
// which may be nil, supplies the project for the rest.
func newVoidCodeActions(prog *ast.Program, open func() backend.FullBackend) *voidCodeActions {
	r := &voidCodeActions{script: map[string]bool{}, open: open, cache: map[string]voidness{}}
	if prog == nil {
		return r
	}
	for _, stmt := range prog.Statements {
		switch s := stmt.(type) {
		case *ast.CreateJavaActionStmt:
			r.script[codeActionKey(false, s.Name.String())] = s.ReturnType.Kind == ast.TypeVoid
		case *ast.CreateJavaScriptActionStmt:
			r.script[codeActionKey(true, s.Name.String())] = s.ReturnType.Kind == ast.TypeVoid
		case *ast.CreateMicroflowStmt:
			r.script[flowKey(false, s.Name.String())] = s.ReturnType == nil || s.ReturnType.Type.Kind == ast.TypeVoid
		case *ast.CreateNanoflowStmt:
			r.script[flowKey(true, s.Name.String())] = s.ReturnType == nil || s.ReturnType.Type.Kind == ast.TypeVoid
		case *ast.CreateEnumerationStmt:
			r.recordScriptKind(s.Name.String(), declKindEnumeration)
		case *ast.CreateEntityStmt:
			r.recordScriptKind(s.Name.String(), declKindEntity)
		}
	}
	return r
}

func (r *voidCodeActions) project() backend.FullBackend {
	if !r.opened {
		r.opened = true
		if r.open != nil {
			r.b = r.open()
		}
	}
	return r.b
}

// isVoid reports whether the named action is known to return Void.
func (r *voidCodeActions) isVoid(javaScript bool, qn string) bool {
	return r.resolve(javaScript, qn).void
}

// resolve looks the named action up: in the script first, then in the project.
func (r *voidCodeActions) resolve(javaScript bool, qn string) voidness {
	if r == nil || qn == "" {
		return voidness{}
	}
	return r.lookup(codeActionKey(javaScript, qn), func(b backend.FullBackend) voidness {
		if javaScript {
			if a, err := b.ReadJavaScriptActionByName(qn); err == nil && a != nil && a.ReturnType != nil {
				return voidness{void: a.ReturnType.TypeString() == "Void", known: true}
			}
			return voidness{}
		}
		// The Java action reader returns a nil ReturnType for Void
		// (codeActionReturnTypeFromGen); the JavaScript one a VoidType.
		if a, err := b.ReadJavaActionByName(qn); err == nil && a != nil {
			return voidness{void: a.ReturnType == nil || a.ReturnType.TypeString() == "Void", known: true}
		}
		return voidness{}
	})
}

// resolveFlow looks a called microflow or nanoflow up: in the script first,
// then in the project.
func (r *voidCodeActions) resolveFlow(nanoflow bool, qn string) voidness {
	if r == nil || qn == "" {
		return voidness{}
	}
	return r.lookup(flowKey(nanoflow, qn), func(b backend.FullBackend) voidness {
		objectType := "microflow"
		if nanoflow {
			objectType = "nanoflow"
		}
		if id, ok := r.microflowID(nanoflow, qn); ok {
			contents, err := b.GetRawUnitBytes(id)
			if err != nil || len(contents) == 0 {
				return voidness{}
			}
			mf, err := b.ParseMicroflowBSON(contents, id, "")
			if err != nil || mf == nil {
				return voidness{}
			}
			return voidness{void: flowReturnsVoid(mf.ReturnType), known: true}
		}
		raw, err := b.GetRawUnitByName(objectType, qn)
		if err != nil || raw == nil || len(raw.Contents) == 0 {
			return voidness{}
		}
		mf, err := b.ParseMicroflowBSON(raw.Contents, model.ID(raw.ID), "")
		if err != nil || mf == nil {
			return voidness{}
		}
		return voidness{void: flowReturnsVoid(mf.ReturnType), known: true}
	})
}

// microflowID is the ID of the stored microflow named qn, from microflowNames
// inverted on first use. A name two microflows share (an excluded twin) has no
// answer here, so the by-name lookup decides between them as before.
func (r *voidCodeActions) microflowID(nanoflow bool, qn string) (model.ID, bool) {
	if nanoflow || r.microflowNames == nil {
		return "", false
	}
	if r.microflowIDs == nil {
		r.microflowIDs = map[string]model.ID{}
		for id, name := range r.microflowNames() {
			if _, dup := r.microflowIDs[name]; dup {
				r.microflowIDs[name] = ""
			} else {
				r.microflowIDs[name] = id
			}
		}
	}
	id := r.microflowIDs[qn]
	return id, id != ""
}

// lookup answers key from the script, the caches, and finally the project
// through read.
func (r *voidCodeActions) lookup(key string, read func(backend.FullBackend) voidness) voidness {
	if v, ok := r.script[key]; ok {
		return voidness{void: v, known: true}
	}
	if v, ok := r.cache[key]; ok {
		return v
	}
	if v, ok := r.shared.get(key); ok {
		r.cache[key] = v
		return v
	}
	var got voidness
	if b := r.project(); b != nil {
		got = read(b)
	}
	r.cache[key] = got
	r.shared.put(key, got)
	return got
}

// treatAsVoid applies the unknown-action policy to a resolution.
func (r *voidCodeActions) treatAsVoid(v voidness) bool {
	if v.known {
		return v.void
	}
	return r != nil && r.unknownIsVoid
}

// callResolution resolves the action a statement calls; ok is false for a
// statement that is not a Java or JavaScript action call.
func (r *voidCodeActions) callResolution(s ast.MicroflowStatement) (v voidness, ok bool) {
	switch st := s.(type) {
	case *ast.CallJavaActionStmt:
		return r.resolve(false, st.ActionName.String()), true
	case *ast.CallJavaScriptActionStmt:
		return r.resolve(true, st.ActionName.String()), true
	case *ast.CallMicroflowStmt:
		return r.resolveFlow(false, st.MicroflowName.String()), true
	case *ast.CallNanoflowStmt:
		return r.resolveFlow(true, st.NanoflowName.String()), true
	}
	return voidness{}, false
}

// callIsVoid reports whether a statement is a call to a void Java or
// JavaScript action — one whose output name declares nothing. An unresolvable
// action counts as void only under unknownIsVoid.
func (r *voidCodeActions) callIsVoid(s ast.MicroflowStatement) bool {
	v, ok := r.callResolution(s)
	return ok && r.treatAsVoid(v)
}

// callIsKnownVoid is callIsVoid without the unknown-action policy: true only
// for a call to an action the script or the project says returns Void.
func (r *voidCodeActions) callIsKnownVoid(s ast.MicroflowStatement) bool {
	v, ok := r.callResolution(s)
	return ok && v.known && v.void
}

// actionIsVoidCall is callIsVoid for a stored action, used by describe.
func (r *voidCodeActions) actionIsVoidCall(action any) bool {
	switch a := action.(type) {
	case *microflows.JavaActionCallAction:
		return r.treatAsVoid(r.resolve(false, a.JavaAction))
	case *microflows.JavaScriptActionCallAction:
		return r.treatAsVoid(r.resolve(true, a.JavaScriptAction))
	case *microflows.MicroflowCallAction:
		if a.MicroflowCall != nil {
			return r.treatAsVoid(r.resolveFlow(false, a.MicroflowCall.Microflow))
		}
	case *microflows.NanoflowCallAction:
		if a.NanoflowCall != nil {
			return r.treatAsVoid(r.resolveFlow(true, a.NanoflowCall.Nanoflow))
		}
	}
	return false
}

// FlowRules runs the microflow and nanoflow rule sets (ValidateMicroflow,
// ValidateNanoflow) for the editor, with the Java/JavaScript actions the flows
// call resolved through the script and, when there is one, the project.
//
// The language server validated without a project, so every call to a stored
// void action counted as declaring its output name, and two such calls — the
// ordinary Studio Pro shape — were squiggled as MDL063 (ako/mxcli#962). Here an
// action the script and the project cannot answer for is treated as POSSIBLY
// void (unknownIsVoid): the editor should not refuse what it cannot see, and
// `check`, which runs before exec writes anything, keeps the strict reading.
// The project is opened only when a call needs it; Close releases it.
type FlowRules struct {
	voids *voidCodeActions
	b     backend.FullBackend
}

// NewFlowRules prepares the flow rules for one program. projectPath may be
// empty: then nothing is read from disk and only the script answers. cache,
// which may be nil, keeps the project's answers between runs.
func NewFlowRules(prog *ast.Program, projectPath string, cache *CodeActionCache) *FlowRules {
	f := &FlowRules{}
	f.voids = newVoidCodeActions(prog, func() backend.FullBackend {
		f.b = openProjectForValidation(projectPath)
		return f.b
	})
	f.voids.unknownIsVoid = true
	if projectPath != "" && cache != nil {
		cache.forProject(projectPath)
		f.voids.shared = cache
	}
	return f
}

// CodeActionCache keeps what a project said about its Java/JavaScript actions
// across FlowRules runs. The editor validates on every keystroke, and reading
// one action from the project costs about 300ms on PedApp — paid again for
// every change of a document that calls one. An entry lives codeActionCacheTTL,
// so an action whose return type changes in Studio Pro is re-read soon after.
type CodeActionCache struct {
	mu      sync.Mutex
	project string
	entries map[string]voidness
	stamp   map[string]time.Time
	now     func() time.Time
}

const codeActionCacheTTL = 30 * time.Second

// NewCodeActionCache returns an empty cache.
func NewCodeActionCache() *CodeActionCache {
	return &CodeActionCache{now: time.Now}
}

// forProject empties the cache when it last served another project.
func (c *CodeActionCache) forProject(path string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.project != path || c.entries == nil {
		c.project = path
		c.entries = map[string]voidness{}
		c.stamp = map[string]time.Time{}
	}
}

func (c *CodeActionCache) get(key string) (voidness, bool) {
	if c == nil {
		return voidness{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	v, ok := c.entries[key]
	if !ok || c.now().Sub(c.stamp[key]) > codeActionCacheTTL {
		return voidness{}, false
	}
	return v, true
}

func (c *CodeActionCache) put(key string, v voidness) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.entries == nil {
		return
	}
	c.entries[key] = v
	c.stamp[key] = c.now()
}

// Microflow is ValidateMicroflow with this program's action resolution.
func (f *FlowRules) Microflow(stmt *ast.CreateMicroflowStmt) []linter.Violation {
	return validateMicroflowWith(stmt, f.voids)
}

// Nanoflow is ValidateNanoflow with this program's action resolution.
func (f *FlowRules) Nanoflow(stmt *ast.CreateNanoflowStmt) []linter.Violation {
	return validateNanoflowWith(stmt, f.voids)
}

// Close releases the project, if a call made the rules open it.
func (f *FlowRules) Close() {
	if f.b != nil {
		_ = f.b.Disconnect()
		f.b = nil
	}
}

// describedCallIsVoid reports whether describe should treat a stored call to
// the named microflow or nanoflow as a call to a flow KNOWN to return nothing.
//
// Studio Pro keeps a result variable name on such a call when the called flow
// used to return a value (`variable`, UseReturnVariable=true) and does not show
// it, since there is nothing to return. Describe printed it as an assignment,
// `$variable = call microflow …`, naming a variable that does not exist
// (mendixlabs/mxcli#1820, ako/mxcli#2138). A flow that cannot be found is not
// known to be void: the stored name is printed as before.
func describedCallIsVoid(ctx *ExecContext, nanoflow bool, qn string) bool {
	if ctx == nil || ctx.Backend == nil || qn == "" {
		return false
	}
	var r *voidCodeActions
	if m := flowAnalysisOf(ctx); m != nil && m.voids != nil {
		r = *m.voids
		if r == nil {
			r = newDescribeVoids(ctx)
			*m.voids = r
		}
	} else {
		r = newDescribeVoids(ctx)
	}
	v := r.resolveFlow(nanoflow, qn)
	return v.known && v.void
}

// newDescribeVoids is the resolver describe reads stored flows through: the
// project behind the context, with microflows found by ID (#1789).
func newDescribeVoids(ctx *ExecContext) *voidCodeActions {
	r := newVoidCodeActions(nil, func() backend.FullBackend { return ctx.Backend })
	if h, err := getHierarchy(ctx); err == nil && h != nil {
		r.microflowNames = func() map[model.ID]string { return getMicroflowNames(ctx, h) }
	}
	return r
}
