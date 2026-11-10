// SPDX-License-Identifier: Apache-2.0

package testrunner

// endpointCallMethods are the helper methods of the `call` route, spliced into
// the request handler class. `call` is the runtime executor: it takes one JSON
// case {flow, params, expect, rollback}, builds the parameters in a transaction
// it owns, runs the microflow through Core.microflowCall, evaluates the
// expectations against the live objects (before the rollback), and returns the
// actual values. No model change, no rebuild: a case is data.
//
// Backticks are not used here; the text is spliced into a Go raw string.
const endpointCallMethods = `
    // ---- call route: tiny JSON reader (no library on the java action classpath is assumed) ----
    private String js;
    private int jp;

    private Object jparse(String s) { js = s; jp = 0; return jval(); }

    private void jws() { while (jp < js.length() && Character.isWhitespace(js.charAt(jp))) jp++; }

    private String jstr() {
        jp++;
        StringBuilder b = new StringBuilder();
        while (js.charAt(jp) != '"') {
            char c = js.charAt(jp++);
            if (c == '\\') {
                char e = js.charAt(jp++);
                switch (e) {
                    case 'n': b.append('\n'); break;
                    case 't': b.append('\t'); break;
                    case 'r': b.append('\r'); break;
                    case 'b': b.append('\b'); break;
                    case 'f': b.append('\f'); break;
                    case 'u': b.append((char) Integer.parseInt(js.substring(jp, jp + 4), 16)); jp += 4; break;
                    default: b.append(e);
                }
            } else b.append(c);
        }
        jp++;
        return b.toString();
    }

    private Object jval() {
        jws();
        char c = js.charAt(jp);
        if (c == '{') {
            jp++;
            java.util.LinkedHashMap<String, Object> m = new java.util.LinkedHashMap<String, Object>();
            jws();
            if (js.charAt(jp) == '}') { jp++; return m; }
            while (true) {
                jws();
                String k = jstr();
                jws(); jp++;
                m.put(k, jval());
                jws();
                if (js.charAt(jp++) == '}') return m;
            }
        }
        if (c == '[') {
            jp++;
            java.util.ArrayList<Object> l = new java.util.ArrayList<Object>();
            jws();
            if (js.charAt(jp) == ']') { jp++; return l; }
            while (true) {
                l.add(jval());
                jws();
                if (js.charAt(jp++) == ']') return l;
            }
        }
        if (c == '"') return jstr();
        if (js.startsWith("true", jp)) { jp += 4; return Boolean.TRUE; }
        if (js.startsWith("false", jp)) { jp += 5; return Boolean.FALSE; }
        if (js.startsWith("null", jp)) { jp += 4; return null; }
        int st = jp;
        while (jp < js.length() && "+-0123456789.eE".indexOf(js.charAt(jp)) >= 0) jp++;
        return new java.math.BigDecimal(js.substring(st, jp));
    }

    // ---- call route: values ----
    private String plain(Object v) {
        if (v instanceof java.math.BigDecimal) return ((java.math.BigDecimal) v).toPlainString();
        return String.valueOf(v);
    }

    private String lastSeg(String s) { int i = s.lastIndexOf('.'); return i >= 0 ? s.substring(i + 1) : s; }

    private java.util.Date toDate(Object v) {
        String s = plain(v);
        if ("$now".equals(s)) return new java.util.Date();
        if (s.matches("-?[0-9]+")) return new java.util.Date(Long.parseLong(s));
        return java.util.Date.from(java.time.Instant.parse(s));
    }

    private Object prim(String type, Object v) {
        String s = plain(v);
        if (type.equals("Integer")) return Integer.valueOf(new java.math.BigDecimal(s).intValue());
        if (type.equals("Long") || type.equals("AutoNumber")) return Long.valueOf(new java.math.BigDecimal(s).longValue());
        if (type.equals("Decimal")) return new java.math.BigDecimal(s);
        if (type.equals("Boolean")) return Boolean.valueOf(s);
        if (type.equals("DateTime") || type.equals("Datetime")) return toDate(v);
        if (type.equals("Enum") || type.equals("Enumeration")) return lastSeg(s);
        return s;
    }

    private com.mendix.systemwideinterfaces.core.IMendixObject build(
            com.mendix.systemwideinterfaces.core.IContext ctx, java.util.Map<String, Object> m,
            java.util.Map<String, Object> named) throws Exception {
        if (m.containsKey("$ref")) {
            Object o = named.get(String.valueOf(m.get("$ref")));
            if (!(o instanceof com.mendix.systemwideinterfaces.core.IMendixObject)) throw new RuntimeException("unknown $ref " + m.get("$ref"));
            return (com.mendix.systemwideinterfaces.core.IMendixObject) o;
        }
        if (m.containsKey("$find")) {
            java.util.List<com.mendix.systemwideinterfaces.core.IMendixObject> r =
                com.mendix.core.Core.createXPathQuery("//" + m.get("$find")).execute(ctx);
            if (r.isEmpty()) throw new RuntimeException("$find matched nothing: " + m.get("$find"));
            return r.get(0);
        }
        String ent = String.valueOf(m.get("$new"));
        com.mendix.systemwideinterfaces.core.meta.IMetaObject mo = com.mendix.core.Core.getMetaObject(ent);
        if (mo == null) throw new RuntimeException("unknown entity " + ent);
        com.mendix.systemwideinterfaces.core.IMendixObject o = com.mendix.core.Core.instantiate(ctx, ent);
        for (java.util.Map.Entry<String, Object> e : m.entrySet()) {
            String k = e.getKey();
            if (k.startsWith("$")) continue;
            Object v = e.getValue();
            if (v instanceof java.util.Map) {
                String an = k.indexOf('.') >= 0 ? k : mo.getModuleName() + "." + k;
                o.setValue(ctx, an, build(ctx, (java.util.Map<String, Object>) v, named).getId());
            } else if (v == null) {
                o.setValue(ctx, k, null);
            } else {
                com.mendix.systemwideinterfaces.core.meta.IMetaPrimitive p = mo.getMetaPrimitive(k);
                if (p == null) throw new RuntimeException("no attribute " + k + " on " + ent);
                o.setValue(ctx, k, prim(p.getType().name(), v));
            }
        }
        boolean commit = !Boolean.FALSE.equals(m.get("$commit")) && Boolean.TRUE.equals(mo.isPersistable());
        if (commit) com.mendix.core.Core.commit(ctx, o);
        if (m.containsKey("$name")) named.put(String.valueOf(m.get("$name")), o);
        return o;
    }

    private Object paramValue(com.mendix.systemwideinterfaces.core.IContext ctx,
            com.mendix.systemwideinterfaces.core.IDataType dt, Object v,
            java.util.Map<String, Object> named) throws Exception {
        if (v == null) return null;
        if (dt != null && dt.isList()) {
            java.util.List<com.mendix.systemwideinterfaces.core.IMendixObject> l = new java.util.ArrayList<com.mendix.systemwideinterfaces.core.IMendixObject>();
            for (Object x : (java.util.List<Object>) v) l.add(build(ctx, (java.util.Map<String, Object>) x, named));
            return l;
        }
        if (v instanceof java.util.Map) return build(ctx, (java.util.Map<String, Object>) v, named);
        if (dt == null) return plain(v);
        return prim(dt.getType().name(), v);
    }

    // ---- call route: reading actual values ----
    private Object member(com.mendix.systemwideinterfaces.core.IContext ctx, Object cur, String seg) throws Exception {
        if (cur == null) return null;
        if (cur instanceof java.util.List) {
            if (seg.equals("length") || seg.equals("count")) return Integer.valueOf(((java.util.List<?>) cur).size());
            return ((java.util.List<?>) cur).get(Integer.parseInt(seg));
        }
        if (!(cur instanceof com.mendix.systemwideinterfaces.core.IMendixObject)) throw new RuntimeException("cannot read " + seg + " of a primitive");
        Object v = ((com.mendix.systemwideinterfaces.core.IMendixObject) cur).getValue(ctx, seg);
        if (v instanceof com.mendix.systemwideinterfaces.core.IMendixIdentifier)
            return com.mendix.core.Core.retrieveId(ctx, (com.mendix.systemwideinterfaces.core.IMendixIdentifier) v);
        if (v instanceof java.util.List && !((java.util.List<?>) v).isEmpty()
            && ((java.util.List<?>) v).get(0) instanceof com.mendix.systemwideinterfaces.core.IMendixIdentifier)
            return com.mendix.core.Core.retrieveIdList(ctx, (java.util.List<com.mendix.systemwideinterfaces.core.IMendixIdentifier>) v);
        return v;
    }

    private Object resolve(com.mendix.systemwideinterfaces.core.IContext ctx, String path, java.util.Map<String, Object> vars) throws Exception {
        if (path.startsWith("count:")) {
            String xp = path.substring(6);
            java.util.regex.Matcher mt = java.util.regex.Pattern.compile("\\{\\$(\\w+)\\}").matcher(xp);
            StringBuffer sb = new StringBuffer();
            while (mt.find()) {
                Object ov = vars.get(mt.group(1));
                if (!(ov instanceof com.mendix.systemwideinterfaces.core.IMendixObject)) throw new RuntimeException("{$" + mt.group(1) + "} is not an object");
                mt.appendReplacement(sb, String.valueOf(((com.mendix.systemwideinterfaces.core.IMendixObject) ov).getId().toLong()));
            }
            mt.appendTail(sb);
            return Integer.valueOf(com.mendix.core.Core.createXPathQuery("//" + sb).execute(ctx).size());
        }
        String[] seg = path.split("/");
        String head = seg[0].startsWith("$") ? seg[0].substring(1) : seg[0];
        if (!vars.containsKey(head)) throw new RuntimeException("unknown variable " + seg[0] + " (use $result or a parameter name)");
        Object cur = vars.get(head);
        if (cur instanceof com.mendix.systemwideinterfaces.core.IMendixObject && seg.length > 1 && !head.equals("result")) {
            try { cur = com.mendix.core.Core.retrieveId(ctx, ((com.mendix.systemwideinterfaces.core.IMendixObject) cur).getId()); } catch (Throwable t) { }
        }
        for (int i = 1; i < seg.length; i++) cur = member(ctx, cur, seg[i]);
        return cur;
    }

    private String show(Object v) {
        if (v == null) return "empty";
        if (v instanceof java.math.BigDecimal) return ((java.math.BigDecimal) v).stripTrailingZeros().toPlainString();
        if (v instanceof java.util.Date) return ((java.util.Date) v).toInstant().toString();
        if (v instanceof com.mendix.systemwideinterfaces.core.IMendixObject) {
            com.mendix.systemwideinterfaces.core.IMendixObject o = (com.mendix.systemwideinterfaces.core.IMendixObject) v;
            return o.getType() + "#" + o.getId().toLong();
        }
        if (v instanceof java.util.List) return "list(" + ((java.util.List<?>) v).size() + ")";
        return String.valueOf(v);
    }

    private boolean check(String op, Object actual, Object expected) {
        String a = show(actual);
        String e = expected == null ? "empty" : plain(expected);
        if (op.equals("isnull") || op.equals("empty")) return actual == null;
        if (op.equals("notnull") || op.equals("notempty")) return actual != null;
        if (op.equals("contains")) return a.contains(e);
        java.math.BigDecimal an = null, en = null;
        try { an = new java.math.BigDecimal(a); en = new java.math.BigDecimal(e); } catch (Exception x) { }
        if (an != null && en != null) {
            int c = an.compareTo(en);
            if (op.equals("=")) return c == 0;
            if (op.equals("!=")) return c != 0;
            if (op.equals("<")) return c < 0;
            if (op.equals("<=")) return c <= 0;
            if (op.equals(">")) return c > 0;
            if (op.equals(">=")) return c >= 0;
        }
        boolean eq = a.equals(e) || (e.matches("\\w+\\.\\w+\\.\\w+") && lastSeg(e).equals(a));
        if (op.equals("=")) return eq;
        if (op.equals("!=")) return !eq;
        throw new RuntimeException("operator " + op + " needs numbers, got " + a + " and " + e);
    }
`

// endpointCallMethod is the `call` route body, a method of the handler class
// placed after processRequest. It reads the case from the "case" query parameter.
const endpointCallMethod = `
    private synchronized void doCall(com.mendix.m2ee.api.IMxRuntimeRequest request,
                                     com.mendix.m2ee.api.IMxRuntimeResponse response,
                                     java.io.Writer out, java.util.Set<String> known) throws Exception {
        {
            {
                long t0 = System.nanoTime();
                String caseJson = request.getParameter("case");
                if (caseJson == null || caseJson.isEmpty()) {
                    response.setStatus(com.mendix.m2ee.api.IMxRuntimeResponse.BAD_REQUEST);
                    out.write("{\"error\":\"missing case parameter\"}");
                    out.flush();
                    return;
                }
                StringBuilder checks = new StringBuilder();
                boolean pass = true;
                String error = null, thrown = null, resultShown = "null";
                boolean rolledBack = false, rollback = true;
                long flowMicros = 0;
                com.mendix.systemwideinterfaces.core.IContext ctx = com.mendix.core.Core.createSystemContext();
                boolean started = false;
                try {
                    java.util.Map<String, Object> c = (java.util.Map<String, Object>) jparse(caseJson);
                    String flow = String.valueOf(c.get("flow"));
                    if (!known.contains(flow)) throw new RuntimeException("unknown microflow " + flow);
                    rollback = !Boolean.FALSE.equals(c.get("rollback"));
                    if (rollback) { ctx.startTransaction(); started = true; }
                    java.util.Map<String, Object> named = new java.util.LinkedHashMap<String, Object>();
                    java.util.Map<String, com.mendix.systemwideinterfaces.core.IDataType> sig = com.mendix.core.Core.getInputParameters(flow);
                    java.util.Map<String, Object> ps = (java.util.Map<String, Object>) c.get("params");
                    java.util.Map<String, Object> args = new java.util.LinkedHashMap<String, Object>();
                    if (ps != null) {
                        for (java.util.Map.Entry<String, Object> e : ps.entrySet()) {
                            if (!sig.containsKey(e.getKey())) throw new RuntimeException("microflow " + flow + " has no parameter " + e.getKey() + " (has " + sig.keySet() + ")");
                            Object pv = paramValue(ctx, sig.get(e.getKey()), e.getValue(), named);
                            args.put(e.getKey(), pv);
                            named.put(e.getKey(), pv);
                        }
                    }
                    java.util.List<Object> su = (java.util.List<Object>) c.get("setup");
                    if (su != null) {
                        for (Object so : su) build(ctx, (java.util.Map<String, Object>) so, named);
                    }
                    // Hand the flow fresh copies of committed object parameters, as a page or
                    // a REST call would: a Java-built instance has no view of reverse
                    // associations created after it (a flow reading $Invoice/Doc_Invoice
                    // would otherwise see empty).
                    for (java.util.Map.Entry<String, Object> ae : args.entrySet()) {
                        Object av = ae.getValue();
                        if (av instanceof com.mendix.systemwideinterfaces.core.IMendixObject) {
                            try {
                                Object fresh = com.mendix.core.Core.retrieveId(ctx, ((com.mendix.systemwideinterfaces.core.IMendixObject) av).getId());
                                ae.setValue(fresh);
                                named.put(ae.getKey(), fresh);
                            } catch (Throwable t) { }
                        }
                    }
                    long f0 = System.nanoTime();
                    Object result = null;
                    try {
                        result = com.mendix.core.Core.microflowCall(flow).withParams(args).execute(ctx);
                    } catch (Throwable t) {
                        Throwable root = t;
                        while (root.getCause() != null && root.getCause() != root) root = root.getCause();
                        thrown = (root.getMessage() == null || root.getMessage().isEmpty()) ? root.getClass().getName() : root.getMessage();
                    }
                    flowMicros = (System.nanoTime() - f0) / 1000L;
                    resultShown = result == null ? "null" : esc(show(result));
                    named.put("result", result);
                    Object thr = c.get("throws");
                    if (c.containsKey("throws")) {
                        boolean okT = thrown != null && thrown.contains(thr == null ? "" : plain(thr));
                        if (!okT) pass = false;
                        if (checks.length() > 0) checks.append(',');
                        checks.append("{\"path\":\"throws\",\"op\":\"contains\",\"expected\":").append(esc(thr == null ? "" : plain(thr)))
                              .append(",\"actual\":").append(esc(thrown == null ? "no error" : thrown)).append(",\"pass\":").append(okT).append('}');
                    } else if (thrown != null) {
                        pass = false;
                        error = thrown;
                    }
                    java.util.List<Object> ex = (java.util.List<Object>) c.get("expect");
                    if (ex != null && thrown == null || (ex != null && c.containsKey("throws"))) {
                        for (Object eo : ex) {
                            java.util.Map<String, Object> e = (java.util.Map<String, Object>) eo;
                            String p = String.valueOf(e.get("path"));
                            String op = e.get("op") == null ? "=" : String.valueOf(e.get("op"));
                            Object act = null;
                            String shown;
                            boolean ok;
                            try {
                                act = resolve(ctx, p, named);
                                shown = show(act);
                                ok = check(op, act, e.get("value"));
                            } catch (Throwable t) {
                                shown = "ERROR " + (t.getMessage() == null ? t.getClass().getName() : t.getMessage());
                                ok = false;
                            }
                            if (!ok) pass = false;
                            if (checks.length() > 0) checks.append(',');
                            checks.append("{\"path\":").append(esc(p)).append(",\"op\":").append(esc(op))
                                  .append(",\"expected\":").append(esc(e.get("value") == null ? "empty" : plain(e.get("value"))))
                                  .append(",\"actual\":").append(esc(shown)).append(",\"pass\":").append(ok).append('}');
                        }
                    }
                } catch (Throwable t) {
                    pass = false;
                    Throwable root = t;
                    while (root.getCause() != null && root.getCause() != root) root = root.getCause();
                    error = (root.getMessage() == null || root.getMessage().isEmpty()) ? root.getClass().getName() : root.getMessage();
                } finally {
                    if (started) {
                        try { ctx.rollbackTransaction(); rolledBack = true; } catch (Throwable t) { }
                    }
                }
                long micros = (System.nanoTime() - t0) / 1000L;
                StringBuilder b = new StringBuilder("{");
                b.append("\"pass\":").append(pass && error == null);
                b.append(",\"durationMicros\":").append(micros);
                b.append(",\"flowMicros\":").append(flowMicros);
                b.append(",\"result\":").append(resultShown);
                if (error != null) b.append(",\"error\":").append(esc(error));
                b.append(",\"rollbackRequested\":").append(rollback);
                b.append(",\"rolledBack\":").append(rolledBack);
                b.append(",\"checks\":[").append(checks).append("]}");
                out.write(b.toString());
                out.flush();
                return;
            }
        }
    }
`
