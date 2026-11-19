package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/rafalmasiarek/terraform-provider-cwquery/internal/cwquery"
)

var metricEntryAttributeTypes = map[string]attr.Type{
	"id":          types.StringType,
	"namespace":   types.StringType,
	"metric_name": types.StringType,
	"stat":        types.StringType,
	"unit":        types.StringType,
	"dimensions":  types.MapType{ElemType: types.StringType},
	"expression":  types.StringType,
	"return_data": types.BoolType,
}

var parseResultAttributeTypes = map[string]attr.Type{
	"kind":                types.StringType,
	"comparison_operator": types.StringType,
	"threshold":           types.Float64Type,
	"threshold_metric_id": types.StringType,
	"metrics":             types.ListType{ElemType: types.ObjectType{AttrTypes: metricEntryAttributeTypes}},
	"alarm_rule":          types.StringType,
}

type parseFunction struct{}

func NewParseFunction() function.Function {
	return &parseFunction{}
}

func (f *parseFunction) Metadata(ctx context.Context, req function.MetadataRequest, resp *function.MetadataResponse) {
	resp.Name = "parse"
}

func (f *parseFunction) Definition(ctx context.Context, req function.DefinitionRequest, resp *function.DefinitionResponse) {
	resp.Definition = function.Definition{
		Summary: "Parses a SQL-like CloudWatch alarm query into a structured object consumable by " +
			"terraform-aws-cloudwatch's metric-alarm and composite-alarm modules.",
		Description: "Supports four query forms: a plain SELECT ... FROM ... WHERE ... HAVING comparison " +
			"(optionally with a stat function like p99(...)), a metric-math SELECT combining several metrics " +
			"with an AS alias, an anomaly-detection HAVING ... OUTSIDE/ABOVE/BELOW BAND(n), and a composite " +
			"ALARM(...)/AND/OR/NOT boolean expression over already-existing alarms. See this provider's " +
			"docs/grammar-spec.md for the full grammar and worked examples. The returned object's `kind` " +
			"field is \"metric\" for the first three forms (consume via terraform-aws-cloudwatch's " +
			"metric-alarm module) or \"composite\" for the last (consume via its composite-alarm module) - " +
			"every field that doesn't apply to the given kind is null.",
		Parameters: []function.Parameter{
			function.StringParameter{
				Name:        "query",
				Description: "The cwquery string to parse.",
			},
		},
		Return: function.ObjectReturn{AttributeTypes: parseResultAttributeTypes},
	}
}

func (f *parseFunction) Run(ctx context.Context, req function.RunRequest, resp *function.RunResponse) {
	var query string
	resp.Error = req.Arguments.Get(ctx, &query)
	if resp.Error != nil {
		return
	}

	compiled, err := cwquery.Parse(query)
	if err != nil {
		resp.Error = function.NewArgumentFuncError(0, fmt.Sprintf("parsing cwquery: %s", err))
		return
	}

	result, diags := toResultObject(compiled)
	if diags.HasError() {
		resp.Error = function.FuncErrorFromDiags(ctx, diags)
		return
	}

	resp.Error = resp.Result.Set(ctx, result)
}

func toResultObject(c *cwquery.Compiled) (types.Object, diag.Diagnostics) {
	var diags diag.Diagnostics

	metrics, d := toMetricsList(c.Metrics)
	diags.Append(d...)
	if diags.HasError() {
		return types.ObjectNull(parseResultAttributeTypes), diags
	}

	obj, d := types.ObjectValue(parseResultAttributeTypes, map[string]attr.Value{
		"kind":                types.StringValue(c.Kind),
		"comparison_operator": nullableString(c.ComparisonOperator),
		"threshold":           nullableFloat64(c.Threshold),
		"threshold_metric_id": nullableStringPtr(c.ThresholdMetricID),
		"metrics":             metrics,
		"alarm_rule":          nullableString(c.AlarmRule),
	})
	diags.Append(d...)
	return obj, diags
}

func toMetricsList(entries []cwquery.MetricEntry) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(entries) == 0 {
		return types.ListNull(types.ObjectType{AttrTypes: metricEntryAttributeTypes}), diags
	}

	values := make([]attr.Value, 0, len(entries))
	for _, e := range entries {
		dims, d := toDimensionsMap(e.Dimensions)
		diags.Append(d...)

		obj, d := types.ObjectValue(metricEntryAttributeTypes, map[string]attr.Value{
			"id":          nullableString(e.ID),
			"namespace":   nullableString(e.Namespace),
			"metric_name": nullableString(e.MetricName),
			"stat":        nullableString(e.Stat),
			"unit":        nullableString(e.Unit),
			"dimensions":  dims,
			"expression":  nullableString(e.Expression),
			"return_data": nullableBoolPtr(e.ReturnData),
		})
		diags.Append(d...)
		values = append(values, obj)
	}

	list, d := types.ListValue(types.ObjectType{AttrTypes: metricEntryAttributeTypes}, values)
	diags.Append(d...)
	return list, diags
}

func nullableString(s string) types.String {
	if s == "" {
		return types.StringNull()
	}
	return types.StringValue(s)
}

func nullableStringPtr(s *string) types.String {
	if s == nil {
		return types.StringNull()
	}
	return types.StringValue(*s)
}

func nullableFloat64(f *float64) types.Float64 {
	if f == nil {
		return types.Float64Null()
	}
	return types.Float64Value(*f)
}

func nullableBoolPtr(b *bool) types.Bool {
	if b == nil {
		return types.BoolNull()
	}
	return types.BoolValue(*b)
}

// toDimensionsMap builds a dimensions map.Value directly from Go values,
// rather than via reflection-based MapValueFrom, so no context.Context is
// needed and the construction stays consistent with every other value in
// this file.
func toDimensionsMap(dims map[string]string) (types.Map, diag.Diagnostics) {
	if len(dims) == 0 {
		return types.MapNull(types.StringType), nil
	}
	values := make(map[string]attr.Value, len(dims))
	for k, v := range dims {
		values[k] = types.StringValue(v)
	}
	return types.MapValue(types.StringType, values)
}
