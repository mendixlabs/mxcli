# CONV005: Snippet Prefix
#
# Snippet names should start with SNIPPET_ or SNP_ to distinguish them from pages
# at a glance. Info severity - advisory only.

RULE_ID = "CONV005"
RULE_NAME = "SnippetPrefix"
DESCRIPTION = "Snippet names should start with SNIPPET_ or SNP_ prefix"
CATEGORY = "naming"
SEVERITY = "info"

def check():
    violations = []

    for snippet in snippets():
        if not (snippet.name.startswith("SNIPPET_") or snippet.name.startswith("SNP_")):
            violations.append(violation(
                message="Snippet '{}' does not start with SNIPPET_ or SNP_ prefix".format(snippet.name),
                location=location(
                    module=snippet.module_name,
                    document_type="Snippet",
                    document_name=snippet.qualified_name,
                ),
                suggestion="Rename to 'SNIPPET_{}' or 'SNP_{}'".format(snippet.name, snippet.name),
            ))

    return violations
