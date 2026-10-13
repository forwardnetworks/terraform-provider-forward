// Copyright (c) HashiCorp, Inc.
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/hashicorp/terraform-plugin-framework/diag"
	"github.com/hashicorp/terraform-plugin-framework/path"
	"github.com/hashicorp/terraform-plugin-framework/resource"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/booldefault"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/int64default"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/listplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/mapplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/planmodifier"
	"github.com/hashicorp/terraform-plugin-framework/resource/schema/stringplanmodifier"
	"github.com/hashicorp/terraform-plugin-framework/types"

	forward "github.com/forwardnetworks/forward-go-sdk"
)

var (
	_ resource.Resource                   = &PredictedSnapshotResource{}
	_ resource.ResourceWithValidateConfig = &PredictedSnapshotResource{}
)

// PredictedSnapshotResource produces a snapshot of the network as it would be
// after a change, without making the change.
//
// It is a resource rather than a data source because it has effects a read must
// not: it opens a change set, stages a draft on it, commits, and leaves a
// snapshot behind. Destroying it discards the change set.
//
// Its point is the id it exposes. The nqe_query and path_analysis data sources
// are snapshot-scoped, so pointing them at this resource asks what Forward
// believes about a network that does not exist yet -- and a check block on the
// answer refuses the apply that would have caused it.
type PredictedSnapshotResource struct {
	providerData *ForwardProviderData
}

// PredictedSnapshotResourceModel stores Terraform state.
type PredictedSnapshotResourceModel struct {
	ID                  types.String        `tfsdk:"id"`
	NetworkID           types.String        `tfsdk:"network_id"`
	BaseSnapshotID      types.String        `tfsdk:"base_snapshot_id"`
	FirewallRules       []FirewallRuleModel `tfsdk:"firewall_rules"`
	DeviceCommands      types.Map           `tfsdk:"device_commands"`
	Note                types.String        `tfsdk:"note"`
	PollIntervalSeconds types.Int64         `tfsdk:"poll_interval_seconds"`
	TimeoutSeconds      types.Int64         `tfsdk:"timeout_seconds"`
	KeepChangeSet       types.Bool          `tfsdk:"keep_change_set"`

	ChangeSetID     types.String `tfsdk:"change_set_id"`
	State           types.String `tfsdk:"state"`
	FirewallChanges types.String `tfsdk:"firewall_changes_json"`
}

// FirewallRuleModel is one security rule to stage on a classic firewall, in Forward's rule model: the on-prem
// half of a hybrid change. Applications are the vendor's predefined names; services default to those
// applications' defaults.
type FirewallRuleModel struct {
	Device             types.String `tfsdk:"device"`
	Name               types.String `tfsdk:"name"`
	Description        types.String `tfsdk:"description"`
	Action             types.String `tfsdk:"action"`
	SourceZones        types.List   `tfsdk:"source_zones"`
	DestinationZones   types.List   `tfsdk:"destination_zones"`
	SourceSubnets      types.List   `tfsdk:"source_subnets"`
	DestinationSubnets types.List   `tfsdk:"destination_subnets"`
	Applications       types.List   `tfsdk:"applications"`
	Predecessor        types.String `tfsdk:"predecessor"`
	After              types.String `tfsdk:"after"`
}

func NewPredictedSnapshotResource() resource.Resource {
	return &PredictedSnapshotResource{}
}

func (r *PredictedSnapshotResource) Metadata(ctx context.Context, req resource.MetadataRequest, resp *resource.MetadataResponse) {
	resp.TypeName = req.ProviderTypeName + "_predicted_snapshot"
}

func (r *PredictedSnapshotResource) Schema(ctx context.Context, req resource.SchemaRequest, resp *resource.SchemaResponse) {
	// Every input replaces rather than updates: a prediction is of one stated
	// change against one base, so changing either makes it a different question
	// and the old snapshot is not an answer to the new one.
	replace := []planmodifier.String{stringplanmodifier.RequiresReplace()}

	resp.Schema = schema.Schema{
		MarkdownDescription: "Predict the network as it would be after a change to classic devices (CLI " +
			"configuration or firewall rules), and expose the resulting snapshot so snapshot-scoped data " +
			"sources can be read against it.",
		Attributes: map[string]schema.Attribute{
			"id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Predicted snapshot identifier. Pass this as `snapshot_id` to a data source.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"network_id": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Network identifier. Defaults to the provider's network.",
				PlanModifiers:       replace,
			},
			"base_snapshot_id": schema.StringAttribute{
				Optional: true,
				Computed: true,
				MarkdownDescription: "Snapshot to predict from. Defaults to the most recent *collected* snapshot, " +
					"which is not always the most recent one: every prediction leaves a snapshot behind, and " +
					"predicting from a prediction is refused.",
				PlanModifiers: replace,
			},
			"device_commands": schema.MapAttribute{
				ElementType: types.StringType,
				Optional:    true,
				MarkdownDescription: "CLI configuration to stage on classic network devices, by Forward device " +
					"name (configuration-mode lines, as the device's own CLI takes them). Staged on the same change " +
					"set as any firewall_rules, so one prediction covers both.",
				PlanModifiers: []planmodifier.Map{mapplanmodifier.RequiresReplace()},
			},
			"firewall_rules": schema.ListNestedAttribute{
				Optional: true,
				MarkdownDescription: "Security rules to stage on classic firewalls, in Forward's rule model. " +
					"Staged on the same change set as any device_commands, so one prediction covers both.",
				PlanModifiers: []planmodifier.List{listplanmodifier.RequiresReplace()},
				NestedObject: schema.NestedAttributeObject{
					Attributes: map[string]schema.Attribute{
						"device":              schema.StringAttribute{Required: true, MarkdownDescription: "Forward device name of the firewall."},
						"name":                schema.StringAttribute{Required: true, MarkdownDescription: "Rule name."},
						"description":         schema.StringAttribute{Optional: true},
						"action":              schema.StringAttribute{Required: true, MarkdownDescription: "`PERMIT` or `DENY`."},
						"source_zones":        schema.ListAttribute{ElementType: types.StringType, Required: true},
						"destination_zones":   schema.ListAttribute{ElementType: types.StringType, Required: true},
						"source_subnets":      schema.ListAttribute{ElementType: types.StringType, Required: true, MarkdownDescription: "CIDRs."},
						"destination_subnets": schema.ListAttribute{ElementType: types.StringType, Required: true, MarkdownDescription: "CIDRs."},
						"applications": schema.ListAttribute{ElementType: types.StringType, Required: true,
							MarkdownDescription: "Vendor-predefined application names (`postgres`, `ssh`, ...); services are their defaults."},
						"predecessor": schema.StringAttribute{Optional: true,
							MarkdownDescription: "Id of the existing rule this one follows; unset puts it at the top."},
						"after": schema.StringAttribute{Optional: true,
							MarkdownDescription: "Name of the existing rule this one follows, resolved to its id on the device; an alternative to `predecessor`."},
					},
				},
			},
			"note": schema.StringAttribute{
				Optional:            true,
				Computed:            true,
				MarkdownDescription: "Note recorded on the commit and the predicted snapshot.",
				PlanModifiers:       replace,
			},
			"poll_interval_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(10),
				MarkdownDescription: "How often to poll while the prediction processes.",
			},
			"timeout_seconds": schema.Int64Attribute{
				Optional:            true,
				Computed:            true,
				Default:             int64default.StaticInt64(2400),
				MarkdownDescription: "How long to wait for the prediction to finish processing.",
			},
			"keep_change_set": schema.BoolAttribute{
				Optional: true,
				Computed: true,
				Default:  booldefault.StaticBool(false),
				MarkdownDescription: "Keep the change set when this resource is destroyed. Off by default, so a " +
					"prediction does not leave change sets behind on every apply.",
			},
			"change_set_id": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Change set the prediction was run from.",
				PlanModifiers:       []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
			"state": schema.StringAttribute{
				Computed:            true,
				MarkdownDescription: "Processing state of the predicted snapshot.",
			},
			"firewall_changes_json": schema.StringAttribute{
				Computed: true,
				MarkdownDescription: "JSON object of device name to the rulebase diff rows the change set alters " +
					"(Forward's DiffEntry shape, a = collected, b = staged), for the firewall_rules staged.",
				PlanModifiers: []planmodifier.String{stringplanmodifier.UseStateForUnknown()},
			},
		},
	}
}

// ValidateConfig refuses a prediction of nothing at plan time rather than
// mid-apply. Unknown values pass: they may well hold a change.
func (r *PredictedSnapshotResource) ValidateConfig(ctx context.Context, req resource.ValidateConfigRequest, resp *resource.ValidateConfigResponse) {
	var commands types.Map
	var rules types.List
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("device_commands"), &commands)...)
	resp.Diagnostics.Append(req.Config.GetAttribute(ctx, path.Root("firewall_rules"), &rules)...)
	if resp.Diagnostics.HasError() || commands.IsUnknown() || rules.IsUnknown() {
		return
	}
	if len(commands.Elements()) == 0 && len(rules.Elements()) == 0 {
		resp.Diagnostics.AddError("No Change Stated",
			"Set device_commands or firewall_rules. Predicting no change would return the base snapshot under a new id.")
	}
}

func (r *PredictedSnapshotResource) Configure(ctx context.Context, req resource.ConfigureRequest, resp *resource.ConfigureResponse) {
	if req.ProviderData == nil {
		return
	}
	providerData, ok := req.ProviderData.(*ForwardProviderData)
	if !ok {
		resp.Diagnostics.AddError(
			"Unexpected Resource Configure Type",
			fmt.Sprintf("Expected *ForwardProviderData, got: %T. Please report this issue to the provider developers.", req.ProviderData),
		)
		return
	}
	r.providerData = providerData
}

func (r *PredictedSnapshotResource) Create(ctx context.Context, req resource.CreateRequest, resp *resource.CreateResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() {
		return
	}
	if r.providerData == nil || r.providerData.Client == nil {
		resp.Diagnostics.AddError("Unconfigured Provider",
			"The provider client was not configured. Re-run terraform init or review provider configuration.")
		return
	}
	client := r.providerData.Client

	firewallRules, diags := firewallRules(ctx, data.FirewallRules)
	resp.Diagnostics.Append(diags...)
	deviceCommands := map[string]string{}
	if !data.DeviceCommands.IsNull() && !data.DeviceCommands.IsUnknown() {
		resp.Diagnostics.Append(data.DeviceCommands.ElementsAs(ctx, &deviceCommands, false)...)
	}
	if resp.Diagnostics.HasError() {
		return
	}
	if len(firewallRules) == 0 && len(deviceCommands) == 0 {
		resp.Diagnostics.AddError("No Change Stated",
			"Set device_commands or firewall_rules. Predicting no change would return the base snapshot under a new id.")
		return
	}

	networkID := data.NetworkID.ValueString()
	if networkID == "" {
		networkID = r.providerData.NetworkID
	}
	if networkID == "" {
		resp.Diagnostics.AddError("Missing Network", "Set network_id on the resource or on the provider.")
		return
	}
	data.NetworkID = types.StringValue(networkID)

	base := data.BaseSnapshotID.ValueString()
	if base == "" {
		latest, _, err := client.Snapshots.LatestCollected(ctx, networkID)
		if err != nil {
			resp.Diagnostics.AddError("No Base Snapshot", err.Error())
			return
		}
		base = string(latest.ID)
	}
	data.BaseSnapshotID = types.StringValue(base)

	note := data.Note.ValueString()
	if note == "" {
		note = "terraform"
	}
	data.Note = types.StringValue(note)

	changeSet, _, err := client.Predict.CreateChangeSet(ctx, networkID, forward.ChangeSetCreateRequest{
		Name:       note,
		SnapshotID: base,
	})
	if err != nil {
		resp.Diagnostics.AddError("Change Set Not Created", err.Error())
		return
	}
	data.ChangeSetID = types.StringValue(string(changeSet.ID))

	// From here the change set exists, so every failure discards it rather than
	// leaving a draft nobody owns behind.
	fail := func(summary string, err error) {
		if !data.KeepChangeSet.ValueBool() {
			_, _ = client.Predict.DeleteChangeSet(ctx, networkID, string(changeSet.ID))
		}
		resp.Diagnostics.AddError(summary, err.Error())
	}

	// Firewall rules and device commands share the change set: one change is one prediction.
	for _, staged := range firewallRules {
		if staged.after != "" {
			id, err := ruleIDByName(ctx, client, networkID, string(changeSet.ID), staged.device, staged.after)
			if err != nil {
				fail("Firewall Rule Not Staged", fmt.Errorf("%s: %w", staged.device, err))
				return
			}
			staged.rule.Predecessor = id
		}
		if _, err := client.Predict.AddSecurityRule(ctx, networkID, string(changeSet.ID), staged.device,
			forward.FirewallScopeLocal, forward.FirewallRulebasePrimary, staged.rule); err != nil {
			fail("Firewall Rule Not Staged", fmt.Errorf("%s: %w", staged.device, err))
			return
		}
	}
	// Device commands too: a device's draft is its configuration-mode lines, staged whole.
	for _, device := range sortedKeys(deviceCommands) {
		if _, err := client.Predict.StageCommands(ctx, networkID, string(changeSet.ID), device, deviceCommands[device]); err != nil {
			fail("Device Commands Not Staged", fmt.Errorf("%s: %w", device, err))
			return
		}
	}
	firewallChanges, err := firewallChanges(ctx, client, networkID, string(changeSet.ID), firewallRules)
	if err != nil {
		fail("Firewall Changes Not Read", err)
		return
	}
	data.FirewallChanges = types.StringValue(firewallChanges)
	if _, err := client.Predict.Commit(ctx, networkID, string(changeSet.ID), note); err != nil {
		fail("Change Set Not Committed", err)
		return
	}
	predicted, _, err := client.Predict.Run(ctx, networkID, string(changeSet.ID), note)
	if err != nil {
		fail("Predict Refused", err)
		return
	}
	data.ID = types.StringValue(string(predicted.ID))
	data.State = types.StringValue(predicted.State)

	// A snapshot still processing has no model to read, so returning here would
	// hand a data source an id it cannot query yet.
	timeout := time.Duration(data.TimeoutSeconds.ValueInt64()) * time.Second
	waitCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	poller, _, err := client.Snapshots.Operation(waitCtx, networkID, string(predicted.ID))
	if err != nil {
		fail("Prediction Did Not Finish", err)
		return
	}
	details, _, err := poller.Wait(waitCtx, forward.PollOptions[forward.Snapshot]{
		Interval: time.Duration(data.PollIntervalSeconds.ValueInt64()) * time.Second,
	})
	if err != nil {
		fail("Prediction Did Not Finish", err)
		return
	}
	data.State = types.StringValue(details.State)

	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Read refreshes the predicted snapshot's state.
//
// A predicted snapshot is immutable once processed, so this only notices that it
// is gone -- which is a real case: snapshots are archived.
func (r *PredictedSnapshotResource) Read(ctx context.Context, req resource.ReadRequest, resp *resource.ReadResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	details, _, err := r.providerData.Client.Snapshots.Get(ctx, data.NetworkID.ValueString(), data.ID.ValueString())
	if err != nil {
		resp.State.RemoveResource(ctx)
		return
	}
	data.State = types.StringValue(details.State)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Update cannot be reached: every input requires replacement.
func (r *PredictedSnapshotResource) Update(ctx context.Context, req resource.UpdateRequest, resp *resource.UpdateResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.Plan.Get(ctx, &data)...)
	resp.Diagnostics.Append(resp.State.Set(ctx, &data)...)
}

// Delete discards the change set. The predicted snapshot itself is left alone:
// it is a record of what was predicted, and Forward ages snapshots out on its
// own schedule.
func (r *PredictedSnapshotResource) Delete(ctx context.Context, req resource.DeleteRequest, resp *resource.DeleteResponse) {
	var data PredictedSnapshotResourceModel
	resp.Diagnostics.Append(req.State.Get(ctx, &data)...)
	if resp.Diagnostics.HasError() || r.providerData == nil || r.providerData.Client == nil {
		return
	}
	if data.KeepChangeSet.ValueBool() || data.ChangeSetID.ValueString() == "" {
		return
	}
	if _, err := r.providerData.Client.Predict.DeleteChangeSet(ctx,
		data.NetworkID.ValueString(), data.ChangeSetID.ValueString()); err != nil {
		resp.Diagnostics.AddWarning("Change Set Not Discarded",
			fmt.Sprintf("The predicted snapshot is gone from state but change set %s remains: %s",
				data.ChangeSetID.ValueString(), err))
	}
}

// firewallChanges reads each staged device's rulebase diff and keeps the rows the change set alters, as JSON
// keyed by device.
func firewallChanges(ctx context.Context, client *forward.Client, networkID, changeSetID string, staged []stagedFirewallRule) (string, error) {
	changes := map[string][]forward.SecurityRuleDiffEntry{}
	for _, rule := range staged {
		if _, seen := changes[rule.device]; seen {
			continue
		}
		diff, _, err := client.Predict.SecurityRulesDiff(ctx, networkID, changeSetID, rule.device)
		if err != nil {
			return "", fmt.Errorf("%s: %w", rule.device, err)
		}
		rows := []forward.SecurityRuleDiffEntry{}
		for _, entry := range diff.Entries {
			if entry.DiffType != "UNCHANGED" {
				rows = append(rows, entry)
			}
		}
		changes[rule.device] = rows
	}
	encoded, err := json.Marshal(changes)
	return string(encoded), err
}

// stagedFirewallRule is one firewall_rules entry translated into Forward's rule model.
type stagedFirewallRule struct {
	device string
	after  string
	rule   forward.NewSecurityRule
}

// ruleIDByName finds the id of the device's rule with that name in its editable rulebase, as the change
// set currently sees it.
func ruleIDByName(ctx context.Context, client *forward.Client, networkID, changeSetID, device, name string) (string, error) {
	diff, _, err := client.Predict.SecurityRulesDiff(ctx, networkID, changeSetID, device)
	if err != nil {
		return "", err
	}
	for _, entry := range diff.Entries {
		rule := entry.B
		if rule == nil {
			rule = entry.A
		}
		if rule != nil && rule.ScopeID == forward.FirewallScopeLocal && rule.Definition.Name == name {
			return rule.Definition.ID, nil
		}
	}
	return "", fmt.Errorf("no rule named %q in the device's local rulebase", name)
}

func firewallRules(ctx context.Context, models []FirewallRuleModel) ([]stagedFirewallRule, diag.Diagnostics) {
	var diags diag.Diagnostics
	out := make([]stagedFirewallRule, 0, len(models))
	strs := func(l types.List) []string {
		var v []string
		diags.Append(l.ElementsAs(ctx, &v, false)...)
		return v
	}
	predefined := func(names []string) []forward.SecurityObjectID {
		ids := make([]forward.SecurityObjectID, 0, len(names))
		for _, n := range names {
			ids = append(ids, forward.Predefined(n))
		}
		return ids
	}
	for _, m := range models {
		action := strings.ToUpper(m.Action.ValueString())
		if action != "PERMIT" && action != "DENY" {
			diags.AddError("Bad Firewall Rule", fmt.Sprintf("%s: action must be PERMIT or DENY, not %q", m.Name.ValueString(), m.Action.ValueString()))
		}
		out = append(out, stagedFirewallRule{
			device: m.Device.ValueString(),
			after:  m.After.ValueString(),
			rule: forward.NewSecurityRule{
				Predecessor: m.Predecessor.ValueString(),
				Definition: forward.SecurityRuleDefinition{
					Name:                 m.Name.ValueString(),
					Description:          m.Description.ValueString(),
					Enabled:              true,
					Action:               action,
					SourceZones:          strs(m.SourceZones),
					DestinationZones:     strs(m.DestinationZones),
					SourceAddresses:      forward.AddressSpecification{Subnets: strs(m.SourceSubnets)},
					DestinationAddresses: forward.AddressSpecification{Subnets: strs(m.DestinationSubnets)},
					SourceUsers:          forward.ObjectSpecification{Objects: predefined([]string{"any"})},
					Applications:         forward.ObjectSpecification{Objects: predefined(strs(m.Applications))},
					Services:             forward.ServiceSpecification{UseApplicationDefaults: true},
				},
			},
		})
	}
	return out, diags
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
