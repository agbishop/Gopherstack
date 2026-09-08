package sesv2

import (
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// ArchivingOptions captures the archiving configuration of a configuration set.
type ArchivingOptions struct {
	ArchiveARN string `json:"archiveArn,omitempty"`
}

// ConfigurationSet represents a SES v2 configuration set.
type ConfigurationSet struct {
	CreatedAt                    time.Time         `json:"createdAt"`
	Name                         string            `json:"name"`
	Tags                         map[string]string `json:"tags,omitempty"`
	TrackingCustomRedirectDomain string            `json:"trackingCustomRedirectDomain,omitempty"`
	TrackingHTTPSPolicy          string            `json:"trackingHttpsPolicy,omitempty"`
	DeliveryTLSPolicy            string            `json:"deliveryTlsPolicy,omitempty"`
	DeliverySendingPoolName      string            `json:"deliverySendingPoolName,omitempty"`
	ArchivingOptions             *ArchivingOptions `json:"archivingOptions,omitempty"`
	VdmOptions                   *VdmOptions       `json:"vdmOptions,omitempty"`
	SuppressionReasons           []string          `json:"suppressionReasons,omitempty"`
	SendingEnabled               bool              `json:"sendingEnabled"`
	ReputationMetricsEnabled     bool              `json:"reputationMetricsEnabled"`
}

// Clone returns a deep copy of ConfigurationSet.
func (cs *ConfigurationSet) Clone() *ConfigurationSet {
	if cs == nil {
		return nil
	}

	cp := *cs
	if cs.Tags != nil {
		cp.Tags = maps.Clone(cs.Tags)
	}
	if cs.ArchivingOptions != nil {
		arch := *cs.ArchivingOptions
		cp.ArchivingOptions = &arch
	}
	if cs.VdmOptions != nil {
		vdm := &VdmOptions{}
		if cs.VdmOptions.DashboardOptions != nil {
			vdm.DashboardOptions = maps.Clone(cs.VdmOptions.DashboardOptions)
		}
		if cs.VdmOptions.GuardianOptions != nil {
			vdm.GuardianOptions = maps.Clone(cs.VdmOptions.GuardianOptions)
		}
		cp.VdmOptions = vdm
	}
	if cs.SuppressionReasons != nil {
		cp.SuppressionReasons = slices.Clone(cs.SuppressionReasons)
	}

	return &cp
}

// configurationSetARN builds the ARN for a configuration set:
// arn:{partition}:ses:{region}:{account}:configuration-set/{name}. Confirmed
// against terraform-provider-aws's configurationSetARN
// (internal/service/sesv2/configuration_set.go), which must construct this
// exact ARN to tag/import real configuration sets.
func (b *InMemoryBackend) configurationSetARN(name string) string {
	return arn.Build("ses", b.region, b.accountID, "configuration-set/"+name)
}

// CreateConfigurationSet creates a new configuration set.
func (b *InMemoryBackend) CreateConfigurationSet(name string, tags map[string]string) (*ConfigurationSet, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("%w: ConfigurationSetName is required", ErrInvalidInput)
	}

	b.mu.Lock("CreateConfigurationSet")
	defer b.mu.Unlock()

	if b.configurationSets.Has(name) {
		return nil, fmt.Errorf("%w: configuration set %s already exists", ErrAlreadyExists, name)
	}

	cs := &ConfigurationSet{
		Name:           name,
		CreatedAt:      time.Now(),
		SendingEnabled: true,
		Tags:           tags,
	}
	b.configurationSets.Put(cs)

	if len(tags) > 0 {
		b.putResourceTagsLocked(b.configurationSetARN(name), tags)
	}

	return cs.Clone(), nil
}

// GetConfigurationSet returns the configuration set with the given name.
func (b *InMemoryBackend) GetConfigurationSet(name string) (*ConfigurationSet, error) {
	b.mu.RLock("GetConfigurationSet")
	defer b.mu.RUnlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return nil, fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cp := cs.Clone()
	cp.Tags = b.liveTagsLocked(b.configurationSetARN(name))

	return cp, nil
}

// ListConfigurationSets returns a paginated, sorted list of configuration sets.
func (b *InMemoryBackend) ListConfigurationSets(
	nextToken string,
	pageSize int,
) page.Page[*ConfigurationSet] {
	b.mu.RLock("ListConfigurationSets")
	defer b.mu.RUnlock()

	snap := b.configurationSets.Snapshot()
	out := make([]*ConfigurationSet, 0, len(snap))

	for _, cs := range snap {
		out = append(out, cs.Clone())
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].Name < out[j].Name
	})

	return page.New(out, nextToken, pageSize, sesv2DefaultMaxItems)
}

// DeleteConfigurationSet removes a configuration set and its event destinations.
func (b *InMemoryBackend) DeleteConfigurationSet(name string) error {
	b.mu.Lock("DeleteConfigurationSet")
	defer b.mu.Unlock()

	if !b.configurationSets.Has(name) {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	b.configurationSets.Delete(name)
	delete(b.resourceTags, b.configurationSetARN(name))

	// Cascade-delete every event destination of this configuration set.
	// slices.Clone the index results first: deleting from b.eventDestinations
	// mutates eventDestinationsByConfigSet's backing slice in place, which
	// would otherwise corrupt this very loop.
	for _, d := range slices.Clone(b.eventDestinationsByConfigSet.Get(name)) {
		b.eventDestinations.Delete(eventDestinationKey(name, d.Name))
	}

	return nil
}

// ---- configuration set operations ----

// PutConfigurationSetArchivingOptions stores the archive ARN on the config set.
func (b *InMemoryBackend) PutConfigurationSetArchivingOptions(name, archiveARN string) error {
	b.mu.Lock("PutConfigurationSetArchivingOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cs.ArchivingOptions = &ArchivingOptions{ArchiveARN: archiveARN}

	return nil
}

// PutConfigurationSetDeliveryOptions stores the TLS policy and sending pool name.
func (b *InMemoryBackend) PutConfigurationSetDeliveryOptions(
	name, tlsPolicy, sendingPoolName string,
) error {
	b.mu.Lock("PutConfigurationSetDeliveryOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cs.DeliveryTLSPolicy = tlsPolicy
	cs.DeliverySendingPoolName = sendingPoolName

	return nil
}

// PutConfigurationSetReputationOptions stores the reputation metrics flag.
func (b *InMemoryBackend) PutConfigurationSetReputationOptions(
	name string,
	metricsEnabled bool,
) error {
	b.mu.Lock("PutConfigurationSetReputationOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cs.ReputationMetricsEnabled = metricsEnabled

	return nil
}

// PutConfigurationSetSendingOptions enables or disables sending for the config set.
func (b *InMemoryBackend) PutConfigurationSetSendingOptions(
	name string,
	sendingEnabled bool,
) error {
	b.mu.Lock("PutConfigurationSetSendingOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cs.SendingEnabled = sendingEnabled

	return nil
}

// PutConfigurationSetSuppressionOptions stores the suppression reason list.
func (b *InMemoryBackend) PutConfigurationSetSuppressionOptions(
	name string,
	suppressedReasons []string,
) error {
	b.mu.Lock("PutConfigurationSetSuppressionOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	reasons := make([]string, len(suppressedReasons))
	copy(reasons, suppressedReasons)
	cs.SuppressionReasons = reasons

	return nil
}

// PutConfigurationSetTrackingOptions stores the custom redirect domain and HTTPS policy.
func (b *InMemoryBackend) PutConfigurationSetTrackingOptions(
	name, customRedirectDomain, httpsPolicy string,
) error {
	b.mu.Lock("PutConfigurationSetTrackingOptions")
	defer b.mu.Unlock()

	cs, ok := b.configurationSets.Get(name)
	if !ok {
		return fmt.Errorf("%w: configuration set %s not found", ErrNotFound, name)
	}

	cs.TrackingCustomRedirectDomain = customRedirectDomain
	cs.TrackingHTTPSPolicy = httpsPolicy

	return nil
}
