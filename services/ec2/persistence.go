package ec2

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/logger"
	"github.com/blackbirdworks/gopherstack/pkgs/persistence"
)

// ec2SnapshotVersion identifies the shape of backendSnapshot's Tables blob
// (i.e. the set/shape of resources registered on b.registry -- see
// registerAllTables in store_setup.go). It must be bumped whenever a change
// there would make an older snapshot unsafe to decode as the current shape.
// Restore compares this against the persisted value and discards (rather
// than attempts to partially decode) any mismatch -- see Restore below. This
// mirrors the services/sqs pilot (commit 0f09d77c).
const ec2SnapshotVersion = 2

// snapTGWRTProp is a type alias used in backendSnapshot to keep line lengths manageable.
type snapTGWRTProp = TransitGatewayRouteTablePropagation

type backendSnapshot struct {
	Tables                         map[string]json.RawMessage                  `json:"tables"`
	SnapshotAttributes             map[string]map[string]string                `json:"snapshotAttributes"`
	ImageDeprecated                map[string]string                           `json:"imageDeprecated"`
	Tags                           map[string]map[string]string                `json:"tags,omitempty"`
	AddressTransfers               map[string]*AddressTransfer                 `json:"addressTransfers"`
	IpamPoolCidrs                  map[string][]*IpamPoolCidr                  `json:"ipamPoolCidrs,omitempty"`
	IpamPrefixListResolverVersions map[string][]int64                          `json:"ipamPLRVersions,omitempty"`
	VpcCidrAssociations            map[string]*VpcCidrBlockAssociation         `json:"vpcCidrAssociations"`
	SpotFleetHistory               map[string][]SpotFleetHistoryRecord         `json:"spotFleetHistory"`
	FleetHistory                   map[string][]FleetHistoryRecord             `json:"fleetHistory,omitempty"`
	SnapshotTiers                  map[string]string                           `json:"snapshotTiers,omitempty"`
	VpcPeeringOptions              map[string]*PeeringConnectionOptions        `json:"vpcPeeringOptions"`
	SubnetCIDRAssociations         map[string][]*SubnetCIDRAssociation         `json:"subnetCIDRAssociations"`
	InstanceCreditSpecs            map[string]string                           `json:"instanceCreditSpecs"`
	InstanceMetadataDefaults       *InstanceMetadataDefaults                   `json:"instanceMetadataDefaults"`
	InstanceEventNotifAttrs        *InstanceEventNotificationAttributes        `json:"instanceEventNotifAttrs"`
	NiIPv6Addresses                map[string][]string                         `json:"niIPv6Addresses,omitempty"`
	IDFormatSettings               map[string]bool                             `json:"idFormatSettings"`
	VpcEndpointServicePermissions  map[string][]string                         `json:"vpcEpSvcPerms"`
	SubnetCIDRReservations         map[string][]*SubnetCIDRReservation         `json:"subnetCIDRReservations"`
	ImageDisabled                  map[string]bool                             `json:"imageDisabled,omitempty"`
	SgVpcAssociations              map[string]map[string]string                `json:"sgVpcAssociations"`
	ImageDeregistrationProtection  map[string]bool                             `json:"imageDeregProtect"`
	ImageAttributes                map[string]map[string]string                `json:"imageAttributes"`
	VgwRoutePropagation            map[string]bool                             `json:"vgwRoutePropagation"`
	TgwRTPropagations              map[string]map[string]*snapTGWRTProp        `json:"tgwRTPropagations,omitempty"`
	FastLaunchImages               map[string]*FastLaunchImageItem             `json:"fastLaunchImages"`
	FastSnapshotRestores           map[string]bool                             `json:"fastSnapshotRestores"`
	SpotDatafeed                   *SpotDatafeed                               `json:"spotDatafeed,omitempty"`
	VpcTenancy                     map[string]string                           `json:"vpcTenancy,omitempty"`
	VpcBlockPublicAccessOptions    *VpcBlockPublicAccessOptions                `json:"vpcBpaOptions,omitempty"`
	CapacityManagerState           *CapacityManagerState                       `json:"cmState,omitempty"`
	IpamPolicyEnabledTargets       map[string]string                           `json:"ipamPolicyEnabled,omitempty"`
	VerifiedAccessEndpointPolicies map[string]*VerifiedAccessPolicy            `json:"vaEndpointPolicies,omitempty"`
	VerifiedAccessGroupPolicies    map[string]*VerifiedAccessPolicy            `json:"vaGroupPolicies,omitempty"`
	ScheduledInstanceLaunched      map[string]int32                            `json:"schedInstLaunched,omitempty"`
	AllowedImagesSettings          *AllowedImagesSettings                      `json:"allowedImagesSettings,omitempty"`
	UsageReportEntries             map[string][]*UsageReportEntry              `json:"usageReportEntries,omitempty"`
	InstanceProductCodes           map[string][]string                         `json:"instanceProductCodes,omitempty"`
	EnclaveCertIamRoles            map[string][]*EnclaveCertIamRoleAssociation `json:"enclaveCertIamRoles,omitempty"`
	AvailabilityZoneGroupOptIns    map[string]string                           `json:"azGroupOptIns,omitempty"`
	SQLHaHistory                   map[string][]*RegisteredSQLHaInstance       `json:"sqlHaHistory,omitempty"`
	ImageWatermarks                map[string][]string                         `json:"imageWatermarks,omitempty"`
	AccountVpcEncryptionControl    *AccountVpcEncryptionControl                `json:"acctVpcEncCtrl,omitempty"`
	IpamOrgAdminAccountID          string                                      `json:"ipamOrgAdminAcct,omitempty"`
	Region                         string                                      `json:"region,omitempty"`
	AccountID                      string                                      `json:"accountID,omitempty"`
	ManagedResourceVisibility      string                                      `json:"mgdResourceVisibility,omitempty"`
	FreePrivateIPs                 []string                                    `json:"freePrivateIPs"`
	Version                        int                                         `json:"version"`
	NextPrivateIPIndex             int                                         `json:"nextPrivateIPIndex"`
	NextElasticIPIndex             int                                         `json:"nextElasticIPIndex"`
	EbsEncryptionByDefault         bool                                        `json:"ebsEncryptionByDefault"`
	SerialConsoleAccess            bool                                        `json:"serialConsoleAccess"`

	// ReachabilityAnalyzerOrgSharing is kept in its own gofmt alignment group:
	// its long name would otherwise widen the tag column for the block above.
	ReachabilityAnalyzerOrgSharing bool `json:"reachabilityAnalyzerOrgSharing,omitempty"`
}

// Snapshot serialises the backend state to JSON.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Snapshot(ctx context.Context) []byte {
	b.mu.RLock("Snapshot")
	defer b.mu.RUnlock()

	tables, err := b.registry.SnapshotAll()
	if err != nil {
		// The registered tables are plain JSON-friendly structs, so a marshal
		// failure here would indicate a programming error rather than bad
		// input data. Log and skip the snapshot rather than panic, matching
		// the persistence.Persistable contract (nil is skipped by the Manager).
		logger.Load(ctx).WarnContext(ctx, "ec2: snapshot table marshal failed", "error", err)

		return nil
	}

	snap := backendSnapshot{
		Version:                        ec2SnapshotVersion,
		Tables:                         tables,
		Tags:                           b.tags,
		AddressTransfers:               b.addressTransfers,
		IpamPoolCidrs:                  b.ipamPoolCidrs,
		IpamPrefixListResolverVersions: b.ipamPrefixListResolverVersions,
		EbsEncryptionByDefault:         b.ebsEncryptionByDefault,
		SerialConsoleAccess:            b.serialConsoleAccess,
		FreePrivateIPs:                 b.freePrivateIPs,
		AccountID:                      b.AccountID,
		Region:                         b.Region,
		NextPrivateIPIndex:             b.nextPrivateIPIndex,
		NextElasticIPIndex:             b.nextElasticIPIndex,
		VpcCidrAssociations:            b.vpcCidrAssociations,
		SpotFleetHistory:               b.spotFleetHistory,
		FleetHistory:                   b.fleetHistory,
		SnapshotTiers:                  b.snapshotTiers,
		SnapshotAttributes:             b.snapshotAttributes,
		SgVpcAssociations:              b.sgVpcAssociations,
		VpcTenancy:                     b.vpcTenancy,
		VpcPeeringOptions:              b.vpcPeeringOptions,
		SubnetCIDRAssociations:         b.subnetCIDRAssociations,
		InstanceCreditSpecs:            b.instanceCreditSpecs,
		InstanceMetadataDefaults:       b.instanceMetadataDefaults,
		InstanceEventNotifAttrs:        b.instanceEventNotifAttrs,
		NiIPv6Addresses:                b.niIPv6Addresses,
		IDFormatSettings:               b.idFormatSettings,
		VpcEndpointServicePermissions:  b.vpcEndpointServicePermissions,
		SubnetCIDRReservations:         b.subnetCIDRReservations,
		ImageDisabled:                  b.imageDisabled,
		ImageDeprecated:                b.imageDeprecated,
		ImageDeregistrationProtection:  b.imageDeregistrationProtection,
		ImageAttributes:                b.imageAttributes,
		VgwRoutePropagation:            b.vgwRoutePropagation,
		FastLaunchImages:               b.fastLaunchImages,
		FastSnapshotRestores:           b.fastSnapshotRestores,
		SpotDatafeed:                   b.spotDatafeed,
		VpcBlockPublicAccessOptions:    b.vpcBlockPublicAccessOptions,
		CapacityManagerState:           b.capacityManagerState,
		IpamPolicyEnabledTargets:       b.ipamPolicyEnabledTargets,
		IpamOrgAdminAccountID:          b.ipamOrgAdminAccountID,
		VerifiedAccessEndpointPolicies: b.verifiedAccessEndpointPolicies,
		VerifiedAccessGroupPolicies:    b.verifiedAccessGroupPolicies,
		ScheduledInstanceLaunched:      b.scheduledInstanceLaunched,
		AllowedImagesSettings:          b.allowedImagesSettings,
		UsageReportEntries:             b.usageReportEntries,
		InstanceProductCodes:           b.instanceProductCodes,
		EnclaveCertIamRoles:            b.enclaveCertIamRoles,
		AvailabilityZoneGroupOptIns:    b.availabilityZoneGroupOptIns,
		SQLHaHistory:                   b.sqlHaHistory,
		TgwRTPropagations:              b.tgwRTPropagations,
		ReachabilityAnalyzerOrgSharing: b.reachabilityAnalyzerOrgSharing,

		ImageWatermarks:             b.imageWatermarks,
		AccountVpcEncryptionControl: b.accountVpcEncryptionControl,
		ManagedResourceVisibility:   b.managedResourceDefaultVisibility,
	}

	data, err := json.Marshal(snap)
	if err != nil {
		logger.Load(ctx).WarnContext(ctx, "ec2: Snapshot marshal failure", "error", err)

		return nil
	}

	return data
}

// Restore loads backend state from a JSON snapshot.
// It implements persistence.Persistable.
func (b *InMemoryBackend) Restore(ctx context.Context, data []byte) error {
	var snap backendSnapshot

	if err := persistence.UnmarshalSnapshot(ctx, "ec2", data, &snap); err != nil {
		return err
	}

	snap.initMissingMaps()

	b.mu.Lock("Restore")
	defer b.mu.Unlock()

	if snap.Version != ec2SnapshotVersion {
		// An incompatible (older/newer/absent) snapshot version must never be
		// partially decoded as the current shape -- that risks silently
		// misinterpreting fields. Discard cleanly and start empty instead of
		// erroring, since this is an expected, recoverable condition (e.g.
		// upgrading gopherstack across a snapshot-format change), not data
		// corruption. Mirrors the services/sqs pilot (commit 0f09d77c).
		logger.Load(ctx).WarnContext(ctx,
			"ec2: discarding incompatible snapshot version, starting empty",
			"gotVersion", snap.Version, "wantVersion", ec2SnapshotVersion)

		b.registry.ResetAll()

		return nil
	}

	if err := b.registry.RestoreAll(snap.Tables); err != nil {
		return fmt.Errorf("ec2: restore snapshot tables: %w", err)
	}

	b.restoreCoreFields(&snap)
	b.restoreExtendedFields(&snap)
	b.rebuildSecondaryIndexesLocked()
	b.restoreMiscMapFields(&snap)

	b.instanceMetadataDefaults = snap.InstanceMetadataDefaults
	b.instanceEventNotifAttrs = snap.InstanceEventNotifAttrs
	b.spotDatafeed = snap.SpotDatafeed
	b.restoreVpcConfigFields(&snap)
	b.restoreCapacityFamilyFields(&snap)
	b.restoreNewFamilyFields(&snap)
	b.restoreParityFinalFields(&snap)
	b.restoreParity4Fields(&snap)

	return nil
}

// restoreMapField sets *dst to src if non-nil, otherwise initializes *dst to
// an empty map. Collapses the repetitive "if snap.X != nil { b.x = snap.X }
// else { b.x = make(...) }" pattern used throughout restoreMiscMapFields into
// a single call per field, keeping cyclomatic complexity down.
func restoreMapField[K comparable, V any](dst *map[K]V, src map[K]V) {
	if src != nil {
		*dst = src
	} else {
		*dst = make(map[K]V)
	}
}

// restoreMiscMapFields restores the grab-bag of standalone (non-store.Table,
// non-op-family-grouped) maps from snap into b. Must be called with b.mu held
// for writing.
func (b *InMemoryBackend) restoreMiscMapFields(snap *backendSnapshot) {
	restoreMapField(&b.vpcCidrAssociations, snap.VpcCidrAssociations)
	restoreMapField(&b.spotFleetHistory, snap.SpotFleetHistory)
	restoreMapField(&b.fleetHistory, snap.FleetHistory)
	restoreMapField(&b.snapshotTiers, snap.SnapshotTiers)
	restoreMapField(&b.snapshotAttributes, snap.SnapshotAttributes)
	restoreMapField(&b.sgVpcAssociations, snap.SgVpcAssociations)
	restoreMapField(&b.vpcTenancy, snap.VpcTenancy)
	restoreMapField(&b.vpcPeeringOptions, snap.VpcPeeringOptions)
	restoreMapField(&b.subnetCIDRAssociations, snap.SubnetCIDRAssociations)
	restoreMapField(&b.instanceCreditSpecs, snap.InstanceCreditSpecs)
	restoreMapField(&b.niIPv6Addresses, snap.NiIPv6Addresses)
	restoreMapField(&b.idFormatSettings, snap.IDFormatSettings)
	restoreMapField(&b.vpcEndpointServicePermissions, snap.VpcEndpointServicePermissions)
	restoreMapField(&b.subnetCIDRReservations, snap.SubnetCIDRReservations)
	restoreMapField(&b.imageDisabled, snap.ImageDisabled)
	restoreMapField(&b.imageDeprecated, snap.ImageDeprecated)
	restoreMapField(&b.imageDeregistrationProtection, snap.ImageDeregistrationProtection)
	restoreMapField(&b.imageAttributes, snap.ImageAttributes)
	restoreMapField(&b.vgwRoutePropagation, snap.VgwRoutePropagation)
	restoreMapField(&b.verifiedAccessEndpointPolicies, snap.VerifiedAccessEndpointPolicies)
	restoreMapField(&b.verifiedAccessGroupPolicies, snap.VerifiedAccessGroupPolicies)
	restoreMapField(&b.fastLaunchImages, snap.FastLaunchImages)
	restoreMapField(&b.fastSnapshotRestores, snap.FastSnapshotRestores)
}

// restoreParityFinalFields copies the final EC2 parity sweep
// (gopherstack-5o9) state — TGW route table propagations, interruptible
// Capacity Reservation allocations, moving-address status, and the
// Reachability Analyzer organization-sharing flag — from snap into b. Must be
// called with b.mu held for writing.
func (b *InMemoryBackend) restoreParityFinalFields(snap *backendSnapshot) {
	if snap.TgwRTPropagations != nil {
		b.tgwRTPropagations = snap.TgwRTPropagations
	} else {
		b.tgwRTPropagations = make(map[string]map[string]*TransitGatewayRouteTablePropagation)
	}

	b.reachabilityAnalyzerOrgSharing = snap.ReachabilityAnalyzerOrgSharing
}

// restoreParity4Fields copies the image watermark / account-level VPC
// Encryption Control / managed resource visibility state added for the
// parity-4 SDK-bump pass from snap into b. Must be called with b.mu held for
// writing.
func (b *InMemoryBackend) restoreParity4Fields(snap *backendSnapshot) {
	if snap.ImageWatermarks != nil {
		b.imageWatermarks = snap.ImageWatermarks
	} else {
		b.imageWatermarks = make(map[string][]string)
	}

	if snap.AccountVpcEncryptionControl != nil {
		b.accountVpcEncryptionControl = snap.AccountVpcEncryptionControl
	} else {
		b.accountVpcEncryptionControl = &AccountVpcEncryptionControl{
			Mode:      accountVpcEncryptionControlModeUnmanaged,
			State:     accountVpcEncryptionControlStateDefault,
			ManagedBy: accountVpcEncryptionControlManagedByAccount,
		}
	}

	if snap.ManagedResourceVisibility != "" {
		b.managedResourceDefaultVisibility = snap.ManagedResourceVisibility
	} else {
		b.managedResourceDefaultVisibility = managedResourceVisibilityHidden
	}
}

// restoreNewFamilyFields copies the Mac modification task / Secondary
// Network / Secondary Subnet / Secondary Interface / Outpost LAG / Service
// Link Virtual Interface / instance-attribute misc / SQL HA fields from snap
// into b. Must be called with b.mu held for writing.
func (b *InMemoryBackend) restoreNewFamilyFields(snap *backendSnapshot) {
	b.availabilityZoneGroupOptIns = snap.AvailabilityZoneGroupOptIns
	b.sqlHaHistory = snap.SQLHaHistory
}

// restoreCapacityFamilyFields copies the Capacity Reservation Fleet, Capacity
// Block, and Capacity Manager state from snap into b. Split out to keep
// Restore's growth in check. Must be called with b.mu held.
func (b *InMemoryBackend) restoreCapacityFamilyFields(snap *backendSnapshot) {
	if snap.CapacityManagerState != nil {
		b.capacityManagerState = snap.CapacityManagerState
	} else {
		b.capacityManagerState = &CapacityManagerState{Status: capacityManagerStatusDisabled}
	}
}

// restoreVpcConfigFields copies the VPC ClassicLink and Block Public Access
// state from snap into b. Split out to keep Restore's growth in check.
// Must be called with b.mu held.
func (b *InMemoryBackend) restoreVpcConfigFields(snap *backendSnapshot) {
	if snap.VpcBlockPublicAccessOptions != nil {
		b.vpcBlockPublicAccessOptions = snap.VpcBlockPublicAccessOptions
	} else {
		b.vpcBlockPublicAccessOptions = &VpcBlockPublicAccessOptions{
			InternetGatewayBlockMode: vpcBPABlockModeOff,
			State:                    vpcBPAStateDefault,
			ExclusionsAllowed:        vpcBPAExclusionsAllowed,
			ManagedBy:                vpcBPAManagedByAccount,
		}
	}
}

// restoreCoreFields copies the core map/bool/scalar fields from snap into b.
// Must be called with b.mu held for writing.
func (b *InMemoryBackend) restoreCoreFields(snap *backendSnapshot) {
	b.tags = snap.Tags
}

// restoreExtendedFields copies extended/appendix fields from snap into b.
// Must be called with b.mu held for writing.
func (b *InMemoryBackend) restoreExtendedFields(snap *backendSnapshot) {
	b.addressTransfers = snap.AddressTransfers
	b.ipamPoolCidrs = snap.IpamPoolCidrs
	b.ipamPrefixListResolverVersions = snap.IpamPrefixListResolverVersions
	b.ipamPolicyEnabledTargets = snap.IpamPolicyEnabledTargets
	b.ipamOrgAdminAccountID = snap.IpamOrgAdminAccountID
	b.ebsEncryptionByDefault = snap.EbsEncryptionByDefault
	b.serialConsoleAccess = snap.SerialConsoleAccess
	b.freePrivateIPs = snap.FreePrivateIPs
	b.AccountID = snap.AccountID
	b.Region = snap.Region
	b.nextPrivateIPIndex = snap.NextPrivateIPIndex
	b.nextElasticIPIndex = snap.NextElasticIPIndex
	b.restoreImageAndPoolFields(snap)
}

// restoreImageAndPoolFields copies the Scheduled Instances / COIP / public IPv4-IPv6 pool /
// Allowed Images Settings / image task / usage report fields from snap into b. Must be
// called with b.mu held for writing.
func (b *InMemoryBackend) restoreImageAndPoolFields(snap *backendSnapshot) {
	b.scheduledInstanceLaunched = snap.ScheduledInstanceLaunched
	b.allowedImagesSettings = snap.AllowedImagesSettings
	b.usageReportEntries = snap.UsageReportEntries
	b.instanceProductCodes = snap.InstanceProductCodes
	b.enclaveCertIamRoles = snap.EnclaveCertIamRoles
}

// initMissingMaps ensures all map fields in the snapshot are non-nil.
// This prevents nil-map panics when the snapshot was created from a backend
// that never populated a particular resource type.
func (s *backendSnapshot) initMissingMaps() {
	s.initCoreMaps()
	s.initNewOpsMaps()
}

// initCoreMaps initialises the original map fields.
func (s *backendSnapshot) initCoreMaps() {
	if s.Tags == nil {
		s.Tags = make(map[string]map[string]string)
	}
}

// initMapIfNil initialises m to an empty map if it is nil.
func initMapIfNil[K comparable, V any](m *map[K]V) {
	if *m == nil {
		*m = make(map[K]V)
	}
}

// initNewOpsMaps initialises the map fields added for the new Accept/Advertise/Allocate operations.
func (s *backendSnapshot) initNewOpsMaps() {
	if s.AddressTransfers == nil {
		s.AddressTransfers = make(map[string]*AddressTransfer)
	}

	s.initAppendixMaps()
}

func (s *backendSnapshot) initAppendixMaps() {
	initMapIfNil(&s.IpamPoolCidrs)
	initMapIfNil(&s.IpamPrefixListResolverVersions)
	initMapIfNil(&s.IpamPolicyEnabledTargets)
	s.initImageAndPoolMaps()
}

// initImageAndPoolMaps initialises the map fields added for Scheduled Instances, COIP,
// public IPv4/IPv6 pools, Allowed Images Settings, and image tasks/usage reports.
func (s *backendSnapshot) initImageAndPoolMaps() {
	initMapIfNil(&s.ScheduledInstanceLaunched)
	initMapIfNil(&s.UsageReportEntries)
	initMapIfNil(&s.InstanceProductCodes)
	initMapIfNil(&s.EnclaveCertIamRoles)
	initMapIfNil(&s.AvailabilityZoneGroupOptIns)
	initMapIfNil(&s.SQLHaHistory)

	if s.AllowedImagesSettings == nil {
		s.AllowedImagesSettings = &AllowedImagesSettings{
			State:     allowedImagesStateDisabled,
			ManagedBy: allowedImagesManagedByAccnt,
		}
	}
}

// Snapshot implements persistence.Persistable by delegating to the backend.
// It type-asserts the backend to check for Snapshot support so that alternative
// backend implementations that do not persist state still compile.
func (h *Handler) Snapshot(ctx context.Context) []byte {
	type snapshotter interface {
		Snapshot(ctx context.Context) []byte
	}
	if s, ok := h.Backend.(snapshotter); ok {
		return s.Snapshot(ctx)
	}

	return nil
}

// Restore implements persistence.Persistable by delegating to the backend.
// It type-asserts the backend to check for Restore support so that alternative
// backend implementations that do not persist state still compile.
func (h *Handler) Restore(ctx context.Context, data []byte) error {
	type restorer interface {
		Restore(context.Context, []byte) error
	}
	if r, ok := h.Backend.(restorer); ok {
		return r.Restore(ctx, data)
	}

	return nil
}
