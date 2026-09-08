---
service: azurearm
sdk_module: hashicorp/terraform-provider-azurerm (pinned version TBD -- see "Audited against" below)
last_audit_commit: 764be44cf
last_audit_date: 2026-09-08
overall: B
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  MetadataEndpoints: {wire: ok, errors: n/a, state: n/a, persist: n/a, note: "GET /metadata/endpoints?api-version=2022-09-01. Field set (name, resourceManager, resourceManagerEndpoint, authentication.{loginEndpoint,audiences,tenant}, graph, graphAudience, gallery, portal, suffixes.{storage,keyVaultDns,sqlServerHostname,acrLoginServer}, resourceIdentifiers.microsoftGraphResourceId) verified against hashicorp/go-azure-sdk's environments.FromEndpoint, which hard-fails without name/resourceManagerEndpoint/resourceIdentifiers.microsoftGraphResourceId. Covered by TestBuildMetadataEndpoints, which asserts every field's presence explicitly."}
  OpenIDConfiguration: {wire: ok, errors: n/a, state: n/a, persist: n/a, note: "GET /{tenant}/v2.0/.well-known/openid-configuration"}
  InstanceDiscovery: {wire: ok, errors: n/a, state: n/a, persist: n/a, note: "GET /common/discovery/instance?api-version=1.1"}
  Token: {wire: ok, errors: partial, state: n/a, persist: n/a, note: "POST /{tenant}/oauth2/token (v1) and /{tenant}/oauth2/v2.0/token (v2) both return {token_type,expires_in,access_token}. Client-credentials grant only (no auth code/device code flows). Token validation is off by default (--azure-arm-validate-tokens opts in); errors=partial because a malformed grant_type/missing client_id currently still issues a token rather than a 400 -- see gaps."}
  Subscriptions: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /subscriptions, GET /subscriptions/{sub} -- single fixed dev subscription, always Enabled"}
  Tenants: {wire: ok, errors: n/a, state: n/a, persist: n/a, note: "GET /tenants -- single fixed dev tenant"}
  ProviderRegistration: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /subscriptions/{sub}/providers[/{ns}], POST .../register. registrationState is per-subscription, defaults NotRegistered, tracked in InMemoryBackend.registeredProviders and snapshotted. Only Microsoft.Storage is a real registered ResourceProvider in M7; other namespaces 404 from GetProvider/RegisterProvider but still work via the generic resource pass-through for PUT/GET/DELETE (see families.generic_resource_plane)."}
  ResourceGroupCRUD: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT/GET/DELETE/LIST /subscriptions/{sub}/resourcegroups[/{name}], case-insensitive on the resourcegroups/resourceGroups segment (verified by TestParseGenericResourcePath and TestHandler_ResourceGroupCRUD). DELETE is idempotent (404-on-delete degrades to 204, matching ARM's own idempotent-delete semantics)."}
  GenericResourcePlane: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT/GET/DELETE /subscriptions/{sub}/resourceGroups/{rg}/providers/{ns}/{type}/{name}[/{childType}/{childName}...] via a single generic path walker (resourceid.go), dispatched by registry.go to a dedicated ResourceProvider when one is registered for {ns}, else a generic metadata-only pass-through (id/name/type/location/tags/properties.provisioningState=Succeeded). List forms (both subscription-scoped and resource-group-scoped) implemented. Nested child resource types (2+ type/name pairs) parse correctly but have no dedicated RP behavior yet (M8/M9)."}
  ListKeys: {wire: ok, errors: ok, state: ok, persist: n/a, note: "POST .../{resourceId}/listKeys. Only Microsoft.Storage/storageAccounts implements it in M7; any other resource type returns ResourceNotFound (matches real ARM's behavior for resource types with no listKeys action, though the error code differs from ARM's own \"operation not supported\" shape -- see gaps)."}
  StorageAccountCRUD: {wire: ok, errors: ok, state: ok, persist: partial, note: "PUT/GET/DELETE/LIST Microsoft.Storage/storageAccounts (rp_storage.go). Metadata-only per AZURE.md section 10.4 -- no data-plane delegation needed since Blob/Queue/Table already discard the account-name segment. Returns sku/kind/location/tags/properties.primaryEndpoints.{blob,queue,table}. persist=partial: the Storage RP's own in-memory account map (StorageProvider.accounts) is NOT yet wired into Handler.Snapshot/Restore -- only the generic InMemoryBackend (resource groups + provider registrations + non-Storage generic resources) is persisted. An ARM-created storage account does not currently survive a snapshot/restore cycle. See gaps."}
  StorageAccountListKeys: {wire: ok, errors: ok, state: ok, persist: n/a, note: "{\"keys\":[{\"keyName\":\"key1\",\"value\":\"<devstoreaccount1 key>\",\"permissions\":\"Full\"},{\"keyName\":\"key2\",...}]}, verified against learn.microsoft.com/en-us/rest/api/storagerp/storage-accounts/list-keys (StorageAccountListKeysResult/StorageAccountKey/KeyPermission schema) -- note \"Full\" (not \"FULL\") is the real enum value. Both keys use pkgs/azureauth.DefaultAccountKey (Azurite's devstoreaccount1 well-known key)."}
families:
  generic_resource_plane: {status: ok, note: "one path walker (resourceid.go's ParseGenericResourcePath/ParseGenericResourceListPath) handles every namespace/type/nested-child-type shape; registry.go falls back to a metadata-only generic store for any namespace without a dedicated ResourceProvider, so future RPs (M8 ServiceBus, M9 DocumentDB, M11 KeyVault, M12 AppConfiguration) are additive."}
  endpoint_advertisement: {status: ok, note: "primaryEndpoints.{blob,queue,table} default to http://<ARM request's own Host hostname>:<10000|10001|10002>/<account>/, captured once at account-creation time from the request context (WithRequestHost/RequestHostFromContext) and reused on subsequent GET/List so the advertised host doesn't change based on which host header a later read happened to arrive on. --azure-arm-advertise-{blob,queue,table}-endpoint / AZURE_ARM_ADVERTISE_*_ENDPOINT override this per AZURE.md section 10.4."}
  auth: {status: partial, note: "Token validation off by default (any client_id/client_secret issues a token; ARM itself accepts any bearer token or none). --azure-arm-validate-tokens is plumbed through Settings but pkgs/aadauth.Issuer.Validate is not yet wired into the ARM request path to actually enforce it end-to-end -- see gaps."}
  tls: {status: ok, note: "The entire services/azurearm listener is served over HTTPS unconditionally (pkgs/devtls self-signed cert, covering localhost/127.0.0.1/::1), per AZURE.md section 10.8's finding that terraform-provider-azurerm hardcodes https:// for metadata_host and validates against the system trust store with no opt-out."}
gaps:
  - "Storage RP account state (StorageProvider.accounts) does not survive a snapshot/restore cycle -- only InMemoryBackend's resource-group/generic-resource/provider-registration state is persisted (see ops.StorageAccountCRUD). Fix is additive: give StorageProvider its own Snapshot/Restore and wire it into Handler.Snapshot/Restore alongside the backend's."
  - "--azure-arm-validate-tokens is declared in Settings but not enforced anywhere in handler.go's request path yet -- opting in currently has no effect. Wiring it requires calling Issuer.Validate against the Authorization header in dispatch() or a middleware, and deciding the exact 401 error shape."
  - "Token endpoint accepts any (or a missing) client_id/grant_type without validation and always issues a token; a malformed client_credentials request should arguably 400 with invalid_request/unsupported_grant_type per RFC 6749, matching AAD's own error shape more closely."
  - "Direct-data-plane storage resources (azurerm_storage_container/_blob/_queue/_table/_share) are explicitly out of scope until M10's storage-suffix spike (AZURE.md section 10.8) determines whether they can be redirected at all -- see AZURE.md section 10.8's \"second, still-open uncertainty\"."
  - "Multi-account namespace isolation: all ARM-created storage accounts alias one shared Blob/Queue/Table container/queue/table namespace (services/azureblob/azurequeue/azuretable discard the account-name path segment entirely). Two ARM storage accounts see each other's containers. Deferred to M10 per AZURE.md section 10.4's original note; StorageAccounts.RegisterAccount/DeleteAccount (interfaces.go) is the forward-compat seam for that fix."
  - "ARM error codes are a small hand-picked table (errors.go's errorDetails), not audited against ARM's full published error-code catalog the way services/sqs's errorDetails table has been over multiple audit passes."
  - "management locks, Azure Policy, RBAC/Microsoft.Authorization, ARM template deployments (Microsoft.Resources/deployments), PATCH-merge semantics beyond full-replace, $filter/$expand and nextLink pagination on list operations, cross-subscription/cross-tenant, resource moves, What-If, and LRO polling are all explicitly out of MVP scope per AZURE.md section 10.1/10.3 -- not bugs, deliberate scope boundaries for this milestone."
deferred:
  - "M7 initial implementation. No prior audit passes to report."
leaks: {status: clean, note: "Handler.StartWorker's listener goroutine is the only background goroutine this service starts; covered by leak_test.go's TestMain (pkgs/testleak) plus TestHandler_StartWorker_BindsServesHTTPS/TestHandler_StartWorker_BindFailureIsSynchronous exercising bind/serve/shutdown."}
---

## Notes

### Scope (M7)

This is the first of four ARM milestones (AZURE.md section 10.10): ARM core
(discovery/auth/resource-group CRUD/generic resource plane/provider
registration) plus the `Microsoft.Storage` resource provider only. See
`AZURE.md` section 10 for the full design rationale, and sections 10.1/10.3
for the explicit MVP scope boundaries (listed above under `gaps` as
deliberate, not oversights).

### TLS and the metadata document

`terraform-provider-azurerm` calls `environments.FromEndpoint(ctx,
"https://"+metadataHost)` unconditionally -- there is no configuration that
produces a plain-HTTP metadata request, and the client validates the
certificate against the system trust store with no `InsecureSkipVerify`
equivalent. `services/azurearm`'s entire listener is therefore served over
HTTPS from the first commit, using `pkgs/devtls`'s self-signed certificate.
See `AZURE.md` section 10.8 for the full investigation (verified against
`hashicorp/terraform-provider-azurerm` and `hashicorp/go-azure-sdk` source).

### Terraform test harness status

`test/terraform/azure/` exists as a separate package (own `TestMain`, own
container, fixed published host ports) proving `azurerm_resource_group` and
`azurerm_storage_account` apply/destroy against a real, unmodified
`hashicorp/azurerm` provider. See that package's own doc comment and this
repository's top-level test run output for current pass/skip status --
whether it runs to completion depends on whether the sandbox running it can
both download the `tofu` binary and the `hashicorp/azurerm` provider AND get
the child `tofu`/`terraform` process to trust the self-signed metadata-host
certificate (via `SSL_CERT_FILE`); if either is blocked in a given
environment, the test skips cleanly with a message identifying which
precondition failed, rather than failing the build. The Go unit tests
(`services/azurearm/*_test.go`) and `test/integration/azurearm_test.go`
provide full behavioral coverage independent of the Terraform layer.

### Why `Microsoft.Storage` is metadata-only

`services/azureblob`, `services/azurequeue`, and `services/azuretable` all
parse an account-name path segment and then discard it (`_, container, blob
:= splitPath(...)`), so an ARM-created storage account named `tfstorage123`
is immediately reachable at `http://host:10000/tfstorage123/...` with zero
data-plane changes -- there is nothing for the Storage RP to delegate. The
cost of that shortcut is the shared-namespace gap listed above.
