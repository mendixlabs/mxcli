# Starlark Lint Rule: Entity Business Key
#
# This rule checks that persistent entities have a unique, not-null attribute
# that can serve as a business key for identification across systems.
#
# Mendix internal IDs (auto-generated) are unique only within a single app.
# When sharing data between systems (integrations, migrations, external APIs),
# a business key is essential for reliable object identification.
#
# A business key should be:
#   - UNIQUE: Ensures no duplicates
#   - NOT NULL: Ensures every object has an identifier
#
# Common business key patterns:
#   - Code: String(50) NOT NULL UNIQUE (e.g., "CUST-001")
#   - ExternalID: String(100) NOT NULL UNIQUE (e.g., UUID from source system)
#   - Email: String(200) NOT NULL UNIQUE (for user entities)
#
# Uses the attributes_for(entity_qualified_name) function to query entity attributes.

RULE_ID = "ARCH003"
RULE_NAME = "Entity Business Key"
DESCRIPTION = "Persistent entities should have a unique, not-null attribute as a business key"
CATEGORY = "architecture"
SEVERITY = "info"

# Attribute names that commonly indicate a business key
BUSINESS_KEY_PATTERNS = ["Code", "ExternalId", "ExternalID", "UUID", "Key", "Identifier", "Reference", "_Id"]

def has_business_key(entity):
    """Check if entity has at least one attribute that is both unique and required."""
    attrs = attributes_for(entity.qualified_name)
    for attr in attrs:
        if attr.is_unique and attr.is_required:
            return True
    return False

def suggest_key_name(entity):
    """Suggest a business key attribute name based on entity name."""
    name = entity.name
    # For entities ending in common suffixes, suggest appropriate key names
    if name.endswith("User") or name.endswith("Account"):
        return "Email or Username"
    if name.endswith("Order") or name.endswith("Invoice"):
        return "OrderNumber or InvoiceNumber"
    if name.endswith("Product") or name.endswith("Item"):
        return "ProductCode or SKU"
    return name + "Code or ExternalID"

def check():
    """
    Check that persistent entities have a business key attribute.
    A business key is an attribute that is both UNIQUE and NOT NULL.
    """
    violations = []

    for entity in entities():
        # Skip non-persistent entities (they don't need business keys)
        if entity.entity_type != "PERSISTENT":
            continue

        # Skip system/administration entities that typically use internal IDs
        if entity.module_name in ["System", "Administration"]:
            continue

        # Check if entity has a business key
        if not has_business_key(entity):
            loc = location(
                module=entity.module_name,
                document_type="Entity",
                document_name=entity.qualified_name
            )

            suggestion = suggest_key_name(entity)
            v = violation(
                message="Entity '{}' has no business key. Add a UNIQUE NOT NULL attribute for cross-system identification.".format(
                    entity.name
                ),
                location=loc,
                suggestion="Add an attribute like '{}' with UNIQUE and NOT NULL constraints. Example: Code: String(50) NOT NULL UNIQUE".format(
                    suggestion
                )
            )
            violations.append(v)

    return violations
