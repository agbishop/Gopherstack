package backup

import (
	"fmt"

	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// TaggedEntry pairs a resource ARN with its tag map, for cross-service tag
// enumeration by the Resource Groups Tagging API (see cli.go's
// wireTaggingBackup).
type TaggedEntry struct {
	Tags map[string]string
	ARN  string
}

// appendBackupTaggedEntry appends a TaggedEntry for arn/t to entries when t
// holds at least one tag.
func appendBackupTaggedEntry(entries []TaggedEntry, arn string, t *tags.Tags) []TaggedEntry {
	if t == nil || t.Len() == 0 {
		return entries
	}

	return append(entries, TaggedEntry{ARN: arn, Tags: t.Clone()})
}

// TaggedResources returns every Backup resource ARN that currently has at
// least one tag, across every taggable Backup resource kind (backup vaults,
// backup plans, frameworks, report plans).
func (b *InMemoryBackend) TaggedResources() []TaggedEntry {
	b.mu.RLock("TaggedResources")
	defer b.mu.RUnlock()

	var out []TaggedEntry

	for _, v := range b.vaults.All() {
		out = appendBackupTaggedEntry(out, v.BackupVaultArn, v.Tags)
	}

	for _, p := range b.plans.All() {
		out = appendBackupTaggedEntry(out, p.BackupPlanArn, p.Tags)
	}

	for _, f := range b.frameworks.All() {
		out = appendBackupTaggedEntry(out, f.FrameworkArn, f.Tags)
	}

	for _, rp := range b.reportPlans.All() {
		out = appendBackupTaggedEntry(out, rp.ReportPlanArn, rp.Tags)
	}

	for _, rav := range b.restoreAccessVaults.All() {
		out = appendBackupTaggedEntry(out, rav.RestoreAccessBackupVaultArn, rav.Tags)
	}

	return out
}

// TagResource adds tags to a resource by ARN.
// Supported resource types: backup vaults, backup plans, frameworks, report
// plans, restore access backup vaults.
func (b *InMemoryBackend) TagResource(resourceArn string, kv map[string]string) error {
	b.mu.Lock("TagResource")
	defer b.mu.Unlock()

	if name, ok := b.vaultARNIndex[resourceArn]; ok {
		v, _ := b.vaults.Get(name)
		v.Tags.Merge(kv)

		return nil
	}

	if name, ok := b.planARNIndex[resourceArn]; ok {
		p, _ := b.plans.Get(name)
		p.Tags.Merge(kv)

		return nil
	}

	if name, ok := b.frameworkARNIndex[resourceArn]; ok {
		f, _ := b.frameworks.Get(name)
		f.Tags.Merge(kv)

		return nil
	}

	if name, ok := b.reportPlanARNIndex[resourceArn]; ok {
		rp, _ := b.reportPlans.Get(name)
		rp.Tags.Merge(kv)

		return nil
	}

	if name, ok := b.restoreAccessVaultARNIndex[resourceArn]; ok {
		rav, _ := b.restoreAccessVaults.Get(name)
		if rav.Tags == nil {
			rav.Tags = tags.New("backup.restore-access-vault." + name + ".tags")
		}
		rav.Tags.Merge(kv)

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceArn)
}

// ListTags returns tags for a resource by ARN.
// Supported resource types: backup vaults, backup plans, frameworks, report plans.
func (b *InMemoryBackend) ListTags(resourceArn string) (map[string]string, error) {
	b.mu.RLock("ListTags")
	defer b.mu.RUnlock()

	if name, ok := b.vaultARNIndex[resourceArn]; ok {
		v, _ := b.vaults.Get(name)

		return v.Tags.Clone(), nil
	}

	if name, ok := b.planARNIndex[resourceArn]; ok {
		p, _ := b.plans.Get(name)

		return p.Tags.Clone(), nil
	}

	if name, ok := b.frameworkARNIndex[resourceArn]; ok {
		f, _ := b.frameworks.Get(name)

		return f.Tags.Clone(), nil
	}

	if name, ok := b.reportPlanARNIndex[resourceArn]; ok {
		rp, _ := b.reportPlans.Get(name)

		return rp.Tags.Clone(), nil
	}

	if name, ok := b.restoreAccessVaultARNIndex[resourceArn]; ok {
		rav, _ := b.restoreAccessVaults.Get(name)
		if rav.Tags == nil {
			return map[string]string{}, nil
		}

		return rav.Tags.Clone(), nil
	}

	return nil, fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceArn)
}

// UntagResource removes the given tag keys from a resource identified by ARN.
// Supported resource types: backup vaults, backup plans, frameworks, report plans.
func (b *InMemoryBackend) UntagResource(resourceArn string, tagKeys []string) error {
	b.mu.Lock("UntagResource")
	defer b.mu.Unlock()

	if name, ok := b.vaultARNIndex[resourceArn]; ok {
		v, _ := b.vaults.Get(name)
		v.Tags.DeleteKeys(tagKeys)

		return nil
	}

	if name, ok := b.planARNIndex[resourceArn]; ok {
		p, _ := b.plans.Get(name)
		p.Tags.DeleteKeys(tagKeys)

		return nil
	}

	if name, ok := b.frameworkARNIndex[resourceArn]; ok {
		f, _ := b.frameworks.Get(name)
		f.Tags.DeleteKeys(tagKeys)

		return nil
	}

	if name, ok := b.reportPlanARNIndex[resourceArn]; ok {
		rp, _ := b.reportPlans.Get(name)
		rp.Tags.DeleteKeys(tagKeys)

		return nil
	}

	if name, ok := b.restoreAccessVaultARNIndex[resourceArn]; ok {
		rav, _ := b.restoreAccessVaults.Get(name)
		if rav.Tags != nil {
			rav.Tags.DeleteKeys(tagKeys)
		}

		return nil
	}

	return fmt.Errorf("%w: resource %s not found", ErrNotFound, resourceArn)
}
