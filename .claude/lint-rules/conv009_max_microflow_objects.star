# CONV009: Maximum Microflow Objects (Strict)
#
# Microflows should not contain more than 15 activities (Mendix best practice
# convention). This is stricter than QUAL003's 25-activity threshold.
# Complex microflows should be split into sub-microflows (SUB_).

RULE_ID = "CONV009"
RULE_NAME = "MaxMicroflowObjects"
DESCRIPTION = "Microflows should not exceed 15 activities (Mendix best practice)"
CATEGORY = "quality"
SEVERITY = "info"

MAX_ACTIVITIES = 25
MIN_ACTIVITIES = 15

def check():
    violations = []

    for mf in microflows():
        if (mf.activity_count > MIN_ACTIVITIES and mf.activity_count <= MAX_ACTIVITIES:
            violations.append(violation(
                message="Microflow '{}' has {} activities (convention max: {}). Split into sub-microflows.".format(
                    mf.name, mf.activity_count, MIN_ACTIVITIES
                ),
                location=location(
                    module=mf.module_name,
                    document_type="Microflow",
                    document_name=mf.qualified_name,
                ),
                suggestion="Extract logical sections into SUB_ microflows to keep each under {} activities".format(MIN_ACTIVITIES),
            ))

    return violations
