package provider

import (
	"context"
	"os"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-framework/provider/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/types"

	"github.com/ECCOShoes/terraform-provider-contentful/internal/client"
)

// defaultAPIURL is used when neither the provider block nor CONTENTFUL_API_URL
// specify one.
const defaultAPIURL = "https://api.contentful.com"

// Ensure contentfulProvider satisfies the provider.Provider interface.
var _ provider.Provider = (*contentfulProvider)(nil)

type contentfulProvider struct {
	version string
}

type providerModel struct {
	ManagementToken types.String `tfsdk:"management_token"`
	APIURL          types.String `tfsdk:"api_url"`
}

// New returns a function that constructs the provider, as required by
// providerserver.Serve.
func New(version string) func() provider.Provider {
	return func() provider.Provider {
		return &contentfulProvider{version: version}
	}
}

func (p *contentfulProvider) Metadata(_ context.Context, _ provider.MetadataRequest, resp *provider.MetadataResponse) {
	resp.TypeName = "contentful"
	resp.Version = p.version
}

func (p *contentfulProvider) Schema(_ context.Context, _ provider.SchemaRequest, resp *provider.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Terraform provider for Contentful.",
		Attributes: map[string]schema.Attribute{
			"management_token": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Contentful Content Management API access token. May also be set via the CONTENTFUL_MANAGEMENT_TOKEN environment variable.",
			},
			"api_url": schema.StringAttribute{
				Optional:    true,
				Description: "Content Management API base URL. Defaults to https://api.contentful.com. May also be set via CONTENTFUL_API_URL.",
			},
		},
	}
}

func (p *contentfulProvider) Configure(ctx context.Context, req provider.ConfigureRequest, resp *provider.ConfigureResponse) {
	var cfg providerModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &cfg)...)
	if resp.Diagnostics.HasError() {
		return
	}

	managementToken := valueOrEnv(cfg.ManagementToken, "CONTENTFUL_MANAGEMENT_TOKEN")
	apiURL := valueOrEnv(cfg.APIURL, "CONTENTFUL_API_URL")
	if apiURL == "" {
		apiURL = defaultAPIURL
	}

	requireAttr(resp, managementToken, "management_token", "CONTENTFUL_MANAGEMENT_TOKEN")
	if resp.Diagnostics.HasError() {
		return
	}

	c, err := client.New(client.Config{
		ManagementToken: managementToken,
		APIURL:          apiURL,
	})
	if err != nil {
		resp.Diagnostics.AddError("Unable to create Contentful client", err.Error())
		return
	}

	resp.ResourceData = c
}

func (p *contentfulProvider) Resources(_ context.Context) []func() resource.Resource {
	return []func() resource.Resource{
		NewWebhookResource,
	}
}

func (p *contentfulProvider) DataSources(_ context.Context) []func() datasource.DataSource {
	return nil
}

func valueOrEnv(v types.String, env string) string {
	if !v.IsNull() && !v.IsUnknown() && v.ValueString() != "" {
		return v.ValueString()
	}
	return os.Getenv(env)
}

func requireAttr(resp *provider.ConfigureResponse, value, attr, env string) {
	if value == "" {
		resp.Diagnostics.AddError(
			"Missing Contentful configuration",
			"The provider requires "+attr+" to be set, either in the provider block or via the "+env+" environment variable.",
		)
	}
}
