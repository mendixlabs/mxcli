# Starlark Lint Rule: Missing Documentation
#
# Undocumented model elements are invisible to `mxcli check` and to the build —
# nothing fails, so nothing reminds you. This rule is the reminder, and it
# covers every document type a user authors, not just the domain model.
#
# Modules (check_modules): the domain model's documentation, which is the only
# documentation a Mendix module has.
#
# Documents swept generically (one option each, all default True):
#   Entity, Page, Snippet, BuildingBlock, Layout, Enumeration,
#   JavaScriptAction, ImageCollection, DataTransformer, Workflow,
#   BusinessEventService, RestClient, PublishedRestService, Constant,
#   JsonStructure, ImportMapping, ExportMapping
#
# Handled separately, because they carry exemptions or children a uniform
# sweep cannot express:
#   Microflow          .description   (nanoflows and trivial flows exempt)
#   JavaAction         .documentation
#   JavaAction params  .description   <- the one Studio Pro shows a CALLER
#
# Members, off by default purely because of volume:
#   Attribute          .description
#   Association        .description
#
# Why Java action parameters default ON while attributes default OFF: an action
# has a handful of parameters and Studio Pro renders each description in the
# dialog where someone wires up the call — an undocumented parameter is a blank
# field next to a name like `pInput` at exactly the moment a caller has to
# decide what to pass. A domain model has hundreds of attributes and
# associations, so the same check there is a wall of text rather than a signal.
#
# Every kind is individually switchable; see the table in _DOC_KINDS and the
# options listed under docs-site/src/tools/starlark-rules.md.

RULE_ID = "QUAL002"
RULE_NAME = "Missing Documentation"
DESCRIPTION = "Model elements should have documentation describing their purpose"
CATEGORY = "quality"
SEVERITY = "info"

# kind (as emitted by documentable_elements) -> (option, noun, suggestion)
#
# A new Mendix document type is covered by adding a row in Go's
# documentableSources and a row here — not by writing another loop.
_DOC_KINDS = {
    "Entity": (
        "check_entities",
        "Entity",
        "Add a description explaining the entity's purpose and what data it represents.",
    ),
    "Page": (
        "check_pages",
        "Page",
        "Describe what the page shows and who reaches it.",
    ),
    "Snippet": (
        "check_snippets",
        "Snippet",
        "Describe what the snippet renders and what context it expects, since it is reused across pages.",
    ),
    "BuildingBlock": (
        "check_building_blocks",
        "Building block",
        "Describe what the building block is for: it exists to be dropped in by someone who did not write it.",
    ),
    "Layout": (
        "check_layouts",
        "Layout",
        "Describe the layout's intended use and its placeholders.",
    ),
    "Enumeration": (
        "check_enumerations",
        "Enumeration",
        "Describe what the enumeration models, especially where the values map to something external.",
    ),
    "JavaScriptAction": (
        "check_javascript_actions",
        "JavaScript action",
        "Document what the action does and what it returns. Like a Java action, its body is code the model cannot show a reader.",
    ),
    "ImageCollection": (
        "check_image_collections",
        "Image collection",
        "Describe what the collection is for and where its images are used.",
    ),
    "DataTransformer": (
        "check_data_transformers",
        "Data transformer",
        "Describe the transformation applied and the shape it expects.",
    ),
    "Workflow": (
        "check_workflows",
        "Workflow",
        "Describe the process the workflow models and who its user tasks are for.",
    ),
    "BusinessEventService": (
        "check_business_event_services",
        "Business event service",
        "Document the events published or consumed, since other applications depend on them.",
    ),
    "RestClient": (
        "check_rest_clients",
        "REST client",
        "Document which external service is consumed and what it is used for.",
    ),
    "PublishedRestService": (
        "check_published_rest_services",
        "Published REST service",
        "Document the contract: this is the description external consumers read.",
    ),
    "Constant": (
        "check_constants",
        "Constant",
        "Describe what the constant configures and what a valid value looks like — it is set per environment by someone who cannot see the code.",
    ),
    "JsonStructure": (
        "check_json_structures",
        "JSON structure",
        "Note which payload the structure was captured from.",
    ),
    "ImportMapping": (
        "check_import_mappings",
        "Import mapping",
        "Describe the source payload and what it maps onto.",
    ),
    "ExportMapping": (
        "check_export_mappings",
        "Export mapping",
        "Describe the target payload and what it is produced for.",
    ),
    "Association": (
        # Off by default with attributes: a real domain model has as many
        # associations as entities, and none of them are documented.
        "check_associations",
        "Association",
        "Add a description, or switch this off with `check_associations: false` if the names are self-describing here.",
    ),
}

# Kinds whose option defaults to False. Everything else defaults to True.
_OFF_BY_DEFAULT = {"check_associations": True}

def _blank(text):
    """True when a documentation field is absent or whitespace-only."""
    return not text or text.strip() == ""

def _flag(violations, module, doc_type, doc_name, message, suggestion):
    violations.append(violation(
        message = message,
        location = location(
            module = module,
            document_type = doc_type,
            document_name = doc_name,
        ),
        suggestion = suggestion,
    ))

def check():
    violations = []

    # ---- every document type, one sweep -------------------------------------
    for el in documentable_elements():
        entry = _DOC_KINDS.get(el.kind)
        if entry == None:
            # A kind Go knows about but this table does not. Staying silent is
            # right: a rule inventing a message for an element it cannot
            # describe is worse than not reporting it.
            continue
        option, noun, suggestion = entry
        if not get_option(option, not _OFF_BY_DEFAULT.get(option, False)):
            continue
        if _blank(el.description):
            _flag(
                violations,
                el.module_name,
                el.kind,
                el.qualified_name,
                "{} '{}' has no documentation.".format(noun, el.name),
                suggestion,
            )

    # ---- modules: a module's documentation is its domain model's ------------
    # It is the one sentence that says what a module is for; the architecture
    # report and the software map show it as the module's purpose, and an
    # empty one leaves them nothing to show (ako/mxcli#2305). modules() lists
    # the app's own modules only: no Marketplace module, no System.
    if get_option("check_modules", True):
        for mod in modules():
            if _blank(mod.domain_model_documentation):
                _flag(
                    violations,
                    mod.name,
                    "Module",
                    mod.name,
                    "Module '{}' has no documentation.".format(mod.name),
                    "Say in one sentence what the module is for: " +
                    "alter module {} set documentation '...', or a /** ... */ comment before create or modify module.".format(mod.name),
                )

    # ---- microflows: exempt nanoflows and trivial flows ---------------------
    if get_option("check_microflows", True):
        # Nanoflows are excluded: they are usually a couple of client-side steps
        # whose name says everything a description would.
        min_activities = get_option("min_activities", 3)
        for mf in microflows():
            if mf.microflow_type != "MICROFLOW":
                continue
            if mf.activity_count < min_activities:
                continue
            if _blank(mf.description):
                _flag(
                    violations,
                    mf.module_name,
                    "Microflow",
                    mf.qualified_name,
                    "Microflow '{}' has no documentation.".format(mf.name),
                    "Add a description explaining what this microflow does and when it should be called.",
                )

    # ---- Java actions and their parameters ----------------------------------
    check_actions = get_option("check_java_actions", True)
    check_params = get_option("check_java_action_params", True)
    if check_actions or check_params:
        for ja in java_actions():
            if check_actions and _blank(ja.documentation):
                _flag(
                    violations,
                    ja.module_name,
                    "JavaAction",
                    ja.qualified_name,
                    "Java action '{}' has no documentation.".format(ja.name),
                    "Add documentation explaining what the action does, and what it returns. " +
                    "Unlike a microflow, its body is Java that the model cannot show a reader.",
                )
            if not check_params:
                continue
            for p in ja.parameters:
                if _blank(p.description):
                    _flag(
                        violations,
                        ja.module_name,
                        "JavaAction",
                        ja.qualified_name,
                        "Java action parameter '{}.{}' has no description.".format(ja.name, p.name),
                        "Add a description: Studio Pro shows it to whoever wires up the call, " +
                        "where the parameter name is all they otherwise have to go on.",
                    )

    # ---- entity attributes (off by default: high volume) --------------------
    if get_option("check_attributes", False):
        for entity in entities():
            for attr in attributes_for(entity.qualified_name):
                if _blank(attr.description):
                    _flag(
                        violations,
                        entity.module_name,
                        "Entity",
                        entity.qualified_name,
                        "Attribute '{}.{}' has no documentation.".format(entity.name, attr.name),
                        "Add a description, or switch this off with `check_attributes: false` if " +
                        "attribute names are self-describing in this project.",
                    )

    return violations
