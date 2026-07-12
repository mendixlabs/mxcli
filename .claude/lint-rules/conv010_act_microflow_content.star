# CONV010: ACT_ Microflow Content Restriction
#
# Microflows prefixed with ACT_ are page action microflows. They should only
# contain UI-related activities:
#   - ShowFormAction (show page)
#   - CloseFormAction (close page)
#   - ShowHomeFormAction (show home page)
#   - ShowMessageAction (show message)
#   - DownloadFileAction (download file)
#   - SubMicroflow (call sub-microflow for logic delegation)
#
# Business logic should be delegated to SUB_ microflows.
# Requires FULL catalog (REFRESH CATALOG FULL).

RULE_ID = "CONV010"
RULE_NAME = "ACTMicroflowContent"
DESCRIPTION = "ACT_ microflows should only contain UI actions and sub-microflow calls"
CATEGORY = "architecture"
SEVERITY = "info"

# Allowed action types in ACT_ microflows
ALLOWED_ACTIONS = (
    "ShowFormAction",
    "CloseFormAction",
    "ShowHomeFormAction",
    "ShowMessageAction",
    "DownloadFileAction",
)

# Allowed activity types (non-action activities)
ALLOWED_ACTIVITY_TYPES = (
    "SubMicroflow",
    "StartEvent",
    "EndEvent",
    "ExclusiveSplit",
    "Annotation",
)

def check():
    violations = []

    for mf in microflows():
        if not mf.name.startswith("ACT_"):
            continue

        for act in activities_for(mf.qualified_name):
            # Skip allowed activity types
            if act.activity_type in ALLOWED_ACTIVITY_TYPES:
                continue

            # For ActionActivity, check the action type
            if act.activity_type == "ActionActivity":
                if act.action_type in ALLOWED_ACTIONS:
                    continue

                violations.append(violation(
                    message="ACT_ microflow '{}' contains '{}' action. Delegate business logic to a SUB_ microflow.".format(
                        mf.name, act.action_type
                    ),
                    location=location(
                        module=mf.module_name,
                        document_type="Microflow",
                        document_name=mf.qualified_name,
                    ),
                    suggestion="Move the '{}' action to a SUB_ microflow and call it from '{}'".format(
                        act.action_type, mf.name
                    ),
                ))
            elif act.activity_type not in ALLOWED_ACTIVITY_TYPES:
                # Any other non-allowed activity type
                violations.append(violation(
                    message="ACT_ microflow '{}' contains '{}' activity. Delegate to a SUB_ microflow.".format(
                        mf.name, act.activity_type
                    ),
                    location=location(
                        module=mf.module_name,
                        document_type="Microflow",
                        document_name=mf.qualified_name,
                    ),
                    suggestion="Move the '{}' to a SUB_ microflow called from '{}'".format(
                        act.activity_type, mf.name
                    ),
                ))

    return violations
