# SEC005: Strict Mode Disabled
#
# Strict mode enforces additional XPath constraint validation at runtime.
# When disabled, certain XPath constraint bypass attacks are possible
# (related to CVE-2023-23835 - XPath Constraint Bypass in Mendix Runtime).
#
# https://cert-portal.siemens.com/productcert/html/ssa-097435.html

RULE_ID = "SEC005"
RULE_NAME = "StrictModeDisabled"
DESCRIPTION = "Strict security mode is disabled - enables additional XPath constraint enforcement"
CATEGORY = "security"
SEVERITY = "info"

def check():
    sec = project_security()
    if sec == None:
        return []

    if sec.strict_mode:
        return []

    # Only warn at Production level - not relevant for dev/prototype
    if sec.security_level != "CheckEverything":
        return []

    return [violation(
        message="Strict mode is disabled. This weakens XPath constraint enforcement and is relevant to CVE-2023-23835.",
        location=location(module="", document_type="security", document_name="ProjectSecurity"),
        suggestion="ALTER PROJECT SECURITY STRICT MODE ON",
    )]
