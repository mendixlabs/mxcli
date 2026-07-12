# Starlark Lint Rule: Missing Documentation
#
# This rule checks that entities and microflows have documentation.
# Good documentation helps with maintainability and onboarding new developers.
#
# Checks:
#   - Entities should have a description explaining their purpose
#   - Microflows should have a description explaining what they do
#
# Entity properties:
#   .description     - Documentation text
#
# Microflow properties:
#   .description     - Documentation text

RULE_ID = "QUAL002"
RULE_NAME = "Missing Documentation"
DESCRIPTION = "Entities and microflows should have documentation describing their purpose"
CATEGORY = "quality"
SEVERITY = "info"

def check():
    """
    Check that entities and microflows have documentation.
    """
    violations = []

    # Check entities
    for entity in entities():
        if entity.entity_type != "Persistent" or entity.is_external:
            continue
            
        if not entity.description or entity.description.strip() == "":
            loc = location(
                module=entity.module_name,
                document_type="Entity",
                document_name=entity.qualified_name
            )
            v = violation(
                message="Entity '{}' has no documentation.".format(entity.name),
                location=loc,
                suggestion="Add a description explaining the entity's purpose and what data it represents."
            )
            violations.append(v)

    # Check microflows (skip nanoflows as they're often simple)
    for mf in microflows():
        # Only check microflows, not nanoflows
        if mf.microflow_type != "MICROFLOW":
            continue

        # Skip simple microflows (1-4 activities)
        if mf.activity_count <= 4:
            continue

        if not mf.description or mf.description.strip() == "":
            loc = location(
                module=mf.module_name,
                document_type="Microflow",
                document_name=mf.qualified_name
            )
            v = violation(
                message="Microflow '{}' has no documentation.".format(mf.name),
                location=loc,
                suggestion="Add a description explaining what this microflow does and when it should be called."
            )
            violations.append(v)

    return violations
