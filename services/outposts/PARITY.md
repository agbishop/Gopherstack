---
# PARITY MANIFEST SCHEMA — copy to services/<svc>/PARITY.md, fill, keep updated.
# Purpose: record audit state so the NEXT audit diffs the delta instead of rescanning.
# Re-audit protocol: `git diff <last_audit_commit>..HEAD -- services/<svc>/` for local drift,
# AND check the SDK module for ops added since sdk_version. Only audit changed/new surface;
# trust rows marked ok whose files are unchanged since last_audit_commit.
service: outposts
sdk_module: aws-sdk-go-v2/service/outposts@v1.66.1   # go.mod's actual pin at this audit (unchanged)
last_audit_commit: 4049cb9ee # HEAD immediately before this 2026-09-06 pass's own fix commit
last_audit_date: 2026-09-06
# 2026-09-06 pass: two filed issues (gopherstack-vsmv, gopherstack-glw7), one real fix, one
# confirmed-no-change verdict.
# (1) gopherstack-vsmv: renewals.go's renewalIdempotency (store.go) caches one CreateRenewalResult
#     per (Outpost, ClientToken) CreateRenewal call, keyed "outpostID::clientToken" (renewals.go:60),
#     and was never pruned when the Outpost was deleted -- confirmed the leak (DeleteOutpost had no
#     touch of renewalIdempotency at all). Unlike Orders/Quotes (checked this pass: DeleteOutpost does
#     NOT prune either -- they are genuinely retained as historical/FK-referenced records, matching
#     the issue's own reasoning), renewalIdempotency is a pure idempotency cache, not a historical
#     record, and its key is prefixed by the deleted Outpost's own ID (not merely a client-supplied
#     token with no Outpost linkage), so pruning by "o.ID + \"::\"" prefix match is exact -- no index
#     needed since the key already encodes the Outpost ID. Zero premature-eviction risk: CreateRenewal
#     (renewals.go:55-58) resolves idOrARN via resolveOutpostLocked BEFORE ever consulting the cache,
#     so a retried CreateRenewal against a deleted Outpost fails at notFoundError and never reaches
#     renewalIdempotency regardless of whether the entry was pruned -- the entries are provably
#     unreachable the instant the Outpost is gone, making this pure memory hygiene, not a behavior
#     change. Fixed: DeleteOutpost (outposts.go) now deletes every renewalIdempotency key with that
#     prefix. Regression: TestDeleteOutpost_PrunesRenewalIdempotencyCache
#     (renewal_idempotency_delete_cleanup_test.go), asserts the cache is empty post-delete via a new
#     export_test.go RenewalIdempotencyLenForTest() accessor -- fails before the fix (len=1), passes
#     after (len=0); confirmed by reverting the production change and re-running (test failure
#     pasted in the session report, fix then restored).
# (2) gopherstack-glw7: re-examined UpdateSiteAddress's doc comment
#     (api_op_UpdateSiteAddress.go:12-18) in full: "Updates the address of the specified site. You
#     can't update a site address if there is an order in progress. You must wait for the order to
#     complete or cancel the order. You can update the operating address before you place an order at
#     the site, or after all Outposts that belong to the site have been deactivated." A prior pass
#     (2026-08-07-ish, see the NOT-fixed note below) had already flagged this as an unresolved
#     ambiguity and deliberately declined to implement the stricter reading. CONFIRMED that judgment
#     this pass with new evidence the prior note did not cite: `grep -rni "deactiv"` over the ENTIRE
#     outposts@v1.66.1 SDK module (every .go file, api_op_*.go/types/*.go/deserializers.go/etc.)
#     matches exactly one line in the whole module -- api_op_UpdateSiteAddress.go:18 itself, the
#     sentence being interpreted. The word "deactivated" names no lifecycle state anywhere else in
#     this SDK: types.Outpost.LifeCycleStatus (types/types.go) is a bare *string documented only as
#     "The life cycle status." with no enum type backing it anywhere in types/enums.go (already noted
#     in consts.go, re-confirmed this pass), and this backend's own two life-cycle values (ACTIVE,
#     PENDING_DECOMMISSION, consts.go:25-26) are themselves an explicitly-marked unconfirmed choice,
#     not a real AWS-published state set. There is consequently no "deactivated" Outpost state
#     representable anywhere in this backend's model (an Outpost here is ACTIVE, PENDING_DECOMMISSION,
#     or deleted via DeleteOutpost -- it never reaches a third, still-existing "deactivated" terminal
#     state) or in the real SDK's own types to hang a new gate on. The error side gives no further
#     signal either: `awk "/deserializeOpErrorUpdateSiteAddress\(/,/^}/" deserializers.go | grep -oE
#     '"[A-Za-z0-9]+"'` -> UnknownError, AccessDeniedException, ConflictException,
#     InternalServerException, NotFoundException, ValidationException -- the same generic
#     ConflictException (types/errors.go:37, doc: "Updating or deleting this resource can cause an
#     inconsistent state.") already used for the existing order-in-progress check, with no
#     deactivation-specific code or ResourceType value to distinguish the two readings.
#     Verdict: NOT implemented, confirming the prior pass's judgment -- the second clause is
#     unrepresentable in this backend/SDK today, not merely unconfirmed. Implementing it would require
#     inventing what "deactivated" means (e.g. reusing PENDING_DECOMMISSION, which is explicitly
#     "pending" not "deactivated", or treating "all Outposts deleted" as the gate, which duplicates
#     DeleteSite's separate FK check under a different name) with zero SDK/doc backing for either
#     choice -- exactly the fabrication risk parity-principles warns against. What would actually
#     settle this: real AWS API behavior (a live UpdateSiteAddress call against a site with a
#     DELIVERED/COMPLETED order and a still-ACTIVE Outpost) or a future SDK/doc revision that names an
#     actual deactivated/decommissioned Outpost state. No code changed for this issue.
# 2026-09-05 concurrency/leak pass (last service in the 163-service campaign): found and fixed two
# real bugs, both in the disclosed-but-unfiled category this campaign specifically watches for.
# (1) The 2026-08-31 sweep's own "OUT-OF-CLASS OBSERVATION" note (below) had already found and
#     described this exact race but left it unfiled -- confirmed still present, reproduced with a
#     new concurrent UpdateOutpost/ListOutposts (and UpdateSite/ListSites) test under go test -race
#     (100% reproducible: races on Description/LifeCycleStatus/Name/SupportedHardwareType, all read
#     unlocked by toOutpostWire/toSiteWire off whatever pointer ListOutposts/ListSites returned,
#     while UpdateOutpost/UpdateSite/StartOutpostDecommission mutate the same live pointer under
#     lock). Fixed by cloning before returning, matching ListAssets/ListCapacityTasks/ListOrders'
#     existing documented convention -- outposts.go's ListOutposts and sites.go's ListSites now call
#     o.clone()/s.clone() (both pre-existing methods, already used by Get*/Update*). Regression:
#     TestListOutposts_ConcurrentUpdate_NoRace / TestListSites_ConcurrentUpdate_NoRace
#     (list_snapshot_race_test.go), reliably race-detector-positive before the fix, clean after.
# (2) New ghost-row leak, found by enumerating every store.Table field on InMemoryBackend against
#     every Delete* path per this campaign's standard method: DeleteOutpost cascades its seeded
#     Asset(s) but never touched runningInstances/runningInstancesByOutpost (capacity_ledger.go),
#     even though ConsumeCapacity keys every row by AssetID/OutpostID. DeleteOutpost only rejects
#     while a capacity task is REQUESTED, so a COMPLETED one (and any capacity/instances it
#     recorded) never blocked deletion -- an Outpost could be deleted while EC2 instances were
#     still recorded as running on its now-deleted Asset, and the row would only ever be removed if
#     services/ec2's TerminateInstances later happened to call ReleaseCapacity for that exact
#     instance ID. Unbounded otherwise, and it survives Restore (runningInstances is a registered
#     store.Table, included in every Snapshot). Fixed: DeleteOutpost now deletes every
#     runningInstancesByOutpost.Get(o.ID) row before deleting the Outpost itself. Regression:
#     TestDeleteOutpost_CleansRunningInstanceLedger (capacity_ledger_delete_cleanup_test.go),
#     asserts the row is gone from a post-delete Snapshot -- fails before the fix (row present),
#     passes after.
# NOT fixed, flagged as a genuine ambiguity rather than fabricated either way: UpdateSiteAddress's
# own doc comment ("You can update the operating address before you place an order at the site, or
# after all Outposts that belong to the site have been deactivated") reads as possibly stricter
# than this backend's current check (siteHasInProgressOrderLocked: PREPARING/IN_PROGRESS only) --
# e.g. after an order reaches DELIVERED/COMPLETED with the Outpost still ACTIVE (not decommissioned),
# the doc's second paragraph could mean the operating address should still be locked. Could not
# confirm from the SDK alone whether this is an independent gate or just a paraphrase of the first
# paragraph (no order in progress); implementing the stricter reading without confirmation risks
# fabricating a rejection AWS doesn't actually perform. Left as-is (not gaps/structural_gaps, since
# this needs a bd issue to track investigating against real AWS behavior, not a decision this pass
# can make) -- see gopherstack-outposts-usa-ambiguity (file if not already).
# Re-verified all five audit dimensions this pass (see caller's report for detail); dimensions 1-4
# unchanged from the 2026-08-29/08-31 sweeps (re-spot-checked, not fully re-derived): AWS behavior
# compliance clean (CreateOrder's LineItems genuinely optional per validators.go/api_op doc --
# confirmed NOT a fabricated-required-field bug), tie-prone-sort clean (capacity_tasks.go:341 and
# orders.go:292 both sort by each resource's own unique ID, a total order).
# Raised to A this pass (gopherstack-b9mg). Closed both remaining buildable gaps the prior pass
# left open:
# (1) Order/CapacityTask lifecycle now transitions through the real SDK-declared intermediate
#     states -- Order: PREPARING -> IN_PROGRESS -> DELIVERED -> COMPLETED; CapacityTask:
#     REQUESTED -> IN_PROGRESS -> COMPLETED, or REQUESTED/IN_PROGRESS -> CANCELLATION_IN_PROGRESS
#     -> CANCELLED on CancelCapacityTask -- via chained pkgs/worker b.work.After calls (one hop
#     schedules the next from inside its own callback), the same pattern services/mgn's
#     exportimport.go scheduleExportLocked already uses for its Pending -> Started -> Succeeded
#     chain. LineItem.Status moves in lockstep at each hop (an invented but documented rollup
#     rule, since the SDK does not encode one). CancelOrder's cancellable window widened from
#     PREPARING-only to PREPARING-or-IN_PROGRESS (closes once DELIVERED); siteHasInProgressOrderLocked
#     (gates UpdateSiteAddress/UpdateSiteRackPhysicalProperties) now also checks IN_PROGRESS, not
#     just PREPARING, matching both ops' own doc comments' literal "order in progress"/"order of
#     IN_PROGRESS" language. WAITING_FOR_EVACUATION still never occurs -- StartCapacityTask's model
#     is additive-only (mergeInstanceTypeCapacity never shrinks InstanceTypeCapacities), so no
#     running instance can ever legitimately block a task; this is a separate, still-real gap (a
#     capacity-reduction path), not the "jumps straight to terminal" problem this pass closed --
#     see gaps. Proven via unit tests (require.Eventually, no unbubbled sleeps) including two
#     snapshot/restore-mid-flight tests proving an intermediate status round-trips, and new
#     test/integration/outposts_test.go subtests driving the real SDK client through each
#     intermediate state.
# (2) quotes.go's buildOrderingRequirements now evaluates 12 of the 17 real OrderingRequirementType
#     checks (up from 2), added as ordering_requirements.go: OUTPOST_NOT_FOUND_ERROR (distinct from
#     OUTPOST_ID_MISSING_ON_QUOTE_ERROR -- fires when an OutpostID is set but the Outpost was
#     deleted after association, real reachable state since DeleteOutpost has no FK check against
#     Quotes), OUTPOST_RENEWAL_REQUIRED_ERROR (reads Outpost.ContractEndDate, now also set at order
#     *completion* time from the order's own PaymentTerm via orders.go's
#     recordOriginalSubscriptionLocked, not just CreateRenewal -- otherwise this check could almost
#     never fire), OPERATING_ADDRESS_EXISTENCE_CHECK_ERROR, SHIPPING_ADDRESS_EXISTENCE_CHECK_ERROR,
#     COUNTRY_CODE_MISMATCH_CHECK_ERROR (quote CountryCode vs Site.OperatingAddress.CountryCode),
#     VALID_ZIP_CODE_CHECK_ERROR (US-only format check -- see structural_gaps for why other
#     countries stay EXEMPT), RACK_PHYSICAL_PROPERTIES_CHECK_ERROR (only applies to a RACK-type
#     Outpost), and the three SHIPPING_ADDRESS_MISSING_CONTACT_* checks. The other 5
#     (MAXIMUM_ALLOWED_ORDERS_CHECK_ERROR, OUTPOST_GENERATION_MISMATCH_ERROR, UNSUPPORTED,
#     ENTERPRISE_SUPPORT_ERROR, OUTPOST_STATE_CHANGED_ERROR) are not produced -- see structural_gaps
#     and gaps for the individual reasoning behind each. Proven via an in-package white-box table
#     test (ordering_requirements_test.go, exempted from testpackage in .golangci.yml: several
#     cases need a partially-populated Address the real SDK client's own validators.go refuses to
#     construct) plus SDK-driven round-trip tests for every check reachable through the real
#     client.
# 2026-08-31 value-semantics sweep (gopherstack-uox6): audited every filter-typed
# field across all 10 List* input structs with a filter (~20 filter fields by
# this pass's own count: ListAssetInstances 4, ListAssets 3, ListCapacityTasks 2,
# ListCatalogItems 3, ListOrderableInstanceTypes 1, ListOrders 1, ListOutposts 3,
# ListSites 3; ListBlockingInstancesForCapacityTask/ListQuotes/ListTagsForResource
# take none). covledger reported no filter_default_semantics row for this
# service (its only row is request_field_never_read, clean, b94d74fe6); no
# prior PARITY.md entry or commit on this specific axis found. ZERO BUGS,
# ZERO CODE CHANGED for this class. Every query-bound filter's key casing
# verified PascalCase against its own op's serializers.go httpBindings
# function (ListAssets/ListAssetInstances/ListCapacityTasks/ListCatalogItems/
# ListOrderableInstanceTypes/ListSites all confirmed byte-for-byte); every
# enum-typed filter compares against the same enum its doc comment names
# (CapacityTaskStatus, LifeCycleStatus -- confirmed to have NO SDK enum type
# at all, a bare *string, so no wrong-enum risk exists there); every
# MaxResults doc comment across all 12 MaxResults-bearing ops states no
# specific number ("The maximum page size." only), so the uniform
# defaultPageLimit=100 violates nothing (same clean verdict as mgn, checked
# same pass); no switch-over-filter-name shape anywhere in this service's
# filter logic. ListAssetInstances' AwsServiceFilter compares every stored
# runningInstance against a single hardcoded "EC2" constant rather than a
# per-instance field -- confirmed NOT a bug: capacity_ledger.go's own doc
# comment states runningInstance is populated exclusively by services/ec2's
# RunInstances (the only cross-service capacity consumer this repo wires),
# so there is no second AWSServiceName value this backend could ever store;
# a per-record field would be dead weight. OUT-OF-CLASS OBSERVATION at the time (different bug
# class, outside that pass's scope), FIXED in the 2026-09-05 pass above: ListOutposts and ListSites
# returned live backend-owned *Outpost/*Site pointers without cloning, unlike every other listing
# in this service -- see the frontmatter's 2026-09-05 entry for the confirmed data race and fix.
overall: A
# Per-op or per-op-family status. Values: ok | partial | gap | deferred.
# wire=response/request shape vs SDK; errors=code+HTTP status; state=real mutate/read; persist=in backendSnapshot.
# All 43 ops are routed, backed by real state, and persisted via InMemoryBackend.Snapshot/Restore
# (persistence.go). "partial" below marks operations where a genuinely unknowable input (no SDK
# enum, no public AWS data) forced a documented, narrower-than-real-AWS behavior -- not a stub.
ops:
  CreateOutpost: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /outposts (outposts.go); seeds one COMPUTE Asset (assets.go); LifeCycleStatus set to ACTIVE immediately -- see structural_gaps; enforces the real 10-Outposts-per-site quota (ServiceQuotaExceededException) as of this pass"}
  GetOutpost: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /outposts/{OutpostId}, id-or-ARN via resolveOutpostLocked (resolve.go)"}
  DeleteOutpost: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /outposts/{OutpostId}; Conflict while a REQUESTED capacity task exists; cascades its seeded Asset(s), its runningInstances rows, and (as of the 2026-09-06 pass, gopherstack-vsmv) its renewalIdempotency cache entries -- Orders/Quotes are deliberately NOT pruned here, they remain historical/FK-referenced records"}
  UpdateOutpost: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH /outposts/{OutpostId}; merges Description/Name/SupportedHardwareType onto existing state"}
  ListOutposts: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /outposts; AvailabilityZoneFilter/AvailabilityZoneIdFilter/LifeCycleStatusFilter all as repeated PascalCase query params (confirmed via serializers.go, NOT lowerCamel like grafana -- see wire.go), paginated via pkgs/page; clones before returning as of the 2026-09-05 pass (was racing UpdateOutpost/StartOutpostDecommission)"}
  StartOutpostDecommission: {wire: ok, errors: ok, state: partial, persist: ok, note: "POST /outposts/{OutpostIdentifier}/decommission; SKIPPED on idempotent replay, REQUESTED otherwise, BLOCKED never occurs (no cross-service blocking-resource data -- see gaps); ValidateOnly performs no mutation"}
  GetOutpostBillingInformation: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /outpost/{OutpostIdentifier}/billing-information (singular path, routed correctly -- see Grade note); accumulates ORIGINAL subscription on order completion (orders.go) and RENEWAL on CreateRenewal (renewals.go)"}
  GetOutpostInstanceTypes: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /outposts/{OutpostId}/instanceTypes; aggregates the CONFIGURED capacity across the Outpost's Assets (mutated by StartCapacityTask completion, and by capacity_ledger.go's ConsumeCapacity/ReleaseCapacity as services/ec2 launches/terminates instances onto it as of this pass -- a fully-depleted instance type drops out of the list), distinct from GetOutpostSupportedInstanceTypes"}
  GetOutpostSupportedInstanceTypes: {wire: ok, errors: ok, state: partial, persist: n/a, note: "GET /outposts/{OutpostIdentifier}/supportedInstanceTypes; returns the static seed catalog filtered by hardware type -- AssetId/OrderId are validated to exist but do not further filter the result (documented simplification, see gaps)"}
  GetRenewalPricing: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /outpost/{OutpostIdentifier}/renewal-pricing (singular path, routed correctly); PRICED for an ACTIVE Outpost, UNABLE_TO_PRICE otherwise"}
  CreateRenewal: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /renewals; ClientToken idempotency implemented (renewals.go's renewalIdempotency cache); pricing is a documented synthetic placeholder formula -- see gaps"}
  CreateSite: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /sites; OperatingAddress flattened to 3 fields on output, ShippingAddress fully stored but only surfaced via GetSiteAddress; enforces the real 100-sites-per-Region quota (ServiceQuotaExceededException) as of this pass"}
  GetSite: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /sites/{SiteId}, id-or-ARN"}
  UpdateSite: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH /sites/{SiteId}; merges Description/Name/Notes"}
  DeleteSite: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /sites/{SiteId}; Conflict while any Outpost still references it"}
  ListSites: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /sites; city/countryCode/stateOrRegion filters against OperatingAddress; clones before returning as of the 2026-09-05 pass (was racing UpdateSite/UpdateSiteAddress/UpdateSiteRackPhysicalProperties)"}
  GetSiteAddress: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /sites/{SiteId}/address; AddressType as query param, returns Shipping or Operating full Address"}
  UpdateSiteAddress: {wire: ok, errors: ok, state: ok, persist: ok, note: "PUT /sites/{SiteId}/address; full replacement (not merge); Conflict while the Site has a PREPARING order"}
  UpdateSiteRackPhysicalProperties: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH /sites/{SiteId}/rackPhysicalProperties; merges only non-empty fields; same in-progress-order Conflict check"}
  CreateOrder: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /orders; OrderType always OUTPOST (CreateOrderInput has no OrderType member); multi-hop PREPARING -> IN_PROGRESS -> DELIVERED -> COMPLETED transition as of this pass (orders.go's scheduleOrderCompletion), LineItems move in lockstep; validates CatalogItemId and consumed Quote; QuoteIdentifier now resolves id-or-ARN, see GetQuote; completion now also sets Outpost.ContractEndDate from PaymentTerm"}
  GetOrder: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /orders/{OrderId}, ID-only (no ARN form on this op)"}
  CancelOrder: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /orders/{OrderId}/cancel; Conflict once DELIVERED or terminal -- window widened this pass from PREPARING-only to PREPARING-or-IN_PROGRESS, now that IN_PROGRESS is a real reachable state"}
  ListOrders: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /list-orders; OutpostIdentifierFilter singular, paginated"}
  CreateQuote: {wire: ok, errors: ok, state: partial, persist: ok, note: "POST /quotes; single synthesized QuoteOption (not an N-option combinatorial shape, unrelated to this pass); OrderingRequirements now covers 12 of 17 real check types (up from 2) -- see ordering_requirements.go and structural_gaps/gaps for the other 5; pricing is a documented synthetic formula"}
  GetQuote: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /quotes/{QuoteIdentifier}; lazily flips CREATED -> EXPIRED past ExpirationDate; QuoteIdentifier now resolves id-or-ARN via resolveQuoteLocked (this pass fixed a real bug -- the prior audit's 'Quotes have no ARN form' note was wrong, GetQuoteInput's own Pattern confirms an ARN-shaped form)"}
  UpdateQuote: {wire: ok, errors: ok, state: ok, persist: ok, note: "PATCH /quotes/{QuoteIdentifier}; OutpostIdentifier tri-state (nil=no-change, empty=clear, value=set) implemented via *string wire field; never returns Conflict (none in this op's wire error set); QuoteIdentifier now resolves id-or-ARN, see GetQuote"}
  DeleteQuote: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /quotes/{QuoteIdentifier}; QuoteIdentifier now resolves id-or-ARN, see GetQuote"}
  ListQuotes: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /quotes; no filters, paginated; lazily expires each"}
  CancelCapacityTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /outposts/{OutpostIdentifier}/capacity/{CapacityTaskId}; as of this pass transitions REQUESTED/IN_PROGRESS -> CANCELLATION_IN_PROGRESS -> CANCELLED (async, see scheduleCapacityTaskCancellation), the real transient state the SDK declares"}
  GetCapacityTask: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET .../capacity/{CapacityTaskId}"}
  StartCapacityTask: {wire: ok, errors: ok, state: partial, persist: ok, note: "POST /outposts/{OutpostIdentifier}/capacity; enforces one-active-task-per-(Outpost,Order) (now also matching IN_PROGRESS, not just REQUESTED); multi-hop REQUESTED -> IN_PROGRESS -> COMPLETED as of this pass mutates the target Asset's real capacity ledger only at COMPLETED; WAITING_FOR_EVACUATION never occurs -- StartCapacityTask's own model is additive-only (mergeInstanceTypeCapacity never shrinks InstanceTypeCapacities), a separate real gap (see gaps), not the single-hop problem this pass closed; DryRun completes synchronously without mutating capacity"}
  ListCapacityTasks: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /capacity/tasks; status + OutpostIdentifierFilter"}
  ListBlockingInstancesForCapacityTask: {wire: ok, errors: ok, state: partial, persist: n/a, note: "GET .../blockingInstances; validates the capacity task exists, always returns empty. As of this pass real EC2-on-Outposts instance data DOES exist (capacity_ledger.go's runningInstances, see ListAssetInstances) but this op answers a narrower question -- instances blocking a capacity REDUCTION -- and StartCapacityTask's model is additive-only (mergeInstanceTypeCapacity only ever grows InstanceTypeCapacities, never shrinks), so no running instance can ever legitimately block a task in this backend; empty remains the honest answer, not a stub, see gaps"}
  ListAssets: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /outposts/{OutpostIdentifier}/assets; filters by AssetTypeFilter/HostIdFilter/StatusFilter against the seeded Asset(s)"}
  ListAssetInstances: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET .../assetInstances; returns real running-instance data as of this pass (InstanceId/InstanceType/AssetId/AccountId/AwsServiceName=EC2), recorded by capacity_ledger.go's ConsumeCapacity when services/ec2's RunInstances launches onto this Outpost -- AccountIdFilter/AssetIdFilter/AwsServiceFilter/InstanceTypeFilter all wired (repeated PascalCase query params, confirmed via serializers.go)"}
  GetCatalogItem: {wire: ok, errors: ok, state: partial, persist: n/a, note: "GET /catalog/item/{CatalogItemId}; served from seed_data.go's static 3-item catalog, not real AWS data -- see gaps"}
  ListCatalogItems: {wire: ok, errors: ok, state: partial, persist: n/a, note: "GET /catalog/items; ItemClass/EC2Family/SupportedStorage filters over the same static seed"}
  ListOrderableInstanceTypes: {wire: ok, errors: ok, state: partial, persist: n/a, note: "GET /instanceTypes; static 5-entry seed (seed_data.go), also backs GetOutpostInstanceTypes'/GetOutpostSupportedInstanceTypes' VCPU lookups"}
  GetConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "GET /connections/{ConnectionId}; synthetic (non-cryptographic) key/tunnel-address placeholders -- see gaps"}
  StartConnection: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /connections; validates AssetId exists"}
  ListTagsForResource: {wire: ok, errors: ok, state: ok, persist: n/a, note: "GET /tags/{ResourceArn}; resolves to Outpost or Site by ARN resource-segment marker"}
  TagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "POST /tags/{ResourceArn}"}
  UntagResource: {wire: ok, errors: ok, state: ok, persist: ok, note: "DELETE /tags/{ResourceArn}; TagKeys via repeated lowerCamel ?tagKeys= query param (the one exception to this service's PascalCase query casing, confirmed via serializers.go:3235)"}
# Families audited as a group (when per-op is impractical):
families:
  tagging: {status: ok, note: "TagResource/UntagResource/ListTagsForResource wired into cli.go's wireResourceGroupsTagging via wireTaggingOutposts, the 31st service. Both Outpost.Tags and Site.Tags share one ARN-keyed store (tagging.go's resolveTaggableLocked), resourceTypeFromARN derives outposts:outpost vs outposts:site per-ARN since this is a two-resource-kind tag store (unlike Grafana's single-kind constantResourceType)."}
  route-matcher: {status: ok, note: "handler.go's routeRequest uses a map-of-topLevelRouteFunc keyed by first path segment (kept cyclomatic complexity low without a nolint) rather than one large switch; RouteMatcher prefixes on all 12 top-level path segments; MatchPriority = PriorityPathVersioned"}
gaps:
  - "ListBlockingInstancesForCapacityTask always returns an empty result after validating the capacity task exists, and WAITING_FOR_EVACUATION (CapacityTaskStatus) never occurs. Real EC2-on-Outposts instance data now exists (capacity_ledger.go's runningInstances, populated by services/ec2's RunInstances -- see ListAssetInstances), but this op only has meaning for a capacity REDUCTION a running instance would block, and StartCapacityTask's model here is additive-only (mergeInstanceTypeCapacity never shrinks InstanceTypeCapacities). Buildable if a real capacity-reduction path is ever added; empty/never-WAITING_FOR_EVACUATION remain the honest answers today, not a stub -- this is a separate, still-open gap from the single-hop lifecycle problem this pass closed."
  - "MAXIMUM_ALLOWED_ORDERS_CHECK_ERROR: reclassified to structural_gaps this pass after confirming (docs.aws.amazon.com/outposts/latest/userguide/outposts-limits.html, fetched directly) AWS publishes only two Outposts quotas -- Outpost sites per Region, Outposts per site -- and no orders-related quota at all, matching this document's existing ServiceQuotaExceededException/CreateOrder note. Not merely 'not attempted': actively checked and found no data source, so it moved out of gaps -- see structural_gaps."
  - "UNSUPPORTED and OUTPOST_STATE_CHANGED_ERROR (OrderingRequirementType) are not produced. UNSUPPORTED is SDK's own literal string with no _CHECK_ERROR suffix like every other member -- a generic catch-all with no documented trigger condition anywhere (SDK, docs) to derive from. OUTPOST_STATE_CHANGED_ERROR would need a 'changed relative to what reference point' concept the SDK never specifies, and OUTPOST_ACTIVE_CHECK_ERROR already covers 'is the Outpost currently active' -- inventing an unspecified snapshot-comparison mechanism for this one is exactly the kind of fabricated behavior parity-principles warns against, not a genuine data-source gap, so both stay in gaps rather than structural_gaps."
structural_gaps:
  - "LifeCycleStatus (types.Outpost.LifeCycleStatus is a bare *string) has NO SDK enum type anywhere in this module (confirmed by direct grep of types/enums.go -- zero LifeCycleStatus-named type exists) and the AWS API docs (API_Outpost.html) publish only a generic non-empty-string Pattern, no value set. Unlike the other gaps above, there is no more SDK/doc source to converge on even in principle: ACTIVE on CreateOutpost and PENDING_DECOMMISSION on StartOutpostDecommission success (consts.go) are this implementation's own defensible choice, and will remain so regardless of future effort unless AWS itself publishes an enum."
  - "ListCatalogItems/GetCatalogItem/ListOrderableInstanceTypes are served from a small static seed table (seed_data.go: 3 catalog items, 5 orderable instance types) standing in for AWS's own published, centrally-maintained hardware catalog and pricing model. This is proprietary AWS operational/billing data (which rack/server SKUs are currently orderable, real subscription pricing) with no public machine-readable source anywhere -- not in the SDK, not in Terraform, not in AWS's docs. No amount of implementation effort in this emulator can produce the real values; this is the exact 'no billing/settlement system' case structural_gaps exists for. pricing.go's deterministic formula is the same case: real Outposts subscription pricing is not published data."
  - "Connection/StartConnection key material (ServerPublicKey, tunnel addresses, UnderlayIpAddress -- connections.go) is synthetic and non-cryptographic. Real values require an actual WireGuard cryptographic handshake with real AWS infrastructure during physical Outpost server installation (per both ops' own doc comments) -- there is no data source an emulator could read or compute this from; it is not a knowledge gap, it is a physical-hardware-install-time cryptographic exchange, the same class of thing structural_gaps' 'no physical hardware' clause covers."
  - "ServiceQuotaExceededException has no trigger path on CreateOrder specifically (CreateSite/CreateOutpost now enforce the two real published quotas -- see ops table), and OrderingRequirementType's MAXIMUM_ALLOWED_ORDERS_CHECK_ERROR (quotes.go's buildOrderingRequirements) is the same underlying gap surfaced a second way. No AWS-published default per-account or per-Outpost Order quota exists anywhere (confirmed directly against docs.aws.amazon.com/outposts/latest/userguide/outposts-limits.html this pass, which publishes only the Site and Outpost-per-site quotas) to enforce without fabricating a number, matching services/grafana's identical treatment of AccessDeniedException."
  - "OUTPOST_GENERATION_MISMATCH_ERROR (OrderingRequirementType) is not produced: types.Outpost (confirmed via types/types.go) carries NO generation field at all -- AvailabilityZone(Id)/Description/LifeCycleStatus/Name/OutpostArn/OutpostId/OwnerId/SiteArn/SiteId/SupportedHardwareType/Tags are the entire struct. OutpostGeneration only exists on OrderableInstanceType and ListOrderableInstanceTypesInput's own filter -- there is no field on the Outpost resource itself this backend (or any emulator) could read to know 'this Outpost's generation' to compare against, unlike SupportedHardwareType which does exist and backs RACK_PHYSICAL_PROPERTIES_CHECK_ERROR."
  - "ENTERPRISE_SUPPORT_ERROR (OrderingRequirementType) is not produced: it requires an AWS Support-plan model (which plan the account subscribes to) this backend has no state for, matching services/grafana's identical treatment of AccessDeniedException and this document's own existing ServiceQuotaExceededException precedent."
  - "UpdateSiteAddress's doc comment second clause (\"or after all Outposts that belong to the site have been deactivated\", api_op_UpdateSiteAddress.go:17-18) is not enforced beyond the existing order-in-progress check (siteHasInProgressOrderLocked). gopherstack-glw7, re-examined 2026-09-06: confirmed with a whole-module grep that \"deactivated\" names no lifecycle state anywhere else in outposts@v1.66.1 -- types.Outpost.LifeCycleStatus has no SDK enum at all (see the LifeCycleStatus structural_gaps entry above), and this backend's own model has no third, still-existing \"deactivated\" Outpost state to gate on (an Outpost is ACTIVE, PENDING_DECOMMISSION, or deleted). Genuinely unrepresentable today, not merely unconfirmed -- see the 2026-09-06 frontmatter entry for the full evidence trail."
leaks: {status: clean, note: "InMemoryBackend.Reset() closes every Outpost's and Site's tags.Tags before clearing (store.go); Close() stops the worker.Group backing every scheduled Order/CapacityTask transition timer, now a 2-3-hop chain instead of one shot (mirrors services/grafana's scheduleWorkspaceActivation pattern the prior audit called out as the thing to watch for; services/mgn's exportimport.go chained-After pattern confirmed the same shape holds for a multi-hop chain, not just one hop). DeleteOutpost now also cleans up runningInstancesByOutpost rows as of the 2026-09-05 pass (was an unbounded ghost-row leak if an Outpost was deleted while EC2 instances were still recorded as running on its Asset)."}
---

## Full write-only-state and wire-shape re-sweep (2026-08-29)

This service had no `wire_field_fixes*_test.go`, yet carried a dated, detailed,
A-graded manifest -- the higher-risk pattern this campaign has previously found a real
bug hiding under (`servicediscovery`: no test file, confident "audited and confirmed
correct" note, real bug inside). A companion `ce` pass that had genuinely found and
fixed two write-only-state bugs (`AnomalyMonitor.MonitorSpecification`,
`AnomalySubscription.ThresholdExpression`) had also been assigned this service but was
cut off before starting it, so no prior pass in this campaign had actually re-verified
this manifest's claims. Confirmed protocol first (`awsRestjson1`, matching this file's
existing Notes section, cross-checked directly against `serializers.go`'s
`awsRestjson1_serializeOpHttpBindings*` function names).

**Primary method (write-only-state):** enumerated every field on every domain record in
`models.go` (Outpost, Site, Order, Quote, CapacityTask, Asset, Connection, and their
nested sub-structs) and traced each write path (`CreateOutpost`, `CreateSite`,
`CreateOrder`, `CreateQuote`, `StartCapacityTask`, `StartConnection` -- read directly from
`orders.go`/`quotes.go`/`capacity_tasks.go`/`connections.go`/`sites.go`) to confirm every
accepted request field is both stored and threaded onto the record, and every stored
field has a real read path (`GetOutpost`/`GetSite`/`GetOrder`/`GetQuote`/
`GetCapacityTask`/`GetConnection` or their List/Summary counterparts). No write-only
field found; the internal-only fields with no wire counterpart (`Outpost.PaymentOption`/
`PaymentTerm`/`ContractEndDate`/`Subscriptions`, `Site.ShippingAddress`) are all
documented, correct simplifications already noted in `models.go`'s doc comments (real
`types.Outpost`/`types.Site` genuinely have no such members; the data surfaces through
`GetOutpostBillingInformation`/`GetSiteAddress` instead, both verified below).

**Get/List/Describe wire-shape sweep:** field-diffed all 23 struct-or-list-returning ops'
real `*Output` types (`api_op_*.go`) and every nested `types.*` struct they reference
(`Outpost`, `Site`, `Order`/`OrderSummary`, `Quote`/`QuoteSummary`/`QuoteOption`/
`CapacitySummary`, `CapacityTaskSummary`, `AssetInfo`/`AssetLocation`/`ComputeAttributes`,
`AssetInstance`, `CatalogItem`, `DetailedInstanceTypeItem`, `ConnectionDetails`,
`InstanceTypeItem`, `PricingOption`/`PricingResult`, `Subscription`, `BlockingInstance`,
`LineItem`, `EC2Capacity`) directly against `wire.go`'s corresponding wire structs,
field-by-field, including nesting depth and Go type (confirmed timestamps are
epoch-seconds `float64` via `deserializers.go`'s `smithytime.ParseEpochSeconds` calls,
matching `wire.go`'s existing convention). Every field matched exactly -- no invented
members, no missing members, no wrong types. Also field-diffed 12 `Create`/`Update`
request `*Input` types against `wire.go`'s request structs (INVENTED MEMBER check on the
request side): exact match on all. Cross-checked `OrderStatus`/`PaymentOption`/
`TaskActionOnBlockingInstances`/`DecommissionRequestStatus`/`SupportedHardwareType`
constants in `consts.go` against `types/enums.go`: all real, correctly spelled values
(WRONG-ENUM-VALUES check clean).

**Tools:** `enumcheck`/`acceptguard`/`zeroguard`/`xmlitemwrap` (run repo-wide, no
per-service flag exists) produced zero findings anywhere under `services/outposts/`.

**Verdict: clean pass, not a skipped one.** No bug found in this service on this pass --
per this campaign's own rule against fabricating findings to justify a pass, this is
recorded as a genuine, actively-re-verified result rather than a stub of "already
audited, trust it." `last_audit_commit`/`last_audit_date` above updated to reflect this
re-verification even though no `services/outposts/*.go` file changed.

## Route table SDK diff (2026-08-13, gopherstack-jqh2 pass 3)

Re-extracted all 43 ops' real method+path directly from `outposts@v1.66.1`
serializers.go and drove them through `ExtractOperation` via the new
`handler_sdk_route_table_test.go` (`TestExtractOperation_SDKRouteTable`, one
subtest per op, `t.Parallel()`). All 43 resolved correctly, including two
real AWS API quirks confirmed directly in serializers.go: singular
`/outpost/{id}/billing-information` and `/outpost/{id}/renewal-pricing`
(every other outpost op uses plural `/outposts/{id}/...`), and the
standalone verb-prefixed `/list-orders` path for `ListOrders` (distinct
from `/orders`, which `CreateOrder` POSTs to and `GetOrder` GETs a specific
order from). No pre-existing table existed to check, and no new routing
bugs found. This test is now the permanent regression guard for
route-table drift.

## Lifecycle and OrderingRequirements pass (2026-08-07, gopherstack-b9mg)

Closed the two remaining buildable gaps the prior pass left open and explicitly declined to close
without raising the grade prematurely -- outposts was the last service below A.

**Gap 1 -- Order/CapacityTask single-hop lifecycle.** Read the real, non-deprecated enum values
straight from the pinned SDK's `types/enums.go` (`OrderStatus`: `PREPARING`/`IN_PROGRESS`/
`DELIVERED`/`COMPLETED`/`CANCELLED`/`ERROR`, plus five deprecated values this backend still never
produces; `CapacityTaskStatus`: `REQUESTED`/`IN_PROGRESS`/`FAILED`/`COMPLETED`/
`WAITING_FOR_EVACUATION`/`CANCELLATION_IN_PROGRESS`/`CANCELLED`) rather than inventing spellings.
Modeled genuine multi-hop transitions using the repo's existing chained-`b.work.After` idiom
(`services/mgn/exportimport.go`'s `scheduleExportLocked` chains `Pending -> Started -> Succeeded`
the same way; each hop's callback schedules the next, only if the resource is still in the status
that hop expects -- a concurrent `CancelOrder`/`CancelCapacityTask`, or a restart with no pending
timer, means it isn't, and the hop silently stops advancing rather than forcing a transition):

- `orders.go`'s `scheduleOrderCompletion` now chains `PREPARING -> IN_PROGRESS -> DELIVERED ->
  COMPLETED` (`orders.go`'s `advanceOrderStatusLocked`), with `LineItem.Status` moving in lockstep
  at each hop (`PREPARING`/`BUILDING`/`DELIVERED`/`INSTALLED`) -- an invented but documented rollup
  rule, since the SDK does not encode one (unchanged judgment call from the original implementation
  pass, just extended to more hops).
- `capacity_tasks.go`'s `scheduleCapacityTaskCompletion` now chains `REQUESTED -> IN_PROGRESS ->
  COMPLETED`; the capacity-ledger mutation (`mergeInstanceTypeCapacity`) only applies at the final
  `COMPLETED` hop, proven by a new test asserting `GetOutpostInstanceTypes` stays empty while
  `IN_PROGRESS`.
- `CancelCapacityTask` now moves to the transient `CANCELLATION_IN_PROGRESS` state and resolves to
  `CANCELLED` asynchronously (`scheduleCapacityTaskCancellation`) instead of the prior single-hop
  simplification straight to `CANCELLED`.
- `WAITING_FOR_EVACUATION` still never occurs -- `StartCapacityTask`'s own model is additive-only
  (`mergeInstanceTypeCapacity` only ever grows `InstanceTypeCapacities`, never shrinks), so no
  running instance can ever legitimately block a task in this backend. This is a separate, still-
  real gap (a capacity-*reduction* path, a materially bigger feature), not the "jumps straight to
  terminal" problem this pass was asked to close -- left open, see `gaps`.
- Two real correctness fixes this multi-hop change required, not scope creep: `CancelOrder`'s
  cancellable window widened from `PREPARING`-only to `PREPARING`-or-`IN_PROGRESS` (closes once
  `DELIVERED`, since real hardware is presumed already shipped by then); `siteHasInProgressOrderLocked`
  (gates `UpdateSiteAddress`/`UpdateSiteRackPhysicalProperties`) now also matches `IN_PROGRESS`, not
  just `PREPARING` -- both real ops' own doc comments say "order in progress"/"an order of
  `IN_PROGRESS`" literally, and leaving this unchanged would have silently broken once orders
  actually reached `IN_PROGRESS`. Also: order *completion* now sets `Outpost.ContractEndDate` from
  the order's own `PaymentTerm` (`recordOriginalSubscriptionLocked`, reusing `pricing.go`'s
  `termYears` -- previously only `CreateRenewal` ever set it), needed to give the new
  `OUTPOST_RENEWAL_REQUIRED_ERROR` check (Gap 2) real state to evaluate on the common path, not just
  after an explicit renewal.

**Proof**: unit tests converted to `require.Eventually` (no unbubbled sleeps) with tick well under
the per-hop delay so intermediate stops are actually observed, not skipped over --
`TestCreateOrder_LifecycleTransitions`, `TestStartCapacityTask_LifecycleTransitions`,
`TestCancelCapacityTask` (table-driven over requested/in-progress cancellation, asserting the
transient state before the async resolve), `TestCancelOrder_RejectedOnceDelivered`. Two new
snapshot/restore tests (`TestPersistence_SnapshotRestoreRoundTrip_MidFlightOrderTransition`/
`..._MidFlightCapacityTaskTransition`) prove an intermediate status -- not just the initial or
terminal one -- round-trips; `Restore` does not re-arm the pending timer (`worker.Group` timers are
never persisted, matching `services/grafana`'s identical behavior), so a restored mid-flight
resource stays parked rather than continuing to advance on its own, which is the expected,
documented behavior. `test/integration/outposts_test.go` gained SDK-driven subtests
(`transitions_through_real_intermediate_states`, `transitions_through_in_progress_before_completing`,
`cancel_pauses_at_cancellation_in_progress`) asserting the real intermediate states through the
genuine AWS SDK client, not just the terminal one.

**Gap 2 -- `buildOrderingRequirements` evaluating only 2 of 17 checks.** Read every
`OrderingRequirementType` value from the pinned SDK's `types/enums.go` and bucketed each of the 17
into implemented / structural / deferred-not-fabricated (see the frontmatter's `overall` note,
`gaps`, and `structural_gaps` for the full per-check reasoning) rather than treating this as one
undifferentiated block. New file `ordering_requirements.go` holds the 12 now-implemented checks as
small, independently-testable functions (`outpostIDMissingRequirement`,
`outpostNotFoundRequirement`, `outpostActiveRequirement`, `outpostRenewalRequiredRequirement`,
`operatingAddressExistenceRequirement`, `shippingAddressExistenceRequirement`,
`countryCodeMismatchRequirement`, `validZipCodeRequirement`, `rackPhysicalPropertiesRequirement`,
and the three `shippingAddressMissingContact*Requirement` functions), composed by
`buildOrderingRequirements` -- a flat slice literal, not a branchy assembler, keeping cyclomatic
complexity low without a `nolint`. `quotes.go`'s `CreateQuote`/`UpdateQuote` call a new
`buildOrderingRequirementsLocked` wrapper that resolves the Outpost's Site
(`siteForOutpostLocked`) and delegates to the pure function.

Real bug/gap found and fixed while building this: `OUTPOST_NOT_FOUND_ERROR` distinguishes "a
quote never had an `OutpostID`" (`OUTPOST_ID_MISSING_ON_QUOTE_ERROR`) from "an `OutpostID` is set
but the Outpost no longer exists" -- genuinely reachable state, since `DeleteOutpost` has no FK
check against `Quotes` (confirmed by reading `outposts.go`'s `DeleteOutpost` directly), so an
Outpost can be deleted out from under a still-live Quote. Proven by
`TestUpdateQuote_OutpostDeletedAfterAssociation` driving the real SDK client end to end
(`CreateQuote` -> `DeleteOutpost` -> `UpdateQuote` re-evaluates and observes the `FAIL`).

**Proof**: `ordering_requirements_test.go` (package `outposts`, white-box, exempted from
`testpackage` in `.golangci.yml` with a documented reason) is a table-driven unit test covering all
12 implemented checks and their `EXEMPT`/`PASS`/`FAIL` boundaries directly against hand-built
`Site`/`Outpost` structs -- necessary because several cases (the `SHIPPING_ADDRESS_MISSING_CONTACT_*`
checks) need a partially-populated `Address` the real SDK client's own `validators.go` refuses to
construct (every `Address` field is client-side required once `OperatingAddress`/`ShippingAddress`
is non-nil at all, confirmed by reading `validateAddress` in the pinned SDK). `quotes_test.go`'s
`TestCreateQuote_WithOutpost` and the new `TestUpdateQuote_OutpostDeletedAfterAssociation` prove the
subset reachable through the genuine SDK client end to end.

**Why this raises the grade to A**: both gaps the prior three passes explicitly left open as
"deferred, not unbuildable" are now closed to the extent they are genuinely buildable; everything
still not produced (`MAXIMUM_ALLOWED_ORDERS_CHECK_ERROR`, `OUTPOST_GENERATION_MISMATCH_ERROR`,
`UNSUPPORTED`, `ENTERPRISE_SUPPORT_ERROR`, `OUTPOST_STATE_CHANGED_ERROR`,
`WAITING_FOR_EVACUATION`) is recorded with an individual, checked-not-assumed justification in
`gaps`/`structural_gaps` rather than silently dropped or fabricated.

## EC2 capacity-coupling pass (2026-08-06, gopherstack-9ij1 + gopherstack-b9mg)

Closed the single highest-value gap the prior pass identified and explicitly could not build
without an `ec2`-side change: `services/ec2`'s `RunInstances` now really consumes this service's
Outposts capacity ledger, and `TerminateInstances` really returns it. This session owned both
`services/ec2` and `services/outposts`, unblocking the fix.

**`services/ec2` additions** (verified against the pinned `aws-sdk-go-v2/service/ec2@v1.319.1`
checkout, not assumed from the filed issue's guess):
- `Subnet.OutpostArn` (`store.go`), settable via `CreateSubnetWithOutpost` (a new method;
  `CreateSubnet` now delegates to it with `outpostArn=""` so none of the ~30 existing call sites,
  including `services/cloudformation`, needed to change). `CreateSubnetInput.OutpostArn` is a
  flat, top-level `*string` field (confirmed via `serializers.go`'s
  `awsEc2query_serializeOpDocumentCreateSubnetInput`) -- no nesting.
- `Instance.OutpostArn` (`store.go`), populated from the launch subnet at `RunInstances` time.
  **Correction to the filed issue's assumption**: this is a top-level field on `types.Instance`,
  a *sibling* of `Placement`, not `Placement.OutpostArn` -- `types.Placement` (the struct used by
  both `RunInstancesInput.Placement` and `Instance.Placement`) has **no** `OutpostArn` member at
  all (confirmed by reading the full `Placement` struct in `types/types.go`); the response XML
  deserializer reads `outpostArn` and `placement` as two separate elements
  (`awsEc2query_deserializeDocumentInstance`). Surfaced on `RunInstances` and `DescribeInstances`
  responses (`instanceItem.OutpostArn`, XML tag `outpostArn`, sibling of the `placement` element).

**Cross-service wiring** (`services/ec2/cross_service.go`, new file): `ec2` imports
`services/outposts` directly and resolves its handler lazily via `SetAppConfig`/`GetOutpostsHandler`
-- both already exist on `*CLI` from the prior `outposts` pass, so **no `cli.go` edit was needed**.
This is the mirror image of `services/grafana`'s/`services/mgn`'s cross-service direction (they read
`ec2`'s state passively); here `ec2`'s own `RunInstances`/`TerminateInstances` must synchronously
call into `outposts` as part of handling the EC2 request itself (validate-and-consume, or fail the
whole launch), the same pattern `services/mgn`'s `launchParticipantInstanceLocked` already uses to
call `ec2Bk.RunInstances` while holding its own lock -- there is real in-repo precedent for a
cross-service call made while the caller's own backend lock is held, so `RunInstances`'s existing
lock scope did not need restructuring. `outposts` was deliberately **not** made to import `ec2` in
the other direction (that would create an `ec2` <-> `outposts` import cycle, since `ec2` already
imports `outposts`) -- `outposts` instead keeps its own minimal `runningInstances` ledger
(`capacity_ledger.go`), populated by the very cross-service calls `ec2` makes into it, rather than
reading `ec2`'s `Instance` table directly.

**`services/outposts/capacity_ledger.go`** (new file): `ConsumeCapacity(outpostArn, instanceType,
accountID, instanceIDs)` atomically checks-then-decrements `Asset.ComputeAttributes.
InstanceTypeCapacities[].Count` for the Outpost's single seeded Asset (there is still no public
`CreateAsset` op, so multi-asset draining logic was deliberately not built -- see the prior pass's
"Asset seeding" note, unchanged) and records one `runningInstance` row per instance ID; `Count`
represents currently-available capacity, decremented by `ConsumeCapacity`/incremented by
`ReleaseCapacity`, distinct from `StartCapacityTask`'s unrelated (and still additive-only, see
gaps) mutation of the same field. `ReleaseCapacity(instanceID)` looks up and deletes that row,
crediting the unit back. Both return/no-op honestly when the Outposts backend isn't wired (unit
tests constructing `ec2.InMemoryBackend` directly) or the referenced Outpost/Asset no longer
exists, matching `services/grafana`'s established graceful-degradation convention for optional
cross-service backends.

**Errors, verified against the real SDK, not invented**: `aws-sdk-go-v2/service/ec2/types/errors.go`
declares no typed exception for either failure mode (EC2's query-protocol error model predates
smithy's typed-exception generation for most codes). `CreateSubnetWithOutpost` rejecting an unknown
`OutpostArn` maps to the generic `InvalidParameterValue` code, matching this file's existing
treatment of every other no-dedicated-code cross-reference failure (e.g. `ErrCoipCidrNotFound`).
`RunInstances` exceeding available capacity maps to `InsufficientInstanceCapacity`, the real,
well-known EC2 client error for capacity shortfalls (used for AZ/Capacity-Reservation/Outpost
capacity failures alike per AWS's own error-code documentation) -- not fabricated for this pass.

**Proof**: `test/integration/outposts_test.go` gained two new SDK-driven cases --
`TestIntegration_Outposts_EC2CapacityCoupling` drives the full loop through the *real* `aws-sdk-go-v2`
EC2 client end to end (create Outpost, configure capacity via `StartCapacityTask`, `CreateSubnet`
with the real `OutpostArn`, `RunInstances`, observe `GetOutpostInstanceTypes`/`ListAssetInstances`
reflect the drop, a second launch rejected with `InsufficientInstanceCapacity`,
`TerminateInstances`, observe capacity and the asset-instance listing both reverse, and the freed
unit consumable again) and `..._NonexistentOutpostArn` proves the `CreateSubnet`-time rejection.
Both ran green against the Docker container (`make build-linux` + the real test harness), alongside
unit-level coverage in both packages (`services/ec2/cross_service_test.go`,
`services/outposts/capacity_ledger_test.go`) for the permutation/error cases (insufficient capacity,
exact-capacity, unconfigured instance type, unknown Outpost, unwired-backend no-ops, filter
matching) that don't need the full container.

**Not raised to A**: see the frontmatter's grade note -- the Order/CapacityTask single-hop
lifecycle and the 15-of-17 unevaluated `OrderingRequirement` checks are unrelated, pre-existing,
still-buildable gaps this pass's task did not touch.

## Integration-test and gap-closure pass (2026-08-06)

Added `test/integration/outposts_test.go` (10 test funcs, real `aws-sdk-go-v2` client against the
Docker container) -- the first SDK-driven parity proof this service has had; the prior B was proven
only by unit tests + an in-process SDK round-trip harness, which parity-principles.md rule 3
excludes as parity evidence. Coverage: Site/Outpost CRUD + decommission + the seeded Asset, the
real 10-Outposts-per-site quota, catalog items + filters, Quote/Order lifecycle including the new
id-or-ARN Quote resolution, CapacityTask lifecycle including the real capacity-ledger mutation,
Connection lifecycle, tagging across both taggable resource kinds (Outpost and Site -- the exact
surface the repo-wide `/tags/` routing fix targeted), NotFoundException across every resource kind,
and semantic ValidationException cases the SDK's own client-side required-field checks can't
intercept (confirmed via reading `validators.go`: it only checks field presence, never enum content
or string length, so "invalid enum value"/"wrong length" cases are genuine server-side proof, while
"missing required field" cases are not -- the SDK rejects those before the request is ever sent).

Fixed real bugs found by reading `docs.aws.amazon.com/outposts/latest/APIReference/` directly
(never assumed from existing code or field names) for every ID this service generates:
- Outpost/Site/Order/Quote/CapacityTask/LineItem/QuoteOption IDs were all 12 lowercase hex
  characters; the real pattern for every one of them is exactly 17 (e.g. `Outpost.OutpostArn`:
  `^arn:aws([a-z-]+)?:outposts:[a-z\d-]+:\d{12}:outpost/op-[a-f0-9]{17}$`).
- Two wrong prefixes: `CapacityTaskId` was `ct-`, the real prefix is `cap-`
  (`API_GetCapacityTask.html`: `^cap-[a-f0-9]{17}$`); `LineItemId` was `li-`, the real prefix is
  `ooi-` (`API_LineItem.html`: `ooi-[a-f0-9]{17}`); `QuoteOptionId` was `qo-`, the real prefix is
  `oqo-` (`API_Order.html`'s `QuoteOptionIdentifier`: `^oqo-[a-f0-9]{17}$`).
- AssetId and ConnectionId both used a `-` in their generated form; their real patterns
  (`^(\w+)$` and `^[a-zA-Z0-9+/=]{1,1024}$` respectively, from `API_StartConnection.html`) do not
  allow `-` at all -- fixed to drop it.
- CatalogItem seed IDs (`cat-rack-m5` etc.) didn't match the real `OR-[A-Z0-9]{7}` pattern
  (`API_CatalogItem.html`) at all -- replaced with `OR-RACKM05`/`OR-RACKC05`/`OR-SRVC6ID`.
- Found and fixed a real bug, not just a format mismatch: `GetQuoteInput`/`UpdateQuoteInput`/
  `DeleteQuoteInput`/`CreateOrderInput`'s `QuoteIdentifier` all accept an ARN-shaped form
  (`^(arn:...:quote/)?oq-[a-f0-9]{17}$}`, confirmed via `API_GetQuote.html`) -- the prior pass's
  "Quotes have no ARN form" conclusion was wrong. Added `resolveQuoteLocked` (mirrors
  `resolveOutpostLocked`/`resolveSiteLocked`) and wired it into all four operations.
- Implemented real `ServiceQuotaExceededException` enforcement on `CreateSite` (100 sites per
  Region) and `CreateOutpost` (10 Outposts per site), using AWS's own published default quotas
  (`docs.aws.amazon.com/outposts/latest/userguide/outposts-limits.html`) -- previously declared but
  never triggered anywhere, exactly as the prior audit left it.
- Confirmed the "No AWS::Outposts::* CloudFormation resource type" line from the prior audit's
  gaps was never a real parity gap (real AWS CloudFormation has no Outposts support either) and
  dropped it entirely rather than re-filing it as a structural_gap.

Reclassified 3 gaps to `structural_gaps` with individual justification (LifeCycleStatus has no SDK
enum anywhere to converge on; catalog/pricing data is proprietary AWS-published data with no public
source; Connection key material requires a real cryptographic hardware-install exchange) -- see the
frontmatter for why each qualifies under the strict "data source cannot exist" bar, not just
"wasn't verified."

**Why overall stays B, not A**: the single highest-value gap flagged for this pass -- wiring
`services/ec2`'s `RunInstances` to decrement the Outposts capacity ledger -- turned out to be a
genuine architectural blocker, not unfinished work. `services/ec2`'s `Subnet`/`Instance`/
`CapacityReservation` structs carry zero Outpost-placement fields (no `Subnet.OutpostArn`, no
`Instance.Placement.OutpostArn`, no `RunInstances` `Placement.OutpostArn` wire input -- confirmed
by directly grepping `services/ec2/store.go` and `instance_attrs.go`, not assumed). `services/grafana`'s
`cross_service.go` read-only pattern only works because `ec2` already exposed the data grafana
needed (`DescribeSubnets`/`DescribeSecurityGroups`); here `ec2` has no comparable surface to read.
Closing this requires an `ec2`-side change, which is out of this session's `services/outposts/`-only
file-ownership scope -- filed as `gopherstack-9ij1` for a future `ec2`-owning pass. Two smaller gaps
(Order/CapacityTask single-hop lifecycle, 15-of-17 unevaluated `OrderingRequirement` checks) were
also left open, deferred for scope/effort this pass rather than closed -- see `gaps` above for why
each is still genuinely buildable, not structural.

## Implementation summary (2026-08-01 pass)

All 43 operations are implemented with real backend state (no stubs): Outpost/Site CRUD with
one seeded COMPUTE Asset per Outpost (there is no public CreateAsset API to provision one
otherwise), Order/Quote/CapacityTask lifecycles with single-hop async transitions via
`pkgs/worker` (mirroring `services/eks`/`services/grafana`'s pattern), a real capacity ledger
(`Asset.ComputeAttributes.InstanceTypeCapacities`) that `StartCapacityTask` actually mutates and
`GetOutpostInstanceTypes` actually reads, a deterministic (documented-synthetic) pricing model
for quotes/renewals, and full tag support for both Outpost and Site wired into
`resourcegroupstaggingapi` (`cli.go`'s `wireTaggingOutposts`, the 31st tagging-wired service).

**File layout**: `models.go` (stored-state types) / `wire.go` + `wire_convert.go` (JSON wire
shapes, PascalCase document members and query params -- see "New finding" below -- and their
conversion to/from stored state) / `store.go` + `store_setup.go` (`InMemoryBackend`, one coarse
`lockmetrics.RWMutex` since operations routinely cross resource boundaries: CreateOutpost seeds
an Asset, CreateOrder reads a Quote and writes an Order, TagResource resolves an ARN into either
the Outposts or Sites table) / `resolve.go` (shared id-or-ARN resolution) / `outposts.go` /
`sites.go` / `orders.go` / `quotes.go` / `capacity_tasks.go` / `assets.go` / `catalog.go` /
`connections.go` / `renewals.go` / `pricing.go` (backend logic) / `seed_data.go` (static
reference-data tables) / `handler.go` + one `handler_<family>.go` per operation family (HTTP
routing/dispatch) / `persistence.go` / `errors.go` / `consts.go` / `provider.go`.

**Tests**: `sdk_completeness_test.go` (all 43 ops) plus real SDK round-trip tests (following
`services/grafana`'s `sdk_roundtrip_helper_test.go` pattern -- the genuine AWS SDK client against
an `httptest` server, not ad-hoc JSON assertions) across every resource family:
`outposts_test.go`, `sites_test.go`, `orders_test.go`, `quotes_test.go`,
`capacity_tasks_test.go`, `assets_test.go`, `catalog_test.go`, `connections_test.go`,
`tags_test.go`, `persistence_test.go`. All pass under `-race`.

### New finding this pass: query-string and document-member casing is PascalCase, not lowerCamel

The prior audit did not check wire-field casing (it only verified method+path). This pass found
that, unlike `services/grafana` (lowerCamel JSON document members and query params), **every
document member AND every query-string parameter in this service's wire protocol is PascalCase**,
matching the Go SDK's exported field names almost verbatim (e.g. `"OutpostId"`, `"MaxResults"`,
`"AvailabilityZoneFilter"` -- confirmed by grepping every `object.Key("...")` in serializers.go
and every `(Add|Set)Query("...")` call, not assumed from the grafana precedent). The ONE
exception across all 43 operations: `UntagResource`'s `TagKeys` serializes as a repeated
lowerCamel `?tagKeys=` query parameter (`serializers.go:3235`), confirmed by direct grep. Getting
this wrong across the board would have been a silent, hard-to-catch wire-compatibility bug that
unit tests (asserting against the handler's own output) would never have caught -- only the real
SDK round-trip tests would (and did, during development, before the casing was corrected).

### Judgment calls made where the audit flagged a genuine unknown

1. **LifeCycleStatus values** (`ACTIVE`, `PENDING_DECOMMISSION` -- consts.go): the SDK declares
   no enum type at all for this field. Chose immediate `ACTIVE` on create (no invented
   transition workflow with zero SDK backing) and `PENDING_DECOMMISSION` on a successful
   `StartOutpostDecommission`. Both are this implementation's own choice, not AWS fact.
2. **SUPERSEDED by the 2026-08-06 pass, see that section above.** ID/ARN formats for
   Site/Order/Quote/CapacityTask/Asset/Connection (`os-`/`oo-`/`oq-`/`ct-`/`asset-`/`conn-`
   prefixes, `site/<id>` ARN resource segment): at the time, none of these had a confirming
   source and `outpost/<id>` was the only one with in-repo precedent. The 2026-08-06 pass found
   `docs.aws.amazon.com/outposts/latest/APIReference/` publishes exact `Pattern` regexes for all
   of them (fixed 6 real ID-format bugs) and that Quote *does* accept an ARN-shaped identifier on
   input (`GetQuote`/`UpdateQuote`/`DeleteQuote`/`CreateOrder`'s `QuoteIdentifier`) even though it
   has no `QuoteArn` output field -- this pass's "no ARN at all" conclusion for Quote was wrong.
   Order/CatalogItem/Asset/Connection still have no ARN form (unchanged, still correct).
3. **Quote pricing and OrderingRequirements are a deliberately narrow model**, not an attempt to
   fake full AWS-equivalence: a synthetic deterministic pricing formula (documented in
   pricing.go, not real AWS numbers), and only 2 of 17 real `OrderingRequirementType` checks are
   evaluated (the ones this backend has actual state to answer). Fabricating pass/fail for the
   other 15 (e.g. `ENTERPRISE_SUPPORT_ERROR`, which would require a support-plan model this
   backend doesn't have) was rejected as inventing behavior the audit explicitly warned against.
4. **Order/CapacityTask lifecycle collapsed to a single async hop** (PREPARING->COMPLETED,
   REQUESTED->COMPLETED) rather than modeling every intermediate SDK-declared state
   (IN_PROGRESS/DELIVERED, WAITING_FOR_EVACUATION/CANCELLATION_IN_PROGRESS) with no rollup rule
   to base a multi-stage timeline on (the audit's hardest-thing #1). `CancelCapacityTask`
   likewise transitions straight to CANCELLED rather than pausing at the transient
   CANCELLATION_IN_PROGRESS state, since this backend's cancellation is synchronous (there is no
   real hardware-side cleanup to wait for).
5. **Asset seeding**: exactly one COMPUTE asset is created per Outpost at `CreateOutpost` time,
   since there is no public `CreateAsset` operation among the 43 and real Outposts assets arrive
   via physical hardware installation. Documented as a deliberate implementation choice, not an
   attempt to model AWS's actual asset-provisioning process.
6. **`ListAssetInstances`/`ListBlockingInstancesForCapacityTask` always return empty** (after
   validating their required resource exists) rather than inventing placement data -- this
   backend genuinely has no cross-service EC2-on-Outposts instance-placement source, exactly the
   gap the audit's "EC2 capacity coupling" section flagged as out of scope.
7. **`ConflictException.ResourceId`/`ResourceType` are only ever populated for Outpost/Order
   conflicts** (the closed `types.ResourceType` enum's only two members) -- Site/Quote/
   CapacityTask conflicts omit both fields rather than fabricate a `SITE`/`QUOTE` enum value the
   SDK does not declare.
8. **`ServiceQuotaExceededException` has no trigger path** -- no account-level quota model exists
   and no AWS-published default quota number was available to enforce without fabricating one
   (same treatment as `services/grafana`'s `AccessDeniedException`).

### What the audit got right (spot-checked, not re-verified line-by-line here)

The two singular-`/outpost/`-path endpoints, the `/list-orders` action-style slug, the single
`PUT` on `UpdateSiteAddress`, the `EC2Capacity.Quantity`/`.MaxSize` string-typed (not numeric)
fields, `CreateRenewal`'s lone client-token idempotency, and the `OutpostId` vs
`OutpostIdentifier` field-naming inconsistency all matched the audit's findings exactly during
implementation -- confirming the audit's method (reading serializers.go/deserializers.go
directly) was sound.

## Operation count and SDK version (verified, not estimated)

`ls api_op_*.go | grep -v _test.go | wc -l` against
`/home/agbishop/go/pkg/mod/github.com/aws/aws-sdk-go-v2/service/outposts@v1.66.0/` returns **43**,
matching this task's estimate exactly. Resolved via a throwaway scratch module
(`go mod init probe && go get github.com/aws/aws-sdk-go-v2/service/outposts@latest`, run in this
session's scratchpad, never touching this repo's go.mod) -- **v1.66.0** is what `go get @latest`
resolved to at audit time (2026-08-01). This repo's go.mod was not modified by this pass.

The 43 operations, alphabetically: CancelCapacityTask, CancelOrder, CreateOrder, CreateOutpost,
CreateQuote, CreateRenewal, CreateSite, DeleteOutpost, DeleteQuote, DeleteSite, GetCapacityTask,
GetCatalogItem, GetConnection, GetOrder, GetOutpost, GetOutpostBillingInformation,
GetOutpostInstanceTypes, GetOutpostSupportedInstanceTypes, GetQuote, GetRenewalPricing, GetSite,
GetSiteAddress, ListAssetInstances, ListAssets, ListBlockingInstancesForCapacityTask,
ListCapacityTasks, ListCatalogItems, ListOrderableInstanceTypes, ListOrders, ListOutposts,
ListQuotes, ListSites, ListTagsForResource, StartCapacityTask, StartConnection,
StartOutpostDecommission, TagResource, UntagResource, UpdateOutpost, UpdateQuote, UpdateSite,
UpdateSiteAddress, UpdateSiteRackPhysicalProperties.

Protocol is `awsRestjson1` (REST-JSON), confirmed from every serializer's struct name
(`awsRestjson1_serializeOp<Op>`) and every deserializer's error path
(`awsRestjson1_deserializeOpError<Op>`, `restjson.GetErrorInfo`).

## Wire-shape traps worth flagging up front (looks-wrong-but-correct, or just easy to miss)

1. **Two endpoints use a SINGULAR `/outpost/` path** where every other Outpost-family endpoint
   uses plural `/outposts/`: `GetOutpostBillingInformation` (`GET /outpost/{OutpostIdentifier}/billing-information`)
   and `GetRenewalPricing` (`GET /outpost/{OutpostIdentifier}/renewal-pricing`). Verified twice by
   independent grep against `serializers.go` lines 1327 and 1643 -- this is the real wire path, not
   a transcription error in this document. A router that naively prefix-matches `/outposts/` will
   silently 404 these two.
2. **The identifier field name for "the Outpost" is inconsistent across operations.** Some ops
   name it `OutpostId` (`CreateOutpost` -- as the required `SiteId`-analog is different there;
   `GetOutpost`, `DeleteOutpost`, `UpdateOutpost`, `GetOutpostInstanceTypes`), others name the
   identical concept `OutpostIdentifier` (`CancelCapacityTask`, `GetCapacityTask`, `CreateOrder`,
   `CreateRenewal`, `GetOutpostBillingInformation`, `GetOutpostSupportedInstanceTypes`,
   `GetRenewalPricing`, `ListAssetInstances`, `ListAssets`, `ListBlockingInstancesForCapacityTask`,
   `StartCapacityTask`, `StartOutpostDecommission` -- note this one specifically says
   `OutpostIdentifier` even though it decommissions "the Outpost"). Both accept an ID-or-ARN
   string per their doc comments. An implementer building one shared "resolve outpost by
   id-or-arn" helper must accept both field names into it; do not assume a single canonical
   struct field name reused verbatim across every handler.
3. **`ListOrders`'s path is `GET /list-orders`**, an action-style slug, not the expected REST
   collection path `GET /orders` (verified via grep against serializers.go:2390 -- real, not a
   note error). `CancelOrder` similarly uses an action-suffix path (`POST /orders/{OrderId}/cancel`)
   rather than `DELETE /orders/{OrderId}`.
4. **`UpdateSiteAddress` is the only `PUT` in the entire service** (`serializers.go:3610`); every
   other partial-update op in this service is `PATCH`, and it requires the full `Address` struct
   (not a partial merge) per its own doc comment.
5. **`GetOutpostInstanceTypes` vs `GetOutpostSupportedInstanceTypes` are NOT the same query with
   different pagination** -- they answer different questions. The former returns instance types
   *currently configured* on the Outpost (what capacity exists right now); the latter returns
   everything the Outpost's hardware generation *could* support, "generally includ[ing] instance
   types that are not currently configured" per the SDK's own doc comment on
   `GetOutpostSupportedInstanceTypes`. Aliasing one to the other would be a silent, hard-to-catch
   correctness bug, not a stub -- flag prominently for the implementer.
6. **`GetCatalogItem` (`/catalog/item/{id}`, singular) and `ListCatalogItems` (`/catalog/items`,
   plural)** live under sibling-but-different path segments -- confirm both, do not assume one
   implies the other's route prefix.
7. **`EC2Capacity.Quantity` and `.MaxSize` are `*string`, not numeric types**, per
   `types/types.go:322-334` -- these look like they should be `int32` but are wire-typed as
   strings (matches the AssociateLicense-style "looks numeric but isn't" trap class the grafana
   audit warned about).
8. **`CreateRenewal` is the only operation with client-token idempotency auto-fill**
   (`addIdempotencyToken_opCreateRenewalMiddleware`, `api_op_CreateRenewal.go:140-171`) -- if
   `ClientToken` is nil, the SDK client generates one before sending. A real emulator does not need
   to replicate the client-side generation (that happens before the request reaches the server),
   but idempotent-replay semantics keyed on `ClientToken` should be honored server-side if AWS's
   real API does so (not confirmed either way by this SDK checkout; the field is present but its
   idempotency-window server behavior isn't spec'd in Go types).

## State machines to simulate (not a CRUD shell)

- **CapacityTaskStatus** (`REQUESTED` -> `IN_PROGRESS` -> `{FAILED | COMPLETED}`, with side-paths
  `WAITING_FOR_EVACUATION` when blocking EC2 instances exist and the caller chose
  `WAIT_FOR_EVACUATION` in `TaskActionOnBlockingInstances`, and `CANCELLATION_IN_PROGRESS` ->
  `CANCELLED` when `CancelCapacityTask` is called mid-flight). `StartCapacityTask`'s own doc
  comment: only one active capacity task is allowed per (order, Outpost) pair at a time -- a real
  uniqueness invariant, not just a nice-to-have. `ListBlockingInstancesForCapacityTask` and
  `InstancesToExclude` exist specifically to compute whether a task can proceed without operator
  intervention.
- **OrderStatus / LineItemStatus**: `Order.Status` has 11 enum values, 5 explicitly marked
  deprecated in the SDK doc comment (`RECEIVED`, `PENDING`, `PROCESSING`, `INSTALLING`,
  `FULFILLED`) alongside the current set (`PREPARING` -> `IN_PROGRESS` -> `DELIVERED` ->
  `COMPLETED`, or `CANCELLED`/`ERROR`). `LineItem.Status` is a separate, finer-grained 9-value
  enum (`PREPARING`/`BUILDING`/`SHIPPED`/`DELIVERED`/`INSTALLING`/`INSTALLED`/`ERROR`/`CANCELLED`/
  `REPLACED`) -- an Order's overall status is presumably a rollup of its LineItems' individual
  statuses, but the SDK does not encode that rollup rule; an implementer must decide it (document
  the choice, per parity-principles.md's "no invented behavior without saying so").
- **QuoteStatus** (`CREATED` -> `ORDER_SUBMITTED`, or time-based `EXPIRED` via `ExpirationDate`).
  `OrderingRequirement`/`OrderingRequirementType` (17 check-types, e.g.
  `OUTPOST_ACTIVE_CHECK_ERROR`, `MAXIMUM_ALLOWED_ORDERS_CHECK_ERROR`,
  `ENTERPRISE_SUPPORT_ERROR`, `OUTPOST_RENEWAL_REQUIRED_ERROR`) with a per-requirement
  `PASS`/`FAIL`/`EXEMPT` status gate whether `CreateOrder` can actually consume a given quote --
  this is a real business-rule surface, not decorative.
- **Outpost lifecycle** (`LifeCycleStatus`, unmodeled as an enum -- see gaps) plus
  `DecommissionRequestStatus` (`SKIPPED`/`BLOCKED`/`REQUESTED`) from
  `StartOutpostDecommission`, which is a request-acceptance status, not a completion state --
  the real decommission is an out-of-band hardware-return process that this emulator can at most
  flag on the Outpost record, not fully simulate to completion.
- **Site/Order/Outpost relationship**: an Outpost is created via `CreateOutpost(SiteId=...)` --
  belongs to exactly one Site. Capacity growth or hardware fulfillment happens via
  `CreateOrder(OutpostIdentifier=..., LineItems=[{CatalogItemId,Quantity}])`, where each
  `LineItem.CatalogItemId` must resolve against `ListCatalogItems`/`GetCatalogItem`'s catalog.
  `CreateQuote`/`CreateOrder` can alternatively flow through `QuoteIdentifier`/
  `QuoteOptionIdentifier` to pre-price a configuration before ordering. None of these FK
  relationships are optional to simulate faithfully -- an Order referencing a nonexistent
  `OutpostIdentifier` or a Quote referencing a nonexistent `CatalogItemId` should fail, not
  silently succeed.
- **Asset / capacity tracking**: `Asset.ComputeAttributes.InstanceTypeCapacities`
  (`[]AssetInstanceTypeCapacity{Count,InstanceType}`) is the actual capacity ledger backing
  `GetOutpostInstanceTypes`'s response -- `StartCapacityTask` (with `InstancePools`) is the
  operation that should mutate it once a task transitions to `COMPLETED`. `ComputeAttributes.State`
  (`ACTIVE`/`ISOLATED`/`RETIRING`/`INSTALLING`) gates whether an asset can accept new capacity
  tasks at all.
- **Connection/private-connectivity**: `StartConnection`/`GetConnection` model a WireGuard-style
  tunnel used for physical Outpost SERVER installation (per both ops' doc comments, which
  explicitly say "Amazon Web Services uses this action to install Outpost servers" and recommend
  CloudTrail monitoring). This is a narrow, install-time-only flow, not general network
  connectivity -- do not over-build it into a general VPN/tunnel simulation.

## Cross-service wiring needed

**Tagging.** `TagResource`/`UntagResource`/`ListTagsForResource` exist
(`api_op_TagResource.go`, `api_op_UntagResource.go`, `api_op_ListTagsForResource.go`), so this
service should be wired into `cli.go`'s `wireResourceGroupsTagging`
(`/home/agbishop/gopherstack/cli.go:5348`), following the `wireTaggingGrafana` pattern
(`cli.go:6675-6701`, itself calling the generic `wireTaggingCtxARNResources` helper used by
`wireTaggingEFS` at `cli.go:6127-6152`). Both `Outpost.Tags` and `Site.Tags`
(`types/types.go:650`, `:1019`) exist, but there is only ONE generic ARN-keyed tag API shared
across both resource kinds -- the tag store backing this wiring needs to be keyed by the full
ARN (Outpost ARN or Site ARN), not scoped to a single resource-type map like most other
`wireTaggingXxx` functions. `wireResourceGroupsTagging` currently wires exactly 30 services
(`cli.go:5327-5399`'s own doc comment enumerates them); Outposts would be the 31st entry, added
alongside the existing `wireTaggingGrafana(bk, byName["Grafana"])` line as
`wireTaggingOutposts(bk, byName["Outposts"])` (name TBD by however this service registers itself
in `byName`, which is keyed by `service.Registerable.Name()` -- not confirmed here since the
service doesn't exist yet to register anything).

**ARN namespace**: everywhere this repo already constructs or asserts an Outposts-related ARN,
it uses **`outposts`** as the ARN service segment, matching the package name (i.e. this is NOT
one of the seven mismatches the broader campaign found, like `stepfunctions`->`states` or
`efs`->`elasticfilesystem`). Evidence, all from in-repo test fixtures (not production code, since
no Outposts service exists yet to build ARNs in the first place):
- `services/ec2/handler_local_gateway_test.go:21`: `OutpostArn: "arn:aws:outposts:us-east-1:000000000000:outpost/op-1"`
- `services/ec2/local_gateway_test.go:17,91,282`: same `arn:aws:outposts:...:outpost/op-1` shape
- `services/route53resolver/outpost_resolvers_test.go:32` and 20+ other lines in that file: same shape
- `services/route53resolver/persistence_test.go:146`: same shape

All of these are test-only string literals passed through opaque `OutpostArn string` fields (see
below) -- none of them are derived from a real ARN-building function, so this is corroborating
precedent, not primary confirmation. **What could NOT be confirmed this pass**, despite attempts:
the SDK itself carries no `@arn`/resource-pattern trait on any of the three tagging ops'
`ResourceArn *string` fields (same limitation the grafana audit hit -- confirmed by reading
`api_op_TagResource.go`/`api_op_UntagResource.go`/`api_op_ListTagsForResource.go` directly, all
three declare `ResourceArn *string` with no pattern doc). Terraform's `internal/service/outposts`
package (fetched via WebFetch from
`raw.githubusercontent.com/hashicorp/terraform-provider-aws/main/...`) does not construct the
Outpost ARN client-side at all -- unlike Grafana's ARN (which Terraform builds locally via
`RegionalARN`), Outposts' `OutpostArn` is returned directly by the real API in every response
shape (`Outpost.OutpostArn`, `Site.SiteArn`, `Quote.OutpostArn`,
`GetOutpostInstanceTypesOutput.OutpostArn`) and Terraform's `outpost_data_source.go` just does
`d.Set(names.AttrARN, outpost.OutpostArn)`. AWS's own Service Authorization Reference page
(`docs.aws.amazon.com/service-authorization/latest/reference/list_awsoutposts.html`) and User
Guide security page returned only a JS-shell body to WebFetch (same failure mode the grafana audit
hit on the same domain) -- **the exact resource-path segment for Site (`site/<id>`?), Order
(`order/<id>`? `order/<outpost-id>/<order-id>`?), Quote, and CatalogItem ARNs is an HONEST
UNKNOWN**, not fabricated here. Only the Outpost's own `outpost/<id>` segment has any supporting
evidence at all, and even that is in-repo precedent rather than a primary source. An implementer
should treat `outpost/<id>` as reasonable-but-unconfirmed and actively verify Site/Order/Quote/
CatalogItem ARN shapes before hardcoding them, rather than trusting this document's guesses.

**Existing opaque `OutpostArn`/Outposts-adjacent fields elsewhere in the tree** -- all confirmed
to be plain unvalidated strings today, with ZERO cross-service call into anything that could
become this new Outposts service:
- `services/ec2/local_gateway.go:51,93,525` -- `LocalGateway.OutpostArn` and (per
  `handler_secondary_net_test.go:114-125,235-254` and `secondary_net_test.go:130-154`) an
  `OutpostLag`/`SeedOutpostLag`/`DescribeOutpostLags` family, all storing `OutpostArn string` with
  `json:"outpostArn,omitempty"` and no existence check against anything.
- `services/route53resolver/handler_outpost_resolvers.go:17,26,42,57-58,66` -- a full
  `CreateOutpostResolver`/`GetOutpostResolver`/`UpdateOutpostResolver`/`DeleteOutpostResolver`/
  `ListOutpostResolvers` CRUD family (`services/route53resolver/outpost_resolvers_test.go` has 20+
  tests for it) that requires `OutpostArn` be non-empty (`handler_outpost_resolvers.go:57-58`:
  `if in.OutpostArn == "" { return ...ErrValidation }`) but never checks it resolves to a real
  Outpost. Also `handler_resolver_endpoints.go:49,73,132,154` and `resolver_endpoints.go:97` --
  Resolver Endpoints optionally carry an `OutpostArn` too.
- `services/s3control/interfaces.go:27,91-119` -- an `OutpostsBucket` type family
  (`CreateBucket`/`GetBucket`/`ListRegionalBuckets`), keyed purely by `bucketName` with no
  `OutpostId`/ARN field on the bucket itself at all (`handler_bucket_test.go:148-152` explicitly
  notes "has no BucketArn or OutpostId field in the real SDK").
- `services/datasync/handler_locations_test.go:177-187` -- `CreateLocationS3`'s `AgentArns` field
  is documented as the Outposts-agent mechanism (`datasync/PARITY.md:14`: "added AgentArns
  (Outposts) input, real member").
- `services/emr` -- cluster records carry an `OutpostArn` field per
  `services/emr/handler_wire_shape_test.go:610`'s comment listing it among cluster-summary fields.
- `services/ram/handler_resources.go:236-237` -- the Resource Groups' resource-share-type registry
  already lists `{ResourceType: "outposts:Outpost", ServiceName: "outposts"}` as a shareable
  resource type (this is RAM's own catalog of shareable AWS resource *types*, not a live
  connection to any Outposts backend -- it independently corroborates the `outposts` ARN service
  namespace, though).
- `services/elbv2/README.md:26` and `PARITY.md:83` note `IpamPools`/`CustomerOwnedIpv4Pool`/
  `EnforceSecurityGroupInboundRulesOnPrivateLinkTraffic` as unimplemented
  Outposts/IPAM-adjacent `LoadBalancer` fields -- these are genuinely Outposts-adjacent (Outposts
  networking touches customer-owned IP pools) but are a separate, already-tracked ELBv2 gap, not
  something this new service should try to satisfy.

None of the above needs to change for a first Outposts implementation to land -- they're
independent opaque-string fields today and will keep working unmodified. But once a real
`services/outposts` backend exists, a *future* pass could sensibly cross-validate these ARNs
against it (e.g. `route53resolver`'s `CreateOutpostResolver` rejecting an `OutpostArn` that
doesn't resolve via the new service's own `GetOutpost`) -- flagged here as a real follow-on
opportunity, explicitly out of scope for this audit and for a first implementation pass.

**CloudFormation**: `grep -rli outpost services/cloudformation/` returned nothing -- there is no
`AWS::Outposts::*` resource type in `services/cloudformation/resources_*.go`, confirmed by both
the grep and by listing every `resources_*.go` file in that directory (none named for outposts).
This appears to reflect reality, not a gopherstack gap: AWS Outposts is provisioned through a
physical-hardware ordering process (this very API), not typical declarative infrastructure, and
this audit found no evidence AWS's real CloudFormation supports it either (not exhaustively
confirmed against AWS's own CFN resource-type registry, but consistent with Outposts' order/site
workflow being fundamentally a support-ticket-adjacent process rather than a declarative resource
lifecycle).

**EC2 capacity coupling**: real AWS ties `RunInstances` on an Outpost-hosted subnet to that
Outpost's currently configured `InstanceTypeCapacity`. **SUPERSEDED by the 2026-08-06 EC2
capacity-coupling pass, see that section above** -- at the time this note was written (before
either service existed), `services/ec2` had no such coupling and building it was out of scope;
`services/ec2` now has `Subnet.OutpostArn`/`Instance.OutpostArn` and calls into
`services/outposts/capacity_ledger.go` from `RunInstances`/`TerminateInstances`.

## Top 5 hardest/riskiest things about implementing this service (for the caller's final report)

1. **The Order/LineItem/CapacityTask status-rollup rules are not encoded in the SDK** -- the
   relationship between an Order's overall `Status`, its LineItems' individual `Status` values,
   and a CapacityTask's own lifecycle is implied by field names and doc-comment prose, not by any
   machine-checkable contract. Any implementation has to invent a defensible rollup rule and
   document it as a deliberate choice (per parity-principles.md), not hide it as though it were
   AWS's actual behavior.
2. **The ARN format for Site/Order/Quote/CatalogItem resources is a genuine unknown**, not just
   an "SDK doesn't confirm it" formality -- multiple independent lookup paths (SDK trait, real
   Terraform provider source, AWS's own docs pages) all failed to produce a citable answer. Only
   `outpost/<id>` has any supporting evidence, and it's second-hand (in-repo test fixtures, not a
   primary source).
3. **Two field-naming inconsistencies (`OutpostId` vs `OutpostIdentifier` for the same concept,
   and the two singular-`/outpost/`-path endpoints) are easy to implement wrong** if a router or
   ID-resolution helper is written by pattern-matching most operations and extrapolating to the
   rest, rather than checking each operation's own serializer.
4. **Static reference-data operations** (`ListCatalogItems`/`GetCatalogItem`,
   `ListOrderableInstanceTypes`) require a seed dataset that doesn't exist anywhere in the SDK or
   this repo -- there's no way to make these "really AWS-accurate" without external data AWS
   doesn't publish machine-readably, so the honest move is a small defensible static seed (à la
   grafana's `ListVersions`), clearly flagged as a stand-in.
5. **`StartOutpostDecommission`'s "state machine" is mostly a request receipt, not a real async
   completion flow** -- `DecommissionRequestStatus` only has `SKIPPED`/`BLOCKED`/`REQUESTED`, no
   terminal "decommissioned" value modeled in this SDK at all, meaning the real end-state
   presumably lives on `Outpost.LifeCycleStatus` (itself unconfirmed, see gap #2 above) rather
   than on the decommission response -- two unconfirmed-enum problems compounding on the same
   feature.
