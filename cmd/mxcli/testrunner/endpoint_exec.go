// SPDX-License-Identifier: Apache-2.0

package testrunner

// endpointAllowExecEnv switches the endpoint's exec route on. mxcli sets it in
// the runtime's environment only for `run --local --test-endpoint --allow-call`,
// so the route is off in every other run, including every `mxcli test --local`.
//
// The route is gated rather than left out of the generated Java so the endpoint
// script stays one constant: a build of it is the same build whatever the run,
// which is what lets a reused deployment (and the reuse key) ignore the flag.
const endpointAllowExecEnv = "MXCLI_TEST_ALLOW_EXEC"

// endpointExecRoute is the route the executor listens on.
const endpointExecRoute = "exec"

// endpointExecJava is the executor: the methods of the request handler that run
// a compiled test (interp.go) inside the running app.
//
// What it widens, and why that is behind --allow-call: the run route only ever
// runs a microflow this runner generated. The executor calls the microflows a
// test names, creates, changes, commits and deletes objects and queries by
// XPath, all in a system context. That is what a test body does in its
// generated microflow too, but here it is reachable without a build, so it is
// off unless the developer asked for it, and like every route it still needs
// the token and a loopback caller.
//
// Every expression goes to Core.evaluateExpression, so it means what it means in
// a microflow. Before each evaluation the executor checks what that evaluator
// does not: a variable it does not know, and an enumeration value that does not
// exist, both of which it returns as empty instead of failing (measured,
// mdl-examples/spikes/evaluate-expression.mdl).
//
// Verdicts are the strings the generated microflows return, decided at the same
// points: an error in a call is a FAIL (or the outcome of an @throws), an error
// anywhere else escapes the test and is reported as an ERROR, an @expect is
// checked only after the whole body ran, and the first failing one is the
// verdict.
//
// It must never contain two dollar signs in a row: the script embeds it in an
// MDL dollar-quoted literal, which ends at the first one.
const endpointExecJava = `
    private final boolean execAllowed = "1".equals(System.getenv("` + endpointAllowExecEnv + `"));

    private void doExec(com.mendix.m2ee.api.IMxRuntimeRequest request,
                        com.mendix.m2ee.api.IMxRuntimeResponse response,
                        java.io.Writer out) throws Exception {
        if (!execAllowed) {
            response.setStatus(com.mendix.m2ee.api.IMxRuntimeResponse.FORBIDDEN);
            out.write("{\"error\":\"the exec route is off: start the app with mxcli run --local --test-endpoint --allow-call\"}");
            out.flush();
            return;
        }
        Object servlet = com.mendix.m2ee.api.IMxRuntimeRequest.class.getMethod("getHttpServletRequest").invoke(request);
        String method = String.valueOf(servlet.getClass().getMethod("getMethod").invoke(servlet));
        if (!"POST".equalsIgnoreCase(method)) {
            response.setStatus(405);
            out.write("{\"error\":\"the exec route takes POST\"}");
            out.flush();
            return;
        }
        java.util.Map<String, Object> prog;
        try {
            java.io.InputStream in = (java.io.InputStream) servlet.getClass().getMethod("getInputStream").invoke(servlet);
            String body = new String(in.readAllBytes(), java.nio.charset.StandardCharsets.UTF_8);
            prog = asMap(new JsonReader(body).read(), "the request");
            Object v = prog.get("v");
            if (!(v instanceof Long) || ((Long) v).longValue() != ` + execProtocolVersionJava + `) {
                throw new IllegalArgumentException("unsupported step format version " + v + "; this endpoint reads ` + execProtocolVersionJava + `");
            }
        } catch (Exception e) {
            response.setStatus(com.mendix.m2ee.api.IMxRuntimeResponse.BAD_REQUEST);
            out.write("{\"error\":" + esc("bad exec request: " + e.getMessage()) + "}");
            out.flush();
            return;
        }

        String test = String.valueOf(prog.get("test"));
        boolean rollback = Boolean.TRUE.equals(prog.get("rollback"));
        long t0 = System.nanoTime();
        com.mendix.systemwideinterfaces.core.IContext ctx = com.mendix.core.Core.createSystemContext();
        String result = null, error = null, rollbackError = null;
        boolean rolledBack = false;
        if (rollback) ctx.startTransaction();
        try {
            result = execProgram(ctx, prog);
        } catch (Throwable t) {
            error = errorMessage(t);
        } finally {
            if (rollback) {
                try {
                    ctx.rollbackTransaction();
                    rolledBack = true;
                } catch (Throwable t) {
                    rollbackError = errorMessage(t);
                }
            }
        }
        long micros = (System.nanoTime() - t0) / 1000L;

        StringBuilder b = new StringBuilder("{");
        b.append("\"mf\":").append(esc(test));
        b.append(",\"ok\":").append(error == null);
        b.append(",\"durationMicros\":").append(micros);
        b.append(",\"result\":").append(result == null ? "null" : esc(result));
        if (error != null) b.append(",\"error\":").append(esc(error));
        b.append(",\"rollbackRequested\":").append(rollback);
        b.append(",\"rolledBack\":").append(rolledBack);
        if (rollbackError != null) b.append(",\"rollbackError\":").append(esc(rollbackError));
        b.append('}');
        out.write(b.toString());
        out.flush();
    }

    // execProgram runs one compiled test and returns its verdict.
    private String execProgram(com.mendix.systemwideinterfaces.core.IContext ctx,
                               java.util.Map<String, Object> prog) throws Exception {
        java.util.Map<String, Object> vars = new java.util.HashMap<String, Object>();
        for (Object s : asList(prog.get("setups"))) {
            String flow = String.valueOf(s);
            try {
                com.mendix.core.Core.microflowCall(flow).execute(ctx);
            } catch (Throwable t) {
                return "` + verdictSetupPrefix + `" + flow;
            }
        }
        java.util.Map<String, Object> throwsSpec = prog.get("throws") == null ? null : asMap(prog.get("throws"), "throws");
        for (Object o : asList(prog.get("steps"))) {
            java.util.Map<String, Object> st = asMap(o, "a step");
            if (!"call".equals(st.get("op"))) {
                // Outside a call, an error escapes the test, as it does from the
                // generated microflow: an ERROR, never a verdict.
                execStep(ctx, st, vars);
                continue;
            }
            try {
                execCall(ctx, st, vars);
            } catch (Throwable t) {
                String msg = errorMessage(t);
                if (throwsSpec == null) return "` + verdictFailPrefix + `exception during execution: " + msg;
                String want = str(throwsSpec.get("want"));
                if (want.isEmpty() || anyMessageContains(t, want)) return "` + verdictPass + `";
                return "` + verdictFailPrefix + `expected an error containing '" + want + "', actual: " + msg;
            }
        }
        if (throwsSpec != null) return "` + verdictFailPrefix + `expected an exception but none was thrown";
        for (Object o : asList(prog.get("expects"))) {
            java.util.Map<String, Object> ex = asMap(o, "an expect");
            Object ok = eval(ctx, vars, str(ex.get("condition")));
            if (Boolean.TRUE.equals(ok)) continue;
            String msg = "` + verdictFailPrefix + `expected " + str(ex.get("raw"));
            String actual = str(ex.get("actual"));
            if (!actual.isEmpty()) {
                Object a = eval(ctx, vars, actual);
                msg += ", actual: " + (a == null ? "" : String.valueOf(a));
            }
            return msg;
        }
        return "` + verdictPass + `";
    }

    private void execStep(com.mendix.systemwideinterfaces.core.IContext ctx,
                          java.util.Map<String, Object> st,
                          java.util.Map<String, Object> vars) throws Exception {
        String op = str(st.get("op"));
        String var = str(st.get("var"));
        if ("declare".equals(op)) {
            if (Boolean.TRUE.equals(st.get("list"))) {
                vars.put(var, new java.util.ArrayList<com.mendix.systemwideinterfaces.core.IMendixObject>());
            } else {
                String expr = str(st.get("expr"));
                vars.put(var, expr.isEmpty() ? null : eval(ctx, vars, expr));
            }
        } else if ("set".equals(op)) {
            if (!vars.containsKey(var)) throw new IllegalArgumentException("set of $" + var + ", which is not declared");
            vars.put(var, eval(ctx, vars, str(st.get("expr"))));
        } else if ("create".equals(op)) {
            com.mendix.systemwideinterfaces.core.IMendixObject obj = com.mendix.core.Core.instantiate(ctx, str(st.get("entity")));
            setMembers(ctx, obj, st, vars);
            commitIf(ctx, obj, st);
            vars.put(var, obj);
        } else if ("change".equals(op)) {
            com.mendix.systemwideinterfaces.core.IMendixObject obj = object(vars, var);
            setMembers(ctx, obj, st, vars);
            commitIf(ctx, obj, st);
        } else if ("commit".equals(op)) {
            commitIf(ctx, vars.get(var), st);
        } else if ("delete".equals(op)) {
            Object v = vars.get(var);
            if (v instanceof java.util.List) {
                for (Object e : (java.util.List<?>) v) com.mendix.core.Core.delete(ctx, (com.mendix.systemwideinterfaces.core.IMendixObject) e);
            } else {
                com.mendix.core.Core.delete(ctx, object(vars, var));
            }
        } else if ("retrieve".equals(op)) {
            String xpath = bindXPath(ctx, vars, str(st.get("xpath")));
            java.util.List<com.mendix.systemwideinterfaces.core.IMendixObject> rows =
                com.mendix.core.Core.createXPathQuery(xpath).execute(ctx);
            if (Boolean.TRUE.equals(st.get("first"))) {
                vars.put(var, rows.isEmpty() ? null : rows.get(0));
            } else {
                vars.put(var, new java.util.ArrayList<com.mendix.systemwideinterfaces.core.IMendixObject>(rows));
            }
        } else if ("count".equals(op)) {
            Object list = vars.get(str(st.get("of")));
            vars.put(var, Long.valueOf(list instanceof java.util.List ? ((java.util.List<?>) list).size() : 0));
        } else {
            throw new IllegalArgumentException("unknown step " + op);
        }
    }

    private void execCall(com.mendix.systemwideinterfaces.core.IContext ctx,
                          java.util.Map<String, Object> st,
                          java.util.Map<String, Object> vars) throws Exception {
        String flow = str(st.get("flow"));
        if (!com.mendix.core.Core.getMicroflowNames().contains(flow)) {
            throw new IllegalArgumentException("unknown microflow " + flow);
        }
        java.util.Map<String, com.mendix.systemwideinterfaces.core.IDataType> sig = com.mendix.core.Core.getInputParameters(flow);
        java.util.Map<String, Object> args = new java.util.HashMap<String, Object>();
        for (Object o : asList(st.get("args"))) {
            java.util.Map<String, Object> a = asMap(o, "an argument");
            String name = str(a.get("name"));
            String expr = str(a.get("expr"));
            if (sig == null || !sig.containsKey(name)) {
                throw new IllegalArgumentException("microflow " + flow + " has no parameter " + name
                    + (sig == null ? "" : " (it has " + sig.keySet() + ")"));
            }
            String type = sig.get(name).getType().name();
            Object v = eval(ctx, vars, expr);
            if (v instanceof Long && "Integer".equalsIgnoreCase(type)) v = Integer.valueOf(((Long) v).intValue());
            if (v != null && !(v instanceof String) && type.toLowerCase().startsWith("enum")) v = eval(ctx, vars, "getKey(" + expr + ")");
            args.put(name, v);
        }
        Object result = com.mendix.core.Core.microflowCall(flow).withParams(args).execute(ctx);
        String var = str(st.get("var"));
        if (!var.isEmpty()) vars.put(var, result);
    }

    private void setMembers(com.mendix.systemwideinterfaces.core.IContext ctx,
                            com.mendix.systemwideinterfaces.core.IMendixObject obj,
                            java.util.Map<String, Object> st,
                            java.util.Map<String, Object> vars) throws Exception {
        for (Object o : asList(st.get("members"))) {
            java.util.Map<String, Object> m = asMap(o, "a member");
            String name = str(m.get("name"));
            String expr = str(m.get("expr"));
            Object v = eval(ctx, vars, expr);
            if (name.contains(".")) {
                // An association: the object, a list of them, or empty.
                if (v instanceof com.mendix.systemwideinterfaces.core.IMendixObject) {
                    v = ((com.mendix.systemwideinterfaces.core.IMendixObject) v).getId();
                } else if (v instanceof java.util.List) {
                    java.util.List<com.mendix.systemwideinterfaces.core.IMendixIdentifier> ids = new java.util.ArrayList<com.mendix.systemwideinterfaces.core.IMendixIdentifier>();
                    for (Object e : (java.util.List<?>) v) ids.add(((com.mendix.systemwideinterfaces.core.IMendixObject) e).getId());
                    v = ids;
                }
                obj.setValue(ctx, name, v);
                continue;
            }
            com.mendix.systemwideinterfaces.core.meta.IMetaPrimitive meta = obj.getMetaObject().getMetaPrimitive(name);
            if (meta == null) throw new IllegalArgumentException(obj.getType() + " has no attribute " + name);
            String type = meta.getType().name();
            if (v instanceof Long && "Integer".equalsIgnoreCase(type)) v = Integer.valueOf(((Long) v).intValue());
            if (v != null && !(v instanceof String) && type.toLowerCase().startsWith("enum")) v = eval(ctx, vars, "getKey(" + expr + ")");
            obj.setValue(ctx, name, v);
        }
    }

    private void commitIf(com.mendix.systemwideinterfaces.core.IContext ctx, Object target,
                          java.util.Map<String, Object> st) throws Exception {
        if (!Boolean.TRUE.equals(st.get("commit")) || target == null) return;
        boolean events = Boolean.TRUE.equals(st.get("events"));
        java.util.List<com.mendix.systemwideinterfaces.core.IMendixObject> objs = new java.util.ArrayList<com.mendix.systemwideinterfaces.core.IMendixObject>();
        if (target instanceof java.util.List) {
            for (Object e : (java.util.List<?>) target) objs.add((com.mendix.systemwideinterfaces.core.IMendixObject) e);
        } else {
            objs.add((com.mendix.systemwideinterfaces.core.IMendixObject) target);
        }
        if (events) com.mendix.core.Core.commit(ctx, objs);
        else com.mendix.core.Core.commitWithoutEvents(ctx, objs);
    }

    private com.mendix.systemwideinterfaces.core.IMendixObject object(java.util.Map<String, Object> vars, String var) {
        Object v = vars.get(var);
        if (!(v instanceof com.mendix.systemwideinterfaces.core.IMendixObject)) {
            throw new IllegalArgumentException("$" + var + " is " + (v == null ? "empty" : "not an object"));
        }
        return (com.mendix.systemwideinterfaces.core.IMendixObject) v;
    }

    // eval checks what Core.evaluateExpression does not, then evaluates.
    private Object eval(com.mendix.systemwideinterfaces.core.IContext ctx,
                        java.util.Map<String, Object> vars, String expr) throws Exception {
        for (String name : variablesIn(expr)) {
            if (!vars.containsKey(name) && !"currentUser".equals(name) && !"currentSession".equals(name)) {
                throw new IllegalArgumentException("unknown variable $" + name + " in " + expr);
            }
        }
        for (String lit : enumLiteralsIn(expr)) {
            if (com.mendix.core.Core.evaluateExpression(ctx, new java.util.HashMap<String, Object>(), lit) == null) {
                throw new IllegalArgumentException("unknown enumeration value " + lit + " in " + expr);
            }
        }
        return com.mendix.core.Core.evaluateExpression(ctx, vars, expr);
    }

    // bindXPath replaces each $variable (or $variable/path) outside a string
    // literal with the literal of its value, which is what a microflow's
    // retrieve does with them.
    private String bindXPath(com.mendix.systemwideinterfaces.core.IContext ctx,
                             java.util.Map<String, Object> vars, String xpath) throws Exception {
        StringBuilder b = new StringBuilder();
        for (int i = 0; i < xpath.length(); i++) {
            char c = xpath.charAt(i);
            if (c == '\'') {
                int j = endOfLiteral(xpath, i);
                b.append(xpath, i, j + 1);
                i = j;
                continue;
            }
            if (c != '$') {
                b.append(c);
                continue;
            }
            int j = i + 1;
            while (j < xpath.length() && (Character.isLetterOrDigit(xpath.charAt(j)) || "_/.".indexOf(xpath.charAt(j)) >= 0)) j++;
            Object v = eval(ctx, vars, xpath.substring(i, j));
            b.append(xpathLiteral(v));
            i = j - 1;
        }
        return b.toString();
    }

    private String xpathLiteral(Object v) {
        if (v == null) return "empty";
        if (v instanceof com.mendix.systemwideinterfaces.core.IMendixObject) return String.valueOf(((com.mendix.systemwideinterfaces.core.IMendixObject) v).getId().toLong());
        if (v instanceof com.mendix.systemwideinterfaces.core.IMendixIdentifier) return String.valueOf(((com.mendix.systemwideinterfaces.core.IMendixIdentifier) v).toLong());
        if (v instanceof Boolean) return ((Boolean) v) ? "true()" : "false()";
        if (v instanceof Number) return v.toString();
        if (v instanceof java.util.Date) return String.valueOf(((java.util.Date) v).getTime());
        return "'" + String.valueOf(v).replace("'", "''") + "'";
    }

    private static int endOfLiteral(String s, int open) {
        for (int i = open + 1; i < s.length(); i++) {
            if (s.charAt(i) != '\'') continue;
            if (i + 1 < s.length() && s.charAt(i + 1) == '\'') { i++; continue; }
            return i;
        }
        return s.length() - 1;
    }

    // variablesIn is expressionVariables in interp.go, in Java.
    private static java.util.List<String> variablesIn(String expr) {
        java.util.List<String> out = new java.util.ArrayList<String>();
        for (int i = 0; i < expr.length(); i++) {
            char c = expr.charAt(i);
            if (c == '\'') { i = endOfLiteral(expr, i); continue; }
            if (c != '$') continue;
            int j = i + 1;
            while (j < expr.length() && (Character.isLetterOrDigit(expr.charAt(j)) || expr.charAt(j) == '_')) j++;
            if (j > i + 1) out.add(expr.substring(i + 1, j));
            i = j - 1;
        }
        return out;
    }

    // enumLiteralsIn finds Module.Enumeration.Value names: three dotted parts,
    // not inside a string, and not a member of a path ($x/Mod.Assoc/Mod.Entity)
    // or a constant (@Mod.Const).
    private static java.util.List<String> enumLiteralsIn(String expr) {
        java.util.List<String> out = new java.util.ArrayList<String>();
        for (int i = 0; i < expr.length(); i++) {
            char c = expr.charAt(i);
            if (c == '\'') { i = endOfLiteral(expr, i); continue; }
            if (!Character.isLetter(c)) continue;
            int j = i;
            while (j < expr.length() && (Character.isLetterOrDigit(expr.charAt(j)) || "_.".indexOf(expr.charAt(j)) >= 0)) j++;
            char before = i == 0 ? ' ' : expr.charAt(i - 1);
            String tok = expr.substring(i, j);
            if (before != '/' && before != '@' && before != '$' && before != '.'
                && tok.split("\\.", -1).length == 3) {
                out.add(tok);
            }
            i = j - 1;
        }
        return out;
    }

    private static String str(Object o) { return o == null ? "" : String.valueOf(o); }

    @SuppressWarnings("unchecked")
    private static java.util.Map<String, Object> asMap(Object o, String what) {
        if (!(o instanceof java.util.Map)) throw new IllegalArgumentException(what + " is not an object");
        return (java.util.Map<String, Object>) o;
    }

    private static java.util.List<?> asList(Object o) {
        return o instanceof java.util.List ? (java.util.List<?>) o : java.util.Collections.emptyList();
    }

    private static String errorMessage(Throwable t) {
        Throwable root = t;
        while (root.getCause() != null && root.getCause() != root) root = root.getCause();
        String msg = root.getMessage();
        return (msg == null || msg.isEmpty()) ? root.getClass().getName() : msg;
    }

    // anyMessageContains matches @throws against every message in the cause
    // chain: which level carries the text a microflow's $latestError/Message
    // shows depends on where the error was raised.
    private static boolean anyMessageContains(Throwable t, String want) {
        for (Throwable c = t; c != null; c = c.getCause() == c ? null : c.getCause()) {
            if (c.getMessage() != null && c.getMessage().contains(want)) return true;
        }
        return false;
    }

    // A JSON reader for the step format: objects, arrays, strings, integers,
    // booleans and null. Integers come back as Long; nothing else is needed.
    private static final class JsonReader {
        private final String s;
        private int i;
        JsonReader(String s) { this.s = s; }

        Object read() {
            Object v = value();
            ws();
            if (i != s.length()) throw new IllegalArgumentException("trailing text at " + i);
            return v;
        }

        private void ws() { while (i < s.length() && Character.isWhitespace(s.charAt(i))) i++; }

        private Object value() {
            ws();
            if (i >= s.length()) throw new IllegalArgumentException("unexpected end");
            char c = s.charAt(i);
            if (c == '{') {
                java.util.Map<String, Object> m = new java.util.LinkedHashMap<String, Object>();
                i++; ws();
                if (s.charAt(i) == '}') { i++; return m; }
                while (true) {
                    ws();
                    String k = string();
                    ws();
                    if (s.charAt(i++) != ':') throw new IllegalArgumentException("expected : at " + i);
                    m.put(k, value());
                    ws();
                    char d = s.charAt(i++);
                    if (d == '}') return m;
                    if (d != ',') throw new IllegalArgumentException("expected , or } at " + i);
                }
            }
            if (c == '[') {
                java.util.List<Object> l = new java.util.ArrayList<Object>();
                i++; ws();
                if (s.charAt(i) == ']') { i++; return l; }
                while (true) {
                    l.add(value());
                    ws();
                    char d = s.charAt(i++);
                    if (d == ']') return l;
                    if (d != ',') throw new IllegalArgumentException("expected , or ] at " + i);
                }
            }
            if (c == '"') return string();
            if (s.startsWith("true", i)) { i += 4; return Boolean.TRUE; }
            if (s.startsWith("false", i)) { i += 5; return Boolean.FALSE; }
            if (s.startsWith("null", i)) { i += 4; return null; }
            int start = i;
            if (c == '-') i++;
            while (i < s.length() && Character.isDigit(s.charAt(i))) i++;
            if (start == i) throw new IllegalArgumentException("unexpected character at " + i);
            return Long.valueOf(s.substring(start, i));
        }

        private String string() {
            if (s.charAt(i) != '"') throw new IllegalArgumentException("expected a string at " + i);
            StringBuilder b = new StringBuilder();
            for (i++; i < s.length(); i++) {
                char c = s.charAt(i);
                if (c == '"') { i++; return b.toString(); }
                if (c != '\\') { b.append(c); continue; }
                char e = s.charAt(++i);
                switch (e) {
                    case 'n': b.append('\n'); break;
                    case 'r': b.append('\r'); break;
                    case 't': b.append('\t'); break;
                    case 'b': b.append('\b'); break;
                    case 'f': b.append('\f'); break;
                    case 'u': b.append((char) Integer.parseInt(s.substring(i + 1, i + 5), 16)); i += 4; break;
                    default: b.append(e);
                }
            }
            throw new IllegalArgumentException("unterminated string");
        }
    }
`

// execProtocolVersionJava is execProtocolVersion as Java source text.
const execProtocolVersionJava = "1"
