package awsconfig

import (
	"fmt"
	"slices"
)

const conformancePackStateComplete = "CREATE_COMPLETE"

// PutConformancePack creates or updates a conformance pack. Real AWS Config
// accepts only one of TemplateBody, TemplateS3Uri, or
// TemplateSSMDocumentDetails ("You must specify only one of the follow
// parameters" -- PutConformancePackInput's doc comment); templateS3URI and
// templateSSMDocumentName are the flattened presence-check for the latter
// two (their contents are never fetched -- see parseConformancePackConfigRules
// and PARITY.md's gaps). Specifying more than one is rejected; specifying
// none is accepted (deploys zero rules) to match this codebase's existing
// tests, which routinely call PutConformancePack with no template just to
// establish a pack's existence for unrelated assertions -- the exact
// zero-sources validation/message real AWS applies there is pre-existing
// deferred scope (see PARITY.md), not fixed by this pass.
//
// When templateBody is a JSON or YAML CloudFormation-shaped template
// containing AWS::Config::ConfigRule resources (see
// conformance_pack_template.go), those rules are created/updated as real
// config rules and linked to the pack, matching real AWS Config where a
// conformance pack literally deploys managed config rules on the account --
// this makes the compliance family (DescribeConformancePackCompliance et al.)
// derivable from genuine per-rule evaluation state instead of an empty stub.
// Updating an existing pack's template replaces its rule set: rules no longer
// present in the new template are deleted along with their evaluations
// (cascade), matching AWS's conformance-pack-update semantics.
func (b *InMemoryBackend) PutConformancePack(
	name, deliveryS3Bucket, deliveryS3KeyPrefix, templateBody, templateS3URI, templateSSMDocumentName string,
	tags []Tag,
) error {
	if name == "" {
		return fmt.Errorf("%w: ConformancePackName is required", ErrInvalidParameterValue)
	}

	sourceCount := 0
	for _, set := range []bool{templateBody != "", templateS3URI != "", templateSSMDocumentName != ""} {
		if set {
			sourceCount++
		}
	}

	if sourceCount > 1 {
		return fmt.Errorf(
			"%w: specify only one of TemplateBody, TemplateS3Uri, or TemplateSSMDocumentDetails",
			ErrInvalidParameterValue,
		)
	}

	rules := parseConformancePackConfigRules(templateBody, name)

	b.mu.Lock("PutConformancePack")
	defer b.mu.Unlock()

	b.replacePackRulesLocked(name, rules)

	if _, exists := b.conformancePacks.Get(name); !exists {
		b.conformancePackCounter++
	}

	packID := fmt.Sprintf("conformance-pack-%08d", b.conformancePackCounter)
	arn := fmt.Sprintf(
		"arn:aws:config:%s:%s:conformance-pack/%s/%s",
		b.region, b.accountID, name, packID,
	)

	b.conformancePacks.Put(&ConformancePack{
		ConformancePackName: name,
		ConformancePackArn:  arn,
		ConformancePackID:   packID,
		DeliveryS3Bucket:    deliveryS3Bucket,
		DeliveryS3KeyPrefix: deliveryS3KeyPrefix,
	})
	b.setResourceTagsLocked(arn, tags)

	return nil
}

// replacePackRulesLocked registers newRules as packName's deployed config
// rules, deleting (cascade: config rule + its evaluations + its link entry)
// any previously-linked rule that is absent from newRules. Caller must already
// hold the write lock.
func (b *InMemoryBackend) replacePackRulesLocked(packName string, newRules []*ConfigRule) {
	newNames := make(map[string]struct{}, len(newRules))
	for _, r := range newRules {
		newNames[r.ConfigRuleName] = struct{}{}
	}

	for _, link := range slices.Clone(b.conformancePackRulesByPack.Get(packName)) {
		if _, keep := newNames[link.ConfigRuleName]; keep {
			continue
		}

		b.configRules.Delete(link.ConfigRuleName)
		b.clearRuleEvaluationsLocked(link.ConfigRuleName)
		b.conformancePackRules.Delete(conformancePackRuleLinkKeyFn(link))
	}

	for _, r := range newRules {
		b.putConfigRuleLocked(r)
		b.conformancePackRules.Put(&ConformancePackRuleLink{
			ConformancePackName: packName,
			ConfigRuleName:      r.ConfigRuleName,
		})
	}
}

// DeleteConformancePack deletes a conformance pack by name, cascade-deleting
// every config rule it deployed (and their evaluations) along with it --
// matching real AWS Config, where deleting a conformance pack removes the
// managed rules it created.
func (b *InMemoryBackend) DeleteConformancePack(name string) error {
	if name == "" {
		// Declared set is NoSuchConformancePackException/ResourceInUseException only --
		// no validation-shaped code fits an empty name (configservice@v1.68.4 deserializers.go).
		return fmt.Errorf("%w: ConformancePackName is required", ErrValidation)
	}

	b.mu.Lock("DeleteConformancePack")
	defer b.mu.Unlock()

	if !b.conformancePacks.Has(name) {
		return fmt.Errorf("%w: %s", ErrNoSuchConformancePack, name)
	}

	b.replacePackRulesLocked(name, nil)
	b.conformancePacks.Delete(name)

	return nil
}

// DescribeConformancePacks returns all conformance packs.
func (b *InMemoryBackend) DescribeConformancePacks() []ConformancePack {
	b.mu.RLock("DescribeConformancePacks")
	defer b.mu.RUnlock()

	all := b.conformancePacks.All()
	out := make([]ConformancePack, 0, len(all))

	for _, p := range all {
		out = append(out, *p)
	}

	return out
}

// DescribeConformancePackStatus returns conformance pack statuses.
// If names is empty, all packs are returned.
func (b *InMemoryBackend) DescribeConformancePackStatus(names []string) []ConformancePackStatus {
	b.mu.RLock("DescribeConformancePackStatus")
	defer b.mu.RUnlock()

	if len(names) == 0 {
		all := b.conformancePacks.All()
		out := make([]ConformancePackStatus, 0, len(all))

		for _, p := range all {
			out = append(out, ConformancePackStatus{
				ConformancePackName:  p.ConformancePackName,
				ConformancePackState: conformancePackStateComplete,
				ConformancePackArn: fmt.Sprintf(
					"arn:aws:config:%s:%s:conformance-pack/%s",
					b.region, b.accountID, p.ConformancePackName,
				),
			})
		}

		return out
	}

	out := make([]ConformancePackStatus, 0, len(names))

	for _, name := range names {
		if p, ok := b.conformancePacks.Get(name); ok {
			out = append(out, ConformancePackStatus{
				ConformancePackName:  p.ConformancePackName,
				ConformancePackState: conformancePackStateComplete,
				ConformancePackArn: fmt.Sprintf(
					"arn:aws:config:%s:%s:conformance-pack/%s",
					b.region, b.accountID, p.ConformancePackName,
				),
			})
		}
	}

	return out
}
