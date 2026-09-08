package waf

import (
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

const (
	updateInsert = "INSERT"
	updateDelete = "DELETE"
)

// InMemoryBackend is the in-memory implementation of StorageBackend for WAF Classic.
type InMemoryBackend struct {
	mu                     *lockmetrics.RWMutex
	registry               *store.Registry
	changeTokens           map[string]string // token → status
	outstandingChangeToken string            // last PROVISIONED, unconsumed token; "" if none
	webACLs                *store.Table[WebACL]
	rules                  *store.Table[Rule]
	rateBasedRules         *store.Table[RateBasedRule]
	ipSets                 *store.Table[IPSet]
	byteMatchSets          *store.Table[ByteMatchSet]
	sizeConstraintSets     *store.Table[SizeConstraintSet]
	sqlInjectionMatchSets  *store.Table[SqlInjectionMatchSet]
	xssMatchSets           *store.Table[XssMatchSet]
	geoMatchSets           *store.Table[GeoMatchSet]
	regexPatternSets       *store.Table[RegexPatternSet]
	regexMatchSets         *store.Table[RegexMatchSet]
	ruleGroups             *store.Table[RuleGroup]
	ruleGroupRules         map[string][]ActivatedRule // ruleGroupId → activated rules
	loggingConfigs         *store.Table[LoggingConfiguration]
	permissionPolicies     map[string]string            // resourceArn → policy JSON
	tags                   map[string]map[string]string // arn → tags
	accountID              string
	region                 string
}

// NewInMemoryBackend constructs a new InMemoryBackend.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		mu:                 lockmetrics.New("waf"),
		registry:           store.NewRegistry(),
		changeTokens:       make(map[string]string),
		ruleGroupRules:     make(map[string][]ActivatedRule),
		permissionPolicies: make(map[string]string),
		tags:               make(map[string]map[string]string),
		accountID:          accountID,
		region:             region,
	}

	registerAllTables(b)

	return b
}

// ruleReferenced reports whether id (a Rule, RateBasedRule, or RuleGroup
// identifier) is still activated in some WebACL or contained in some
// RuleGroup -- the two containers that hold ActivatedRule.RuleId references
// (WafRuleType REGULAR/RATE_BASED/GROUP all share the same RuleId shape).
// Callers must hold b.mu.
func (b *InMemoryBackend) ruleReferenced(id string) bool {
	for _, acl := range b.webACLs.All() {
		for _, r := range acl.Rules {
			if r.RuleId == id {
				return true
			}
		}
	}

	for _, rules := range b.ruleGroupRules {
		for _, r := range rules {
			if r.RuleId == id {
				return true
			}
		}
	}

	return false
}

// matchSetReferenced reports whether id (an IPSet, ByteMatchSet,
// SqlInjectionMatchSet, XssMatchSet, SizeConstraintSet, GeoMatchSet, or
// RegexMatchSet identifier) is still used as a Predicate.DataId by any Rule
// or RateBasedRule. Callers must hold b.mu.
func (b *InMemoryBackend) matchSetReferenced(id string) bool {
	for _, r := range b.rules.All() {
		for _, p := range r.Predicates {
			if p.DataId == id {
				return true
			}
		}
	}

	for _, r := range b.rateBasedRules.All() {
		for _, p := range r.MatchPredicates {
			if p.DataId == id {
				return true
			}
		}
	}

	return false
}

// regexPatternSetReferenced reports whether id is still used as a
// RegexMatchTuple.RegexPatternSetId by any RegexMatchSet. Callers must hold
// b.mu.
func (b *InMemoryBackend) regexPatternSetReferenced(id string) bool {
	for _, s := range b.regexMatchSets.All() {
		for _, t := range s.RegexMatchTuples {
			if t.RegexPatternSetId == id {
				return true
			}
		}
	}

	return false
}

// AccountID returns the configured account ID.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Region returns the configured region.
func (b *InMemoryBackend) Region() string { return b.region }

// Reset clears all backend state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.changeTokens = make(map[string]string)
	b.outstandingChangeToken = ""
	b.ruleGroupRules = make(map[string][]ActivatedRule)
	b.permissionPolicies = make(map[string]string)
	b.tags = make(map[string]map[string]string)
}
