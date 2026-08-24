package provider

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"github.com/hashicorp/terraform-plugin-framework-validators/listvalidator"
	"github.com/hashicorp/terraform-plugin-framework-validators/stringvalidator"
	"github.com/hashicorp/terraform-plugin-framework/attr"
	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-framework/types/basetypes"

	"github.com/ECCOShoes/terraform-provider-contentful/internal/client"
)

// allowedFilterProperties are the entity properties Contentful's webhook
// filter UI exposes; anything else can't be modeled by this resource.
var allowedFilterProperties = []string{
	"sys.id",
	"sys.environment.sys.id",
	"sys.contentType.sys.id",
	"sys.createdBy.sys.id",
	"sys.updatedBy.sys.id",
	"sys.deletedBy.sys.id",
}

// filterValueCharset matches the charset Contentful allows for equals/in
// filter values (letters, digits, underscore, hyphen, dot).
var filterValueCharset = regexp.MustCompile(`^[A-Za-z0-9_.-]+$`)

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
	SpaceID           types.String `tfsdk:"space_id"`
	Version           types.Int64  `tfsdk:"version"`
	Name              types.String `tfsdk:"name"`
	URL               types.String `tfsdk:"url"`
	Topics            types.List   `tfsdk:"topics"`
	Active            types.Bool   `tfsdk:"active"`
	HTTPBasicUsername types.String `tfsdk:"http_basic_username"`
	HTTPBasicPassword types.String `tfsdk:"http_basic_password"`
	Headers           types.List   `tfsdk:"header"`
	Filters           types.List   `tfsdk:"filter"`
	Transformation    types.Object `tfsdk:"transformation"`
}

// headerElement is the Go-native shape of a single "header" block, used for
// conversion to/from types.List via the framework's reflection helpers.
type headerElement struct {
	Key    string `tfsdk:"key"`
	Value  string `tfsdk:"value"`
	Secret bool   `tfsdk:"secret"`
}

// filterElement is the Go-native shape of a single "filter" block. Exactly
// one of Equals, In or Regexp is set (enforced by an ExactlyOneOf schema
// validator).
type filterElement struct {
	Property types.String `tfsdk:"property"`
	Equals   types.String `tfsdk:"equals"`
	In       types.List   `tfsdk:"in"`
	Regexp   types.String `tfsdk:"regexp"`
	Negate   types.Bool   `tfsdk:"negate"`
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
			"space_id": schema.StringAttribute{
				Required:      true,
				Description:   "Contentful space this webhook belongs to. Webhooks cannot move between spaces; changing this replaces the resource.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.RequiresReplace()},
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
			"filter": schema.ListNestedAttribute{
				Optional:    true,
				Description: "Restricts which events the webhook is called for. Multiple filter blocks are combined with AND.",
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"property": schema.StringAttribute{
							Required:    true,
							Description: "Property of the triggering entity to check, e.g. \"sys.environment.sys.id\".",
							Validators:  []validator.String{stringvalidator.OneOf(allowedFilterProperties...)},
						},
						"equals": schema.StringAttribute{
							Optional:    true,
							Description: "Match if the property equals this value. Exactly one of equals, in or regexp is required.",
							Validators: []validator.String{
								stringvalidator.LengthBetween(1, 255),
								stringvalidator.RegexMatches(filterValueCharset, "must contain only letters, digits, underscores, hyphens and dots"),
								// Checked on this attribute (rather than the object as a whole) because
								// objectvalidator.ExactlyOneOf always counts the object itself as "specified".
								stringvalidator.ExactlyOneOf(
									path.MatchRelative().AtParent().AtName("in"),
									path.MatchRelative().AtParent().AtName("regexp"),
								),
							},
						},
						"in": schema.ListAttribute{
							ElementType: types.StringType,
							Optional:    true,
							Description: "Match if the property equals any of these values. Exactly one of equals, in or regexp is required.",
							Validators: []validator.List{
								listvalidator.SizeAtLeast(1),
								listvalidator.ValueStringsAre(
									stringvalidator.LengthBetween(1, 255),
									stringvalidator.RegexMatches(filterValueCharset, "must contain only letters, digits, underscores, hyphens and dots"),
								),
							},
						},
						"regexp": schema.StringAttribute{
							Optional:    true,
							Description: "Match if the property matches this regular expression. Exactly one of equals, in or regexp is required.",
							Validators:  []validator.String{stringvalidator.LengthBetween(1, 1024)},
						},
						"negate": schema.BoolAttribute{
							Optional:    true,
							Computed:    true,
							Default:     booldefault.StaticBool(false),
							Description: "If true, inverts the match (e.g. equals becomes \"not equals\"). Defaults to false.",
						},
					},
				},
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

	wh, err := r.client.CreateWebhook(ctx, plan.SpaceID.ValueString(), draft)
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

	wh, err := r.client.GetWebhook(ctx, state.SpaceID.ValueString(), state.ID.ValueString())
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

	filters, diags := filterElementsFromAPI(ctx, wh.Filters)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}
	state.Filters = filters

	// http_basic_password, secret header values and transformation are
	// either never returned by the API (write-only) or can't be
	// round-tripped byte-for-byte; leave them as configured instead of
	// overwriting with API-normalized values.

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

	wh, err := r.client.UpdateWebhook(ctx, state.SpaceID.ValueString(), state.ID.ValueString(), int(state.Version.ValueInt64()), draft)
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

	err := r.client.DeleteWebhook(ctx, state.SpaceID.ValueString(), state.ID.ValueString(), int(state.Version.ValueInt64()))
	if err != nil && !client.IsNotFound(err) {
		resp.Diagnostics.AddError("Unable to delete webhook", err.Error())
	}
}

func (r *webhookResource) ImportState(ctx context.Context, req resource.ImportStateRequest, resp *resource.ImportStateResponse) {
	spaceID, id, ok := strings.Cut(req.ID, "/")
	if !ok {
		resp.Diagnostics.AddError(
			"Invalid import ID",
			"Expected an import ID in the form <space_id>/<webhook_id>, e.g. \"abc123/wh456\".",
		)
		return
	}
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("space_id"), spaceID)...)
	resp.Diagnostics.Append(resp.State.SetAttribute(ctx, path.Root("id"), id)...)
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

	if !m.Filters.IsNull() && !m.Filters.IsUnknown() {
		var filters []filterElement
		diags.Append(m.Filters.ElementsAs(ctx, &filters, false)...)
		for _, f := range filters {
			wf := client.WebhookFilter{
				Property: f.Property.ValueString(),
				Negate:   f.Negate.ValueBool(),
			}
			switch {
			case !f.Equals.IsNull() && !f.Equals.IsUnknown():
				v := f.Equals.ValueString()
				wf.Equals = &v
			case !f.In.IsNull() && !f.In.IsUnknown():
				diags.Append(f.In.ElementsAs(ctx, &wf.In, false)...)
			case !f.Regexp.IsNull() && !f.Regexp.IsUnknown():
				v := f.Regexp.ValueString()
				wf.Regexp = &v
			}
			draft.Filters = append(draft.Filters, wf)
		}
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

// filterObjectType is the attr.Type of a single "filter" list element,
// matching the filterElement struct/schema shape.
var filterObjectType = types.ObjectType{
	AttrTypes: map[string]attr.Type{
		"property": types.StringType,
		"equals":   types.StringType,
		"in":       types.ListType{ElemType: types.StringType},
		"regexp":   types.StringType,
		"negate":   types.BoolType,
	},
}

// filterElementsFromAPI converts the filters on a webhook API response into
// the types.List stored in state.
func filterElementsFromAPI(ctx context.Context, filters []client.WebhookFilter) (types.List, diag.Diagnostics) {
	var diags diag.Diagnostics
	if len(filters) == 0 {
		return types.ListNull(filterObjectType), diags
	}

	elements := make([]filterElement, 0, len(filters))
	for _, f := range filters {
		el := filterElement{
			Property: types.StringValue(f.Property),
			Equals:   optionalString(f.Equals),
			Regexp:   optionalString(f.Regexp),
			Negate:   types.BoolValue(f.Negate),
			In:       types.ListNull(types.StringType),
		}
		if f.In != nil {
			in, d := types.ListValueFrom(ctx, types.StringType, f.In)
			diags.Append(d...)
			el.In = in
		}
		elements = append(elements, el)
	}

	list, d := types.ListValueFrom(ctx, filterObjectType, elements)
	diags.Append(d...)
	return list, diags
}
