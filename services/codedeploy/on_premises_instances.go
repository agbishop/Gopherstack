package codedeploy

import (
	"fmt"
	"regexp"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// onPremInstanceNameRe validates on-premises instance names per AWS spec.
var onPremInstanceNameRe = regexp.MustCompile(`^[A-Za-z0-9._\-]{1,100}$`)

// AddTagsToOnPremisesInstances adds tags to on-premises instances, registering them if needed.
func (b *InMemoryBackend) AddTagsToOnPremisesInstances(instanceNames []string, kv map[string]string) error {
	b.mu.Lock("AddTagsToOnPremisesInstances")
	defer b.mu.Unlock()

	for _, name := range instanceNames {
		inst, ok := b.onPremisesInstances.Get(name)
		if !ok {
			t := tags.New("codedeploy.onprem." + name + ".tags")
			inst = &OnPremisesInstance{
				InstanceName: name,
				RegisterTime: time.Now().UTC(),
				Tags:         t,
			}
			b.onPremisesInstances.Put(inst)
		}

		inst.Tags.Merge(kv)
	}

	return nil
}

// RemoveTagsFromOnPremisesInstances removes tag keys from the given on-premises instances.
func (b *InMemoryBackend) RemoveTagsFromOnPremisesInstances(instanceNames []string, keys []string) error {
	b.mu.Lock("RemoveTagsFromOnPremisesInstances")
	defer b.mu.Unlock()

	for _, name := range instanceNames {
		inst, ok := b.onPremisesInstances.Get(name)
		if !ok {
			continue
		}

		inst.Tags.DeleteKeys(keys)
	}

	return nil
}

// RegisterOnPremisesInstance registers a new on-premises instance.
// Exactly one of iamSessionArn or iamUserArn must be set.
func (b *InMemoryBackend) RegisterOnPremisesInstance(name, iamSessionArn, iamUserArn string) error {
	b.mu.Lock("RegisterOnPremisesInstance")
	defer b.mu.Unlock()

	if !onPremInstanceNameRe.MatchString(name) {
		return fmt.Errorf(
			"%w: instance name %q does not match pattern [A-Za-z0-9._-]{1,100}", ErrInvalidInstanceName, name,
		)
	}

	if iamSessionArn != "" && iamUserArn != "" {
		return fmt.Errorf("%w: only one of iamSessionArn or iamUserArn may be set", ErrMultipleIamArns)
	}

	if iamSessionArn == "" && iamUserArn == "" {
		return fmt.Errorf("%w: one of iamSessionArn or iamUserArn must be set", ErrIamArnRequired)
	}

	if b.onPremisesInstances.Has(name) {
		return nil
	}

	t := tags.New("codedeploy.onprem." + name + ".tags")
	b.onPremisesInstances.Put(&OnPremisesInstance{
		InstanceName:  name,
		IamSessionArn: iamSessionArn,
		IamUserArn:    iamUserArn,
		RegisterTime:  time.Now().UTC(),
		Tags:          t,
	})

	return nil
}

// DeregisterOnPremisesInstance marks an on-premises instance as deregistered.
func (b *InMemoryBackend) DeregisterOnPremisesInstance(name string) error {
	b.mu.Lock("DeregisterOnPremisesInstance")
	defer b.mu.Unlock()

	inst, ok := b.onPremisesInstances.Get(name)
	if !ok {
		// DeregisterOnPremisesInstance's deserializer models neither
		// InstanceDoesNotExistException nor any other not-found code
		// (aws-sdk-go-v2/service/codedeploy deserializers.go) -- this is
		// provably the wrong code, but whether the real op is a silent
		// idempotent success or maps to a different code is unconfirmed.
		// Do NOT "fix" this by guessing; needs real evidence (gopherstack-3pz8).
		return fmt.Errorf("%w: instance %s not found", ErrOnPremisesInstanceNotFound, name)
	}

	now := time.Now().UTC()
	inst.DeregisterTime = &now

	return nil
}

// OnPremisesInstanceARN builds an ARN for an on-premises instance, matching
// the "instance:<name>" resource format already used for the same
// InstanceTarget.TargetArn shape in deployment_instances.go.
func (b *InMemoryBackend) OnPremisesInstanceARN(name string) string {
	return arn.Build("codedeploy", b.region, b.accountID, "instance:"+name)
}

// GetOnPremisesInstance returns an on-premises instance by name.
func (b *InMemoryBackend) GetOnPremisesInstance(name string) (*OnPremisesInstance, error) {
	b.mu.RLock("GetOnPremisesInstance")
	defer b.mu.RUnlock()

	inst, ok := b.onPremisesInstances.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: instance %s not found", ErrOnPremisesInstanceNotRegistered, name)
	}

	cp := *inst

	return &cp, nil
}

// ListOnPremisesInstances returns instance names, optionally filtered by registration status and tag filters.
func (b *InMemoryBackend) ListOnPremisesInstances(registrationStatus string, tagFilters []TagFilter) []string {
	b.mu.RLock("ListOnPremisesInstances")
	defer b.mu.RUnlock()

	all := b.onPremisesInstances.All()
	names := make([]string, 0, len(all))
	for _, inst := range all {
		if registrationStatus == "Deregistered" && inst.DeregisterTime == nil {
			continue
		}
		if registrationStatus == "Registered" && inst.DeregisterTime != nil {
			continue
		}
		if len(tagFilters) > 0 && !matchesTagFilters(inst.Tags, tagFilters) {
			continue
		}
		names = append(names, inst.InstanceName)
	}

	sort.Strings(names)

	return names
}

// matchesTagFilters returns true if the tags satisfy all the given filters
// (AND semantics across filters, matching real CodeDeploy's single-tag-set
// group behavior).
func matchesTagFilters(t *tags.Tags, filters []TagFilter) bool {
	if t == nil {
		return len(filters) == 0
	}

	return matchesTagFiltersMap(t.Clone(), filters)
}

// matchesTagFiltersMap is matchesTagFilters' core, operating directly on a
// plain key/value map so callers with a resource's tags already in that form
// (e.g. services/ec2's TagsForResource) don't need to wrap them in a *tags.Tags.
func matchesTagFiltersMap(kv map[string]string, filters []TagFilter) bool {
	for _, f := range filters {
		if !matchesOneTagFilter(kv, f) {
			return false
		}
	}

	return true
}

// matchesOneTagFilter evaluates a single TagFilter against kv, honoring the
// three real CodeDeploy TagFilterType values: KEY_ONLY (key must exist, any
// value), VALUE_ONLY (value must exist under any key), and KEY_AND_VALUE
// (exact key=value match) -- the last of which is also this function's
// default for an empty/unset Type, matching how AWS treats a missing filter
// type as an exact match. Factored out of matchesTagFilters to keep that
// loop's own complexity low.
func matchesOneTagFilter(kv map[string]string, f TagFilter) bool {
	switch f.Type {
	case "KEY_ONLY":
		_, ok := kv[f.Key]

		return ok
	case "VALUE_ONLY":
		for _, v := range kv {
			if v == f.Value {
				return true
			}
		}

		return false
	default: // KEY_AND_VALUE, or empty (missing type defaults to exact match)
		v, ok := kv[f.Key]

		return ok && v == f.Value
	}
}

// matchesTagSetGroups reports whether kv satisfies at least one AND-group in
// groups (OR across groups, AND within a group), matching real CodeDeploy's
// Ec2TagSet/OnPremisesTagSet semantics. An empty groups list never matches
// (there is nothing to satisfy), distinct from matchesTagFilters(t, nil)
// which vacuously matches everything -- a tag *set* with zero groups means
// "no tag-set-based targeting configured", not "targets everything".
func matchesTagSetGroups(t *tags.Tags, groups [][]TagFilter) bool {
	for _, group := range groups {
		if matchesTagFilters(t, group) {
			return true
		}
	}

	return false
}

// matchesTagSetGroupsMap is matchesTagSetGroups' core, operating directly on
// a plain key/value map; see matchesTagFiltersMap.
func matchesTagSetGroupsMap(kv map[string]string, groups [][]TagFilter) bool {
	for _, group := range groups {
		if matchesTagFiltersMap(kv, group) {
			return true
		}
	}

	return false
}

// matchesOnPremisesTargeting reports whether an on-premises instance's tags
// satisfy a deployment group's on-premises targeting configuration.
// OnPremisesTagSet (AND-groups OR'd together) takes precedence over the
// simpler flat OnPremisesInstanceTagFilters (AND only) when both happen to be
// set, matching how the real CreateDeploymentGroup API treats the two as
// mutually exclusive alternate representations of the same concept. A
// deployment group with neither configured never targets any on-premises
// instance.
func matchesOnPremisesTargeting(t *tags.Tags, dg *DeploymentGroup) bool {
	if dg.OnPremisesTagSet != nil && len(dg.OnPremisesTagSet.OnPremisesTagSetList) > 0 {
		return matchesTagSetGroups(t, dg.OnPremisesTagSet.OnPremisesTagSetList)
	}

	if len(dg.OnPremisesInstanceTagFilters) > 0 {
		return matchesTagFilters(t, dg.OnPremisesInstanceTagFilters)
	}

	return false
}

// matchesEc2Targeting reports whether an EC2 instance's tags (as returned by
// services/ec2's TagsForResource) satisfy a deployment group's EC2 targeting
// configuration, mirroring matchesOnPremisesTargeting's
// Ec2TagSet-precedes-Ec2TagFilters rule for the EC2 side of the same
// mutually-exclusive-representation API contract.
func matchesEc2Targeting(kv map[string]string, dg *DeploymentGroup) bool {
	if dg.Ec2TagSet != nil && len(dg.Ec2TagSet.Ec2TagSetList) > 0 {
		return matchesTagSetGroupsMap(kv, dg.Ec2TagSet.Ec2TagSetList)
	}

	if len(dg.Ec2TagFilters) > 0 {
		return matchesTagFiltersMap(kv, dg.Ec2TagFilters)
	}

	return false
}

// BatchGetOnPremisesInstances returns on-premises instance info for the given names.
// Names that do not exist are silently omitted.
func (b *InMemoryBackend) BatchGetOnPremisesInstances(instanceNames []string) []*OnPremisesInstance {
	b.mu.RLock("BatchGetOnPremisesInstances")
	defer b.mu.RUnlock()

	result := make([]*OnPremisesInstance, 0, len(instanceNames))

	for _, name := range instanceNames {
		inst, ok := b.onPremisesInstances.Get(name)
		if !ok {
			continue
		}

		cp := *inst
		result = append(result, &cp)
	}

	return result
}

// AddOnPremisesInstanceInternal adds an on-premises instance directly to the backend.
// Used for test seeding only.
func (b *InMemoryBackend) AddOnPremisesInstanceInternal(inst *OnPremisesInstance) {
	b.mu.Lock("AddOnPremisesInstanceInternal")
	defer b.mu.Unlock()

	inst.Tags = ensureTags(inst.Tags, "codedeploy.onprem."+inst.InstanceName+".tags")

	if inst.RegisterTime.IsZero() {
		inst.RegisterTime = time.Now().UTC()
	}

	b.onPremisesInstances.Put(inst)
}
