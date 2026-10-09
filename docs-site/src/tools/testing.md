# Testing

mxcli includes a testing framework for a Mendix app's own logic: each test calls
into the running app — a microflow, a retrieve — and asserts on what came back or
what was written. It is not a check that an MDL script parses; that is
[`mxcli check`](../tutorial/validation.md).

## Overview

The testing framework supports two test file formats:

| Format | Extension | Description |
|--------|-----------|-------------|
| MDL test files | `.test.mdl` | Tests as MDL blocks with annotations |
| Markdown test files | `.test.md` | Literate tests with prose and embedded `mdl-test` blocks |

## Prerequisites

Tests execute against a real Mendix runtime. **Docker is one way to get one, not the only one** — `--local` uses mxcli's own runtime with no daemon, and is the faster path (see [Running Tests](running-tests.md) for the mode comparison and the `--watch` / `--attach` loops). Either way the runner:

1. Installs a temporary `MxTest` module holding one microflow per test
2. Boots (or reuses) the app and invokes each test over a token-guarded endpoint
3. Evaluates that test's assertions and reports pass, fail or error
4. Removes what it installed, leaving the project byte-identical

## Quick Start

```bash
# Run all tests in a directory, including its subfolders
mxcli test tests/ -p app.mpr

# Run a specific test file
mxcli test tests/sales.test.mdl -p app.mpr
```

## Test Workflow

1. Write test files using `.test.mdl` or `.test.md` format
2. Name each test with `@test` and assert with `@expect` / `@verify` / `@throws`
3. Run tests with `mxcli test`
4. Review results

## Example Test

```mdl
-- tests/customer.test.mdl

/**
 * @test a new customer is active by default
 * @expect $customer/IsActive = true
 */
$customer = call microflow MyFirstModule.ACT_CreateCustomer(Name = 'Acme');
/

/**
 * @test the seed microflow writes five customers
 * @cleanup none
 * @expect count($customers) = 5
 * @verify select count(*) as n from MyFirstModule.Customer = 5
 */
call microflow MyFirstModule.ACT_Seed();
retrieve $customers from MyFirstModule.Customer;
/
```

An assertion the runner cannot evaluate is reported as an ERROR, never as a
pass — see [Test Annotations](test-annotations.md).

## Playwright UI Testing

For browser-based testing that verifies widgets render correctly in the DOM, see [Playwright Testing](playwright.md). `mxcli test` asserts on what the app's logic returns and writes; Playwright testing asserts on what the app renders -- that widgets are visible, forms accept input, and navigation works.

```bash
# MDL validation (this page)
mxcli test tests/ -p app.mpr

# Browser-based UI verification
mxcli playwright check /p/Customer_Overview -p app.mpr   # does it render? one call, text verdict
mxcli playwright verify tests/ -p app.mpr
```

## Related Pages

- [Test Formats](test-formats.md) -- `.test.mdl` and `.test.md` file formats
- [Test Annotations](test-annotations.md) -- `@test`, `@expect`, `@verify`, `@throws`, `@cleanup`
- [Running Tests](running-tests.md) -- `mxcli test` command and Docker requirements
- [Playwright Testing](playwright.md) -- Browser-based UI testing with playwright-cli
- [Diff](diff.md) -- Comparing scripts against project state
