# Starlark Lint Rule: Cross-Module Data Access
#
# This rule enforces that pages and widgets only use data from entities
# in the same module. Data from other modules should be accessed through:
#   - View entities (which expose a controlled interface)
#   - Microflows (which encapsulate the data retrieval logic)
#
# This promotes loose coupling between modules and better encapsulation.
#
# Exempt: the System workflow entities (System.Workflow*). Mendix requires them
# on a page — a task page takes a System.WorkflowUserTask parameter
# (CE7410/CE7412), and `complete task` / `open user task` / `open workflow` only
# work inside a System.WorkflowUserTask or System.Workflow data container
# (CE1843) — so there is no view entity or microflow to use instead
# (ako/mxcli#2034).
#
# Widget properties:
#   .id                       - Widget ID
#   .name                     - Widget name
#   .widget_type              - Type of widget (e.g., "DataView", "ListView")
#   .container_id             - ID of containing page/snippet
#   .container_qualified_name - Qualified name of container (e.g., "MyModule.MyPage")
#   .container_type           - Type of container ("Page" or "Snippet")
#   .module_name              - Module the widget is in
#   .entity_ref               - Qualified name of referenced entity (e.g., "OtherModule.Customer")
#   .attribute_ref            - Referenced attribute path

RULE_ID = "ARCH001"
RULE_NAME = "Cross-Module Data Access"
DESCRIPTION = "Pages and widgets should only use data from entities in the same module"
CATEGORY = "architecture"
SEVERITY = "warning"

def get_module_from_qualified_name(qualified_name):
    """Extract module name from a qualified name like 'Module.Entity'."""
    if not qualified_name or "." not in qualified_name:
        return ""
    return qualified_name.split(".")[0]

def is_platform_required(entity_ref):
    """The System workflow entities a task page or task inbox must use."""
    return entity_ref.startswith("System.Workflow")

def check():
    """
    Check that widgets don't reference entities from other modules.
    Cross-module data access should go through view entities or microflows.
    """
    violations = []

    # Track violations per container to avoid duplicate messages
    seen = {}

    for widget in widgets():
        # Skip widgets without entity references
        if not widget.entity_ref:
            continue
        if is_platform_required(widget.entity_ref):
            continue

        # Get the module of the referenced entity
        entity_module = get_module_from_qualified_name(widget.entity_ref)
        if not entity_module:
            continue

        # Check if entity is from a different module
        if entity_module != widget.module_name:
            # Create a unique key for this violation
            key = "{}:{}".format(widget.container_qualified_name, widget.entity_ref)
            if key in seen:
                continue
            seen[key] = True

            loc = location(
                module=widget.module_name,
                document_type=widget.container_type if widget.container_type else "Page",
                document_name=widget.container_qualified_name
            )
            v = violation(
                message="Widget '{}' in '{}' references entity '{}' from module '{}'. Use a view entity or microflow instead.".format(
                    widget.name,
                    widget.container_qualified_name,
                    widget.entity_ref,
                    entity_module
                ),
                location=loc,
                suggestion="Create a view entity in '{}' that exposes the needed data from '{}', or use a microflow to retrieve the data.".format(
                    widget.module_name,
                    entity_module
                )
            )
            violations.append(v)

    return violations
