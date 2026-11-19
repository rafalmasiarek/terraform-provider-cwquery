package provider

import (
	"context"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/function"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
)

type cwqueryProvider struct {
	version string
}

func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &cwqueryProvider{version: version}
	}
}

func (p *cwqueryProvider) Metadata(ctx context.Context, req provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "cwquery"
	resp.Version = p.version
}

func (p *cwqueryProvider) Schema(ctx context.Context, req provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "A small, hand-written SQL-like query language for defining CloudWatch alarms - " +
			"parses a query string into the structured shape terraform-aws-cloudwatch's metric-alarm and " +
			"composite-alarm modules consume. Pure parser, no configuration needed.",
	}
}

func (p *cwqueryProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	// No configuration, no provider-level state - the sole function is a
	// pure function of its string argument.
}

func (p *cwqueryProvider) DataSources(ctx context.Context) []func() datasource.DataSource {
	return nil
}

func (p *cwqueryProvider) Resources(ctx context.Context) []func() resource.Resource {
	return nil
}

func (p *cwqueryProvider) Functions(ctx context.Context) []func() function.Function {
	return []func() function.Function{
		NewParseFunction,
	}
}
