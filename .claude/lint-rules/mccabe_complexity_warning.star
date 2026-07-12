# Starlark Lint Rule: McCabe Cyclomatic Complexity
#
# This rule checks that microflows don't exceed a McCabe complexity threshold.
# McCabe complexity measures the number of independent paths through code:
#   - Base complexity is 1
#   - Each decision point (IF, type check) adds 1
#   - Each loop adds 1
#   - Each error handler adds 1
#
# Thresholds:
#   1-10  : Simple, low risk
#   11-20 : Moderate complexity, moderate risk
#   21-50 : Complex, high risk
#   50+   : Untestable, very high risk
#
# Microflow properties:
#   .id              - Document ID
#   .name            - Simple name (e.g., "ProcessOrder")
#   .qualified_name  - Full name (e.g., "MyModule.ProcessOrder")
#   .module_name     - Module name
#   .microflow_type  - "MICROFLOW" or "NANOFLOW"
#   .description     - Documentation
#   .return_type     - Return type
#   .parameter_count - Number of parameters
#   .activity_count  - Number of activities
#   .complexity      - McCabe cyclomatic complexity

RULE_ID = "QUAL001"
RULE_NAME = "McCabe Complexity"
DESCRIPTION = "Microflows should not exceed McCabe cyclomatic complexity of 20"
CATEGORY = "complexity"
SEVERITY = "warning"

# Maximum allowed complexity - customize as needed
MAX_COMPLEXITY = 20

def check():
    """
    Check that microflows don't exceed the McCabe complexity threshold.
    High complexity indicates code that is hard to test and maintain.
    """
    violations = []

    for mf in microflows():
        if mf.complexity > MAX_COMPLEXITY:
            loc = location(
                module=mf.module_name,
                document_type="Microflow",
                document_name=mf.qualified_name
            )
            v = violation(
                message="Microflow '{}' has complexity {} (max: {}). Consider splitting into smaller microflows.".format(
                    mf.name,
                    mf.complexity,
                    MAX_COMPLEXITY
                ),
                location=loc,
                suggestion="Break down complex logic into sub-microflows. Extract decision branches into separate SUB_ microflows."
            )
            violations.append(v)

    return violations
