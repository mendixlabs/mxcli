# Phase 4: Validation and Handoff

mxcli provides a multi-level validation pipeline. The agent uses these tools to self-correct before a human ever looks at the result.

## Validation Steps

```bash
# 1. Syntax check (fast, no project used)
mxcli check script.mdl --no-project

# 2. Reference validation (checks entity/microflow names exist)
mxcli check script.mdl -p app.mpr --references

# 3. Lint the full project (19 built-in + 31 Starlark rules)
mxcli lint -p app.mpr

# 4. Quality report (scored 0-100 per category)
mxcli report -p app.mpr --format markdown

# 5. Mendix compiler check (requires Docker)
mxcli docker check -p app.mpr

# 6. Build and run (requires Docker)
mxcli docker run -p app.mpr
```

## Lint Categories

The linter covers 6 categories:

| Category | Focus | Example Rules |
|----------|-------|---------------|
| **Naming** (CONV) | Naming conventions | Entity naming, microflow prefixes |
| **Security** (SEC) | Access control | Missing access rules, open security |
| **Quality** (QUAL) | Code quality | Unused variables, empty microflows |
| **Architecture** (ARCH) | Structure | Module dependencies, circular references |
| **Performance** (DESIGN) | Efficiency | Missing indexes, large retrieve-all |
| **Design** (MDL) | Best practices | Entity design, association patterns |

## Quality Reports

The `assess-quality` skill guides the agent to run `mxcli report` and interpret the scores. A migration is ready for handoff when:

- No critical lint issues (SEC, QUAL categories)
- Quality score above 70 across all categories
- `mxcli docker check` passes with no errors
- All tests pass

## Automated Testing

Write `.test.mdl` files to validate migrated business logic:

```sql
-- tests/order_tests.test.mdl
-- @test: Order total is calculated correctly

$Customer = CREATE CRM.Customer (Name = 'Test');
COMMIT $Customer;

$Order = CREATE Sales.Order (OrderNumber = 'ORD-001', OrderDate = now());
CHANGE $Order (Sales.Order_Customer = $Customer);
COMMIT $Order;

$Line = CREATE Sales.OrderLine (Price = 10.00, Quantity = 3);
CHANGE $Line (Sales.OrderLine_Order = $Order);
COMMIT $Line;

CALL MICROFLOW Sales.ACT_Order_CalculateTotal (Order = $Order);

-- @assert: $Order/TotalAmount = 30.00
```

```bash
# Run tests (Docker, or --local for mxcli's own runtime)
mxcli test tests/ -p app.mpr
```

## Handoff to Studio Pro

The final step transitions to Mendix Studio Pro for visual refinement:

| Task | Why Studio Pro |
|------|---------------|
| **Page layout tuning** | Visual drag-and-drop for pixel-perfect layouts |
| **Styling and theming** | CSS/SCSS editing with live preview |
| **Complex workflows** | Workflow editor for multi-step approval processes |
| **Marketplace modules** | Install and configure marketplace modules |
| **Deployment** | Configure deployment pipelines and environments |

### Handoff Checklist

```bash
# Final validation before opening in Studio Pro
mxcli docker check -p app.mpr          # No compiler errors
mxcli lint -p app.mpr                  # No critical lint issues
mxcli report -p app.mpr               # Review quality scores
mxcli test tests/ -p app.mpr          # All tests pass
mxcli -p app.mpr -c "DESCRIBE STRUCTURE DEPTH 2"  # Review structure
```

## Iterative Workflow

The mxcli/Studio Pro workflow is iterative -- you can switch between them:

```
  mxcli/MDL                    Studio Pro
 ┌──────────┐               ┌──────────────┐
 │ Generate  │──── open ───▶│ Visual edit   │
 │ entities, │               │ styling,     │
 │ microflows│◀── save ─────│ test, deploy  │
 │ pages     │               │              │
 └──────────┘               └──────────────┘
```

Changes made in either tool are persisted in the `.mpr` file. Always close mxcli before opening in Studio Pro, and vice versa.
