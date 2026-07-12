# Starlark Lint Rule: Entity Attribute Count
#
# This rule checks that entities don't have too many attributes.
# Entities with many attributes often indicate:
#   - Missing normalization (split into related entities)
#   - God object anti-pattern
#   - Need for entity inheritance/generalization
#
# Entity properties:
#   .id                - Document ID
#   .name              - Simple name (e.g., "Customer")
#   .qualified_name    - Full name (e.g., "MyModule.Customer")
#   .module_name       - Module name
#   .entity_type       - "Persistent", "NonPersistent", or "View"
#   .description       - Documentation
#   .generalization    - Parent entity (if any)
#   .attribute_count   - Number of attributes
#   .access_rule_count - Number of access rules
#   .is_external       - True if entity is from an external service
#
# Full API reference: .claude/skills/mendix/write-lint-rules.md

RULE_ID = "DESIGN001"
RULE_NAME = "Entity Attribute Count"
DESCRIPTION = "Entities should not have more than 10 attributes"
CATEGORY = "design"
SEVERITY = "warning"

# Maximum allowed attributes per entity - customize as needed
MAX_ATTRIBUTES = 20

def check():
    """
    Check that entities don't have too many attributes.
    Large entities are harder to maintain and may indicate design issues.
    """
    violations = []

    for entity in entities():
        if entity.attribute_count > MAX_ATTRIBUTES:
            loc = location(
                module=entity.module_name,
                document_type="Entity",
                document_name=entity.qualified_name
            )
            v = violation(
                message="Entity '{}' has {} attributes (max: {}). Consider splitting into smaller entities.".format(
                    entity.name,
                    entity.attribute_count,
                    MAX_ATTRIBUTES
                ),
                location=loc,
                suggestion="Extract related attributes into separate entities with associations, or use entity generalization."
            )
            violations.append(v)

    return violations
