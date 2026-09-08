---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: s3tables
sdk_module: aws-sdk-go-v2/service/s3tables@v1.18.4   # version audited against
last_audit_commit: 6742921a1                      # HEAD this pass started from (2026-09-04); pass's own commit not yet made
last_audit_date: 2026-09-04
overall: A            # 2026-09-04 FIXED: two missing-precondition bugs (see Notes). DeleteTable silently
                      # accepted a stale/omitted optional versionToken and always succeeded -- real
                      # DeleteTableInput.VersionToken is optional but, when supplied, is an optimistic-
                      # concurrency check (ConflictException on mismatch), same pattern this service
                      # already enforces on PutTableReplication/DeleteTableReplication; the backend method
                      # didn't even take a versionToken parameter. Separately, DeleteTableBucket cascade-
                      # deleted all child namespaces/tables instead of requiring the bucket be empty first,
                      # and DeleteNamespace deleted a namespace that still contained tables instead of
                      # rejecting it -- both contradict AWS's own docs ("Before you delete a table bucket,
                      # you must first delete all namespaces and tables within the bucket" /
                      # "Before you delete a table namespace ... you must delete all tables within the
                      # namespace"), confirmed by directly fetching s3-tables-buckets-delete.html and
                      # s3-tables-namespace-delete.html. A prior pass's DeleteTableBucket note ("cascade ...
                      # verified correct") predates this doc citation and was wrong. Both now return the
                      # already-modelled ConflictException (new ErrTableBucketNotEmpty/ErrNamespaceNotEmpty
                      # sentinels) instead of cascading.
                      # gopherstack-wla0 (2026-08-23) FIXED: GetTable/ListTables/GetNamespace/ListNamespaces
                      # wrote the fabricated wire key "tableBucketARN" instead of the real "tableBucketId" (a
                      # different, system-assigned value neither shape's real deserializer even has a
                      # tableBucketARN case for); GetTableBucket/ListTableBuckets never populated the same real
                      # tableBucketId field at all; GetTableBucketStorageClass returned a flat
                      # {tableBucketARN,storageClass} instead of the real {storageClassConfiguration:{storageClass}}
                      # nested shape, also with a fabricated tableBucketARN; UpdateTableMetadataLocation had the
                      # same harmless-extra fabricated tableBucketARN with no real counterpart at all. Fixed by
                      # adding a real, system-assigned TableBucket.BucketID (synthesized via uuid.NewString() at
                      # CreateTableBucket, same pattern as this service's existing NamespaceID/
                      # MetricsConfigurationID), threaded through Namespace.TableBucketID/Table.TableBucketID at
                      # creation time -- see Notes. gopherstack-r80d batch 9 checked the reachable-empty-required-output
                      # class (batch 8's cleanrooms lesson) and found no real bug there; its GetTableBucketEncryption
                      # "fix" was itself wrong (real AWS 404s when unconfigured) and was reverted 2026-08-22 -- see
                      # Notes. Replication family had a real (not just deferred) wire-shape bug from a prior pass;
                      # now fixed.
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
ops:
  CreateTableBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now applies encryptionConfiguration/storageClassConfiguration/tags from request body instead of discarding them"}
  GetTableBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-wla0 FIXED: now includes the real, system-assigned tableBucketId (TableBucket.BucketID, synthesized at CreateTableBucket) -- previously omitted entirely (not fabricated, just absent, since no ID existed to source it from)."}
  ListTableBuckets: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: continuationToken/maxBuckets/prefix/type were silently ignored; now paginates via pkgs/page and filters. gopherstack-wla0 FIXED: TableBucketSummary entries now include the real tableBucketId, same fix as GetTableBucket."}
  DeleteTableBucket: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04 FIXED: was cascading to namespaces/tables/tags/replication/expiry instead of requiring the bucket be empty first, contradicting AWS docs; now rejects a non-empty bucket with ConflictException (ErrTableBucketNotEmpty) -- see Notes"}
  PutTableBucketPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableBucketPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTableBucketPolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableBucketMaintenanceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableBucketMaintenanceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableBucketEncryption: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableBucketEncryption: {wire: ok, errors: ok, state: ok, persist: ok, note: "404 NotFoundException when no PutTableBucketEncryption override was ever set is correct AWS behavior, not a bug -- see 2026-08-22 Notes entry. gopherstack-r80d batch 9 (2026-08-21) mistakenly 'fixed' this to fall back to AES256; that broke real terraform applies (aws_s3tables_table_bucket's encryption_configuration is Optional-only, not Computed, in hashicorp/aws) and was reverted."}
  DeleteTableBucketEncryption: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableBucketMetricsConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableBucketMetricsConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTableBucketMetricsConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableBucketStorageClass: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableBucketStorageClass: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-wla0 FIXED (real wire-shape bug, not just the tableBucketId gap): response was a flat {tableBucketARN,storageClass}; the real GetTableBucketStorageClassOutput has a single required member, storageClassConfiguration (nested {storageClass}), and no tableBucketARN/tableBucketId field at all -- confirmed via awsRestjson1_deserializeOpDocumentGetTableBucketStorageClassOutput. A real client's StorageClassConfiguration decoded nil on every call. Now nests correctly and drops the fabricated key."}
  PutTableBucketReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: request/response were a fabricated {tableBucketARN, replicationConfiguration:{destinations}} shape with an invented destinationBucketARN field and no status/versionToken in the Put response (204 instead of required 200 body); now {configuration:{role,rules:[{destinations:[{destinationTableBucketARN}]}]}} with versionToken optimistic concurrency"}
  GetTableBucketReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: see PutTableBucketReplication note -- Get had the same fabricated top-level shape"}
  DeleteTableBucketReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now accepts+enforces the optional versionToken query param"}
  CreateNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "CreateNamespaceOutput really does require tableBucketARN (confirmed) -- unaffected by the gopherstack-wla0 fix below, which only touches ops whose real shape has no tableBucketARN member."}
  GetNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-wla0 FIXED: GetNamespaceOutput has no tableBucketARN member at all (confirmed via awsRestjson1_deserializeOpDocumentGetNamespaceOutput) -- gopherstack was emitting a fabricated tableBucketARN key and omitting the real, system-assigned tableBucketId. Now emits the real key, backed by a new Namespace.TableBucketID set at CreateNamespace time."}
  DeleteNamespace: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04 FIXED: deleted a namespace that still contained tables instead of rejecting it, contradicting AWS docs; now rejects with ConflictException (ErrNamespaceNotEmpty) -- see Notes"}
  ListNamespaces: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: continuationToken/maxNamespaces/prefix were silently ignored; now paginates + filters. gopherstack-wla0 FIXED: NamespaceSummary entries had the same fabricated-tableBucketARN/missing-tableBucketId bug as GetNamespace, same fix."}
  CreateTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: now applies encryptionConfiguration/storageClassConfiguration/tags from request body instead of discarding them"}
  GetTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: real GetTableInput accepts tableArn alone OR tableBucketARN+namespace+name; only the triple was honored before, so ARN-only callers always got 400. gopherstack-wla0 FIXED: response wrote the wire key \"tableBucketARN\", but GetTableOutput's real deserializer (awsRestjson1_deserializeOpDocumentGetTableOutput) has no such member -- only \"tableBucketId\", the table's owning bucket's system-assigned ID. Every real client's GetTableOutput.TableBucketId decoded nil on every call before this fix. Now backed by a new Table.TableBucketID set at CreateTable time from the resolved TableBucket's own BucketID."}
  DeleteTable: {wire: ok, errors: ok, state: ok, persist: ok, note: "2026-09-04 FIXED: the optional versionToken query param was parsed nowhere and the backend method didn't even accept one, so a stale token was silently ignored and delete always succeeded; now enforces optimistic concurrency (ConflictException on mismatch) when a token is supplied, matching PutTableReplication/DeleteTableReplication's existing pattern -- see Notes"}
  ListTables: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: continuationToken/maxTables/prefix were silently ignored; now paginates + filters. gopherstack-wla0 FIXED: TableSummary had the identical fabricated-tableBucketARN/missing-tableBucketId bug as GetTable, same fix."}
  RenameTable: {wire: ok, errors: ok, state: ok, persist: ok}
  UpdateTableMetadataLocation: {wire: ok, errors: ok, state: ok, persist: ok, note: "gopherstack-wla0: dropped a fabricated tableBucketARN key from the response -- UpdateTableMetadataLocationOutput genuinely has no bucket-identifying member at all (no tableBucketARN, no tableBucketId), unlike GetTable/ListTables above; this was the harmless-extra case, not a wrong-key bug, so nothing was added back."}
  GetTableMetadataLocation: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableMaintenanceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableMaintenanceConfiguration: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTableMaintenanceJobStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: status map was always {} regardless of configured maintenance types; now one entry per configured type reporting the real JobStatus enum's Not_Yet_Run value (this backend runs no background jobs)"}
  GetTableEncryption: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: was hardcoded AES256 regardless of actual config; now reflects table override -> bucket default -> AES256 fallback"}
  GetTableStorageClass: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTablePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  GetTablePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  DeleteTablePolicy: {wire: ok, errors: ok, state: ok, persist: ok}
  PutTableReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: same class of bug as PutTableBucketReplication -- request body was flat {destinations} with invented destinationBucketARN, response was empty (204) instead of the required {status,versionToken}; now real {role,rules:[{destinations:[{destinationTableBucketARN}]}]} + versionToken optimistic concurrency, backed by a new typed store.Table[TableReplicationConfig] replacing the old map[string]bool + map[string]map[string]any pair"}
  GetTableReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: response was {configuration:<raw map>, versionToken:\"\"} (hardcoded empty token, invented destination field); now real {configuration:{role,rules},versionToken}"}
  DeleteTableReplication: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: versionToken is a required DeleteTableReplicationInput member on the real API but was previously ignored entirely (deletion always succeeded with no token, and no NotFound distinction for 'never configured'); now required + checked against the stored token (ConflictException on mismatch)"}
  GetTableReplicationStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: destinations was always [] regardless of configured replication rules; now one ReplicationDestinationStatusModel entry per configured destination (replicationStatus: completed, since this backend performs no real cross-bucket replication and applies config synchronously)"}
  PutTableRecordExpirationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: settings.days (retention period) was accepted on the wire but silently discarded -- TableRecordExpiryConfig had no field for it; status casing also normalized to the real lowercase enum (enabled/disabled, not ENABLED/DISABLED)"}
  GetTableRecordExpirationConfiguration: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: response was a fabricated top-level {tableARN,status} shape; the real GetTableRecordExpirationConfigurationOutput has a single required configuration member ({status,settings:{days}}) and no tableARN field at all"}
  GetTableRecordExpirationJobStatus: {wire: ok, errors: ok, state: ok, persist: ok, note: "fixed: hardcoded status \"SUCCEEDED\", which matches no value of the real TableRecordExpirationJobStatus enum (NotYetRun/Successful/Failed/Disabled); now NotYetRun when expiration is enabled, Disabled otherwise (this backend runs no background jobs)"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: ok}
# Families audited as a group (when per-op is impractical):
families:
  route-matcher: {status: ok, note: "verified every op's HTTP method + path prefix against aws-sdk-go-v2/service/s3tables@v1.14.3 serializers.go (49/49 ops); tableBucketARN path segments correctly URL-decoded as single segments via rawPathSegments (RawPath + url.PathUnescape per segment, not naive Split), so ARNs containing '/' and ':' route correctly"}
  timestamps: {status: ok, note: "createdAt/modifiedAt correctly use RFC3339 date-time strings (smithytime.ParseDateTime on the client side), NOT epoch-seconds -- restjson1 s3tables model uses date-time trait, unlike some other json services"}
  naming-rules: {status: ok, note: "FIXED this pass (gopherstack-spp4): CreateTableBucket/CreateNamespace/CreateTable now enforce real S3 Tables naming rules (validation.go), field-diffed against https://docs.aws.amazon.com/AmazonS3/latest/userguide/s3-tables-buckets-naming.html -- bucket names: 3-63 chars, lowercase+digits+hyphens only, must begin/end with letter or number, no underscore/period, reserved prefix denylist (xn--, sthree-, amzn-s3-demo-, aws) and reserved suffix denylist (-s3alias, --ol-s3, --x-s3, --table-s3); namespace/table names: 1-255 chars, lowercase+digits+underscores ONLY (no hyphens/periods), must begin with letter or number, namespace additionally rejects the reserved 'aws' prefix (table names do not have this restriction). Real error: BadRequestException (new ErrInvalidBucketName/ErrInvalidName sentinels), matching the exception type actually present in types/errors.go (there is no dedicated naming-violation exception). This was previously blocked on a test-fixture migration (~10+ files used hyphenated namespace/table names and underscore-containing bucket-name suffixes derived from t.Name()) -- that migration was done this pass rather than deferred again: every hyphenated namespace/table literal across handler_namespaces_test.go, handler_table_maintenance_test.go, handler_tables_test.go, persistence_test.go, tables_test.go, table_buckets_test.go, and test/integration/s3tables_test.go was renamed to the real underscore-only convention, and every bucket name built from a table-test case's name now runs through a new bucketSuffix() helper (lowercases + swaps '_' for '-') in handler_test.go. New table tests: TestBackend_CreateTableBucket_NameValidation, TestBackend_CreateNamespace_NameValidation, TestBackend_CreateTable_NameValidation."}
gaps:                     # known divergences NOT fixed — link bd issue ids
  - CreateTable's Metadata field (Iceberg schema at creation) is accepted by the real API but not parsed/stored by this emulator; no read path currently exposes table schema, so this was left deferred rather than half-wired (bd: TODO -- file if schema support becomes a priority). gopherstack-u8my: the real API added a second schema form since v1.14.3, IcebergMetadata.SchemaV2 (nested/complex Iceberg types via SchemaV2Field/document.Interface, alongside the pre-existing primitive-only Schema) -- same gap, now two unparsed shapes instead of one; CreateTableInput's Schema also went from required to optional (exactly one of Schema/SchemaV2), which is moot here since gopherstack's createTableRequest never required or read either one.
leaks: {status: clean, note: "no goroutines/janitors in this service; all state lives in InMemoryBackend's store.Table/map fields guarded by lockmetrics.RWMutex, snapshotted via Handler.Snapshot/Restore delegation to InMemoryBackend"}
---

## Notes

## 2026-09-04: DeleteTable ignored its optional versionToken; DeleteTableBucket/DeleteNamespace cascaded instead of requiring emptiness

**DeleteTable's versionToken was parsed nowhere.** `DeleteTableInput.VersionToken`
is optional (`aws-sdk-go-v2/service/s3tables@v1.18.4`
`api_op_DeleteTable.go`: no "This member is required" on `VersionToken`,
unlike `Name`/`Namespace`/`TableBucketARN`; confirmed the client doesn't
require it via `validateOpDeleteTableInput` in `validators.go`, which only
checks the other three), bound as the `versionToken` query parameter
(`awsRestjson1_serializeOpHttpBindingsDeleteTableInput`, serializers.go).
When supplied it's an optimistic-concurrency check, the same pattern this
service already enforces on `PutTableReplication`/`DeleteTableReplication`
(both accept an optional/required versionToken and return the already-
modelled `ConflictException` on mismatch). `handleDeleteTable`
(`handler_tables.go`) never read `r.URL.Query().Get(keyVersionToken)` at
all, and `InMemoryBackend.DeleteTable` (`tables.go`) didn't even take a
versionToken parameter -- a stale or fabricated token supplied by a real
client was silently accepted and the table deleted regardless.
`DeleteTable`'s modelled error set (`awsRestjson1_deserializeOpErrorDeleteTable`,
deserializers.go) includes `ConflictException`.

Fixed: `DeleteTable` gained a `versionToken` parameter enforcing the same
`!= "" && != stored` mismatch check as `PutTableReplication`; the handler
now reads the query param and passes it through. `TestBackend_DeleteTable_VersionToken`
(`tables_test.go`) covers stale (rejected, `ErrTableVersionConflict`),
matching, and omitted tokens (both delete). Neutered the guard
(`if versionToken != "" && ...` -> `if false && ...`) and confirmed the
stale-token subtest failed with `Expected error with "ConflictException"
in chain but got nil` while the other two subtests still passed; restored
byte-identical (`md5sum`-verified) after.

**DeleteTableBucket/DeleteNamespace cascaded instead of requiring
emptiness, contradicting AWS's own docs.** Fetched
`s3-tables-buckets-delete.html` directly: "Before you delete a table
bucket, you must first delete all namespaces and tables within the
bucket." Fetched `s3-tables-namespace-delete.html` directly: "Before you
delete a table namespace from an Amazon S3 table bucket, you must delete
all tables within the namespace, or move them under another namespace."
Both `DeleteTableBucket` and `DeleteNamespace` model `ConflictException`
(`awsRestjson1_deserializeOpErrorDeleteTableBucket`/`...DeleteNamespace`,
deserializers.go). `InMemoryBackend.DeleteTableBucket` (`table_buckets.go`)
instead walked `tablesByBucket`/`namespacesByBucket` and deleted every
child namespace/table/tag/replication/expiry config unconditionally;
`InMemoryBackend.DeleteNamespace` (`namespaces.go`) deleted a namespace
with no check for tables still in it at all. A prior pass's `DeleteTableBucket`
note ("cascade to namespaces/tables/tags/replication/expiry verified
correct") predates this doc citation and was simply wrong -- "verified
correct" apparently meant only that the cascade didn't leak index entries,
not that cascading matches real AWS's documented precondition.

Fixed both to reject with the newly-added `ErrTableBucketNotEmpty`/
`ErrNamespaceNotEmpty` sentinels (`ConflictException`) when the bucket
still has any namespace, or the namespace still has any table,
respectively -- checking bucket-level emptiness by namespace count alone
is sufficient since a namespace can no longer be deleted while it has
tables, so zero namespaces transitively implies zero tables. Removed the
now-dead cascade code and the `slices` import it needed;
`store_setup.go`'s index-purpose comments updated from "cascade" to
"not-empty precondition". `TestBackend_DeleteTableBucket_NotEmpty`
(`table_buckets_test.go`) and `TestBackend_DeleteNamespace_NotEmpty`
(`namespaces_test.go`) each prove reject-then-succeed-after-child-deleted;
`TestInMemoryBackend_DeleteTableBucketCascade_PostRestore`
(`persistence_test.go`) -- which asserted the old cascade-succeeds
behavior against a Restore-rebuilt backend -- is renamed
`...Precondition_PostRestore` and rewritten to assert the corrected
reject/then-succeed sequence against the same Restore-rebuilt indexes.
Neutered each new guard independently (single occurrence each, confirmed
via `grep -c` before editing) and confirmed each failed only its own
test with `Expected error with "ConflictException" in chain but got nil`
while the sibling guard's test still passed; both restored byte-identical
(`md5sum`-verified) after.

No real caller in this repo relied on the cascade: the only backend-level
cascade call site was the now-rewritten persistence test; the SDK
integration tests (`test/integration/s3tables_test.go`) already delete an
empty bucket/namespace in their lifecycle tests, so behavior there is
unaffected.

**2026-08-22 (CI fix, gopherstack-101r): batch 9's GetTableBucketEncryption
default-fallback below is reverted -- it was wrong.** `TestTerraform_S3Tables/
success` failed against OpenTofu + the real `hashicorp/aws` provider
(v5.100.0) applying a bare `aws_s3tables_table_bucket` with no
`encryption_configuration` block: "Provider produced inconsistent result
after apply: .encryption_configuration: was null, but now
...sse_algorithm:"AES256"...". Confirmed batch 9's change is the cause by
experiment: hand-reverting only `handleGetTableBucketEncryption` (back to
`awserr.ErrNotFound` when `tb.Encryption == nil`) made the test pass;
restoring the fallback reproduced the failure; the hand-revert/confirm/
restore cycle was md5sum-verified byte-identical.

What real AWS does, checked directly rather than inferred: fetched
`internal/service/s3tables/table_bucket.go` from
`github.com/hashicorp/terraform-provider-aws` pinned at the exact `v5.100.0`
tag actually installed (via `gh api .../contents/...?ref=v5.100.0`), plus
`tofu providers schema -json` against that same installed provider binary.
Both agree: `aws_s3tables_table_bucket`'s `encryption_configuration` schema
attribute is `Optional: true` with **no** `Computed`, unlike
`maintenance_configuration` on the same resource (`Optional+Computed`) and
unlike `encryption_configuration` on `aws_s3tables_table`
(`Optional+Computed`, in `table.go`). An Optional-only attribute's final
state must equal what the config supplied (here, null) -- the provider is
not allowed to substitute a computed default, which is exactly the error
CI hit. The provider's own `findTableBucketEncryptionConfiguration` helper
explicitly maps a `*awstypes.NotFoundException` from `GetTableBucketEncryption`
to a swallowed `retry.NotFoundError` (Create/Read then simply leave the
field unset on that path) -- code that is only correct, non-buggy behavior
against an Optional-only schema attribute if real AWS genuinely returns
`NotFoundException` for an unconfigured bucket. If real AWS defaulted to
AES256 instead, this widely-used, actively-maintained provider would hit
this exact "inconsistent result" error for every user who omits the block;
it does not, which is strong evidence real AWS 404s. `s3tables@v1.18.4`'s
`GetTableBucketEncryptionOutput.EncryptionConfiguration` being `*types.
EncryptionConfiguration` ("This member is required" only binds the success
shape) plus `deserializers.go`'s `awsRestjson1_deserializeOpErrorGetTable
BucketEncryption` explicitly modeling `NotFoundException` as a legal error
response for this op corroborates it: the SDK models a not-found path for
this exact operation.

Table-level `GetTableEncryption` fallback (`tables.go`, table → bucket →
AES256) is unaffected and confirmed correct by the same evidence:
`aws_s3tables_table`'s `encryption_configuration` is `Optional+Computed` in
the same provider schema, and `table.go`'s Create/Read likewise treat a
`GetTableEncryption` `NotFoundException` as an expected, swallowed case --
but because that attribute is Computed, the provider is allowed to fill in
a value even when the config omitted it, matching gopherstack's existing
always-resolves inheritance chain. Batch 9's citation of this as precedent
was correct; only its extension of the same fallback to the bucket-level op
was not.

Reverted: `handleGetTableBucketEncryption` (`handler_table_buckets.go`)
back to `awserr.ErrNotFound` when `tb.Encryption == nil`;
`DeleteTableBucketEncryption`'s backend doc comment (`table_buckets.go`)
back to "reverting ... to the AWS default (no configuration set)" -- batch
9 called this a conflation of the default value with an absent response,
but the terraform evidence above shows the absent-response reading was
correct all along. `TestHandler_TableBucketEncryption_PutGetDeleteRoundTrip`,
`TestHandler_DeleteTableBucketEncryptionClearsConfig`
(`handler_table_bucket_config_test.go`), and `TestHandler_Encryption`/
`TestHandler_BucketEncryptionAndMetricsRoute`
(`handler_table_maintenance_test.go`) are reverted to assert 404.
`wire_output_required_r80d_test.go`, added solely to prove the now-reverted
behavior, is deleted outright rather than rewritten, since it had no
remaining purpose.

**2026-08-21 (gopherstack-r80d batch 9):** checked specifically for the
reachable-empty-required-output-field class that batch 8 (cleanrooms,
commit c790de062) found and named as this service's open item -- 60
required fields / 28 ops-with-required, all 28 ops read end to end against
`s3tables@v1.18.4`'s `api_op_*.go`, plus every handler that constructs its
JSON response body (this service builds responses as explicit
`map[string]any` literals via `json.Marshal`, not tagged structs, so the
literal cleanrooms-style `omitempty`-on-a-struct-tag shape does not apply
here -- confirmed only 2 `omitempty` tags exist in the whole package
(`models.go`'s `MetricsConfigurationID`, `TableRecordExpiryConfig.Days`),
neither of which is ever engaged by a response marshal). Every required
List/map field (`TableBuckets`, `Namespaces`, `Tables`, `Destinations`,
`Configuration`/`Status` maps) is already built via `make(..., 0, len(...))`
or an explicit non-nil literal and assigned to the response map
unconditionally (never gated behind an `if len(...) > 0`), so the omitted-
key shape cleanrooms hit does not reproduce here structurally.

One real bug found instead, in the adjacent "required-but-defaultable
means present-with-a-derived-default, not absent" shape (the same
principle behind batch 8's `PrivacyBudgetTemplate.AutoRefresh` fix, just
manifesting as a full `NotFoundException` instead of a dropped field):
`GetTableBucketEncryption` (`handler_table_buckets.go`) returned
`awserr.ErrNotFound` whenever `tb.Encryption == nil` -- i.e. whenever no
`PutTableBucketEncryption` override was ever set, which is the common,
default path since `encryptionConfiguration` is optional on
`CreateTableBucketInput` (confirmed via
`aws-sdk-go-v2/service/s3tables@v1.18.4`'s
`api_op_CreateTableBucket.go`/`api_op_GetTableBucketEncryption.go`: no
member is conditionally required, and `GetTableBucketEncryptionOutput.
EncryptionConfiguration` is unconditionally `This member is required`).
Real S3 Tables buckets always have encryption at rest (SSE-S3/AES256 by
default) -- this service's own `GetTableEncryption` (table-level) already
implements the correct table-override → bucket-default → AES256-fallback
chain (`tables.go:328-360`, `defaultSSEAlgorithm`) and explicitly documents
"There is no PutTableEncryption operation for individual tables ... every
table has an effective encryption configuration"; the bucket-level sibling
had the same real Put/Delete pair (`PutTableBucketEncryption`/
`DeleteTableBucketEncryption` both exist as real SDK ops -- `Delete`'s own
SDK doc comment says only "Deletes the encryption configuration for a
table bucket", i.e. clears a customization, not "removes encryption
entirely") but never got the matching fallback, so a fresh bucket's
encryption was unreachable via `GetTableBucketEncryption` even though the
real API would return the AES256 default. Fixed by giving
`handleGetTableBucketEncryption` the same AES256 fallback
`GetTableEncryption` already has; `DeleteTableBucketEncryption`'s own
backend comment was corrected from "reverting ... to the AWS default (no
configuration set)" (which conflated the default *value* with an absent
*response* -- the bug's original root-cause misunderstanding, dated back
to a prior pass) to "reverting ... to the AWS default (SSE-S3/AES256)".
Proven via a real `aws-sdk-go-v2/service/s3tables` client round trip
(`wire_output_required_r80d_test.go`,
`Test_SDKRoundTrip_GetTableBucketEncryption_NeverNotFound`) that fails
against the hand-reverted handler (404 `NotFoundException`) and passes
against the fix; hand-revert/confirm-fail/restore was md5sum-verified
byte-identical. Three pre-existing tests baked in the old (wrong)
NotFound-for-unconfigured assumption and were updated to assert the
AES256-default behavior instead: `TestHandler_TableBucketEncryption_
PutGetDeleteRoundTrip`, `TestHandler_DeleteTableBucketEncryptionClears
Config` (`handler_table_bucket_config_test.go`), and `TestHandler_
Encryption`/`TestHandler_BucketEncryptionAndMetricsRoute`
(`handler_table_maintenance_test.go`).

No other bugs found in this cut. `GetTableBucketPolicy`/`GetTablePolicy`
correctly keep 404-until-configured semantics (real AWS has no default
resource policy, unlike encryption, so this is not the same shape).
`GetTableBucketReplication`/`GetTableReplication` correctly 404 until a
replication config is ever Put (real AWS requires an explicit Put before
replication exists at all; confirmed no `TableBucketReplicationConfiguration.
Rules`/`Destinations` nil-vs-empty gap is reachable via a real client, since
both are validated non-nil-but-possibly-empty client-side by `validators.go`
and gopherstack's `parseReplicationConfiguration`/`parseReplicationDestinations`
already build non-nil empty slices via `make(..., 0, len(...))` in that case).

Only s3tables was taken this batch (gopherstack-r80d batch 9); sagemaker
remained off-limits (concurrent agent). See
`services/_REQUIRED_OUTPUT_CANDIDATES.md` for the updated ranked table and
settled-services entry.

**2026-08-13 (gopherstack-jqh2 pass 3):** re-extracted all 49 ops' real
method+path directly from `s3tables@v1.18.4` serializers.go and drove them
through `ExtractOperation` via the new `handler_sdk_route_table_test.go`
(`TestExtractOperation_SDKRouteTable`, one subtest per op, `t.Parallel()`).
All 49 resolved correctly, including the standalone `/get-table` path
(distinct from `/tables/{bucket}/{ns}/{name}`) and every
same-path/different-method collision. Also spot-checked with a real
percent-encoded table-bucket ARN on three representative ops to confirm the
RawPath-based segment splitting (see the ARN-in-path note below) correctly
handles a real ARN containing `/` — it does. No pre-existing table existed
to check, and no new routing bugs found. This test is now the permanent
regression guard for route-table drift.

Protocol: restjson1. Verified against `aws-sdk-go-v2/service/s3tables@v1.14.3`
`serializers.go`/`deserializers.go` directly (not against gopherstack's own output).

### Route matcher / ARN-in-path handling
`tableBucketARN` and `resourceArn` (tag ops) appear as path segments and contain
`/` and `:` (e.g. `arn:aws:s3tables:us-east-1:000000000000:bucket/my-bucket`).
`rawPathSegments` in handler.go correctly uses `r.URL.RawPath` (falling back to
`r.URL.Path`) and `url.PathUnescape`s each `/`-delimited raw segment
individually, so a URL-encoded ARN segment (client must percent-encode the `/`
as `%2F` per smithy's `httpLabel` binding) survives as one logical segment
instead of being split. All 49 SDK ops' HTTP method + path template were
cross-checked against `serializers.go` line-by-line; no method/path mismatches
found (this codebase already had this exactly right before this audit).

### GetTable's dual identification modes
Real `GetTableInput` has four *optional* fields: `Name`, `Namespace`,
`TableArn`, `TableBucketARN` — none marked required in the smithy model. A
caller may identify the table either by `tableArn` alone or by the
`tableBucketARN`+`namespace`+`name` triple. Before this audit, gopherstack
only accepted the triple and returned 400 `BadRequestException` for
`tableArn`-only requests — a real wire-shape bug, not a hypothetical: any SDK
caller passing `GetTableInput{TableArn: ...}` would break. Fixed by adding
`InMemoryBackend.GetTableByARN` and branching in `handleGetTable`.

### GetTableEncryption was a disguised no-op
`handleGetTableEncryption` unconditionally returned
`{"sseAlgorithm": "AES256"}` regardless of what `encryptionConfiguration` was
passed to `CreateTable`, and regardless of the owning bucket's own encryption
configuration. There is no `PutTableEncryption` operation in the real API
(encryption can only be set once, at `CreateTable` time, or inherited from the
bucket), so this was pure fabrication for any table created with an SSE-KMS
override. Fixed: `Table` now carries an `Encryption` field (mirroring
`TableBucket.Encryption`); `GetTableEncryption` on the backend resolves
table-override → bucket-default → AES256-default, matching AWS's documented
inheritance model.

### CreateTableBucket / CreateTable silently discarded encryptionConfiguration/storageClassConfiguration/tags
Both `CreateTableBucketInput` and `CreateTableInput` accept optional
`encryptionConfiguration`, `storageClassConfiguration`, and `tags` fields
alongside the required `name`(/`format`). gopherstack's request structs only
ever parsed `name`/`format`, so a bucket or table created with any of these
fields silently lost them — a subsequent `GetTableBucketEncryption` or
`GetTableBucketStorageClass` (etc.) would report the *unconfigured* default
even though the client explicitly configured it at creation. Fixed via
`CreateTableBucketOptions`/`CreateTableOptions` passed through from the
handler; tags are applied via the existing `TagResource` internally (same
lock already held at the correct point in the documented lock order
`muBuckets → muNamespaces → muTables → muState`, so no new lock ordering was
introduced).

### List* pagination was completely absent
`ListTableBuckets`, `ListNamespaces`, and `ListTables` all support
`continuationToken` + a max-results field (`maxBuckets`/`maxNamespaces`/
`maxTables`) + `prefix` on the wire (confirmed via `serializers.go`'s query
bindings), and their outputs include an optional `continuationToken` for the
next page (confirmed via `deserializers.go`). gopherstack ignored all of these
and always returned every matching resource in one page with no
`continuationToken` at all — a caller setting `MaxBuckets: 1` (a common
pattern to bound response size, or exercised by SDK paginators) got every
bucket back in a single unbounded page instead of the requested page size.
Fixed using `pkgs/page.New`/`page.ValidateToken` (matching the pattern already
used by `services/acm`), with a 1000-item default page size when unspecified.
`ListTableBuckets` additionally now respects the `type` filter (`aws` vs
`customer`); every bucket this backend creates is `customer`-typed, so a
`type=aws` filter now correctly returns an empty page instead of ignoring the
filter.

### Traps for the next auditor
- The extra `tableBucketARN` key gopherstack emits on `TableSummary`/
  `NamespaceSummary` list entries has no counterpart in the real
  `TableSummary`/`NamespaceSummary` smithy shapes (checked
  `deserializeDocumentTableSummary`/`NamespaceSummary` in `deserializers.go`
  directly) — this is **not** a bug: unknown JSON fields are silently ignored
  by the SDK's deserializer (`default: _, _ = key, value` in the generated
  switch), and dropping the field would only make debugging integration
  tests harder for no wire-compat benefit. Left as-is intentionally.
- `createdAt`/`modifiedAt` are formatted with the fixed layout
  `"2006-01-02T15:04:05.999Z"` throughout this package rather than
  `pkgs/awstime`. This is correct for s3tables specifically (RFC3339 string,
  not epoch), so do not "fix" it to `awstime.Epoch()` — that would break this
  service. Confirmed by reading `smithytime.ParseDateTime` call sites in
  `deserializers.go`.

### The entire replication family was a fabricated wire shape, not just missing versionToken enforcement
The prior audit's `deferred` note said only that versionToken optimistic
locking wasn't enforced. Re-diffing `PutTableBucketReplication`/
`GetTableBucketReplication`/`PutTableReplication`/`GetTableReplication`/
`DeleteTable(Bucket)Replication` against `serializers.go`/`deserializers.go`
found the shape itself was fabricated, not just missing a field:
- The real `TableBucketReplicationConfiguration`/`TableReplicationConfiguration`
  is `{role, rules: [{destinations: [{destinationTableBucketARN}]}]}` --
  gopherstack modeled a flat `{destinations: [{destinationBucketARN}]}` with
  no `role`/`rules` nesting at all, and `destinationBucketARN` is an
  invented field name (the real one is `destinationTableBucketARN`).
- `GetTableBucketReplicationOutput` is `{configuration, versionToken}` (both
  required) -- gopherstack returned `{tableBucketARN, replicationConfiguration:
  {destinations}}`, an entirely invented top-level shape with a hardcoded
  empty `versionToken` on the table-level `GetTableReplication` sibling.
- `PutTableBucketReplicationOutput`/`PutTableReplicationOutput` both require
  `{status, versionToken}` -- gopherstack returned an empty 204 body,
  silently dropping two required response members.
- `DeleteTableReplicationInput.VersionToken` is a *required* input member
  (delete-time optimistic-concurrency check) -- gopherstack ignored it
  entirely; a delete always succeeded regardless of token.
Fixed by replacing the ad-hoc `map[string]any` config storage with typed
`BucketReplicationConfig`/`TableReplicationConfig` (role + `[]ReplicationRule`
+ `VersionToken`), backed by a new `*store.Table[TableReplicationConfig]`
(mirroring the existing `bucketReplication`/`tableRecordExpiry` off-registry
DTO pattern in persistence.go) replacing the old `tableReplication
map[string]bool` + `tableReplicationConfigs map[string]map[string]any]` pair.
Bumped `s3tablesSnapshotVersion` 1 -> 2 since the persisted shape changed.

### GetTableRecordExpirationConfiguration had the same fabricated-top-level-shape bug, plus enum casing
`GetTableRecordExpirationConfigurationOutput` is `{configuration: {status,
settings: {days}}}` with no top-level `tableARN` field at all --
gopherstack returned `{tableARN, status}`. Also, `TableRecordExpirationStatus`
and `TableRecordExpirationJobStatus` are lowercase/mixed-case smithy enums
(`"enabled"`/`"disabled"`; `"NotYetRun"`/`"Successful"`/`"Failed"`/`"Disabled"`)
-- gopherstack used invented values (`"ENABLED"`/`"DISABLED"`, and a
hardcoded `"SUCCEEDED"` for job status that matches no real enum value at
all). Fixed: `TableRecordExpiryConfig` gained a `Days` field (previously
`settings.days` was accepted on the wire and silently discarded), the
default/normalized status is now the real lowercase wire value, and
`GetTableRecordExpirationJobStatus` reports `NotYetRun`/`Disabled` based on
whether expiration is configured+enabled (this backend runs no background
jobs, so nothing has ever "run").

### GetTableMaintenanceJobStatus's status map was always empty
`GetTableMaintenanceJobStatusOutput.Status` is a map keyed by maintenance
type (`icebergCompaction`/`icebergSnapshotManagement`), one entry per type
actually configured via `PutTableMaintenanceConfiguration` -- gopherstack
returned `{}` unconditionally regardless of configuration. Fixed to report
one entry per configured type with the real `JobStatus` enum's
`"Not_Yet_Run"` value (note the different casing convention from
`TableRecordExpirationJobStatus` above -- confirmed via `enums.go`, not
assumed).

### FIXED this pass (gopherstack-spp4): table bucket / namespace / table naming rules
A prior pass confirmed via AWS documentation (not guessed) that real S3
Tables enforces naming rules server-side that the `aws-sdk-go-v2` client does
NOT pre-validate (its generated `validateOpCreateTableBucketInput` etc. only
check required-ness) -- so an invalid name really did reach this emulator
unrejected. That pass attempted bucket-name validation, found it broke ~10
existing test files that build bucket names from `t.Name()`-derived strings
containing underscores, and reverted rather than doing the fixture migration.

This pass did the migration instead of deferring it again. `validation.go`
adds `validateBucketName`/`validateTableOrNamespaceName`/
`validateNamespaceParts`, wired into `CreateTableBucket`/`CreateNamespace`/
`CreateTable` respectively:
- Bucket names: 3-63 chars, lowercase+digits+hyphens only, must begin/end
  with a letter or number, no underscore/period, reserved prefix denylist
  (`xn--`, `sthree-`, `amzn-s3-demo-`, `aws`) and reserved suffix denylist
  (`-s3alias`, `--ol-s3`, `--x-s3`, `--table-s3`).
- Namespace/table names: 1-255 chars, lowercase+digits+underscores ONLY (no
  hyphens/periods), must begin with a letter or number. Namespaces
  additionally reject the reserved `aws` prefix; table names do not have
  this restriction (confirmed via the docs -- it's namespace-only).
- Real error code is `BadRequestException` (new `ErrInvalidBucketName`/
  `ErrInvalidName` sentinels) -- there is no dedicated naming-violation
  exception type in `types/errors.go`, so this reuses the generic
  client-error exception, same as `ErrInvalidTableMetadataLocation`.

Every hyphenated namespace/table fixture literal (`"acme-ns"`, `"test-ns"`,
`"my-ns"`, `"maint-ns"`/`"maint-table"`, `"enc-ns"`/`"enc-table"`,
`"rename-ns"`, `"policy-ns"`/`"policy-table"`, `"encoded-ns"`/
`"encoded-table"`, `"opts-table"`, etc.) across
`handler_namespaces_test.go`, `handler_table_maintenance_test.go`,
`handler_tables_test.go`, `persistence_test.go`, `tables_test.go`,
`table_buckets_test.go`, and `test/integration/s3tables_test.go` was renamed
to the real underscore-only convention. Every bucket name built by
concatenating a table-test case's (underscore-separated, sometimes
mixed-case) name onto a prefix now runs through a new `bucketSuffix()` helper
(`handler_test.go`) that lowercases and swaps `_` for `-`; two subtest names
were also shortened where prefix+name would have exceeded the real 63-char
bucket-name limit (`handler_table_bucket_config_test.go`).

New table tests prove the rule itself, not just that fixtures still pass:
`TestBackend_CreateTableBucket_NameValidation`,
`TestBackend_CreateNamespace_NameValidation`,
`TestBackend_CreateTable_NameValidation` (each in the corresponding existing
`_test.go` file) cover length bounds, character-class violations, reserved
prefixes/suffixes, and the namespace-only `aws`-prefix rule.

## gopherstack-o7gx (2026-08-22): ReadBody-failure path wrote untyped errors

`Handler()`'s `httputils.ReadBody` failure branch wrote a bare
`c.String(http.StatusInternalServerError, "internal server error")`.
s3tables is restjson1 (confirmed from `s3tables@v1.18.4` deserializers.go's
`awsRestjson1_deserializeOpError*` prefix); its client-side decoder
JSON-decodes the body, so plain text doesn't decode -- a real client got
`*json.SyntaxError`, not even `UnknownError`.

Fixed by routing the ReadBody error through this handler's own
`handleError(c, err)`: none of its typed `case`s
(`awserr.ErrNotFound`/`ErrConflict`/`ErrInvalidParameter`,
`errInvalidRequest`, `errUnknownPath`) match a `*http.MaxBytesError`/read
error, so it falls through to the pre-existing default (`errType =
"InternalError"`, 500, `x-amzn-errortype` header set).

NOTE (pre-existing, NOT fixed by this pass, out of this ticket's scope):
that default's `"InternalError"` does not match
`s3tables@v1.18.4` `types/errors.go`'s modeled
`InternalServerErrorException` (line 124) -- a possible separate wire-type
mismatch in the genuine per-operation error path, distinct from the
ReadBody-failure defect this fix addresses. Flagging for a future pass
rather than changing it here, since it's the service's existing default for
every unmatched backend error, not something introduced by this fix.

Proven with a real `aws-sdk-go-v2/service/s3tables` client's
`CreateTableBucket`, whose `Name` field alone exceeds
`httputils.MaxRequestBodyBytes` (16 MiB).
`TestHandler_OversizedBodySurfacesInternalError`
(`handler_oversized_body_test.go`) asserts `apiErr.ErrorCode() ==
"InternalError"`; confirmed it fails pre-fix with `*json.SyntaxError`
(hand-reverted, byte-identical restore after).

## gopherstack-o7gx follow-up (2026-08-22): default error path fixed to InternalServerErrorException

The NOTE above flagged `handleError`'s default `errType = "InternalError"` as
a possible mismatch. Confirmed: `s3tables@v1.18.4` `types/errors.go:116-139`
models `InternalServerErrorException` (`ErrorFault: FaultServer`) as the
service's 5xx fault, and it is wired into all 49 of 49 operation error
switches in `deserializers.go` (`awsRestjson1_deserializeOpError*`, e.g.
`CreateTableBucket`'s at line ~125) -- universal, not just a majority.
`"InternalError"` appears nowhere in either file, so a real client's
`errors.As(&types.InternalServerErrorException{})` never matched.

Fixed `handler.go`'s `handleError` default to `errType =
"InternalServerErrorException"`. `TestHandler_OversizedBodySurfacesInternalError`
(`handler_oversized_body_test.go`) now additionally asserts
`errors.As(err, &types.InternalServerErrorException{})` and
`ErrorFault() == smithy.FaultServer`; confirmed it fails pre-fix with the
old `"InternalError"` code (hand-reverted, byte-identical restore after).

## gopherstack-wla0 (2026-08-23): GetTable/ListTables/GetNamespace/ListNamespaces wrote the wrong wire key entirely; GetTableBucket/ListTableBuckets never populated the real one

**Verified real, not stale.** The bd issue's own claim (GetTable/ListTables
write `"tableBucketARN"`; the real deserializer only has `"tableBucketId"`)
was re-confirmed directly against `s3tables@v1.18.4`'s `deserializers.go`:
`awsRestjson1_deserializeOpDocumentGetTableOutput`'s case list is
`createdAt, createdBy, format, managedByService, managedTableInformation,
metadataLocation, modifiedAt, modifiedBy, name, namespace, namespaceId,
ownerAccountId, tableARN, tableBucketId, type, versionToken,
warehouseLocation` -- no `tableBucketARN`. Same for
`awsRestjson1_deserializeDocumentTableSummary` (`ListTables` items).
`services/s3tables/handler_tables.go` (pre-fix, lines ~194/~254) wrote
`keyTableBucketARN: table.TableBucketARN` in both. Every real client's
`GetTableOutput.TableBucketId`/`TableSummary.TableBucketId` decoded `nil`
on every call.

**The bd issue undersold the scope.** It said `GetNamespace`/
`GetTableBucketStorageClass`/`ListNamespaces`/`UpdateTableMetadataLocation`
also write `tableBucketARN` but "their real response shapes have no
tableBucketId (or any bucket-identifying field) at all -- harmless extras,
not this bug." Re-checked each independently against
`deserializers.go`/`api_op_*.go`:

- `GetTableBucketStorageClassOutput` and `UpdateTableMetadataLocationOutput`
  genuinely have no `tableBucketARN`/`tableBucketId` member at all -- the
  bd issue was right about those two, they got the harmless-extra
  treatment (key dropped, nothing added back). `GetTableBucketStorageClass`
  additionally had its own, separate wire-shape bug (see below).
- `GetNamespaceOutput` and `NamespaceSummary` (`ListNamespaces` items) --
  **contrary to the bd issue** -- DO have a real `tableBucketId` member
  (confirmed via `awsRestjson1_deserializeOpDocumentGetNamespaceOutput`'s
  case list: `createdAt, createdBy, namespace, namespaceId,
  ownerAccountId, tableBucketId`). These two were in scope for the same
  wrong-key bug as `GetTable`/`ListTables`, not the harmless-extra
  category the bd issue put them in.

**Also found while touching `GetTableBucketStorageClass`**: its response
was a flat `{tableBucketARN, storageClass}`, but the real
`GetTableBucketStorageClassOutput` has a single required member,
`storageClassConfiguration` (nested `{storageClass}}` --
`StorageClassConfiguration` is a real 1-member struct, confirmed via
`awsRestjson1_deserializeDocumentStorageClassConfiguration`). A real
client's `StorageClassConfiguration` decoded `nil` on every call --
`PutTableBucketStorageClass`'s request-side parsing already correctly
handled the nested shape (`storageClassFromConfig(req.StorageClassConfiguration)`),
only the Get response was flat. Fixed both the nesting and the fabricated
key in the same change.

**Fix.** Added `TableBucket.BucketID string` (`models.go`), synthesized via
`uuid.NewString()` at `CreateTableBucket` time (`table_buckets.go`) --
the same established pattern this package already uses for
`Namespace.NamespaceID`/`TableBucket.MetricsConfigurationID`. Threaded into
`Namespace.TableBucketID`/`Table.TableBucketID` at `CreateNamespace`/
`CreateTable` time (both already look up the owning `TableBucket` for
other reasons, so no new lookup was needed). This is the same remediation
pattern `gopherstack-jcto` (directoryservice, a sibling case from the same
`gopherstack-zquj` sweep) describes as the correct way to close this bug
class: synthesize the real identifier once, at the resource's creation
time, and return it consistently thereafter -- not a placeholder value
invented at read time to make a test pass, which both issues correctly
warn against. `GetTableBucket`/`ListTableBuckets` now also emit
`tableBucketId` (previously omitted, not fabricated, since no ID existed
to source it from before this pass).

**Proof**: `TestSDKRoundTrip_TableBucketIDFix`
(`handler_sdk_roundtrip_test.go`) drives a real
`aws-sdk-go-v2/service/s3tables` client through `CreateTableBucket` ->
`GetTableBucket`/`ListTableBuckets` -> `CreateNamespace` ->
`GetNamespace`/`ListNamespaces` -> `CreateTable` -> `GetTable`/`ListTables`,
asserting every response's `TableBucketId` equals the bucket's own,
non-empty ID. Hand-reverted `GetTable`'s response back to
`keyTableBucketARN: table.TableBucketARN`: the test failed with
`expected: "80bf3a24-...", actual: ""` -- exactly the predicted symptom.
Restored, `md5sum`-confirmed byte-identical, re-ran green.

**Existing tests that asserted the old (bug-matching) shape** were
corrected, not deleted: `TestHandler_GetNamespaceIncludesTableBucketArn` /
`TestHandler_ListNamespacesIncludesTableBucketArn`
(`handler_namespaces_test.go`) renamed to `...IncludesTableBucketId`, now
assert the real key's value and the fabricated key's absence;
`TestHandler_GetTableResponseUsesLowercaseArns` /
`TestHandler_ListTablesResponseUsesLowercaseArns`
(`handler_tables_test.go`) similarly updated (their ARN-casing assertions
were unaffected and left as-is); `TestHandler_GetTableBucketStorageClass`
(`handler_table_bucket_config_test.go`) and
`TestHandler_CreateTableBucket_AppliesEncryptionStorageClassAndTags`
(`handler_table_buckets_test.go`) updated to read the nested
`storageClassConfiguration.storageClass` instead of a flat `storageClass`.

**Persisted struct changed, no snapshot version bump**: `TableBucket`,
`Namespace`, and `Table` (`models.go`) each gained one new plain string
field (`BucketID`/`TableBucketID`/`TableBucketID`), additive-only --
`s3tablesSnapshotVersion` stays `2`, per this repo's standing rule that an
additive field never needs a version bump (`pkgs/persistence`'s
`TestSnapshotVersionGuard`, gopherstack-s8bk). **`pkgs/persistence/testdata/snapshot_inventory.json`
needs a `-update` refresh** to pick up the three new field names --
confirmed by running `TestSnapshotVersionGuard` locally, which fails with
"s3tables: backendSnapshot fields changed without a version bump; golden
is out of date, run with -update to refresh it (this is bookkeeping, not a
version-bump case...)" -- exactly the expected/required failure mode this
guard is designed to produce for an additive change. Not refreshed by this
pass per the session's constraint on shared goldens; the orchestrator
should run `go test ./pkgs/persistence/... -run TestSnapshotVersionGuard -update`
once.

**Gates**: `go build ./...`, `go vet ./services/s3tables/...`,
`gofmt -l services/s3tables/*.go` (clean), `go test -race -count=1
./services/s3tables/...` (green), `golangci-lint run ./services/s3tables/...`
(0 issues) all pass. No banned `nolint:cyclop/gocyclo/gocognit/funlen`
added.
