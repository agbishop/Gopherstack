package ram

import (
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	// statusActive is the active status for a resource share.
	statusActive = "ACTIVE"
	// statusDeleted is the deleted status for a resource share.
	statusDeleted = "DELETED"
	// associationStatusAssociated is the associated status for an association.
	associationStatusAssociated = "ASSOCIATED"
	// associationStatusDisassociated is the disassociated status for an association.
	associationStatusDisassociated = "DISASSOCIATED"
	// associationTypePrincipal is the principal association type.
	associationTypePrincipal = "PRINCIPAL"
	// associationTypeResource is the resource association type.
	associationTypeResource = "RESOURCE"
	// invitationStatusPending is the pending status for an invitation.
	invitationStatusPending = "PENDING"
	// invitationStatusAccepted is the accepted status for an invitation.
	invitationStatusAccepted = "ACCEPTED"
	// invitationStatusRejected is the rejected status for an invitation.
	invitationStatusRejected = "REJECTED"
	// invitationStatusExpired is the expired status for an invitation.
	invitationStatusExpired = "EXPIRED"
	// invitationExpiryWindow is how long a PENDING invitation stays acceptable before
	// it lazily transitions to EXPIRED. AWS RAM user guide: "For shared resource types
	// not on the [7-day] list... After 12 hours, the invitation expires and the end
	// user principal in the resource share is disassociated." Most resource types get
	// this 12h default; the 7-day carve-out for a specific resource-type allowlist
	// (EC2 capacity reservations, VPC subnets, etc.) is not modeled.
	invitationExpiryWindow = 12 * time.Hour
	// permissionTypeCustomer is the customer managed permission type.
	permissionTypeCustomer = "CUSTOMER_MANAGED"
	// permissionTypeAWSManaged is the AWS-managed permission type.
	permissionTypeAWSManaged = "AWS_MANAGED"
	// permissionTypeCreatedFromPolicy is the type for permissions auto-created from
	// resource policies, promotable to CUSTOMER_MANAGED via PromotePermissionCreatedFromPolicy.
	permissionTypeCreatedFromPolicy = "CREATED_FROM_POLICY"
	// permissionTypeFilterAll is ListPermissionsInput.PermissionType's "both types" value
	// (types.PermissionTypeFilterAll) -- distinct from any actual Permission.PermissionType,
	// so it must be special-cased rather than compared for equality against stored values.
	permissionTypeFilterAll = "ALL"
	// resourceOwnerSelf is the owner filter for resources owned by the calling account.
	resourceOwnerSelf = "SELF"
	// resourceOwnerOtherAccounts is the owner filter for resources shared by other accounts.
	resourceOwnerOtherAccounts = "OTHER-ACCOUNTS"
	// resourceRegionScopeRegional indicates a resource is region-scoped.
	resourceRegionScopeRegional = "REGIONAL"
	// resourceRegionScopeGlobal indicates a resource is globally scoped.
	resourceRegionScopeGlobal = "GLOBAL"
	// replaceWorkStatusCompleted is the terminal status for a ReplacePermissionAssociations
	// background work item. This mock performs the association swap synchronously, so a
	// work item is always COMPLETED by the time it is persisted (IN_PROGRESS/FAILED are
	// valid real-AWS states but never occur here).
	replaceWorkStatusCompleted = "COMPLETED"
	// accountIDLen is the number of digits in an AWS account ID.
	accountIDLen = 12
	// arnPartCountPrincipal is the min number of colon-separated parts to extract an account from an ARN.
	arnPartCountPrincipal = 6
	// arnAccountIdx is the index of the account field in a colon-split ARN.
	arnAccountIdx = 4
	// arnServiceIdx is the index of the service field in a colon-split ARN.
	arnServiceIdx = 2
	// arnServiceIAM is the ARN service segment for IAM role/user principals.
	arnServiceIAM = "iam"
	// arnPrefix is the first colon-split field of every AWS ARN.
	arnPrefix = "arn"

	// Resource type strings shared between backend and built-in permission seeds.
	resourceTypeEC2Subnet         = "ec2:Subnet"
	resourceTypeEC2VPC            = "ec2:VPC"
	resourceTypeEC2TransitGateway = "ec2:TransitGateway"
	resourceTypeEC2PrefixList     = "ec2:PrefixList"
	resourceTypeS3Bucket          = "s3:Bucket"
	resourceTypeRoute53Resolver   = "route53resolver:ResolverRule"
	resourceTypeLicenseManager    = "license-manager:LicenseConfiguration"
)

// builtInPermDef defines a built-in AWS-managed RAM permission.
type builtInPermDef struct {
	name                string
	resourceType        string
	resourceRegionScope string
	policy              string
}

//nolint:gochecknoglobals,lll // read-only table initialized once; JSON policy strings must remain intact
var awsBuiltInPermissions = []builtInPermDef{
	{
		name:                "AWSRAMDefaultPermissionEC2Subnet",
		resourceType:        resourceTypeEC2Subnet,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["ec2:CreateTags","ec2:Describe*","ec2:*NetworkInterface*","ec2:*Route*","ec2:*SubnetCidrBlock*"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
	{
		name:                "AWSRAMDefaultPermissionEC2VPC",
		resourceType:        resourceTypeEC2VPC,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["ec2:CreateTags","ec2:Describe*","ec2:*Vpc*","ec2:*SecurityGroup*","ec2:*Dhcp*"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
	{
		name:                "AWSRAMDefaultPermissionEC2TransitGateway",
		resourceType:        resourceTypeEC2TransitGateway,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["ec2:CreateTags","ec2:Describe*","ec2:*TransitGateway*"],"Resource":"*"}`,
	},
	{
		name:                "AWSRAMDefaultPermissionEC2PrefixList",
		resourceType:        resourceTypeEC2PrefixList,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["ec2:CreateTags","ec2:Describe*","ec2:*ManagedPrefixList*","ec2:GetManagedPrefixListEntries"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
	{
		name:                "AWSRAMDefaultPermissionS3Bucket",
		resourceType:        resourceTypeS3Bucket,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["s3:ListBucket","s3:GetObject","s3:GetBucketLocation","s3:GetBucketAcl"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
	{
		name:                "AWSRAMDefaultPermissionRoute53ResolverResolverRule",
		resourceType:        resourceTypeRoute53Resolver,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["route53resolver:ListResolverRules","route53resolver:GetResolverRule","route53resolver:*ResolverRuleAssociation*"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
	{
		name:                "AWSRAMDefaultPermissionLicenseManagerLicenseConfiguration",
		resourceType:        resourceTypeLicenseManager,
		resourceRegionScope: resourceRegionScopeRegional,
		policy:              `{"Effect":"Allow","Action":["license-manager:ListAssociationsForLicenseConfiguration","license-manager:GetLicenseConfiguration","license-manager:ListLicenseConfigurations","license-manager:CheckInLicense","license-manager:CheckoutLicense"],"Resource":"*"}`, //nolint:lll // JSON policy must be on one line
	},
}

// awsManagedPermARN builds the ARN for an AWS-managed permission.
// AWS-managed permissions use an empty account and region component.
func awsManagedPermARN(name string) string {
	return "arn:aws:ram::aws:permission/" + name
}

// InMemoryBackend is an in-memory store for AWS RAM resources.
type InMemoryBackend struct {
	resourceShares   *store.Table[ResourceShare]
	permissions      *store.Table[Permission]
	sharePermissions map[string]map[string]int32 // shareARN -> permissionARN -> version
	invitations      *store.Table[ResourceShareInvitation]
	replaceWorks     *store.Table[ReplacePermissionAssociationsWork]
	registry         *store.Registry
	mu               *lockmetrics.RWMutex
	accountID        string
	region           string
	associations     []*ResourceShareAssociation
}

// NewInMemoryBackend creates a new in-memory RAM backend seeded with AWS-managed permissions.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		associations:     make([]*ResourceShareAssociation, 0),
		sharePermissions: make(map[string]map[string]int32),
		accountID:        accountID,
		region:           region,
		mu:               lockmetrics.New("ram"),
		registry:         store.NewRegistry(),
	}
	registerAllTables(b)
	b.seedBuiltInPermissions()

	return b
}

// seedBuiltInPermissions populates the backend with AWS-managed default permissions.
// Called at construction and after Reset so built-in permissions are always available.
func (b *InMemoryBackend) seedBuiltInPermissions() {
	now := time.Now()

	for _, def := range awsBuiltInPermissions {
		permARN := awsManagedPermARN(def.name)
		pv := &PermissionVersion{
			Version:         1,
			PolicyTemplate:  def.policy,
			CreationTime:    now,
			LastUpdatedTime: now,
		}
		p := &Permission{
			ARN:                   permARN,
			Name:                  def.name,
			ResourceType:          def.resourceType,
			PermissionType:        permissionTypeAWSManaged,
			ResourceRegionScope:   def.resourceRegionScope,
			Tags:                  make(map[string]string),
			CreationTime:          now,
			LastUpdatedTime:       now,
			DefaultVersion:        1,
			LatestVersion:         1,
			IsResourceTypeDefault: true,
			Versions:              map[int32]*PermissionVersion{1: pv},
		}
		b.permissions.Put(p)
	}
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string    { return b.region }
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Reset clears all in-memory state from the backend and re-seeds built-in permissions.
// It is used by the POST /_gopherstack/reset endpoint for CI pipelines and rapid local development.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.associations = make([]*ResourceShareAssociation, 0)
	b.sharePermissions = make(map[string]map[string]int32)
	b.seedBuiltInPermissions()
}
