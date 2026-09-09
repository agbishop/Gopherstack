package azurearm

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/azureauth"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/logger"
)

// storageAccountsType is the only resource type this RP serves in M7.
// Direct-data-plane storage resources (containers/blobs/queues/tables as ARM
// resources) are explicitly out of scope until M10 -- see AZURE.md section
// 10.8's storage-suffix spike and PARITY.md.
const storageAccountsType = "storageAccounts"

const namespaceMicrosoftStorage = "Microsoft.Storage"

// storageAPIVersions returns the api-versions this RP claims to support in
// its /providers/{ns} registration response. Response BODY SHAPE does not
// branch on the caller's api-version (AZURE.md section 10.1) -- these are
// only ever echoed/advertised.
func storageAPIVersions() []string {
	return []string{"2023-01-01", "2022-09-01", "2021-09-01"}
}

// StorageEndpointConfig configures how the Storage resource provider
// advertises Blob/Queue/Table data-plane endpoints in
// properties.primaryEndpoints, per AZURE.md section 10.4's
// endpoint-advertisement design.
type StorageEndpointConfig struct {
	BlobOverride  string
	QueueOverride string
	TableOverride string
	BlobPort      int
	QueuePort     int
	TablePort     int
}

// storedStorageAccount is the Storage RP's own internal representation of
// one ARM-created storage account.
type storedStorageAccount struct {
	name          string
	resourceGroup string
	location      string
	tags          map[string]string
	sku           map[string]any
	kind          string
	properties    map[string]any
	// host is the hostname of the ARM request's own Host header at the time
	// this account was created (via WithRequestHost/RequestHostFromContext),
	// used to default the advertised data-plane endpoints (AZURE.md section
	// 10.4). Empty if the request context carried no host (e.g. a unit test
	// constructing the provider directly), in which case "localhost" is used.
	host string
}

// StorageProvider implements ResourceProvider for Microsoft.Storage. Per
// AZURE.md section 10.4, it is metadata-only: Blob/Queue/Table are already
// account-name-agnostic, so no data-plane delegation is required for an
// ARM-created account to be immediately usable. StorageAccounts (accounts
// StorageAccounts interface) is called best-effort for forward-compat with
// M10's per-account namespacing.
type StorageProvider struct {
	dataPlane StorageAccounts
	mu        *lockmetrics.RWMutex
	accounts  map[string]*storedStorageAccount
	cfg       StorageEndpointConfig
}

// NewStorageProvider creates a StorageProvider. dataPlane may be nil, in
// which case a no-op default is used (see interfaces.go).
func NewStorageProvider(cfg StorageEndpointConfig, dataPlane StorageAccounts) *StorageProvider {
	if dataPlane == nil {
		dataPlane = noopStorageAccounts{}
	}

	if cfg.BlobPort == 0 {
		cfg.BlobPort = DefaultBlobEndpointPort
	}

	if cfg.QueuePort == 0 {
		cfg.QueuePort = DefaultQueueEndpointPort
	}

	if cfg.TablePort == 0 {
		cfg.TablePort = DefaultTableEndpointPort
	}

	return &StorageProvider{
		mu:        lockmetrics.New("azurearm.storageprovider"),
		accounts:  make(map[string]*storedStorageAccount),
		cfg:       cfg,
		dataPlane: dataPlane,
	}
}

var _ ResourceProvider = (*StorageProvider)(nil)

// Namespace implements ResourceProvider.
func (p *StorageProvider) Namespace() string { return namespaceMicrosoftStorage }

// ResourceTypes implements ResourceProvider.
func (p *StorageProvider) ResourceTypes() []ResourceTypeDef {
	return []ResourceTypeDef{{Type: storageAccountsType, APIVersions: storageAPIVersions(), HasChildren: false}}
}

// errStorageResourceTypeMismatch is returned by every StorageProvider
// operation when id.LeafType() isn't storageAccountsType -- Put already
// checked this; CodeRabbit flagged that Get/Delete/List/ListKeys didn't,
// silently operating on storage accounts for a request path naming some
// other (hypothetical future) Microsoft.Storage/<otherType> resource.
func (p *StorageProvider) checkResourceType(id ResourceID) error {
	if !strings.EqualFold(id.LeafType(), storageAccountsType) {
		return fmt.Errorf("%w: unsupported Microsoft.Storage resource type %q", ErrResourceNotFound, id.LeafType())
	}

	return nil
}

// errAccountExistsInOtherResourceGroup is returned by Put when name is
// already used by an account in a different resource group -- real Azure
// storage account names are globally unique (DNS-based), so this is a
// genuine 409 Conflict, not a case this emulator can silently allow by
// namespacing accounts per resource group.
var errAccountExistsInOtherResourceGroup = errors.New(
	"azurearm: storage account name already exists in another resource group",
)

// Put implements ResourceProvider: creates or updates a storage account.
func (p *StorageProvider) Put(ctx context.Context, id ResourceID, body map[string]any) (map[string]any, error) {
	if err := p.checkResourceType(id); err != nil {
		return nil, err
	}

	name := id.LeafName()
	key := strings.ToLower(name)

	p.mu.Lock("Put")

	existing, existed := p.accounts[key]
	if existed && !resourceGroupsEqual(existing.resourceGroup, id.ResourceGroup) {
		p.mu.Unlock()

		return nil, errAccountExistsInOtherResourceGroup
	}

	location := DefaultLocation
	if loc, ok := body["location"].(string); ok && loc != "" {
		location = loc
	} else if existed {
		location = existing.location
	}

	host := RequestHostFromContext(ctx)
	if host == "" && existed {
		host = existing.host
	}

	acct := &storedStorageAccount{
		name:          name,
		resourceGroup: id.ResourceGroup,
		location:      location,
		tags:          stringTags(body["tags"]),
		properties:    map[string]any{},
		host:          host,
	}

	if sku, ok := body["sku"].(map[string]any); ok {
		acct.sku = sku
	} else if existed {
		acct.sku = existing.sku
	}

	if kind, ok := body["kind"].(string); ok {
		acct.kind = kind
	} else if existed {
		acct.kind = existing.kind
	}

	p.accounts[key] = acct
	p.mu.Unlock()

	if !existed {
		if err := p.dataPlane.RegisterAccount(name); err != nil {
			logStorageAdapterError(ctx, "RegisterAccount", name, err)
		}
	}

	return p.buildBody(id, acct), nil
}

// resourceGroupsEqual compares two resource-group names case-insensitively,
// matching AZURE.md section 10.1's resourceGroups/resourcegroups handling.
func resourceGroupsEqual(a, b string) bool {
	return strings.EqualFold(a, b)
}

// lookupOwnedAccount returns the account named by id.LeafName(), but only if
// it belongs to id.ResourceGroup -- a Get/Delete/ListKeys via the wrong
// resource group's path must 404, not silently operate on another group's
// account (CodeRabbit-flagged: previously keyed by name alone, ignoring
// which resource group's path the request actually came in on).
func (p *StorageProvider) lookupOwnedAccount(id ResourceID) (*storedStorageAccount, bool) {
	acct, ok := p.accounts[strings.ToLower(id.LeafName())]
	if !ok || !resourceGroupsEqual(acct.resourceGroup, id.ResourceGroup) {
		return nil, false
	}

	return acct, true
}

// Get implements ResourceProvider.
func (p *StorageProvider) Get(_ context.Context, id ResourceID) (map[string]any, error) {
	if err := p.checkResourceType(id); err != nil {
		return nil, err
	}

	p.mu.RLock("Get")
	defer p.mu.RUnlock()

	acct, ok := p.lookupOwnedAccount(id)
	if !ok {
		return nil, ErrStorageAccountNotFound
	}

	return p.buildBody(id, acct), nil
}

// Delete implements ResourceProvider.
func (p *StorageProvider) Delete(ctx context.Context, id ResourceID) error {
	if err := p.checkResourceType(id); err != nil {
		return err
	}

	p.mu.Lock("Delete")

	if _, ok := p.lookupOwnedAccount(id); !ok {
		p.mu.Unlock()

		return ErrStorageAccountNotFound
	}

	key := strings.ToLower(id.LeafName())
	delete(p.accounts, key)
	p.mu.Unlock()

	if err := p.dataPlane.DeleteAccount(id.LeafName()); err != nil {
		logStorageAdapterError(ctx, "DeleteAccount", id.LeafName(), err)
	}

	return nil
}

// List implements ResourceProvider, scoped to id.ResourceGroup if set (per
// the ResourceProvider interface contract), else every account in the
// subscription.
func (p *StorageProvider) List(_ context.Context, id ResourceID) ([]map[string]any, error) {
	p.mu.RLock("List")
	defer p.mu.RUnlock()

	out := make([]map[string]any, 0, len(p.accounts))

	for _, acct := range p.accounts {
		if id.ResourceGroup != "" && !resourceGroupsEqual(acct.resourceGroup, id.ResourceGroup) {
			continue
		}

		accountID := ResourceID{
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  acct.resourceGroup,
			Namespace:      namespaceMicrosoftStorage,
			Types:          []string{storageAccountsType},
			Names:          []string{acct.name},
		}
		out = append(out, p.buildBody(accountID, acct))
	}

	sort.Slice(out, func(i, j int) bool {
		return stringField(out[i], fieldName) < stringField(out[j], fieldName)
	})

	return out, nil
}

// Reset implements the reset hook Registry.ResetAll calls across every
// registered provider (Handler.Reset -- CodeRabbit-flagged: without this,
// accounts created before a /_gopherstack/reset survived it, since only
// InMemoryBackend was being cleared).
func (p *StorageProvider) Reset() {
	p.mu.Lock("Reset")
	defer p.mu.Unlock()

	p.accounts = make(map[string]*storedStorageAccount)
}

// DeleteResourcesInGroup implements the cascade-delete hook
// Registry.DeleteResourcesInGroup calls when a resource group is deleted
// (CodeRabbit-flagged: DELETE .../resourceGroups/{name} previously only
// cleared InMemoryBackend's generic resources, leaving StorageProvider
// accounts in that group orphaned but still reachable).
func (p *StorageProvider) DeleteResourcesInGroup(ctx context.Context, resourceGroup string) {
	p.mu.Lock("DeleteResourcesInGroup")

	var toDelete []string

	for key, acct := range p.accounts {
		if resourceGroupsEqual(acct.resourceGroup, resourceGroup) {
			toDelete = append(toDelete, key)
		}
	}

	for _, key := range toDelete {
		delete(p.accounts, key)
	}

	p.mu.Unlock()

	for _, key := range toDelete {
		if err := p.dataPlane.DeleteAccount(key); err != nil {
			logStorageAdapterError(ctx, "DeleteAccount", key, err)
		}
	}
}

// ListKeys implements ResourceProvider. The response shape --
// {"keys":[{"keyName","value","permissions"}, ...]} -- was verified against
// the real ARM "Storage Accounts - List Keys" REST API documentation
// (learn.microsoft.com/en-us/rest/api/storagerp/storage-accounts/list-keys):
// StorageAccountListKeysResult.keys is a StorageAccountKey[] with exactly
// those three fields, "permissions" taking the KeyPermission enum value
// "Full" (not "FULL"). Key values are pkgs/azureauth's well-known
// devstoreaccount1 development key, matching every other Azure service's
// well-known-credential convention.
func (p *StorageProvider) ListKeys(_ context.Context, id ResourceID) (map[string]any, error) {
	if err := p.checkResourceType(id); err != nil {
		return nil, err
	}

	p.mu.RLock("ListKeys")
	_, ok := p.lookupOwnedAccount(id)
	p.mu.RUnlock()

	if !ok {
		return nil, ErrStorageAccountNotFound
	}

	return map[string]any{
		"keys": []map[string]any{
			{"keyName": "key1", fieldValue: azureauth.DefaultAccountKey, "permissions": "Full"},
			{"keyName": "key2", fieldValue: azureauth.DefaultAccountKey, "permissions": "Full"},
		},
	}, nil
}

// buildBody builds the full ARM wire response body for a storage account,
// including properties.primaryEndpoints (AZURE.md section 10.4's
// endpoint-advertisement design). acct.host is the hostname of the ARM
// request's Host header at account-creation time (see WithRequestHost /
// RequestHostFromContext), captured once in Put and reused by Get/List/
// buildBody; it defaults to "localhost" when no request-scoped host was
// ever available (e.g. a unit test constructing the provider directly).
func (p *StorageProvider) buildBody(id ResourceID, acct *storedStorageAccount) map[string]any {
	props := map[string]any{
		"provisioningState": provisioningStateSucceeded,
		"primaryEndpoints": map[string]any{
			"blob":  advertiseEndpoint(p.cfg.BlobOverride, acct.host, p.cfg.BlobPort, acct.name),
			"queue": advertiseEndpoint(p.cfg.QueueOverride, acct.host, p.cfg.QueuePort, acct.name),
			"table": advertiseEndpoint(p.cfg.TableOverride, acct.host, p.cfg.TablePort, acct.name),
		},
	}

	maps.Copy(props, acct.properties)

	body := map[string]any{
		"id":            id.ARMID(),
		fieldName:       acct.name,
		fieldType:       namespaceMicrosoftStorage + "/" + storageAccountsType,
		fieldLocation:   acct.location,
		fieldTags:       tagsOrEmpty(acct.tags),
		fieldProperties: props,
	}

	if len(acct.sku) > 0 {
		body["sku"] = acct.sku
	} else {
		body["sku"] = map[string]any{"name": "Standard_LRS", "tier": "Standard"}
	}

	if acct.kind != "" {
		body["kind"] = acct.kind
	} else {
		body["kind"] = "StorageV2"
	}

	return body
}

// advertiseEndpoint builds one primaryEndpoints URL: override if set, else
// scheme://host:port/account/, defaulting host to "localhost" if unset.
func advertiseEndpoint(override, host string, port int, account string) string {
	if override != "" {
		return strings.TrimSuffix(override, "/") + "/" + account + "/"
	}

	if host == "" {
		host = "localhost"
	}

	return "http://" + net.JoinHostPort(host, strconv.Itoa(port)) + "/" + account + "/"
}

// logStorageAdapterError logs a failure from the StorageAccounts adapter.
// The nil-safe default (noopStorageAccounts) never errors; a real adapter
// (M10) failing here is logged but never fails the ARM operation itself --
// ARM's own state is the source of truth for M7.
func logStorageAdapterError(ctx context.Context, op, account string, err error) {
	logger.Load(ctx).WarnContext(ctx, "azurearm: storage data-plane adapter call failed",
		"op", op, "account", account, "error", err)
}
