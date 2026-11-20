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
# Exempt too: an entity the page hands to a snippet or page of the entity's own
# module that takes it as a parameter. Conversational UI's chat snippet needs a
# data view on ConversationalUI.ChatContext, exactly as the module's own
# full-screen chat page has it; a view entity or microflow cannot stand in for
# the persistent object the snippet expects (ako/mxcli#2069).
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

def takes_parameter(document, entity_ref):
    """Whether a snippet or page has a parameter of type entity_ref."""
    for ref in refs_from(document):
        if ref.ref_kind == "parameter" and ref.target_name == entity_ref:
            return True
    return False

def fed_to_owning_module(container, entity_ref, entity_module, cache):
    """Whether container calls a snippet, or links to a page, of entity_module
    that takes entity_ref as a parameter — the module's own UI asking for its
    own entity, which only a data view on that entity can supply."""
    key = container + "|" + entity_ref
    if key in cache:
        return cache[key]
    found = False
    for ref in refs_from(container):
        if ref.target_type not in ("SNIPPET", "PAGE"):
            continue
        if get_module_from_qualified_name(ref.target_name) != entity_module:
            continue
        if takes_parameter(ref.target_name, entity_ref):
            found = True
            break
    cache[key] = found
    return found

def check():
    """
    Check that widgets don't reference entities from other modules.
    Cross-module data access should go through view entities or microflows.
    """
    violations = []

    # Track violations per container to avoid duplicate messages
    seen = {}
    fed = {}

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
            if fed_to_owning_module(widget.container_qualified_name, widget.entity_ref, entity_module, fed):
                continue
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
