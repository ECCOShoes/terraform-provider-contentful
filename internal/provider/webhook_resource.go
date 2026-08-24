package provider

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ECCOShoes/terraform-provider-contentful/internal/client"
)

var (
	_ resource.Resource                = (*webhookResource)(nil)
	_ resource.ResourceWithConfigure   = (*webhookResource)(nil)
	_ resource.ResourceWithImportState = (*webhookResource)(nil)
)

type webhookResource struct {
	client *client.Client
}

type webhookModel struct {
	ID                types.String `tfsdk:"id"`
	Version           types.Int64  `tfsdk:"version"`
	Name              types.String `tfsdk:"name"`
	URL               types.String `tfsdk:"url"`
	Topics            types.List   `tfsdk:"topics"`
	Active            types.Bool   `tfsdk:"active"`
	HTTPBasicUsername types.String `tfsdk:"http_basic_username"`
	HTTPBasicPassword types.String `tfsdk:"http_basic_password"`
	Headers           types.List   `tfsdk:"header"`
	Filters           types.String `tfsdk:"filters"`
	Transformation    types.Object `tfsdk:"transformation"`
}

// headerElement is the Go-native shape of a single "header" block, used for
// conversion to/from types.List via the framework's reflection helpers.
type headerElement struct {
	Key    string `tfsdk:"key"`
	Value  string `tfsdk:"value"`
	Secret bool   `tfsdk:"secret"`
}

// transformationElement is the Go-native shape of the "transformation"
// object. Fields use types.String/types.Bool (rather than native Go types)
// because every field is optional and therefore may be null.
type transformationElement struct {
	Method               types.String `tfsdk:"method"`
	ContentType          types.String `tfsdk:"content_type"`
	IncludeContentLength types.Bool   `tfsdk:"include_content_length"`
	Body                 types.String `tfsdk:"body"`
}

// NewWebhookResource constructs the webhook_definition resource, which
// manages a Contentful webhook (space-scoped, fires on content events).
func NewWebhookResource() resource.Resource {
	return &webhookResource{}
}

func (r *webhookResource) Metadata(_ context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_webhook_definition"
}

func (r *webhookResource) Schema(_ context.Context, _ resource.SchemaRequest, resp *resource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Description: "Manages a Contentful webhook definition. Webhooks are scoped to a " +
			"space (not a single environment); use filters to restrict delivery to specific " +
			"environments, content types, etc.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:      true,
				Description:   "Unique identifier of the webhook definition.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"version": schema.Int64Attribute{
				Computed:    true,
				Description: "Current version of the webhook definition, used internally for optimistic-concurrency updates.",
			},
			"name": schema.StringAttribute{
				Required:    true,
				Description: "Name of the webhook.",
			},
			"url": schema.StringAttribute{
				Required:    true,
				Description: "URL the webhook payload is sent to.",
			},
			"topics": schema.ListAttribute{
				ElementType: types.StringType,
				Required:    true,
				Description: "Events that trigger the webhook, e.g. [\"Entry.publish\", \"Asset.*\", \"*.*\"].",
			},
			"active": schema.BoolAttribute{
				Optional:    true,
				Computed:    true,
				Default:     booldefault.StaticBool(true),
				Description: "Whether the webhook is enabled. Defaults to true.",
			},
			"http_basic_username": schema.StringAttribute{
				Optional:    true,
				Description: "Username for HTTP basic authentication on the webhook call.",
			},
			"http_basic_password": schema.StringAttribute{
				Optional:    true,
				Sensitive:   true,
				Description: "Password for HTTP basic authentication on the webhook call. Never returned by the API; left unset on import unless reconfigured.",
			},
			"header": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Custom headers sent with every webhook call.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"key": schema.StringAttribute{
							Required:    true,
							Description: "Header name.",
						},
						"value": schema.StringAttribute{
							Required:    true,
							Sensitive:   true,
							Description: "Header value.",
						},
						"secret": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
							Description: "If true, the value is never returned by the API; Terraform keeps the last configured value.",
						},
					},
				},
			},
			"filters": schema.StringAttribute{
				Optional: true,
				Description: "Raw JSON array of Contentful webhook filter expressions, e.g. " +
					"[{\"equals\":[{\"doc\":\"sys.environment.sys.id\"},\"master\"]}]. Passed through as-is.",
			},
			"transformation": schema.SingleNestedAttribute{
				Optional:    true,
				Description: "Customizes the HTTP method, content type and body of the outgoing webhook call.",
				Attributes: map[string]schema.Attribute{
					"method": schema.StringAttribute{
						Optional:    true,
						Description: "HTTP method used to call the webhook, e.g. POST, PUT, GET, PATCH, DELETE.",
					},
					"content_type": schema.StringAttribute{
						Optional:    true,
						Description: "Content-Type header sent with the webhook call.",
					},
					"include_content_length": schema.BoolAttribute{
						Optional:    true,
						Description: "Whether to include a Content-Length header.",
					},
					"body": schema.StringAttribute{
						Optional:    true,
						Description: "Custom payload template for the webhook body.",
					},
				},
			},
		},
	}
}

func (r *webhookResource) Configure(_ context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	c, ok := req.ProviderData.(*client.Client)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected provider data type",
			fmt.Sprintf("Expected *client.Client, got %T.", req.ProviderData),
		)
		return
	}
	r.client = c
}

func (r *webhookResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var plan webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	if resp.Diagnostics.HasError() {
		return
	}

	draft, diags := draftFromModel(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.CreateWebhook(ctx, draft)
	if err != nil {
		resp.Diagnostics.AddError("Unable to create webhook", err.Error())
		return
	}

	// The plan already holds the exact configured values; only
	// provider-computed fields are taken from the API response.
	plan.ID = types.StringValue(wh.ID)
	plan.Version = types.Int64Value(int64(wh.Version))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.GetWebhook(ctx, state.ID.ValueString())
	if err != nil {
		if client.IsNotFound(err) {
			resp.State.RemoveResource(ctx)
			return
		}
		resp.Diagnostics.AddError("Unable to read webhook", err.Error())
		return
	}

	state.Version = types.Int64Value(int64(wh.Version))
	state.Name = types.StringValue(wh.Name)
	state.URL = types.StringValue(wh.URL)
	state.Active = types.BoolValue(wh.Active == nil || *wh.Active)
	state.HTTPBasicUsername = optionalString(wh.HTTPBasicUsername)

	topics, diags := types.ListValueFrom(ctx, types.StringType, wh.Topics)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Topics = topics

	// http_basic_password, secret header values, filters and transformation
	// are either never returned by the API (write-only) or can't be
	// round-tripped byte-for-byte (raw JSON formatting); leave them as
	// configured instead of overwriting with API-normalized values.

	resp.Diagnostics.Append(resp.State.Set(ctx, &state)...)
}

func (r *webhookResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var plan, state webhookModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &plan)...)
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	draft, diags := draftFromModel(ctx, plan)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	wh, err := r.client.UpdateWebhook(ctx, state.ID.ValueString(), int(state.Version.ValueInt64()), draft)
	if err != nil {
		resp.Diagnostics.AddError("Unable to update webhook", err.Error())
		return
	}

	plan.ID = state.ID
	plan.Version = types.Int64Value(int64(wh.Version))

	resp.Diagnostics.Append(resp.State.Set(ctx, &plan)...)
}

func (r *webhookResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var state webhookModel
	resp.Diagnostics.Append(req.State.Get(ctx, &state)...)
	if resp.Diagnostics.HasError() {
		return
	}

	err := r.client.DeleteWebhook(ctx, state.ID.ValueString(), int(state.Version.ValueInt64()))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete webhook", err.Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	resource.ImportStatePassthroughID(ctx, path.Root("id"), req, resp)
}

// draftFromModel converts a webhookModel (plan) into the client.WebhookDraft
// sent to the API.
func draftFromModel(ctx context.Context, m webhookModel) (client.WebhookDraft, diag.Diagnostics) {
	var diags diag.Diagnostics

	var topics []string
	diags.Append(m.Topics.ElementsAs(ctx, &topics, false)...)

	draft := client.WebhookDraft{
		Name:              m.Name.ValueString(),
		URL:               m.URL.ValueString(),
		Topics:            topics,
		Active:            m.Active.ValueBoolPointer(),
		HTTPBasicUsername: nullableString(m.HTTPBasicUsername),
		HTTPBasicPassword: nullableString(m.HTTPBasicPassword),
	}

	if !m.Headers.IsNull() && !m.Headers.IsUnknown() {
		var headers []headerElement
		diags.Append(m.Headers.ElementsAs(ctx, &headers, false)...)
		for _, h := range headers {
			draft.Headers = append(draft.Headers, client.WebhookHeader{
				Key:    h.Key,
				Value:  h.Value,
				Secret: h.Secret,
			})
		}
	}

	if !m.Filters.IsNull() && !m.Filters.IsUnknown() && m.Filters.ValueString() != "" {
		draft.Filters = json.RawMessage(m.Filters.ValueString())
	}

	if !m.Transformation.IsNull() && !m.Transformation.IsUnknown() {
		var t transformationElement
		diags.Append(m.Transformation.As(ctx, &t, basetypes.ObjectAsOptions{})...)
		includeContentLength := t.IncludeContentLength.ValueBool()
		draft.Transformation = &client.WebhookTransformation{
			Method:               t.Method.ValueString(),
			ContentType:          t.ContentType.ValueString(),
			IncludeContentLength: &includeContentLength,
			Body:                 nullableString(t.Body),
		}
	}

	return draft, diags
}
