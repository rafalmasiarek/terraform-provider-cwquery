# cwquery grammar spec

This is the canonical contract for `provider::cwquery::parse(query)`'s input
grammar and output shape. It's a cross-repo contract: the output shape is
consumed directly by `terraform-aws-cloudwatch`'s `modules/metric-alarm` and
`modules/composite-alarm` - don't change either side without updating both.

## Scope

Four tiers, each mapping to a real CloudWatch concept - there is no fifth
tier, and no plan to add general-purpose SQL features (`JOIN`, subqueries,
`GROUP BY`, ...) that have no corresponding AWS concept to map onto:

1. **Simple comparison + stat/aggregation** - one metric, optional stat
   function, one comparator, one threshold. Produces a plain
   `aws_cloudwatch_metric_alarm` (or, for a percentile stat, one that uses
   `metric_query` instead of the legacy `statistic` attribute, since
   percentiles aren't valid there).
2. **Metric math** - several metrics from the same namespace/dimensions,
   combined by an arithmetic expression with a required `AS` alias.
   Produces `metric_query` blocks, one raw metric per reference plus one
   expression entry.
3. **Composite** - `AND`/`OR`/`NOT` over already-existing alarms. Produces
   `aws_cloudwatch_composite_alarm`'s `alarm_rule`, not a metric alarm at
   all. The alarm names inside `ALARM("...")` are plain string literals -
   the caller resolves them via ordinary Terraform interpolation
   (`"ALARM(\"${module.x.alarm_name}\")"`) before the string ever reaches
   this parser, which has zero knowledge of the Terraform resource graph.
4. **Anomaly detection** - a band around a predicted value, not a plain
   threshold. Produces a `metric_query` entry whose `expression =
   ANOMALY_DETECTION_BAND(m, n)`, with the alarm's `threshold_metric_id`
   pointing at it instead of a numeric `threshold`.

## Grammar

```
query            := select_query | composite_query ;

select_query     := "SELECT" select_list
                     "FROM" namespace
                     "WHERE" where_clause
                     "HAVING" having_clause ;

namespace        := ident "/" ident ;                  -- "AWS/NATGateway"

where_clause     := dim_pred ("AND" dim_pred)* ;
dim_pred         := ident "=" string ;

select_list      := metric_term
                   | math_expr "AS" ident ;             -- tier 2 always needs AS

metric_term      := ident "(" ident ")"                -- stat-call: p99(Latency)
                   | ident ;                            -- bare metric name

math_expr        := math_term (("+"|"-") math_term)* ;
math_term        := math_factor (("*"|"/") math_factor)* ;
math_factor      := number | ident | "(" math_expr ")" ;

having_clause    := value_ref comparator number
                   | value_ref band_pred ;
value_ref        := ident ;        -- must be "value" with no alias, or the
                                    -- declared alias exactly, checked at
                                    -- compile time, not by the grammar
comparator       := ">" | ">=" | "<" | "<=" ;
                    -- "=" and "!=" parse but are REJECTED at compile time:
                    -- CloudWatch's comparison_operator has no equality form.
band_pred        := ("OUTSIDE"|"ABOVE"|"BELOW") "BAND" "(" number ")" ;

composite_query  := composite_expr ;
composite_expr   := composite_term ("OR" composite_term)* ;
composite_term   := composite_factor ("AND" composite_factor)* ;
composite_factor := "NOT" composite_factor
                   | "(" composite_expr ")"
                   | "ALARM" "(" string ")" ;

ident  := letter (letter | digit | "_")* ;
string := "'" ... "'" | '"' ... '"' ;      -- WHERE values typically use ',
                                            -- ALARM(...) typically uses " -
                                            -- both are accepted everywhere
number := digit+ ("." digit+)? (("e"|"E") ("+"|"-")? digit+)? ;
```

Top-level disambiguation between `select_query` and `composite_query` is a
plain LL(1) dispatch on the first token (`SELECT` vs. `ALARM`/`NOT`/`(`) -
the two forms never share a legal first token.

## v1 simplifications (deliberate, not oversights)

- **One namespace/dimension set per metric-math query.** All metrics
  referenced in a single math expression share the query's one `FROM`
  namespace and one `WHERE` dimension filter - matching the real NAT
  bandwidth-utilization case this tier was modeled on. Combining metrics
  from different namespaces or with different dimensions is not supported;
  there's no syntax for it.
- **Composite expressions only cover `ALARM(...)` + `AND`/`OR`/`NOT`.**
  AWS's real `alarm_rule` syntax also has `OK(...)`, `INSUFFICIENT_DATA(...)`,
  `TRUE`, and `FALSE` - deliberately out of scope until something in this
  module family actually needs them.
- **No equality comparator.** `=`/`!=` are syntactically legal (so a typo
  produces a clear semantic error instead of a confusing parse error) but
  always rejected during compilation, because CloudWatch's
  `comparison_operator` enum has no equality variant at all.

## Output shape

```go
type Compiled struct {
    Kind               string          // "metric" | "composite"
    ComparisonOperator string          // one of the 7 real AWS enum values; "" for composite
    Threshold          *float64        // nil for an anomaly-band or composite result
    ThresholdMetricID  *string         // non-nil only for tier 4 (anomaly detection)
    Metrics            []MetricEntry   // nil/empty for composite
    AlarmRule          string          // "" unless Kind == "composite"
}

type MetricEntry struct {
    ID         string
    Namespace  string
    MetricName string
    Stat       string
    Unit       string
    Dimensions map[string]string
    Expression string
    ReturnData *bool
}
```

`MetricEntry` is deliberately field-for-field identical to
`terraform-aws-cloudwatch/modules/metric-alarm`'s own `metrics` variable
object shape - assigning `metrics = provider::cwquery::parse(var.query).metrics`
needs no transformation.

## Worked examples

See `examples/functions/main.tf` in this repository for one runnable
example per tier, and `terraform-aws-cloudwatch/modules/metric-alarm` and
`modules/composite-alarm`'s own `docs/usage.md` for how the parsed result
is actually consumed inside a real alarm resource.
