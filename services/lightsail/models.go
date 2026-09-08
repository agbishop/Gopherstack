// Package lightsail emulates the Amazon Lightsail control-plane API
// (aws-sdk-go-v2/service/lightsail, protocol awsjson1.1 -- a single POST /
// dispatched by X-Amz-Target: Lightsail_20161128.<Op>, not REST). See
// PARITY.md for the full pre-implementation audit this package was built
// from.
//
// Every Lightsail resource kind is modeled INDEPENDENTLY of this repo's real
// services/ec2, services/rds, services/elb, services/elbv2 state (see
// PARITY.md section 5.2): Lightsail's own wire shapes carry no VPC/subnet/
// security-group/AMI concepts, and the real product hides its backing
// infrastructure entirely, so an honest emulator only needs to reproduce
// what is OBSERVABLE through these 161 ops.
package lightsail

import (
	"maps"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// ResourceLocation mirrors types.ResourceLocation.
type ResourceLocation struct {
	AvailabilityZone string
	RegionName       string
}

// Operation mirrors types.Operation -- see PARITY.md section 2: nearly
// every mutating op returns one or more of these, and callers poll
// GetOperation/GetOperations/GetOperationsForResource (family Z) to observe
// a real NotStarted/Started -> Succeeded/Failed transition, never a
// synchronously-fabricated terminal status.
type Operation struct {
	CreatedAt       time.Time
	StatusChangedAt time.Time
	Location        ResourceLocation
	ID              string
	Type            string
	Status          string
	ResourceName    string
	ResourceType    string
	Details         string
	ErrorCode       string
	ErrorDetails    string
	IsTerminal      bool
}

func (o *Operation) clone() *Operation {
	cp := *o

	return &cp
}

// AddOn mirrors types.AddOn -- a live add-on attached to an Instance or
// Disk (AddOnType has exactly 2 values, PARITY.md family F).
type AddOn struct {
	Name                  string
	Status                string
	SnapshotTimeOfDay     string
	NextSnapshotTimeOfDay string
	Duration              string
	Threshold             string
}

// AutoSnapshotDetails mirrors types.AutoSnapshotDetails -- one dated entry
// in an instance/disk's AutoSnapshot add-on history (GetAutoSnapshots).
type AutoSnapshotDetails struct {
	Date              string
	Status            string
	CreatedAt         time.Time
	FromAttachedDisks []AttachedDisk
}

// AttachedDisk mirrors types.AttachedDisk.
type AttachedDisk struct {
	Path     string
	SizeInGb int32
}

// AddOnRequest mirrors types.AddOnRequest -- an input-only shape (never
// stored directly; EnableAddOn/CreateInstances/CreateDisk convert it into a
// live AddOn via addons.go's applyAddOnRequestLocked).
type AddOnRequest struct {
	Type                        string
	AutoSnapshotTimeOfDay       string
	StopInstanceOnIdleDuration  string
	StopInstanceOnIdleThreshold string
}

func cloneAddOns(in []AddOn) []AddOn {
	if in == nil {
		return nil
	}

	return append([]AddOn(nil), in...)
}

func cloneStrings(in []string) []string {
	if in == nil {
		return nil
	}

	return append([]string(nil), in...)
}

func cloneAutoSnapshots(in []AutoSnapshotDetails) []AutoSnapshotDetails {
	if in == nil {
		return nil
	}

	out := make([]AutoSnapshotDetails, len(in))
	for i, s := range in {
		s.FromAttachedDisks = append([]AttachedDisk(nil), s.FromAttachedDisks...)
		out[i] = s
	}

	return out
}

// InstancePortInfo mirrors types.InstancePortInfo -- one configured
// firewall rule (PutInstancePublicPorts/Open.../Close...). PortState's own
// SDK doc comment says "The port state for Lightsail instances is always
// open" (PARITY.md 4.1), so this backend never models a closed state for a
// configured rule -- GetInstancePortStates hardcodes PortStateOpen for
// every entry here rather than a real closed sub-state.
type InstancePortInfo struct {
	Protocol        string
	AccessFrom      string
	AccessType      string
	CommonName      string
	AccessDirection string
	Cidrs           []string
	Ipv6Cidrs       []string
	CidrListAliases []string
	FromPort        int32
	ToPort          int32
}

func clonePortInfos(in []InstancePortInfo) []InstancePortInfo {
	if in == nil {
		return nil
	}

	out := make([]InstancePortInfo, len(in))
	for i, p := range in {
		p.Cidrs = cloneStrings(p.Cidrs)
		p.Ipv6Cidrs = cloneStrings(p.Ipv6Cidrs)
		p.CidrListAliases = cloneStrings(p.CidrListAliases)
		out[i] = p
	}

	return out
}

// HostKeyAttributes mirrors types.HostKeyAttributes -- SSH/RDP host key
// fingerprints reported by GetInstanceAccessDetails.
type HostKeyAttributes struct {
	NotValidBefore    time.Time
	NotValidAfter     time.Time
	WitnessedAt       time.Time
	Algorithm         string
	FingerprintSHA1   string
	FingerprintSHA256 string
	PublicKey         string
}

// InstanceMetadataOptions mirrors types.InstanceMetadataOptions (IMDS
// knobs, UpdateInstanceMetadataOptions).
type InstanceMetadataOptions struct {
	State                   string
	HTTPTokens              string
	HTTPEndpoint            string
	HTTPProtocolIpv6        string
	HTTPPutResponseHopLimit int32
}

// Instance mirrors types.Instance. StateCode/StateName back the free-form
// InstanceState{Code, Name} the SDK exposes -- there is no typed SDK enum
// (PARITY.md 4.1); the numeric codes below are the conventional EC2-style
// mapping (0=pending,16=running,32=shutting-down,48=terminated,64=stopping,
// 80=stopped), an UNCONFIRMED, non-SDK-sourced assumption ported from EC2's
// own real convention, not a Lightsail-documented fact.
type Instance struct {
	CreatedAt        time.Time
	Tags             *tags.Tags
	Location         ResourceLocation
	BlueprintName    string
	SSHKeyName       string
	BundleID         string
	PrivateIPAddress string
	PublicIPAddress  string
	IPAddressType    string
	Username         string
	windowsPassword  string
	StateName        string
	BlueprintID      string
	SupportCode      string
	Name             string
	Arn              string
	MetadataOptions  InstanceMetadataOptions
	IPv6Addresses    []string
	HostKeys         []HostKeyAttributes
	AutoSnapshots    []AutoSnapshotDetails
	Ports            []InstancePortInfo
	AddOns           []AddOn
	CPUCount         int32
	RAMSizeInGb      float32
	DiskSizeInGb     int32
	MonthlyTransfer  int32
	StateCode        int32
	// PublicIPGeneration counts stopped-to-running transitions that assigned
	// a fresh dynamic public IP; it seeds publicIPForName's hash so restarts
	// change the address deterministically instead of drawing on time.Now
	// or crypto/rand (gopherstack-i2s6).
	PublicIPGeneration   int32
	IsStaticIP           bool
	KnownHostKeysDeleted bool
}

func (i *Instance) clone() *Instance {
	cp := *i
	cp.IPv6Addresses = cloneStrings(i.IPv6Addresses)
	cp.AddOns = cloneAddOns(i.AddOns)
	cp.Ports = clonePortInfos(i.Ports)
	cp.AutoSnapshots = cloneAutoSnapshots(i.AutoSnapshots)
	cp.HostKeys = append([]HostKeyAttributes(nil), i.HostKeys...)

	return &cp
}

// Disk mirrors types.Disk. DiskState IS a real typed SDK enum (pending/
// error/available/in-use/unknown, PARITY.md 4.2) -- unlike InstanceState,
// this one genuinely drives an available<->in-use transition on
// AttachDisk/DetachDisk.
type Disk struct {
	CreatedAt       time.Time
	Tags            *tags.Tags
	Location        ResourceLocation
	AttachedTo      string
	Arn             string
	AttachmentState string
	AutoMountStatus string
	Path            string
	State           string
	SupportCode     string
	Name            string
	AddOns          []AddOn
	AutoSnapshots   []AutoSnapshotDetails
	SizeInGb        int32
	Iops            int32
	GbInUse         int32
	IsSystemDisk    bool
	IsAttached      bool
}

func (d *Disk) clone() *Disk {
	cp := *d
	cp.AddOns = cloneAddOns(d.AddOns)
	cp.AutoSnapshots = cloneAutoSnapshots(d.AutoSnapshots)

	return &cp
}

// StaticIP mirrors types.StaticIp -- a simplified Elastic-IP analogue
// scoped only to attach/detach against a named Instance (PARITY.md 4.1).
type StaticIP struct {
	Name        string
	Arn         string
	SupportCode string
	IPAddress   string
	AttachedTo  string
	CreatedAt   time.Time
	Location    ResourceLocation
	IsAttached  bool
}

func (s *StaticIP) clone() *StaticIP {
	cp := *s

	return &cp
}

// KeyPair mirrors types.KeyPair -- public metadata only. Private key
// material (PrivateKeyBase64/PublicKeyBase64) is returned exactly once by
// CreateKeyPair/ImportKeyPair/DownloadDefaultKeyPair and never stored here
// (PARITY.md 4.1).
type KeyPair struct {
	CreatedAt   time.Time
	Tags        *tags.Tags
	Location    ResourceLocation
	Name        string
	Arn         string
	SupportCode string
	Fingerprint string
}

func (k *KeyPair) clone() *KeyPair {
	cp := *k

	return &cp
}

// InstanceSnapshot mirrors types.InstanceSnapshot.
type InstanceSnapshot struct {
	CreatedAt          time.Time
	Tags               *tags.Tags
	Location           ResourceLocation
	State              string
	Progress           string
	FromInstanceName   string
	FromInstanceArn    string
	FromBlueprintID    string
	FromBundleID       string
	Name               string
	SupportCode        string
	Arn                string
	FromAttachedDisks  []AttachedDisk
	SizeInGb           int32
	IsFromAutoSnapshot bool
}

func (s *InstanceSnapshot) clone() *InstanceSnapshot {
	cp := *s
	cp.FromAttachedDisks = append([]AttachedDisk(nil), s.FromAttachedDisks...)

	return &cp
}

// DiskSnapshot mirrors types.DiskSnapshot. DiskSnapshotState has 4 values
// (pending/completed/error/unknown, PARITY.md 4.2).
type DiskSnapshot struct {
	CreatedAt          time.Time
	Tags               *tags.Tags
	Location           ResourceLocation
	State              string
	Progress           string
	FromDiskName       string
	FromDiskArn        string
	FromInstanceName   string
	FromInstanceArn    string
	Name               string
	SupportCode        string
	Arn                string
	SizeInGb           int32
	IsFromAutoSnapshot bool
}

func (s *DiskSnapshot) clone() *DiskSnapshot {
	cp := *s

	return &cp
}

// ExportSnapshotRecord mirrors types.ExportSnapshotRecord -- created by
// ExportSnapshot (family K), the real Lightsail-to-EC2 migration path.
type ExportSnapshotRecord struct {
	Name               string
	Arn                string
	State              string
	SourceName         string
	SourceArn          string
	SourceResourceType string
	CreatedAt          time.Time
	Location           ResourceLocation
}

func (r *ExportSnapshotRecord) clone() *ExportSnapshotRecord {
	cp := *r

	return &cp
}

// CloudFormationStackRecord mirrors types.CloudFormationStackRecord --
// created by CreateCloudFormationStack, the one confirmed genuine
// cross-service handoff in this surface (PARITY.md 5.2 point 4): it hands
// off to a real services/cloudformation stack when wired (see
// exportcfn.go).
type CloudFormationStackRecord struct {
	Name              string
	Arn               string
	State             string
	DestinationInfoID string
	CreatedAt         time.Time
	Location          ResourceLocation
	SourceInstances   []string
}

func (r *CloudFormationStackRecord) clone() *CloudFormationStackRecord {
	cp := *r
	cp.SourceInstances = cloneStrings(r.SourceInstances)

	return &cp
}

// LoadBalancer mirrors types.LoadBalancer -- a simplified single-listener
// ALB analogue (LoadBalancerProtocol has exactly 2 values, PARITY.md family
// L). InstanceHealthSummary tracks one InstanceHealthState per attached
// instance.
type LoadBalancer struct {
	CreatedAt               time.Time
	Tags                    *tags.Tags
	InstanceHealth          map[string]instanceHealth
	Location                ResourceLocation
	State                   string
	Arn                     string
	IPAddressType           string
	TLSPolicyName           string
	HealthCheckPath         string
	Name                    string
	DNSName                 string
	Protocol                string
	SupportCode             string
	AttachedInstances       []string
	PublicPorts             []int32
	TLSCertificateNames     []string
	InstancePort            int32
	HTTPSRedirectionEnabled bool
}

type instanceHealth struct {
	State  string
	Reason string
}

func (l *LoadBalancer) clone() *LoadBalancer {
	cp := *l
	cp.AttachedInstances = cloneStrings(l.AttachedInstances)
	cp.TLSCertificateNames = cloneStrings(l.TLSCertificateNames)
	cp.PublicPorts = append([]int32(nil), l.PublicPorts...)
	cp.InstanceHealth = make(map[string]instanceHealth, len(l.InstanceHealth))
	maps.Copy(cp.InstanceHealth, l.InstanceHealth)

	return &cp
}

// LoadBalancerTLSCertificate mirrors types.LoadBalancerTlsCertificate.
type LoadBalancerTLSCertificate struct {
	CreatedAt               time.Time
	NotAfter                time.Time
	NotBefore               time.Time
	IssuedAt                time.Time
	Tags                    *tags.Tags
	Location                ResourceLocation
	Status                  string
	SerialNumber            string
	SupportCode             string
	Subject                 string
	Issuer                  string
	Name                    string
	DomainName              string
	LoadBalancerName        string
	Arn                     string
	SubjectAlternativeNames []string
	IsAttached              bool
}

func (c *LoadBalancerTLSCertificate) clone() *LoadBalancerTLSCertificate {
	cp := *c
	cp.SubjectAlternativeNames = cloneStrings(c.SubjectAlternativeNames)

	return &cp
}

// RelationalDatabase mirrors types.RelationalDatabase. State is a bare
// *string on the wire -- NO typed RelationalDatabaseState enum exists
// anywhere in this SDK module (PARITY.md 4.3); the values below (creating/
// available/modifying/backing-up/deleting/starting/stopping/stopped/
// rebooting) are an honest, non-SDK-sourced convention following general
// AWS RDS-family behavior, flagged as UNCONFIRMED for Lightsail
// specifically. RelationalDatabaseEngine's only SDK-known value is "mysql"
// (PARITY.md 4.3), but Engine is stored as a plain string here and this
// backend does not reject a caller-supplied non-mysql engine, since the SDK
// enum documents itself as open/expandable.
type RelationalDatabase struct {
	CreatedAt                  time.Time
	LatestRestorableTime       time.Time
	Tags                       *tags.Tags
	Parameters                 map[string]RelationalDatabaseParameter
	Location                   ResourceLocation
	BundleID                   string
	SecondaryAvailabilityZone  string
	MasterUsername             string
	MasterUserPassword         string
	PreviousMasterUserPassword string
	PreferredBackupWindow      string
	PreferredMaintenanceWindow string
	ParameterApplyStatus       string
	BlueprintID                string
	Name                       string
	CaCertificateIdentifier    string
	MasterDatabaseName         string
	EngineVersion              string
	Engine                     string
	State                      string
	Arn                        string
	SupportCode                string
	Events                     []RelationalDatabaseEvent
	RAMSizeInGb                float32
	DiskSizeInGb               int32
	CPUCount                   int32
	PubliclyAccessible         bool
	BackupRetentionEnabled     bool
}

// RelationalDatabaseParameter mirrors types.RelationalDatabaseParameter.
type RelationalDatabaseParameter struct {
	ParameterName  string
	ParameterValue string
	Description    string
	AllowedValues  string
	DataType       string
	ApplyType      string
	ApplyMethod    string
	IsModifiable   bool
}

// RelationalDatabaseEvent mirrors types.RelationalDatabaseEvent.
type RelationalDatabaseEvent struct {
	Message         string
	Resource        string
	CreatedAt       time.Time
	EventCategories []string
}

func (r *RelationalDatabase) clone() *RelationalDatabase {
	cp := *r
	cp.Parameters = make(map[string]RelationalDatabaseParameter, len(r.Parameters))
	maps.Copy(cp.Parameters, r.Parameters)

	cp.Events = make([]RelationalDatabaseEvent, len(r.Events))
	for i, e := range r.Events {
		e.EventCategories = cloneStrings(e.EventCategories)
		cp.Events[i] = e
	}

	return &cp
}

// RelationalDatabaseSnapshot mirrors types.RelationalDatabaseSnapshot.
type RelationalDatabaseSnapshot struct {
	CreatedAt                         time.Time
	Tags                              *tags.Tags
	Location                          ResourceLocation
	FromRelationalDatabaseName        string
	Engine                            string
	EngineVersion                     string
	Name                              string
	FromRelationalDatabaseArn         string
	FromRelationalDatabaseBlueprintID string
	FromRelationalDatabaseBundleID    string
	State                             string
	SupportCode                       string
	Arn                               string
	SizeInGb                          int32
}

func (s *RelationalDatabaseSnapshot) clone() *RelationalDatabaseSnapshot {
	cp := *s

	return &cp
}

// ContainerService mirrors types.ContainerService -- see PARITY.md 4.5:
// the most structurally complex family. This backend models the full
// ContainerServiceState/ContainerServiceStateDetailCode/
// ContainerServiceDeploymentState state-machine bookkeeping (see
// containers.go's doc comment for the explicit real-vs-bookkeeping-only
// decision) without actually running images via pkgs/container.
type ContainerService struct {
	CreatedAt          time.Time
	Tags               *tags.Tags
	NextImageVersion   map[string]int
	NextDeployment     *ContainerServiceDeployment
	CurrentDeployment  *ContainerServiceDeployment
	PublicDomainNames  map[string][]string
	Location           ResourceLocation
	StateDetailCode    string
	URL                string
	PrincipalArn       string
	PrivateDomainName  string
	StateDetailMessage string
	Name               string
	State              string
	PowerID            string
	Power              string
	Arn                string
	Images             []ContainerImage
	Scale              int32
	IsDisabled         bool
}

// ContainerServiceDeployment mirrors types.ContainerServiceDeployment.
type ContainerServiceDeployment struct {
	CreatedAt      time.Time
	Containers     map[string]ContainerDefinition
	PublicEndpoint *ContainerServiceEndpoint
	State          string
	Version        int32
}

// ContainerDefinition mirrors types.Container.
type ContainerDefinition struct {
	Environment map[string]string
	Ports       map[string]string
	Image       string
	Command     []string
}

// ContainerServiceEndpoint mirrors types.ContainerServiceEndpoint.
type ContainerServiceEndpoint struct {
	ContainerName string
	ContainerPort int32
}

// ContainerImage mirrors types.ContainerImage -- RegisterContainerImage
// versions per label (":container-service-1.mystaticsite.3" convention,
// PARITY.md 4.5).
type ContainerImage struct {
	CreatedAt time.Time
	Image     string
	Digest    string
}

func cloneContainerDeployment(d *ContainerServiceDeployment) *ContainerServiceDeployment {
	if d == nil {
		return nil
	}

	cp := *d
	cp.Containers = make(map[string]ContainerDefinition, len(d.Containers))

	for k, v := range d.Containers {
		v.Command = cloneStrings(v.Command)
		env := make(map[string]string, len(v.Environment))
		maps.Copy(env, v.Environment)
		v.Environment = env

		ports := make(map[string]string, len(v.Ports))
		maps.Copy(ports, v.Ports)
		v.Ports = ports

		cp.Containers[k] = v
	}

	if d.PublicEndpoint != nil {
		ep := *d.PublicEndpoint
		cp.PublicEndpoint = &ep
	}

	return &cp
}

func (c *ContainerService) clone() *ContainerService {
	cp := *c
	cp.PublicDomainNames = make(map[string][]string, len(c.PublicDomainNames))

	for k, v := range c.PublicDomainNames {
		cp.PublicDomainNames[k] = cloneStrings(v)
	}

	cp.CurrentDeployment = cloneContainerDeployment(c.CurrentDeployment)
	cp.NextDeployment = cloneContainerDeployment(c.NextDeployment)
	cp.Images = append([]ContainerImage(nil), c.Images...)
	cp.NextImageVersion = make(map[string]int, len(c.NextImageVersion))
	maps.Copy(cp.NextImageVersion, c.NextImageVersion)

	return &cp
}

// Bucket mirrors types.Bucket -- Lightsail's own object storage, distinct
// from and NOT backed by this repo's real services/s3 (PARITY.md 4.6).
// ResourceType is deliberately a plain string field (not a typed constant
// reused from elsewhere) since the SDK's own Bucket.ResourceType is
// genuinely a *string, not the typed ResourceType enum most other resources
// use -- a confirmed wire-shape asymmetry preserved faithfully.
type Bucket struct {
	CreatedAt              time.Time
	Tags                   *tags.Tags
	Location               ResourceLocation
	BundleID               string
	State                  string
	StateMessage           string
	ObjectVersioning       string
	URL                    string
	Name                   string
	SupportCode            string
	Arn                    string
	ReadonlyAccessAccounts []string
	AccessKeys             []AccessKey
	AbleToUpdateBundle     bool
}

// AccessKey mirrors types.AccessKey. SecretAccessKey is returned in full
// only on CreateBucketAccessKey and never again (PARITY.md 4.6, same
// write-once pattern as CreateKeyPair).
type AccessKey struct {
	CreatedAt       time.Time
	AccessKeyID     string
	SecretAccessKey string
	Status          string
}

func (b *Bucket) clone() *Bucket {
	cp := *b
	cp.ReadonlyAccessAccounts = cloneStrings(b.ReadonlyAccessAccounts)
	cp.AccessKeys = append([]AccessKey(nil), b.AccessKeys...)

	return &cp
}

// Distribution mirrors types.LightsailDistribution. Location.RegionName is
// ALWAYS "us-east-1" regardless of the backend's configured region -- a
// real, doc-comment-quoted AWS behavior (PARITY.md 4.7), not an assumption.
type Distribution struct {
	CreatedAt              time.Time
	Tags                   *tags.Tags
	Origin                 DistributionOrigin
	Location               ResourceLocation
	CacheBehaviorSettings  *CacheSettings
	IPAddressType          string
	DomainName             string
	OriginPublicDNS        string
	CertificateName        string
	Name                   string
	ViewerMinTLSVersion    string
	Status                 string
	BundleID               string
	SupportCode            string
	Arn                    string
	DefaultCacheBehavior   CacheBehavior
	CacheBehaviors         []CacheBehaviorPerPath
	AlternativeDomainNames []string
	IsEnabled              bool
	AbleToUpdateBundle     bool
}

// DistributionOrigin mirrors types.Origin.
type DistributionOrigin struct {
	Name           string
	RegionName     string
	ProtocolPolicy string
}

// CacheBehavior mirrors types.CacheBehavior -- Behavior is "cache" or
// "dont-cache" (BehaviorEnum).
type CacheBehavior struct {
	Behavior string
}

// CacheBehaviorPerPath mirrors types.CacheBehaviorPerPath.
type CacheBehaviorPerPath struct {
	Behavior string
	Path     string
}

// CacheSettings mirrors types.CacheSettings, the distribution's
// CacheBehaviorSettings.
type CacheSettings struct {
	ForwardedCookies      *CookieObject
	ForwardedHeaders      *HeaderObject
	ForwardedQueryStrings *QueryStringObject
	AllowedHTTPMethods    string
	CachedHTTPMethods     string
	DefaultTTL            int64
	MaximumTTL            int64
	MinimumTTL            int64
}

// CookieObject mirrors types.CookieObject.
type CookieObject struct {
	Option           string
	CookiesAllowList []string
}

// HeaderObject mirrors types.HeaderObject.
type HeaderObject struct {
	Option           string
	HeadersAllowList []string
}

// QueryStringObject mirrors types.QueryStringObject. Option is a pointer
// because the real member is nullable tri-state (unset/false/true), not a
// plain bool: per CreateDistribution's SDK doc comment, if option is true,
// the distribution forwards all query strings regardless of
// queryStringsAllowList.
type QueryStringObject struct {
	Option                *bool
	QueryStringsAllowList []string
}

func (d *Distribution) clone() *Distribution {
	cp := *d
	cp.AlternativeDomainNames = cloneStrings(d.AlternativeDomainNames)
	cp.CacheBehaviors = append([]CacheBehaviorPerPath(nil), d.CacheBehaviors...)

	if d.CacheBehaviorSettings != nil {
		settings := *d.CacheBehaviorSettings

		if d.CacheBehaviorSettings.ForwardedCookies != nil {
			cookies := *d.CacheBehaviorSettings.ForwardedCookies
			cookies.CookiesAllowList = cloneStrings(cookies.CookiesAllowList)
			settings.ForwardedCookies = &cookies
		}

		if d.CacheBehaviorSettings.ForwardedHeaders != nil {
			headers := *d.CacheBehaviorSettings.ForwardedHeaders
			headers.HeadersAllowList = cloneStrings(headers.HeadersAllowList)
			settings.ForwardedHeaders = &headers
		}

		if d.CacheBehaviorSettings.ForwardedQueryStrings != nil {
			qs := *d.CacheBehaviorSettings.ForwardedQueryStrings
			qs.QueryStringsAllowList = cloneStrings(qs.QueryStringsAllowList)
			settings.ForwardedQueryStrings = &qs
		}

		cp.CacheBehaviorSettings = &settings
	}

	return &cp
}

// Domain mirrors types.Domain -- CONFIRMED a GLOBAL resource (Domain.Arn's
// own SDK doc comment gives the literal example
// "arn:aws:lightsail:global:...", PARITY.md 4.7/5.1).
type Domain struct {
	CreatedAt   time.Time
	Tags        *tags.Tags
	Location    ResourceLocation
	Name        string
	Arn         string
	SupportCode string
	Entries     []DomainEntry
}

// DomainEntry mirrors types.DomainEntry. Type is a plain string (A/AAAA/
// CNAME/MX/NS/SOA/SRV/TXT per the SDK's own doc comment, not a typed enum,
// PARITY.md 4.7).
type DomainEntry struct {
	Options map[string]string
	ID      string
	Name    string
	Type    string
	Target  string
	IsAlias bool
}

func cloneDomainEntries(in []DomainEntry) []DomainEntry {
	if in == nil {
		return nil
	}

	out := make([]DomainEntry, len(in))
	for i, e := range in {
		opts := make(map[string]string, len(e.Options))
		maps.Copy(opts, e.Options)
		e.Options = opts
		out[i] = e
	}

	return out
}

func (d *Domain) clone() *Domain {
	cp := *d
	cp.Entries = cloneDomainEntries(d.Entries)

	return &cp
}

// Certificate mirrors types.CertificateSummary/types.Certificate --
// CDN/distribution-facing (family V), distinct from the LB-TLS-certificate
// family (M) and from real ACM. CertificateProvider has exactly 1 value
// (LetsEncrypt, PARITY.md 4.7).
type Certificate struct {
	CreatedAt               time.Time
	IssuedAt                time.Time
	NotBefore               time.Time
	NotAfter                time.Time
	Tags                    *tags.Tags
	Name                    string
	Arn                     string
	DomainName              string
	Status                  string
	SubjectAlternativeNames []string
}

func (c *Certificate) clone() *Certificate {
	cp := *c
	cp.SubjectAlternativeNames = cloneStrings(c.SubjectAlternativeNames)

	return &cp
}

// Alarm mirrors types.Alarm.
type Alarm struct {
	CreatedAt             time.Time
	Tags                  *tags.Tags
	Location              ResourceLocation
	TreatMissingData      string
	SupportCode           string
	MonitoredResourceName string
	MonitoredResourceArn  string
	Statistic             string
	Unit                  string
	State                 string
	Name                  string
	ComparisonOperator    string
	MetricName            string
	Arn                   string
	NotificationTriggers  []string
	ContactProtocols      []string
	Threshold             float64
	EvaluationPeriods     int32
	DatapointsToAlarm     int32
	NotificationEnabled   bool
}

func (a *Alarm) clone() *Alarm {
	cp := *a
	cp.ContactProtocols = cloneStrings(a.ContactProtocols)
	cp.NotificationTriggers = cloneStrings(a.NotificationTriggers)

	return &cp
}

// ContactMethod mirrors types.ContactMethod -- keyed by Protocol (there is
// no separate Name concept; ContactProtocol has exactly 2 values, Email/SMS,
// PARITY.md 4.8).
type ContactMethod struct {
	CreatedAt       time.Time
	Tags            *tags.Tags
	Location        ResourceLocation
	Protocol        string
	ContactEndpoint string
	Status          string
	Arn             string
	SupportCode     string
}

func (c *ContactMethod) clone() *ContactMethod {
	cp := *c

	return &cp
}

// GUISession mirrors the per-resource session state CreateGUISessionAccessDetails/
// StartGUISession/StopGUISession (family AA) track.
type GUISession struct {
	ResourceName string
	Status       string
	URL          string
}

func (s *GUISession) clone() *GUISession {
	cp := *s

	return &cp
}
