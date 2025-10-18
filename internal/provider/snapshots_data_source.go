// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-framework/datasource"
	"github.com/hashicorp/terraform-plugin-framework/datasource/schema"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/types"
	"github.com/hashicorp/terraform-plugin-log/tflog"

	"github.com/forwardnetworks/terraform-provider-forward/internal/sdk"
)

var _ datasource.DataSource = &SnapshotsDataSource{}

// NewSnapshotsDataSource instantiates the snapshots data source.
func NewSnapshotsDataSource() datasource.DataSource {
	return &SnapshotsDataSource{}
}

// SnapshotsDataSource retrieves snapshots for a network.
type SnapshotsDataSource struct {
	providerData *ForwardProviderData
}

type snapshotsDataSourceModel struct {
	NetworkID       types.String   `tfsdk:"network_id"`
	Limit           types.Int64    `tfsdk:"limit"`
	IncludeArchived types.Bool     `tfsdk:"include_archived"`
	Snapshots       []snapshotItem `tfsdk:"snapshots"`
}

type snapshotItem struct {
	ID                types.String `tfsdk:"id"`
	State             types.String `tfsdk:"state"`
	ProcessingTrigger types.String `tfsdk:"processing_trigger"`
	ParentSnapshotID  types.String `tfsdk:"parent_snapshot_id"`
	Note              types.String `tfsdk:"note"`
	IsDraft           types.Bool   `tfsdk:"is_draft"`
	CreationMillis    types.Int64  `tfsdk:"creation_date_millis"`
	ProcessedMillis   types.Int64  `tfsdk:"processed_at_millis"`
	RestoredMillis    types.Int64  `tfsdk:"restored_at_millis"`
	FavoritedBy       types.String `tfsdk:"favorited_by"`
	FavoritedByUserID types.String `tfsdk:"favorited_by_user_id"`
	FavoritedMillis   types.Int64  `tfsdk:"favorited_at_millis"`
}

func (d *SnapshotsDataSource) Metadata(ctx context.Context, req datasource.MetadataRequest, resp *datasource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_snapshots"
}

func (d *SnapshotsDataSource) Schema(ctx context.Context, req datasource.SchemaRequest, resp *datasource.SchemaResponse) {
	resp.Schema = schema.Schema{
		MarkdownDescription: "Retrieve Forward Enterprise snapshots for a network.",
		Attributes: map[string]schema.Attribute{
			"network_id": schema.StringAttribute{
				MarkdownDescription: "Network ID to query. Defaults to the provider `network_id` when omitted.",
				Optional:            true,
			},
			"limit": schema.Int64Attribute{
				MarkdownDescription: "Maximum number of snapshots to return.",
				Optional:            true,
			},
			"include_archived": schema.BoolAttribute{
				MarkdownDescription: "Include archived snapshots in the result set.",
				Optional:            true,
			},
			"snapshots": schema.ListNestedAttribute{
				MarkdownDescription: "Snapshots returned by the Forward Enterprise API.",
				Computed:            true,
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"id":                   schema.StringAttribute{Computed: true},
						"state":                schema.StringAttribute{Computed: true},
						"processing_trigger":   schema.StringAttribute{Computed: true},
						"parent_snapshot_id":   schema.StringAttribute{Computed: true},
						"note":                 schema.StringAttribute{Computed: true},
						"is_draft":             schema.BoolAttribute{Computed: true},
						"creation_date_millis": schema.Int64Attribute{Computed: true},
						"processed_at_millis":  schema.Int64Attribute{Computed: true},
						"restored_at_millis":   schema.Int64Attribute{Computed: true},
						"favorited_by":         schema.StringAttribute{Computed: true},
						"favorited_by_user_id": schema.StringAttribute{Computed: true},
						"favorited_at_millis":  schema.Int64Attribute{Computed: true},
					},
				},
			},
		},
	}
}

func (d *SnapshotsDataSource) Configure(ctx context.Context, req datasource.ConfigureRequest, resp *datasource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}

	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Data Source Configure Type",
			fmt.Sprintf("Expected *ForwardProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}

	d.providerData = providerData
}

func (d *SnapshotsDataSource) Read(ctx context.Context, req datasource.ReadRequest, resp *datasource.ReadResponse) {
	if d.providerData == nil {
		resp.Diagnostics.AddError(
			"Client Not Configured",
			"The provider client was not configured. Ensure the provider block is present before using this data source.",
		)
		return
	}

	var data snapshotsDataSourceModel
	resp.Diagnostics.Append(req.Config.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}

	networkID := d.providerData.NetworkID
	if !data.NetworkID.IsNull() && !data.NetworkID.IsUnknown() {
		networkID = data.NetworkID.ValueString()
	}

	if networkID == "" {
		resp.Diagnostics.AddAttributeError(
			path.Root("network_id"),
			"Missing Network ID",
			"Network ID must be specified either on the provider or data source.",
		)
		return
	}

	options := sdk.SnapshotListOptions{}
	if !data.Limit.IsNull() && !data.Limit.IsUnknown() {
		limit := int(data.Limit.ValueInt64())
		if limit < 0 {
			resp.Diagnostics.AddAttributeError(
				path.Root("limit"),
				"Invalid Limit",
				"Limit must be zero or a positive integer.",
			)
			return
		}
		options.Limit = &limit
	}

	if !data.IncludeArchived.IsNull() && !data.IncludeArchived.IsUnknown() {
		value := data.IncludeArchived.ValueBool()
		options.IncludeArchived = &value
	}

	snapshots, err := d.providerData.Client.ListSnapshots(ctx, networkID, options)
	if err != nil {
		resp.Diagnostics.AddError(
			"Unable to Retrieve Snapshots",
			err.Error(),
		)
		return
	}

	items := make([]snapshotItem, 0, len(snapshots))
	for _, snapshot := range snapshots {
		item := snapshotItem{
			ID:                types.StringValue(snapshot.ID),
			State:             types.StringNull(),
			ProcessingTrigger: types.StringNull(),
			ParentSnapshotID:  types.StringNull(),
			Note:              types.StringNull(),
			IsDraft:           types.BoolNull(),
			CreationMillis:    types.Int64Null(),
			ProcessedMillis:   types.Int64Null(),
			RestoredMillis:    types.Int64Null(),
			FavoritedBy:       types.StringNull(),
			FavoritedByUserID: types.StringNull(),
			FavoritedMillis:   types.Int64Null(),
		}

		if snapshot.State != "" {
			item.State = types.StringValue(snapshot.State)
		}
		if snapshot.ProcessingTrigger != "" {
			item.ProcessingTrigger = types.StringValue(snapshot.ProcessingTrigger)
		}
		if snapshot.ParentSnapshotID != "" {
			item.ParentSnapshotID = types.StringValue(snapshot.ParentSnapshotID)
		}
		if snapshot.Note != "" {
			item.Note = types.StringValue(snapshot.Note)
		}
		if snapshot.IsDraft != nil {
			item.IsDraft = types.BoolValue(*snapshot.IsDraft)
		}
		if snapshot.CreationDateMillis != nil {
			item.CreationMillis = types.Int64Value(*snapshot.CreationDateMillis)
		}
		if snapshot.ProcessedAtMillis != nil {
			item.ProcessedMillis = types.Int64Value(*snapshot.ProcessedAtMillis)
		}
		if snapshot.RestoredAtMillis != nil {
			item.RestoredMillis = types.Int64Value(*snapshot.RestoredAtMillis)
		}
		if snapshot.FavoritedBy != "" {
			item.FavoritedBy = types.StringValue(snapshot.FavoritedBy)
		}
		if snapshot.FavoritedByUserID != "" {
			item.FavoritedByUserID = types.StringValue(snapshot.FavoritedByUserID)
		}
		if snapshot.FavoritedAtMillis != nil {
			item.FavoritedMillis = types.Int64Value(*snapshot.FavoritedAtMillis)
		}

		items = append(items, item)
	}

	data.Snapshots = items

	tflog.Trace(ctx, "retrieved forward snapshots", map[string]any{"count": len(items)})

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}
