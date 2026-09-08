package azurearm

import (
	"context"
	"fmt"
	"sort"
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

// storageAPIVersions lists the api-versions this RP claims to support in
// its /providers/{ns} registration response. Response BODY SHAPE does not
// branch on the caller's api-version (AZURE.md section 10.1) -- these are
// only ever echoed/advertised.
var storageAPIVersions = []string{"2023-01-01", "2022-09-01", "2021-09-01"} //nolint:gochecknoglobals // static config list

// StorageEndpointConfig configures how the Storage resource provider
// advertises Blob/Queue/Table data-plane endpoints in
// properties.primaryEndpoints, per AZURE.md section 10.4's
// endpoint-advertisement design.
type StorageEndpointConfig struct {
	// BlobPort/QueuePort/TablePort are the data-plane services' own
	// configured ports (defaults DefaultBlobEndpointPort/
	// DefaultQueueEndpointPort/DefaultTableEndpointPort).
	BlobPort, QueuePort, TablePort int
	// BlobOverride/QueueOverride/TableOverride, if non-empty, are used
	// verbatim as the full endpoint (e.g. "http://storage.example.com:9000")
	// instead of deriving scheme://<request host>:<port>.
	BlobOverride, QueueOverride, TableOverride string
}

// storedStorageAccount is the Storage RP's own internal representation of
// one ARM-created storage account.
type storedStorageAccount struct {
	name       string
	location   string
	tags       map[string]string
	sku        map[string]any
	kind       string
	properties map[string]any
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
	mu        *lockmetrics.RWMutex
	accounts  map[string]*storedStorageAccount // keyed by strings.ToLower(name)
	cfg       StorageEndpointConfig
	dataPlane StorageAccounts
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
	return []ResourceTypeDef{{Type: storageAccountsType, APIVersions: storageAPIVersions, HasChildren: false}}
}

// Put implements ResourceProvider: creates or updates a storage account.
func (p *StorageProvider) Put(ctx context.Context, id ResourceID, body map[string]any) (map[string]any, error) {
	if !strings.EqualFold(id.LeafType(), storageAccountsType) {
		return nil, fmt.Errorf("%w: unsupported Microsoft.Storage resource type %q", ErrResourceNotFound, id.LeafType())
	}

	name := id.LeafName()
	key := strings.ToLower(name)

	p.mu.Lock("Put")

	existing, existed := p.accounts[key]

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
		name:       name,
		location:   location,
		tags:       stringTags(body["tags"]),
		properties: map[string]any{},
		host:       host,
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

// Get implements ResourceProvider.
func (p *StorageProvider) Get(_ context.Context, id ResourceID) (map[string]any, error) {
	p.mu.RLock("Get")
	defer p.mu.RUnlock()

	acct, ok := p.accounts[strings.ToLower(id.LeafName())]
	if !ok {
		return nil, ErrStorageAccountNotFound
	}

	return p.buildBody(id, acct), nil
}

// Delete implements ResourceProvider.
func (p *StorageProvider) Delete(ctx context.Context, id ResourceID) error {
	key := strings.ToLower(id.LeafName())

	p.mu.Lock("Delete")

	if _, ok := p.accounts[key]; !ok {
		p.mu.Unlock()

		return ErrStorageAccountNotFound
	}

	delete(p.accounts, key)
	p.mu.Unlock()

	if err := p.dataPlane.DeleteAccount(id.LeafName()); err != nil {
		logStorageAdapterError(ctx, "DeleteAccount", id.LeafName(), err)
	}

	return nil
}

// List implements ResourceProvider.
func (p *StorageProvider) List(_ context.Context, id ResourceID) ([]map[string]any, error) {
	p.mu.RLock("List")
	defer p.mu.RUnlock()

	out := make([]map[string]any, 0, len(p.accounts))

	for _, acct := range p.accounts {
		accountID := ResourceID{
			SubscriptionID: id.SubscriptionID,
			ResourceGroup:  id.ResourceGroup,
			Namespace:      namespaceMicrosoftStorage,
			Types:          []string{storageAccountsType},
			Names:          []string{acct.name},
		}
		out = append(out, p.buildBody(accountID, acct))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i]["name"].(string) < out[j]["name"].(string) //nolint:forcetypeassert // buildBody always sets a string name
	})

	return out, nil
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
	p.mu.RLock("ListKeys")
	_, ok := p.accounts[strings.ToLower(id.LeafName())]
	p.mu.RUnlock()

	if !ok {
		return nil, ErrStorageAccountNotFound
	}

	return map[string]any{
		"keys": []map[string]any{
			{"keyName": "key1", "value": azureauth.DefaultAccountKey, "permissions": "Full"},
			{"keyName": "key2", "value": azureauth.DefaultAccountKey, "permissions": "Full"},
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

	for k, v := range acct.properties {
		props[k] = v
	}

	body := map[string]any{
		"id":         id.ARMID(),
		"name":       acct.name,
		"type":       namespaceMicrosoftStorage + "/" + storageAccountsType,
		"location":   acct.location,
		"tags":       tagsOrEmpty(acct.tags),
		"properties": props,
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

	return fmt.Sprintf("http://%s:%d/%s/", host, port, account)
}

// logStorageAdapterError logs a failure from the StorageAccounts adapter.
// The nil-safe default (noopStorageAccounts) never errors; a real adapter
// (M10) failing here is logged but never fails the ARM operation itself --
// ARM's own state is the source of truth for M7.
func logStorageAdapterError(ctx context.Context, op, account string, err error) {
	logger.Load(ctx).WarnContext(ctx, "azurearm: storage data-plane adapter call failed",
		"op", op, "account", account, "error", err)
}
