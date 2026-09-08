package elbv2

import (
	"fmt"
	"maps"
	"sort"
	"strings"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/collections"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

// isValidTargetType returns true if the target type is a recognized ELBv2 value.
func isValidTargetType(tt string) bool {
	switch tt {
	case "instance", "ip", targetTypeLambda, "alb":
		return true
	}

	return false
}

// isValidTGProtocol returns true if the protocol is accepted for non-lambda target groups.
func isValidTGProtocol(proto string) bool {
	switch proto {
	case protoHTTP, protoHTTPS, "TCP", protoTLS, "UDP", "TCP_UDP", "GENEVE":
		return true
	}

	return false
}

func (b *InMemoryBackend) tgARN(name string) string {
	return arn.Build(
		"elasticloadbalancing",
		b.region,
		b.accountID,
		"targetgroup/"+name+"/0123456789abcdef",
	)
}

// checkAllTGArnsFound returns ErrTargetGroupNotFound if any queried ARN is absent from result.
func checkAllTGArnsFound(arns []string, result []TargetGroup) error {
	for _, a := range arns {
		found := false
		for _, tg := range result {
			if tg.TargetGroupArn == a {
				found = true

				break
			}
		}

		if !found {
			return ErrTargetGroupNotFound
		}
	}

	return nil
}

// checkAllTGNamesFound returns ErrTargetGroupNotFound if any queried name is absent from result.
func checkAllTGNamesFound(names []string, result []TargetGroup) error {
	for _, n := range names {
		found := false
		for _, tg := range result {
			if tg.TargetGroupName == n {
				found = true

				break
			}
		}

		if !found {
			return ErrTargetGroupNotFound
		}
	}

	return nil
}

// CreateTargetGroup creates a new target group.
func (b *InMemoryBackend) CreateTargetGroup(input CreateTargetGroupInput) (*TargetGroup, error) {
	b.mu.Lock("CreateTargetGroup")
	defer b.mu.Unlock()

	if input.Name == "" {
		return nil, fmt.Errorf("%w: Name is required", ErrInvalidParameter)
	}

	if err := validateResourceName(input.Name, "target group"); err != nil {
		return nil, err
	}

	for _, tg := range b.targetGroups.All() {
		if tg.TargetGroupName == input.Name {
			return nil, ErrTargetGroupAlreadyExists
		}
	}

	tgArn := b.tgARN(input.Name)

	targetType := input.TargetType
	if targetType == "" {
		targetType = "instance"
	}

	if !isValidTargetType(targetType) {
		return nil, fmt.Errorf(
			"%w: invalid TargetType %q; must be instance, ip, lambda, or alb",
			ErrInvalidParameter, targetType,
		)
	}

	proto := input.Protocol
	if targetType == targetTypeLambda {
		// Lambda target groups have no protocol or port.
		proto = ""
	} else {
		if proto == "" {
			proto = protoHTTP
		}

		if !isValidTGProtocol(proto) {
			return nil, fmt.Errorf(
				"%w: invalid Protocol %q for target group",
				ErrInvalidParameter, proto,
			)
		}
	}

	t := tags.New("elbv2.tg." + input.Name + ".tags")
	for _, kv := range input.Tags {
		t.Set(kv.Key, kv.Value)
	}

	// Apply health-check defaults.
	input = applyTGHealthCheckDefaults(proto, input)

	if err := validateHealthCheckPath(input.HealthCheckPath); err != nil {
		return nil, err
	}

	matcher := defaultTGMatcher(input.HealthCheckProtocol, input.Matcher)

	tg := &TargetGroup{
		TargetGroupArn:             tgArn,
		TargetGroupName:            input.Name,
		Protocol:                   proto,
		ProtocolVersion:            input.ProtocolVersion,
		Port:                       input.Port,
		VpcID:                      input.VpcID,
		TargetType:                 targetType,
		HealthCheckProtocol:        input.HealthCheckProtocol,
		HealthCheckPort:            input.HealthCheckPort,
		HealthCheckPath:            input.HealthCheckPath,
		Matcher:                    matcher,
		HealthCheckEnabled:         input.HealthCheckEnabled,
		HealthCheckIntervalSeconds: input.HealthCheckIntervalSeconds,
		HealthCheckTimeoutSeconds:  input.HealthCheckTimeoutSeconds,
		HealthyThresholdCount:      input.HealthyThresholdCount,
		UnhealthyThresholdCount:    input.UnhealthyThresholdCount,
		CrossZoneLoadBalancing:     true,
		Targets:                    []Target{},
		TargetGroupAttributes: map[string]string{
			"deregistration_delay.timeout_seconds": "300",
			"stickiness.enabled":                   attrValueFalse,
			"stickiness.type":                      "lb_cookie",
			"load_balancing.algorithm.type":        "round_robin",
			"slow_start.duration_seconds":          "0",
			attrCrossZoneLoadBalancingEnabled:      attrValueTrue,
		},
		Tags: t,
	}

	b.targetGroups.Put(tg)

	cp := *tg

	return &cp, nil
}

// albDefaultAttributes returns the default attributes map for an Application Load Balancer.
func albDefaultAttributes() map[string]string {
	return map[string]string{
		attrAccessLogsS3Enabled:                                            attrValueFalse,
		attrDeletionProtectionEnabled:                                      attrValueFalse,
		attrCrossZoneLoadBalancingEnabled:                                  attrValueTrue,
		"idle_timeout.timeout_seconds":                                     "60",
		"routing.http2.enabled":                                            attrValueTrue,
		"routing.http.desync_mitigation_mode":                              "defensive",
		"routing.http.drop_invalid_header_fields.enabled":                  attrValueFalse,
		"routing.http.preserve_host_header.enabled":                        attrValueFalse,
		"routing.http.xff_client_port.enabled":                             attrValueFalse,
		"routing.http.xff_header_processing.mode":                          "append",
		"waf.fail_open.enabled":                                            attrValueFalse,
		"routing.http.response.server.enabled":                             attrValueTrue,
		"routing.http.response.strict_transport_security.enabled":          attrValueFalse,
		"routing.http.response.access_control_allow_origin.header_value":   "",
		"routing.http.response.access_control_allow_methods.header_value":  "",
		"routing.http.response.access_control_allow_headers.header_value":  "",
		"routing.http.response.access_control_expose_headers.header_value": "",
		"routing.http.response.access_control_max_age.header_value":        "",
		"routing.http.response.content_security_policy.header_value":       "",
		"routing.http.response.x_content_type_options.header_value":        "",
		"routing.http.response.x_frame_options.header_value":               "",
	}
}

// validateHealthCheckPath returns ErrInvalidParameter if the path is non-empty and does not start with '/'.
func validateHealthCheckPath(path string) error {
	if path != "" && !strings.HasPrefix(path, "/") {
		return fmt.Errorf("%w: HealthCheckPath must begin with '/'", ErrInvalidParameter)
	}

	return nil
}

func applyTGHealthCheckDefaults(proto string, input CreateTargetGroupInput) CreateTargetGroupInput {
	if input.HealthCheckProtocol == "" {
		input.HealthCheckProtocol = proto
	}

	if input.HealthCheckPort == "" {
		input.HealthCheckPort = "traffic-port"
	}

	if input.HealthCheckPath == "" && (proto == protoHTTP || proto == protoHTTPS) {
		input.HealthCheckPath = "/"
	}

	if input.HealthCheckIntervalSeconds == 0 {
		input.HealthCheckIntervalSeconds = 30
	}

	if input.HealthCheckTimeoutSeconds == 0 {
		input.HealthCheckTimeoutSeconds = 5
	}

	if input.HealthyThresholdCount == 0 {
		input.HealthyThresholdCount = 3
	}

	if input.UnhealthyThresholdCount == 0 {
		input.UnhealthyThresholdCount = 3
	}

	return input
}

func defaultTGMatcher(hcProtocol string, matcher Matcher) Matcher {
	if matcher.HTTPCode == "" && matcher.GrpcCode == "" &&
		(hcProtocol == protoHTTP || hcProtocol == protoHTTPS) {
		matcher.HTTPCode = "200"
	}

	return matcher
}

func collectTGArns(actions []Action, arns map[string]bool) {
	for _, a := range actions {
		if a.TargetGroupArn != "" {
			arns[a.TargetGroupArn] = true
		}

		if a.ForwardConfig != nil {
			for _, tgt := range a.ForwardConfig.TargetGroups {
				if tgt.TargetGroupArn != "" {
					arns[tgt.TargetGroupArn] = true
				}
			}
		}
	}
}

// validateForwardTargetGroupsExist returns ErrTargetGroupNotFound if any
// action's forward target group reference does not exist. AWS:
// CreateListener/ModifyListener/CreateRule/ModifyRule each model
// TargetGroupNotFound for exactly this condition. Caller must hold b.mu.
func (b *InMemoryBackend) validateForwardTargetGroupsExist(actions []Action) error {
	arns := make(map[string]bool)
	collectTGArns(actions, arns)

	for tgArn := range arns {
		if !b.targetGroups.Has(tgArn) {
			return ErrTargetGroupNotFound
		}
	}

	return nil
}

func collectLBArnsForTG(lbArn string, actions []Action, result map[string]map[string]bool) {
	for _, a := range actions {
		if a.TargetGroupArn != "" {
			if result[a.TargetGroupArn] == nil {
				result[a.TargetGroupArn] = make(map[string]bool)
			}

			result[a.TargetGroupArn][lbArn] = true
		}

		if a.ForwardConfig != nil {
			for _, tgt := range a.ForwardConfig.TargetGroups {
				if tgt.TargetGroupArn != "" {
					if result[tgt.TargetGroupArn] == nil {
						result[tgt.TargetGroupArn] = make(map[string]bool)
					}

					result[tgt.TargetGroupArn][lbArn] = true
				}
			}
		}
	}
}

func actionsReferenceTG(actions []Action, tgArn string) bool {
	for _, a := range actions {
		if a.TargetGroupArn == tgArn {
			return true
		}

		if a.ForwardConfig != nil {
			for _, tgt := range a.ForwardConfig.TargetGroups {
				if tgt.TargetGroupArn == tgArn {
					return true
				}
			}
		}
	}

	return false
}

// tgArnsForLB returns the set of target group ARNs referenced by listeners and rules on the given LB.
// Caller must hold b.mu (read or write).
func (b *InMemoryBackend) tgArnsForLB(lbArn string) map[string]bool {
	arns := make(map[string]bool)

	for _, l := range b.listenersByLB.Get(lbArn) {
		collectTGArns(l.DefaultActions, arns)

		for _, r := range b.rulesByListener.Get(l.ListenerArn) {
			collectTGArns(r.Actions, arns)
		}
	}

	return arns
}

// tgToLBArnsLocked returns a map from target group ARN to the set of LB ARNs that reference it.
// Caller must hold b.mu (read or write).
func (b *InMemoryBackend) tgToLBArnsLocked() map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	for _, l := range b.listeners.All() {
		collectLBArnsForTG(l.LoadBalancerArn, l.DefaultActions, result)

		for _, r := range b.rulesByListener.Get(l.ListenerArn) {
			collectLBArnsForTG(l.LoadBalancerArn, r.Actions, result)
		}
	}

	return result
}

// DescribeTargetGroups returns target groups filtered by ARNs, names, or load balancer ARN.
// The returned TargetGroup values contain a Tags pointer that is backend-owned; callers must treat it as read-only.
//
// Fast path: when only ARNs are supplied (no names, no lbArn), look them up
// directly in the ARN-keyed map instead of scanning every target group.
func (b *InMemoryBackend) DescribeTargetGroups(
	arns []string,
	names []string,
	lbArn string,
) ([]TargetGroup, error) {
	b.mu.RLock("DescribeTargetGroups")
	defer b.mu.RUnlock()

	// Build TG -> LB ARNs mapping to populate LoadBalancerArns field.
	tgLBMap := b.tgToLBArnsLocked()

	if len(arns) > 0 && len(names) == 0 && lbArn == "" {
		result := make([]TargetGroup, 0, len(arns))

		for _, a := range arns {
			tg, ok := b.targetGroups.Get(a)
			if !ok {
				continue
			}

			cp := *tg
			cp.LoadBalancerArns = sortedLBArns(tgLBMap[tg.TargetGroupArn])
			result = append(result, cp)
		}

		sortTargetGroupsByName(result)

		if err := checkAllTGArnsFound(arns, result); err != nil {
			return nil, err
		}

		return result, nil
	}

	result := b.filterTargetGroupsLocked(arns, names, lbArn, tgLBMap)
	sortTargetGroupsByName(result)

	if len(arns) > 0 {
		if err := checkAllTGArnsFound(arns, result); err != nil {
			return nil, err
		}
	}

	if len(names) > 0 {
		if err := checkAllTGNamesFound(names, result); err != nil {
			return nil, err
		}
	}

	return result, nil
}

// filterTargetGroupsLocked scans all target groups and returns those matching
// the supplied filters. Caller must hold b.mu (read or write).
func (b *InMemoryBackend) filterTargetGroupsLocked(
	arns, names []string,
	lbArn string,
	tgLBMap map[string]map[string]bool,
) []TargetGroup {
	arnSet := make(map[string]bool, len(arns))
	for _, a := range arns {
		arnSet[a] = true
	}

	nameSet := make(map[string]bool, len(names))
	for _, n := range names {
		nameSet[n] = true
	}

	var lbTGArns map[string]bool
	if lbArn != "" {
		lbTGArns = b.tgArnsForLB(lbArn)
	}

	result := make([]TargetGroup, 0, b.targetGroups.Len())

	for _, tg := range b.targetGroups.All() {
		if len(arns) > 0 && !arnSet[tg.TargetGroupArn] {
			continue
		}

		if len(names) > 0 && !nameSet[tg.TargetGroupName] {
			continue
		}

		if lbTGArns != nil && !lbTGArns[tg.TargetGroupArn] {
			continue
		}

		cp := *tg
		cp.LoadBalancerArns = sortedLBArns(tgLBMap[tg.TargetGroupArn])
		result = append(result, cp)
	}

	return result
}

// sortTargetGroupsByName sorts target groups by name in ascending order.
func sortTargetGroupsByName(result []TargetGroup) {
	sort.Slice(result, func(i, j int) bool {
		return result[i].TargetGroupName < result[j].TargetGroupName
	})
}

// sortedLBArns flattens a set of load balancer ARNs into a sorted slice.
func sortedLBArns(set map[string]bool) []string {
	out := collections.SortedKeys(set)

	return out
}

// isTGInUseLocked returns true if the target group ARN is referenced by any listener or rule.
// Caller must hold b.mu (read or write).
func (b *InMemoryBackend) isTGInUseLocked(tgArn string) bool {
	for _, l := range b.listeners.All() {
		if actionsReferenceTG(l.DefaultActions, tgArn) {
			return true
		}
	}

	for _, r := range b.rules.All() {
		if actionsReferenceTG(r.Actions, tgArn) {
			return true
		}
	}

	return false
}

// DeleteTargetGroup deletes a target group by ARN.
// AWS: DeleteTargetGroup's own error switch models only ResourceInUse -- no
// TargetGroupNotFound -- so it is idempotent on a missing target group.
func (b *InMemoryBackend) DeleteTargetGroup(tgArn string) error {
	b.mu.Lock("DeleteTargetGroup")
	defer b.mu.Unlock()

	if _, ok := b.targetGroups.Get(tgArn); !ok {
		return nil
	}

	if b.isTGInUseLocked(tgArn) {
		return fmt.Errorf(
			"%w: target group %s is still in use by a listener or rule",
			ErrTargetGroupInUse,
			tgArn,
		)
	}

	tg, _ := b.targetGroups.Get(tgArn)
	tg.Tags.Close()
	b.targetGroups.Delete(tgArn)
	delete(b.targetReadyAt, tgArn)
	delete(b.targetDrainingUntil, tgArn)

	return nil
}

// ModifyTargetGroup updates health-check settings on a target group.
func (b *InMemoryBackend) ModifyTargetGroup(input ModifyTargetGroupInput) (*TargetGroup, error) {
	b.mu.Lock("ModifyTargetGroup")
	defer b.mu.Unlock()

	tg, ok := b.targetGroups.Get(input.TargetGroupArn)
	if !ok {
		return nil, ErrTargetGroupNotFound
	}

	if input.HealthCheckProtocol != "" {
		tg.HealthCheckProtocol = input.HealthCheckProtocol
	}

	if input.HealthCheckPort != "" {
		tg.HealthCheckPort = input.HealthCheckPort
	}

	if input.HealthCheckPath != "" {
		if err := validateHealthCheckPath(input.HealthCheckPath); err != nil {
			return nil, err
		}

		tg.HealthCheckPath = input.HealthCheckPath
	}

	if input.Matcher.HTTPCode != "" || input.Matcher.GrpcCode != "" {
		tg.Matcher = input.Matcher
	}

	if input.HealthCheckEnabled != nil {
		tg.HealthCheckEnabled = *input.HealthCheckEnabled
	}

	if input.HealthCheckIntervalSeconds != 0 {
		tg.HealthCheckIntervalSeconds = input.HealthCheckIntervalSeconds
	}

	if input.HealthCheckTimeoutSeconds != 0 {
		tg.HealthCheckTimeoutSeconds = input.HealthCheckTimeoutSeconds
	}

	if input.HealthyThresholdCount != 0 {
		tg.HealthyThresholdCount = input.HealthyThresholdCount
	}

	if input.UnhealthyThresholdCount != 0 {
		tg.UnhealthyThresholdCount = input.UnhealthyThresholdCount
	}

	cp := *tg

	return &cp, nil
}

// ModifyTargetGroupAttributes updates attributes on a target group.
func (b *InMemoryBackend) ModifyTargetGroupAttributes(
	tgArn string,
	attrs map[string]string,
) (*TargetGroup, error) {
	b.mu.Lock("ModifyTargetGroupAttributes")
	defer b.mu.Unlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return nil, ErrTargetGroupNotFound
	}

	if tg.TargetGroupAttributes == nil {
		tg.TargetGroupAttributes = make(map[string]string)
	}

	maps.Copy(tg.TargetGroupAttributes, attrs)

	cp := *tg

	return &cp, nil
}

// DescribeTargetGroupAttributes returns attributes for a target group.
func (b *InMemoryBackend) DescribeTargetGroupAttributes(tgArn string) (map[string]string, error) {
	b.mu.RLock("DescribeTargetGroupAttributes")
	defer b.mu.RUnlock()

	tg, ok := b.targetGroups.Get(tgArn)
	if !ok {
		return nil, ErrTargetGroupNotFound
	}

	result := make(map[string]string, len(tg.TargetGroupAttributes))
	maps.Copy(result, tg.TargetGroupAttributes)

	return result, nil
}
