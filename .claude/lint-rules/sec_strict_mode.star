# SEC005: Strict Mode Disabled
#
# Strict mode restricts the client API: data.action, data.create, data.commit,
# data.remove and data.rollback are disabled in the runtime, and data.get works
# only by GUID. Data access then follows the pages, widgets, microflows and
# nanoflows in the model, so a user cannot fetch or change objects from the
# browser console or a JavaScript action beyond what the app offers — a
# backstop for access rules that are wider than intended.
#
# It can be enabled at the Production security level with the React client
# only, so the rule is silent elsewhere.
#
# https://docs.mendix.com/refguide/strict-mode/

RULE_ID = "SEC005"
RULE_NAME = "StrictModeDisabled"
DESCRIPTION = "Strict mode is off: the client API may read and change any object the user's role can access"
CATEGORY = "security"
SEVERITY = "warning"

def check():
    sec = project_security()
    if sec == None:
        return []

    if sec.strict_mode:
        return []

    # Strict mode exists at Production level only.
    if sec.security_level != "CheckEverything":
        return []

    # ...and with the React client only: under the classic client it cannot
    # be turned on, so there is nothing to fix.
    settings = project_settings()
    if settings != None and settings.web_client == "No":
        return []

    return [violation(
        message="Strict mode is disabled, so the client API (data.get, data.commit, data.remove and others) " +
                "may read and change any object the user's role can access, not only what the app's pages and flows offer.",
        location=location(module="", document_type="security", document_name="ProjectSecurity"),
        suggestion="Enable it with: alter app security ( StrictMode: true ); (or in Studio Pro under App Security). " +
                   "Check first that no JavaScript action calls those APIs (use a nanoflow instead); " +
                   "File Uploader's delete and Web Actions' Take picture do not work in strict mode.",
    )]
