---
title: Metrics activities — increment counter / set gauge
status: implemented
---

# Proposal: Metrics activities in MDL

## Problem Statement

Studio Pro's toolbox has a **Metrics Activities** section: *Counter*, *Increment
Counter* and *Gauge*. They are how a Mendix app reports its own business metrics to
OpenTelemetry, Prometheus, JMX, InfluxDB or StatsD (through Micrometer and the
`Metrics.Registries` custom runtime setting). MDL had no spelling for them. The
metamodel types were generated, but the reader mapped them to `UnsupportedAction`:
describe printed `-- Unsupported action: Microflows$CounterMeterAction`, a rebuild of
the microflow was refused rather than drop the activity, and a script could not
create one.

## BSON Structure

Measured from a Studio Pro 11.16.1 project (ako/TestApp,
`BusinessEventsClient.MetricsMf`, one activity of each kind):

```
Microflows$CounterMeterAction            Microflows$IncrementCounterMeterAction   Microflows$GaugeMeterAction
  Description: ""                          Description: ""                          Description: ""
  ErrorHandlingType: "Rollback"            ErrorHandlingType: "Rollback"            ErrorHandlingType: "Rollback"
  Name: "@Metrics.NAME_COUNTER"            Name: "@Metrics.NAME_COUNTER"            Name: "@Metrics.NAME_COUNTER"
  Tags: [2, MeterTagMapping…]              Tags: [2, MeterTagMapping…]              Tags: [2, MeterTagMapping…]
  Value: "4"                                                                        Value: "3.14"

Microflows$MeterTagMapping
  Key:   "'tag-abc'"
  Value: "'123'"
```

What this settled, against the reference guide:

- **The name is an expression**, not literal text: Studio Pro stored a constant
  reference. A literal name is `'acme.orders'`, quoted.
- **Tag keys and values are expressions** too, both quoted.
- `Tags` is list marker **2** on all three; `ErrorHandlingType` and an empty
  `Description` are always written.
- The types date from 9.5.0 (mendixmodelsdk type versions).

`Description` is not in the documented activity dialog and was empty in every
measured activity. mxcli reads it, but MDL has no key for it, so an activity
rebuilt from MDL stores `""`. Whether Studio Pro ever sets it is not measured.

## MDL Syntax

ADR-0013: words for the verb and main operand, one property list for the dialog.

```sql
increment counter 'acme.orders.placed' (Tags: ('region': $Order/Region));  -- Increment Counter
increment counter @Shop.ItemsMetric by $Order/ItemCount;                    -- Counter
set gauge 'acme.queue.depth' to $Depth (Tags: ('queue': 'mail'));           -- Gauge
```

`increment counter` covers both counter activities because the only difference is
the amount: without `by` it is one. `set gauge … to` matches what a gauge does: it
holds the last value set.

## Validation (measured on mxbuild 11.16.1)

| Case | mxbuild | mxcli `check` |
|---|---|---|
| literal name empty / starts `mx` (any case) / bad characters or leading digit | CE7040 / CE7048 / CE7049 | MDL-METRIC01 |
| `by -1` | **accepted** | MDL-METRIC02 |
| `on error continue` | CE6035 on all three | MDL076 |
| name or tag not String, `by` not Integer/Long, `to` not numeric | CE0117 | E009 (`--references`) |
| `set gauge … to 2` (Integer), negative gauge, custom handler | clean | clean |
| in a nanoflow | — (Studio Pro: microflow only) | nanoflow activity rule |

Version-gated at 9.5.0 (`metrics_activities`).

## Not covered

- The MCP (Studio Pro PED) backend: it rejects the action with its "not supported
  yet" error, like other unmapped activities.
- `Description`, as above.
