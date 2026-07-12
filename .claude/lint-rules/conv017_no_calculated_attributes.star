# CONV017: No Calculated Attributes
#
# Calculated (virtual) attributes are computed by a microflow on every access,
# which can cause performance issues. Use a regular stored attribute updated
# by an event handler or explicit microflow instead.

RULE_ID = "CONV017"
RULE_NAME = "NoCalculatedAttributes"
DESCRIPTION = "Avoid calculated attributes; use stored attributes updated by microflows"
CATEGORY = "performance"
SEVERITY = "info"

def check():
    violations = []

    for entity in entities():
        if entity.entity_type != "Persistent" or entity.is_external:
            continue

        for attr in attributes_for(entity.qualified_name):
            if attr.is_calculated:
                violations.append(violation(
                    message="Attribute '{}.{}' is calculated. Calculated attributes can cause performance issues.".format(
                        entity.name, attr.name
                    ),
                    location=location(
                        module=entity.module_name,
                        document_type="Entity",
                        document_name=entity.qualified_name,
                    ),
                    suggestion="Replace with a stored attribute updated by a microflow (e.g., ACO_{})".format(entity.name),
                ))

    return violations
