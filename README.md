# terraform-provider-cwquery
![](https://raw.githubusercontent.com/rafalmasiarek/readme-gen/main/banners/.gitkeep)

A small, hand-written SQL-like query language for defining CloudWatch
alarms. One provider function parses a query string into a structured
object that `terraform-aws-cloudwatch`'s `modules/metric-alarm` and
`modules/composite-alarm` consume directly - the typed-field interface
those modules already have stays the primary, production-grade way to use
them; `cwquery` is an additional, optional convenience layer on top, not a
replacement.

This is a separate provider from
[terraform-provider-netutils](https://github.com/rafalmasiarek/terraform-provider-netutils)
on purpose - CloudWatch alarm querying is a different domain from
netutils's networking utilities, not an extension of it.


## Provider configuration

No configuration - `cwquery` is a pure parser with no provider-level state.

```hcl
provider "cwquery" {}
```


## Functions

| Name    | Summary                                                               |
| ------- | ---------------------------------------------------------------------- |
| `parse` | Parses a cwquery string into the structured object described below.   |


### `parse`

```hcl
provider::cwquery::parse(query: string) -> object
```

Supports four query forms - see `docs/grammar-spec.md` for the full
grammar, v1 simplifications, and worked examples:

```sql
-- 1. Simple comparison + stat/aggregation
SELECT ErrorPortAllocation FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-0123456789abcdef0'
HAVING value > 0

SELECT p99(Latency) FROM AWS/ApplicationELB
WHERE LoadBalancer = 'app/my-alb/abc123'
HAVING value > 500

-- 2. Metric math (several metrics, one namespace, one AS alias)
SELECT (BytesOutToDestination + BytesInFromDestination) * 8 / 300 / 1e11 * 100 AS utilization
FROM AWS/NATGateway
WHERE NatGatewayId = 'nat-123'
HAVING utilization > 80

-- 3. Composite: AND/OR/NOT over already-existing alarms
(ALARM("cpu-high") OR ALARM("memory-high")) AND NOT ALARM("maintenance-window")

-- 4. Anomaly detection (a band, not a plain threshold)
SELECT CPUUtilization FROM AWS/EC2
WHERE AutoScalingGroupName = 'my-asg'
HAVING value OUTSIDE BAND(2)
```

The returned object's `kind` field is `"metric"` for forms 1/2/4 or
`"composite"` for form 3; every field that doesn't apply to the given kind
is `null`:

| Attribute             | Type                | Set when                          |
| ---------------------- | -------------------- | ---------------------------------- |
| `kind`                  | `string`              | always                              |
| `comparison_operator`   | `string`              | `kind == "metric"`                 |
| `threshold`             | `number`              | `kind == "metric"`, not anomaly    |
| `threshold_metric_id`   | `string`              | `kind == "metric"`, anomaly only   |
| `metrics`               | `list(object({...}))` | `kind == "metric"`                 |
| `alarm_rule`            | `string`              | `kind == "composite"`              |

`metrics`' object shape is deliberately identical, field-for-field, to
`terraform-aws-cloudwatch/modules/metric-alarm`'s own `metrics` variable -
assigning the parsed result straight into it needs no transformation.


## Development

```
go build -o bin/terraform-provider-cwquery .
go test ./...
```

Local testing without a published release: point Terraform at the built
binary via `dev_overrides` in a CLI config file (not committed to this
repo):

```hcl
provider_installation {
  dev_overrides {
    "rafalmasiarek/cwquery" = "/absolute/path/to/bin"
  }
  direct {}
}
```

Set `TF_CLI_CONFIG_FILE` to that file's path, then run `terraform plan`
directly - `terraform init` is unnecessary (and will fail) with
`dev_overrides` active for a provider.



## License
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)

```text
The MIT License (MIT)

Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files, to deal in the Software
without restriction.

The above copyright notice and this permission notice shall be included in
all copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND.

Source: https://opensource.org/licenses/MIT
```
See [LICENSE](LICENSE) for full details.
## Authors

- Rafał Masiarek | [website](https://masiarek.pl) | [email](mailto:rafal@masiarek.pl) | [github](https://github.com/rafalmasiarek)


<!-- references -->

[repo_link]: https://github.com/rafalmasiarek/terraform-provider-cwquery
