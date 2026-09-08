package elbv2

import (
	"fmt"
	"maps"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

func (b *InMemoryBackend) listenerARN(lbName string, port int32) string {
	return arn.Build(
		"elasticloadbalancing",
		b.region,
		b.accountID,
		fmt.Sprintf("listener/app/%s/0123456789abcdef/%d", lbName, port),
	)
}

func isALBProtocol(proto string) bool {
	switch proto {
	case protoHTTP, protoHTTPS:
		return true
	}

	return false
}

func isNLBProtocol(proto string) bool {
	switch proto {
	case "TCP", "UDP", protoTLS, "TCP_UDP":
		return true
	}

	return false
}

func isGWLBProtocol(proto string) bool { return proto == "GENEVE" }

func validateListenerProtocol(lbType, proto string) error {
	switch lbType {
	case lbTypeApplication:
		if !isALBProtocol(proto) {
			return fmt.Errorf(
				"%w: protocol %s is not supported for Application Load Balancers; use HTTP or HTTPS",
				ErrInvalidConfigurationRequest,
				proto,
			)
		}
	case "network":
		if !isNLBProtocol(proto) {
			return fmt.Errorf(
				"%w: protocol %s is not supported for Network Load Balancers; use TCP, UDP, TLS, or TCP_UDP",
				ErrInvalidConfigurationRequest,
				proto,
			)
		}
	case "gateway":
		if !isGWLBProtocol(proto) {
			return fmt.Errorf(
				"%w: protocol %s is not supported for Gateway Load Balancers; use GENEVE",
				ErrInvalidConfigurationRequest, proto,
			)
		}
	}

	return nil
}

func requireCertsForProtocol(proto string, certs []Certificate) error {
	if (proto == protoHTTPS || proto == protoTLS) && len(certs) == 0 {
		return fmt.Errorf(
			"%w: %s listener requires at least one certificate",
			ErrInvalidParameter, proto,
		)
	}

	return nil
}

// checkDuplicateListenerPort returns ErrDuplicateListener if any listener in candidates
// (expected to already be scoped to a single load balancer, e.g. via the listenersByLB index)
// is bound to port.
func checkDuplicateListenerPort(candidates []*Listener, port int32) error {
	for _, existing := range candidates {
		if existing.Port == port {
			return fmt.Errorf(
				"%w: a listener on port %d already exists on this load balancer",
				ErrDuplicateListener, port,
			)
		}
	}

	return nil
}

// CreateListener creates a new listener on a load balancer.
func (b *InMemoryBackend) CreateListener(input CreateListenerInput) (*Listener, error) {
	b.mu.Lock("CreateListener")
	defer b.mu.Unlock()

	lb, ok := b.loadBalancers.Get(input.LoadBalancerArn)
	if !ok {
		return nil, ErrLoadBalancerNotFound
	}

	// Validate protocol against LB type.
	proto := input.Protocol
	if err := validateListenerProtocol(lb.Type, proto); err != nil {
		return nil, err
	}

	if err := requireCertsForProtocol(proto, input.Certificates); err != nil {
		return nil, err
	}

	if err := b.validateCertificates(input.Certificates); err != nil {
		return nil, err
	}

	if err := b.validateForwardTargetGroupsExist(input.DefaultActions); err != nil {
		return nil, err
	}

	// Default SSL policy for HTTPS/TLS listeners.
	if (proto == protoHTTPS || proto == protoTLS) && input.SSLPolicy == "" {
		input.SSLPolicy = "ELBSecurityPolicy-2016-08"
	}

	if err := checkDuplicateListenerPort(b.listenersByLB.Get(input.LoadBalancerArn), input.Port); err != nil {
		return nil, err
	}

	listenerArn := b.listenerARN(lb.LoadBalancerName, input.Port)

	t := tags.New(fmt.Sprintf("elbv2.listener.%s.%d.tags", lb.LoadBalancerName, input.Port))
	for _, kv := range input.Tags {
		t.Set(kv.Key, kv.Value)
	}

	// Initialize default listener attributes based on protocol.
	listenerAttrs := map[string]string{}
	if proto == protoHTTP || proto == protoHTTPS {
		listenerAttrs["routing.http2.enabled"] = attrValueTrue
		listenerAttrs["idle_timeout.timeout_seconds"] = "60"
		listenerAttrs["routing.http.desync_mitigation_mode"] = "defensive"
	}

	listener := &Listener{
		ListenerArn:          listenerArn,
		LoadBalancerArn:      input.LoadBalancerArn,
		Protocol:             proto,
		Port:                 input.Port,
		DefaultActions:       input.DefaultActions,
		Certificates:         input.Certificates,
		SSLPolicy:            input.SSLPolicy,
		AlpnPolicy:           input.AlpnPolicy,
		MutualAuthentication: input.MutualAuthentication,
		Attributes:           listenerAttrs,
		Tags:                 t,
	}

	b.listeners.Put(listener)
	b.markCertificatesInUse(listenerArn, input.Certificates)

	// Auto-create default rule (AWS behaviour: every listener has a default rule).
	defaultRuleArn := b.ruleARN(listenerArn, priorityDefault)
	defaultTags := tags.New("elbv2.rule." + defaultRuleArn + ".tags")
	defaultActions := make([]Action, len(input.DefaultActions))
	copy(defaultActions, input.DefaultActions)
	b.rules.Put(&Rule{
		RuleArn:     defaultRuleArn,
		ListenerArn: listenerArn,
		Priority:    priorityDefault,
		IsDefault:   true,
		Actions:     defaultActions,
		Tags:        defaultTags,
	})

	cp := *listener

	return &cp, nil
}

// describeListenersByARNs resolves listeners by exact ARN lookup and returns them sorted by port.
// Callers must hold at least a read lock.
func (b *InMemoryBackend) describeListenersByARNs(listenerArns []string) ([]Listener, error) {
	result := make([]Listener, 0, len(listenerArns))

	for _, a := range listenerArns {
		if l, ok := b.listeners.Get(a); ok {
			result = append(result, *l)
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Port < result[j].Port
	})

	return result, checkAllListenerArnsFound(listenerArns, result)
}

// checkLBExists returns ErrLoadBalancerNotFound when lbArn is non-empty and not in the store.
// Callers must hold at least a read lock.
func (b *InMemoryBackend) checkLBExists(lbArn string) error {
	if lbArn != "" {
		if _, ok := b.loadBalancers.Get(lbArn); !ok {
			return ErrLoadBalancerNotFound
		}
	}

	return nil
}

// DescribeListeners returns listeners filtered by load balancer ARN and/or listener ARNs.
// The returned Listener values contain a Tags pointer that is backend-owned; callers must treat it as read-only.
//
// Fast path: when only listener ARNs are supplied (no lbArn filter), look them
// up directly in the ARN-keyed map instead of scanning every listener.
func (b *InMemoryBackend) DescribeListeners(
	lbArn string,
	listenerArns []string,
) ([]Listener, error) {
	b.mu.RLock("DescribeListeners")
	defer b.mu.RUnlock()

	if err := b.checkLBExists(lbArn); err != nil {
		return nil, err
	}

	if lbArn == "" && len(listenerArns) > 0 {
		return b.describeListenersByARNs(listenerArns)
	}

	arnSet := make(map[string]bool, len(listenerArns))
	for _, a := range listenerArns {
		arnSet[a] = true
	}

	result := make([]Listener, 0, b.listeners.Len())

	for _, l := range b.listeners.All() {
		if lbArn != "" && l.LoadBalancerArn != lbArn {
			continue
		}

		if len(listenerArns) > 0 && !arnSet[l.ListenerArn] {
			continue
		}

		result = append(result, *l)
	}

	// Port is only unique per-load-balancer (CreateListener checks
	// checkDuplicateListenerPort against b.listenersByLB), so an unfiltered
	// DescribeListeners call routinely sees ties across load balancers.
	// ListenerArn breaks them so the sort order is a stable total order
	// across calls -- required because DescribeListeners pagination resumes
	// by matching a ListenerArn marker against this sorted slice, and that
	// scan silently drops listeners if tied entries can reorder between the
	// call that issued the marker and the call that consumes it (source
	// rows come from a randomized map walk).
	sort.Slice(result, func(i, j int) bool {
		if result[i].Port != result[j].Port {
			return result[i].Port < result[j].Port
		}

		return result[i].ListenerArn < result[j].ListenerArn
	})

	if len(listenerArns) > 0 {
		if err := checkAllListenerArnsFound(listenerArns, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// checkAllListenerArnsFound returns ErrListenerNotFound if any queried ARN is absent from result.
func checkAllListenerArnsFound(arns []string, result []Listener) error {
	for _, a := range arns {
		found := false
		for _, l := range result {
			if l.ListenerArn == a {
				found = true

				break
			}
		}

		if !found {
			return ErrListenerNotFound
		}
	}

	return nil
}

// DeleteListener deletes a listener by ARN.
func (b *InMemoryBackend) DeleteListener(listenerArn string) error {
	b.mu.Lock("DeleteListener")
	defer b.mu.Unlock()

	listener, ok := b.listeners.Get(listenerArn)
	if !ok {
		return ErrListenerNotFound
	}

	// Cascade: delete all rules belonging to this listener. The index lookup is
	// copied into a fresh slice first because Table.Delete mutates the very
	// index group Index.Get returns (see DeleteLoadBalancer).
	rulesToDelete := append([]*Rule(nil), b.rulesByListener.Get(listenerArn)...)
	for _, r := range rulesToDelete {
		r.Tags.Close()
		b.rules.Delete(r.RuleArn)
	}

	b.unmarkCertificatesInUse(listenerArn, listener.Certificates)
	listener.Tags.Close()
	b.listeners.Delete(listenerArn)

	return nil
}

// ModifyListener updates the properties of an existing listener.
func (b *InMemoryBackend) ModifyListener(input ModifyListenerInput) (*Listener, error) {
	b.mu.Lock("ModifyListener")
	defer b.mu.Unlock()

	l, ok := b.listeners.Get(input.ListenerArn)
	if !ok {
		return nil, ErrListenerNotFound
	}

	if err := b.applyListenerProtocol(l, input); err != nil {
		return nil, err
	}

	if input.Port != 0 && input.Port != l.Port {
		if err := checkDuplicateListenerPort(b.listenersByLB.Get(l.LoadBalancerArn), input.Port); err != nil {
			return nil, err
		}

		l.Port = input.Port
	}

	if len(input.DefaultActions) > 0 {
		if err := b.validateForwardTargetGroupsExist(input.DefaultActions); err != nil {
			return nil, err
		}

		l.DefaultActions = input.DefaultActions
		b.syncDefaultRuleActions(input.ListenerArn, input.DefaultActions)
	}

	if len(input.Certificates) > 0 {
		if err := b.validateCertificates(input.Certificates); err != nil {
			return nil, err
		}

		oldCerts := l.Certificates
		l.Certificates = input.Certificates
		b.unmarkCertificatesInUse(input.ListenerArn, oldCerts)
		b.markCertificatesInUse(input.ListenerArn, input.Certificates)
	}

	if input.SSLPolicy != "" {
		l.SSLPolicy = input.SSLPolicy
	}

	if len(input.AlpnPolicy) > 0 {
		l.AlpnPolicy = input.AlpnPolicy
	}

	if input.MutualAuthentication != nil {
		l.MutualAuthentication = input.MutualAuthentication
	}

	cp := *l

	return &cp, nil
}

// applyListenerProtocol validates and applies a protocol change to a listener.
// Caller must hold b.mu (write).
func (b *InMemoryBackend) applyListenerProtocol(l *Listener, input ModifyListenerInput) error {
	if input.Protocol == "" {
		return nil
	}

	if lb, ok := b.loadBalancers.Get(l.LoadBalancerArn); ok {
		if err := validateListenerProtocol(lb.Type, input.Protocol); err != nil {
			return err
		}
	}

	candidateCerts := l.Certificates
	if len(input.Certificates) > 0 {
		candidateCerts = input.Certificates
	}

	if err := requireCertsForProtocol(input.Protocol, candidateCerts); err != nil {
		return err
	}

	l.Protocol = input.Protocol

	return nil
}

// ModifyListenerAttributes updates attributes on a listener.
func (b *InMemoryBackend) ModifyListenerAttributes(
	listenerArn string,
	attrs map[string]string,
) (*Listener, error) {
	b.mu.Lock("ModifyListenerAttributes")
	defer b.mu.Unlock()

	l, ok := b.listeners.Get(listenerArn)
	if !ok {
		return nil, ErrListenerNotFound
	}

	if l.Attributes == nil {
		l.Attributes = make(map[string]string)
	}

	maps.Copy(l.Attributes, attrs)

	cp := *l

	return &cp, nil
}

// DescribeListenerAttributes returns attributes for a listener.
func (b *InMemoryBackend) DescribeListenerAttributes(
	listenerArn string,
) (map[string]string, error) {
	b.mu.RLock("DescribeListenerAttributes")
	defer b.mu.RUnlock()

	l, ok := b.listeners.Get(listenerArn)
	if !ok {
		return nil, ErrListenerNotFound
	}

	result := make(map[string]string, len(l.Attributes))
	maps.Copy(result, l.Attributes)

	return result, nil
}
