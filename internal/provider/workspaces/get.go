package workspaces

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/schema/validator"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/singlestore-labs/singlestore-go/management"
	"github.com/singlestore-labs/terraform-provider-singlestoredb/internal/provider/config"
	"github.com/singlestore-labs/terraform-provider-singlestoredb/internal/provider/util"
)

const (
	DataSourceGetName = "workspace"
)

// workspacesDataSourceGet is the data source implementation.
type workspacesDataSourceGet struct {
	management.ClientWithResponsesInterface
}

// workspaceDataSourceModel maps workspace schema data.
type workspaceDataSourceModel struct {
	ID               types.String `tfsdk:"id"`
	WorkspaceGroupID types.String `tfsdk:"workspace_group_id"`
	Name             types.String `tfsdk:"name"`
	State            types.String `tfsdk:"state"`
	Size             types.String `tfsdk:"size"`
	Suspended        types.Bool   `tfsdk:"suspended"`
	CreatedAt        types.String `tfsdk:"created_at"`
	Endpoint         types.String `tfsdk:"endpoint"`
	LastResumedAt    types.String `tfsdk:"last_resumed_at"`
}

type workspaceDataSourceSchemaConfig struct {
	computeWorkspaceID    bool
	requireWorkspaceID    bool
	workspaceIDValidators []validator.String
}

var _ datasource.DataSourceWithConfigure = &workspacesDataSourceGet{}

// NewDataSourceGet is a helper function to simplify the provider implementation.
func NewDataSourceGet() datasource.DataSource {
	return &workspacesDataSourceGet{}
}

// Metadata returns the data source type name.
func (d *workspacesDataSourceGet) Metadata(_ context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = util.DataSourceTypeName(req, DataSourceGetName)
}

// Schema defines the schema for the data source.
func (d *workspacesDataSourceGet) Schema(_ context.Context, _ datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		Attributes: newWorkspaceDataSourceSchemaAttributes(workspaceDataSourceSchemaConfig{
			requireWorkspaceID:    true,
			workspaceIDValidators: []validator.String{util.NewUUIDValidator()},
		}),
	}
}

// Read refreshes the Terraform state with the latest data.
func (d *workspacesDataSourceGet) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	var data workspaceDataSourceModel
	diags := req.Config.Get(ctx, &data)
	resp.Diagnostics.Append(diags...)
	if resp.Diagnostics.HasError() {
		return
	}

	id, err := uuid.Parse(data.ID.ValueString())
	if err != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root(config.IDAttribute),
			"Invalid workspace ID",
			"The workspace ID should be a valid UUID",
		)

		return
	}

	workspace, err := d.GetV1WorkspacesWorkspaceIDWithResponse(ctx, id, &management.GetV1WorkspacesWorkspaceIDParams{})
	if serr := util.StatusOK(workspace, err); serr != nil {
		resp.Diagnostics.AddError(
			serr.Summary,
			serr.Detail,
		)

		return
	}

	if workspace.JSON200.TerminatedAt != nil {
		resp.Diagnostics.AddAttributeError(
			path.Root(config.IDAttribute),
			fmt.Sprintf("Workspace with the specified ID existed, but got terminated at %s", *workspace.JSON200.TerminatedAt),
			"Make sure to set the workspace ID of the workspace that exists.",
		)

		return
	}

	if workspace.JSON200.State == management.WorkspaceStateFAILED {
		resp.Diagnostics.AddError(
			fmt.Sprintf("Workspace with the specified ID exists, but is at the %s state", workspace.JSON200.State),
			config.ContactSupportErrorDetail,
		)

		return
	}

	result, terr := toWorkspaceDataSourceModel(*workspace.JSON200)
	if terr != nil {
		resp.Diagnostics.AddError(terr.Summary, terr.Detail)

		return
	}

	diags = resp.State.Set(ctx, &result)
	resp.Diagnostics.Append(diags...)
}

// Configure adds the provider configured client to the data source.
func (d *workspacesDataSourceGet) Configure(_ context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return // Should not return an error for unknown reasons.
	}

	d.ClientWithResponsesInterface = req.ProviderData.(management.ClientWithResponsesInterface)
}

func newWorkspaceDataSourceSchemaAttributes(conf workspaceDataSourceSchemaConfig) map[string]schema.Attribute {
	return map[string]schema.Attribute{
		config.IDAttribute: schema.StringAttribute{
			Computed:            conf.computeWorkspaceID,
			Required:            conf.requireWorkspaceID,
			MarkdownDescription: "ID of the workspace",
			Validators:          conf.workspaceIDValidators,
		},
		"workspace_group_id": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "ID of the workspace group",
		},
		"name": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Name of the workspace",
		},
		"state": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "State of the workspace",
		},
		"created_at": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "The timestamp of when the workspace was created",
		},
		"size": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Size of the workspace (in workspace size notation), such as S-00 or S-1",
		},
		"suspended": schema.BoolAttribute{
			Computed:            true,
			MarkdownDescription: "State of the workspace, suspended if set to true",
		},
		"endpoint": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "Endpoint to connect to the workspace",
		},
		"last_resumed_at": schema.StringAttribute{
			Computed:            true,
			MarkdownDescription: "(If included in the output) The timestamp of when the workspace was last resumed",
		},
	}
}

func toWorkspaceDataSourceModel(workspace management.Workspace) (workspaceDataSourceModel, *util.SummaryWithDetailError) {
	return workspaceDataSourceModel{
		ID:               util.UUIDStringValue(workspace.WorkspaceID),
		WorkspaceGroupID: util.UUIDStringValue(workspace.WorkspaceGroupID),
		Name:             types.StringValue(workspace.Name),
		State:            util.WorkspaceStateStringValue(workspace.State),
		Size:             types.StringValue(workspace.Size),
		Suspended:        types.BoolValue(workspace.State == management.WorkspaceStateSUSPENDED),
		CreatedAt:        types.StringValue(workspace.CreatedAt),
		Endpoint:         util.MaybeStringValue(workspace.Endpoint),
		LastResumedAt:    util.MaybeStringValue(workspace.LastResumedAt),
	}, nil
}
