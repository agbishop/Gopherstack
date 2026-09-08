package apprunner

import (
	"fmt"
	"strings"
	"time"
)

// This file holds the cross-resource resolution/validation/normalization
// helpers CreateService and UpdateService need to thread
// AutoScalingConfigurationArn, NetworkConfiguration, HealthCheckConfiguration,
// and ServiceObservabilityConfiguration through real backend state instead of
// silently accepting-and-ignoring them (the pre-sweep behavior called out in
// PARITY.md's "deferred" section).

// asgConfigMarker/obsConfigMarker are the ARN resource-type segments used to
// pull a configuration name out of a "name-only" ARN, matching the two
// formats CreateServiceInput.AutoScalingConfigurationArn and
// ServiceObservabilityConfiguration.ObservabilityConfigurationArn both
// document accepting: a full ARN with name+revision, or an ARN with just the
// name (which resolves to the latest revision).
const (
	asgConfigMarker = "autoscalingconfiguration/"
	obsConfigMarker = "observabilityconfiguration/"
)

// nameFromIdentifier extracts the configuration name from identifier, which
// may be a full ARN (name/revision/id), a name-only ARN (name), or a bare
// name. marker is the ARN resource-type path segment (e.g.
// "autoscalingconfiguration/").
func nameFromIdentifier(identifier, marker string) string {
	_, rest, hasMarker := strings.Cut(identifier, marker)
	if !hasMarker {
		return identifier
	}

	if name, _, hasSlash := strings.Cut(rest, "/"); hasSlash {
		return name
	}

	return rest
}

// resolveASG resolves identifier (full ARN, name-only ARN, or bare name) to
// its live storedAutoScalingConfiguration, preferring an exact ARN match and
// falling back to the latest revision under that name.
func (b *InMemoryBackend) resolveASG(identifier string) (*storedAutoScalingConfiguration, bool) {
	if cfg, ok := b.autoScalingConfigs.Get(identifier); ok {
		return cfg, true
	}

	revs := b.asgByName[nameFromIdentifier(identifier, asgConfigMarker)]
	if len(revs) == 0 {
		return nil, false
	}

	return revs[len(revs)-1], true
}

// defaultASG returns the account's current default auto scaling
// configuration. It's always present after NewInMemoryBackend/Reset/Restore
// (see ensureDefaultAutoScalingConfiguration).
func (b *InMemoryBackend) defaultASG() *storedAutoScalingConfiguration {
	var found *storedAutoScalingConfiguration

	b.autoScalingConfigs.Range(func(c *storedAutoScalingConfiguration) bool {
		if c.IsDefault {
			found = c

			return false
		}

		return true
	})

	return found
}

// resolveOrDefaultASG resolves identifier, or -- when identifier is empty --
// returns the account default, matching CreateServiceInput's documented
// behavior ("If not provided, App Runner associates the latest revision of a
// default auto scaling configuration").
func (b *InMemoryBackend) resolveOrDefaultASG(identifier string) (*storedAutoScalingConfiguration, error) {
	if identifier == "" {
		if cfg := b.defaultASG(); cfg != nil {
			return cfg, nil
		}

		return nil, errNoDefaultASG
	}

	cfg, ok := b.resolveASG(identifier)
	if !ok {
		// CreateService's documented error set has no ResourceNotFoundException
		// (verified against awsAwsjson10_deserializeOpErrorCreateService's
		// switch), so an unresolvable reference is InvalidRequestException.
		return nil, fmt.Errorf("auto scaling configuration %s not found: %w", identifier, ErrInvalidParameter)
	}

	return cfg, nil
}

// recomputeASGAssociation recalculates HasAssociatedService for asgArn from
// current backend state (any live service still referencing it). Called
// after a service stops referencing a configuration (delete, or update to a
// different one) since HasAssociatedService can only be known by scanning
// services, not tracked as a simple increment/decrement.
func (b *InMemoryBackend) recomputeASGAssociation(asgArn string) {
	if asgArn == "" {
		return
	}

	cfg, ok := b.autoScalingConfigs.Get(asgArn)
	if !ok {
		return
	}

	associated := false
	b.services.Range(func(s *storedService) bool {
		if s.AutoScalingConfigurationArn == asgArn {
			associated = true

			return false
		}

		return true
	})
	cfg.HasAssociatedService = associated
}

// ensureDefaultAutoScalingConfiguration guarantees exactly one
// IsDefault=true auto scaling configuration exists, seeding App Runner's
// standard "DefaultConfiguration" revision 1 if none is present. Real App
// Runner always has this default available on every account before any
// CreateAutoScalingConfiguration call; without it, CreateService would have
// nothing to associate when AutoScalingConfigurationArn is omitted.
func (b *InMemoryBackend) ensureDefaultAutoScalingConfiguration() {
	if b.defaultASG() != nil {
		return
	}

	name := defaultASGConfigName
	id := newID()
	cfg := &storedAutoScalingConfiguration{
		AutoScalingConfigurationArn:      b.asgARN(name, 1, id),
		AutoScalingConfigurationName:     name,
		AutoScalingConfigurationRevision: 1,
		Status:                           asgStatusActive,
		MaxConcurrency:                   defaultMaxConcurrency,
		MaxSize:                          defaultMaxSize,
		MinSize:                          defaultMinSize,
		IsDefault:                        true,
		Latest:                           true,
		CreatedAt:                        time.Now().UTC(),
	}
	b.autoScalingConfigs.Put(cfg)
	b.asgByName[name] = append(b.asgByName[name], cfg)
}

// resolveObs resolves identifier (full ARN, name-only ARN, or bare name) to
// its live storedObservabilityConfiguration, mirroring resolveASG.
func (b *InMemoryBackend) resolveObs(identifier string) (*storedObservabilityConfiguration, bool) {
	if cfg, ok := b.observabilityConfigs.Get(identifier); ok {
		return cfg, true
	}

	revs := b.obsByName[nameFromIdentifier(identifier, obsConfigMarker)]
	if len(revs) == 0 {
		return nil, false
	}

	return revs[len(revs)-1], true
}

// validateNetworkConfig checks that an EgressType=VPC network configuration
// references a real VPC connector. A nil n is valid (defaults apply).
func (b *InMemoryBackend) validateNetworkConfig(n *NetworkConfig) error {
	if n == nil || n.EgressType != egressTypeVPC {
		return nil
	}

	if n.EgressVpcConnectorArn == "" {
		return fmt.Errorf(
			"%w: NetworkConfiguration.EgressConfiguration.VpcConnectorArn is required when EgressType is VPC",
			ErrInvalidParameter,
		)
	}

	if !b.vpcConnectors.Has(n.EgressVpcConnectorArn) {
		return fmt.Errorf("vpc connector %s not found: %w", n.EgressVpcConnectorArn, ErrInvalidParameter)
	}

	return nil
}

// validateObservability checks that an enabled observability configuration
// with an explicit ARN resolves to a real configuration. A nil o, or one
// with Enabled=false, is valid.
func (b *InMemoryBackend) validateObservability(o *ServiceObservability) error {
	if o == nil || !o.Enabled || o.ConfigurationArn == "" {
		return nil
	}

	if _, ok := b.resolveObs(o.ConfigurationArn); !ok {
		return fmt.Errorf("observability configuration %s not found: %w", o.ConfigurationArn, ErrInvalidParameter)
	}

	return nil
}

// serviceUsesConnection reports whether any live service's source
// authentication still references connArn. Deleted services are already
// absent from b.services, so no status filter is needed (mirrors
// recomputeASGAssociation's scan).
func (b *InMemoryBackend) serviceUsesConnection(connArn string) bool {
	inUse := false
	b.services.Range(func(s *storedService) bool {
		if s.Source.ConnectionArn == connArn {
			inUse = true

			return false
		}

		return true
	})

	return inUse
}

// serviceUsesVpcConnector reports whether any live service's egress network
// configuration still references vcArn.
func (b *InMemoryBackend) serviceUsesVpcConnector(vcArn string) bool {
	inUse := false
	b.services.Range(func(s *storedService) bool {
		if s.Network.EgressVpcConnectorArn == vcArn {
			inUse = true

			return false
		}

		return true
	})

	return inUse
}

// serviceUsesObservabilityConfig reports whether any live service's enabled
// observability configuration still references obsArn.
func (b *InMemoryBackend) serviceUsesObservabilityConfig(obsArn string) bool {
	inUse := false
	b.services.Range(func(s *storedService) bool {
		if s.Observability.Enabled && s.Observability.ConfigurationArn == obsArn {
			inUse = true

			return false
		}

		return true
	})

	return inUse
}

// hasActiveVpcIngressConnections reports whether any VPC ingress connection
// still references serviceArn. DeleteVpcIngressConnection already removes
// its row from b.vpcIngressConnections on delete, so any entry found here is
// inherently active (no status filter needed).
func (b *InMemoryBackend) hasActiveVpcIngressConnections(serviceArn string) bool {
	active := false
	b.vpcIngressConnections.Range(func(v *storedVpcIngressConnection) bool {
		if v.ServiceArn == serviceArn {
			active = true

			return false
		}

		return true
	})

	return active
}

// validateSourceAuth checks that an AuthenticationConfiguration.ConnectionArn
// (required for GitHub code repositories) resolves to a real connection when
// present. A nil ConnectionArn is valid (e.g. image-based sources or
// ECR-authenticated code sources that don't need an App Runner connection).
func (b *InMemoryBackend) validateSourceAuth(s SourceConfig) error {
	if s.ConnectionArn == "" {
		return nil
	}

	if !b.connections.Has(s.ConnectionArn) {
		return fmt.Errorf("connection %s not found: %w", s.ConnectionArn, ErrInvalidParameter)
	}

	return nil
}
