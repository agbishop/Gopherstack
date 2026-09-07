package elb

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strconv"
)

const (
	policyTypeAppCookie = "AppCookieStickinessPolicyType"

	policyTypeLBCookie = "LBCookieStickinessPolicyType"

	policyTypeSSLNeg = "SSLNegotiationPolicyType"

	attrRefSecurityPolicy = "Reference-Security-Policy"

	attrProtocolTLS12 = "Protocol-TLSv1.2"

	attrProtocolTLS11 = "Protocol-TLSv1.1"

	attrProtocolTLS10 = "Protocol-TLSv1"

	attrServerCipherOrder = "Server-Defined-Cipher-Order"

	attrTypeBoolean = "Boolean"

	attrTypeString = "String"

	cardinalityZeroOrOne = "ZERO_OR_ONE"
)

var (
	// policyNameRe matches valid Classic ELB policy names: 1-32 chars, alphanumeric + hyphens,
	// must start and end with alphanumeric.
	policyNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9\-]{0,30}[a-zA-Z0-9]$|^[a-zA-Z0-9]$`)

	// knownPolicyTypes is the set of built-in Classic ELB policy type names.
	knownPolicyTypes = map[string]struct{}{ //nolint:gochecknoglobals // immutable lookup table
		policyTypeAppCookie:                     {},
		policyTypeLBCookie:                      {},
		"ProxyProtocolPolicyType":               {},
		"PublicKeyPolicyType":                   {},
		policyTypeSSLNeg:                        {},
		"BackendServerAuthenticationPolicyType": {},
	}
)

// policyKey returns the compound key used to look up a policy in the policies map.
func policyKey(lbName, policyName string) string {
	return lbName + "/" + policyName
}

// validatePolicyName returns ErrInvalidParameter if the policy name is empty or does not
// match the allowed format (1-32 alphanumeric + hyphen chars, start/end alphanumeric).
func validatePolicyName(policyName string) error {
	if policyName == "" {
		return fmt.Errorf("%w: PolicyName is required", ErrInvalidParameter)
	}

	if !policyNameRe.MatchString(policyName) {
		return fmt.Errorf(
			"%w: PolicyName must be 1-32 alphanumeric characters or hyphens, starting and ending with alphanumeric",
			ErrInvalidParameter,
		)
	}

	return nil
}

// isStickinessPolicy returns true if the policy type is an app or LB cookie policy.
func isStickinessPolicy(pol *LoadBalancerPolicy) bool {
	return pol.PolicyTypeName == policyTypeAppCookie ||
		pol.PolicyTypeName == policyTypeLBCookie
}

// SetLoadBalancerPoliciesOfListener sets the policies for an existing listener.
func (b *InMemoryBackend) SetLoadBalancerPoliciesOfListener(
	ctx context.Context, name string, port int32, policyNames []string,
) error {
	b.mu.Lock("SetLoadBalancerPoliciesOfListener")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	lb, ok := b.lbs.Get(lbTableKey(region, name))
	if !ok {
		return fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	// Validate each policy exists for this LB.
	for _, p := range policyNames {
		if !b.policies.Has(policyTableKey(region, name, p)) {
			return fmt.Errorf("%w: %q", ErrPolicyNotFound, p)
		}
	}

	// Validate stickiness policies are not attached to TCP/SSL listeners.
	proto := listenerProtocolForPort(lb, port)
	if proto == protoTCP || proto == protoSSL {
		for _, pName := range policyNames {
			pol, _ := b.policies.Get(policyTableKey(region, name, pName))
			if isStickinessPolicy(pol) {
				return fmt.Errorf(
					"%w: stickiness policies cannot be applied to TCP or SSL listeners",
					ErrInvalidConfiguration,
				)
			}
		}
	}

	for i := range lb.Listeners {
		if lb.Listeners[i].LoadBalancerPort == port {
			cp := make([]string, len(policyNames))
			copy(cp, policyNames)
			lb.Listeners[i].PolicyNames = cp

			return nil
		}
	}

	return fmt.Errorf("%w: no listener on port %d", ErrListenerNotFound, port)
}

// CreateAppCookieStickinessPolicy creates an application-cookie stickiness policy.
func (b *InMemoryBackend) CreateAppCookieStickinessPolicy(
	ctx context.Context,
	name, policyName, cookieName string,
) error {
	if err := validatePolicyName(policyName); err != nil {
		return err
	}

	if cookieName == "" {
		return fmt.Errorf("%w: CookieName is required for AppCookieStickinessPolicy", ErrInvalidParameter)
	}

	b.mu.Lock("CreateAppCookieStickinessPolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.lbs.Has(lbTableKey(region, name)) {
		return fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	if b.policies.Has(policyTableKey(region, name, policyName)) {
		return fmt.Errorf("%w: %q", ErrPolicyAlreadyExists, policyName)
	}

	b.policies.Put(&LoadBalancerPolicy{
		PolicyName:       policyName,
		PolicyTypeName:   policyTypeAppCookie,
		LoadBalancerName: name,
		Region:           region,
		PolicyAttributeDescriptions: []PolicyAttribute{
			{AttributeName: "CookieName", AttributeValue: cookieName},
		},
	})

	return nil
}

// CreateLBCookieStickinessPolicy creates an LB-cookie stickiness policy.
func (b *InMemoryBackend) CreateLBCookieStickinessPolicy(
	ctx context.Context, name, policyName string, cookieExpirationPeriod int64,
) error {
	if err := validatePolicyName(policyName); err != nil {
		return err
	}

	b.mu.Lock("CreateLBCookieStickinessPolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.lbs.Has(lbTableKey(region, name)) {
		return fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	if b.policies.Has(policyTableKey(region, name, policyName)) {
		return fmt.Errorf("%w: %q", ErrPolicyAlreadyExists, policyName)
	}

	expStr := ""
	if cookieExpirationPeriod > 0 {
		expStr = strconv.FormatInt(cookieExpirationPeriod, 10)
	}

	b.policies.Put(&LoadBalancerPolicy{
		PolicyName:       policyName,
		PolicyTypeName:   policyTypeLBCookie,
		LoadBalancerName: name,
		Region:           region,
		PolicyAttributeDescriptions: []PolicyAttribute{
			{AttributeName: "CookieExpirationPeriod", AttributeValue: expStr},
		},
	})

	return nil
}

// validatePolicyAttributes returns ErrInvalidConfiguration for any attribute name in attrs
// that policyTypeName's schema (builtinPolicyTypes) does not declare (AWS:
// InvalidConfigurationRequestException, one of CreateLoadBalancerPolicy's typed errors --
// see deserializers.go's awsAwsquery_deserializeOpErrorCreateLoadBalancerPolicy). An unknown
// policyTypeName is not handled here: handler_policies.go's knownPolicyTypes check runs
// first and rejects those with PolicyTypeNotFound before this is ever called.
//
// Cardinality (ONE/ZERO_OR_ONE/ZERO_OR_MORE/ONE_OR_MORE) is deliberately not enforced: this
// package's own tests create ProxyProtocolPolicyType -- Cardinality "ONE", DefaultValue
// "false" -- while supplying zero PolicyAttributes, so a literal "single value required"
// reading contradicts this backend's established default-substitution behavior.
func validatePolicyAttributes(policyTypeName string, attrs []PolicyAttribute) error {
	if len(attrs) == 0 {
		return nil
	}

	var schema []PolicyAttributeTypeDescription

	for _, pt := range builtinPolicyTypes() {
		if pt.PolicyTypeName == policyTypeName {
			schema = pt.PolicyAttributeTypeDescriptions

			break
		}
	}

	if schema == nil {
		return nil
	}

	declared := make(map[string]bool, len(schema))
	for _, a := range schema {
		declared[a.AttributeName] = true
	}

	for _, a := range attrs {
		if !declared[a.AttributeName] {
			return fmt.Errorf(
				"%w: %q is not a valid attribute for policy type %q",
				ErrInvalidConfiguration,
				a.AttributeName,
				policyTypeName,
			)
		}
	}

	return nil
}

// CreateLoadBalancerPolicy creates a policy with custom attributes.
func (b *InMemoryBackend) CreateLoadBalancerPolicy(
	ctx context.Context,
	name, policyName, policyTypeName string,
	attrs []PolicyAttribute,
) error {
	if err := validatePolicyName(policyName); err != nil {
		return err
	}

	if err := validatePolicyAttributes(policyTypeName, attrs); err != nil {
		return err
	}

	b.mu.Lock("CreateLoadBalancerPolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	if !b.lbs.Has(lbTableKey(region, name)) {
		return fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	if b.policies.Has(policyTableKey(region, name, policyName)) {
		return fmt.Errorf("%w: %q", ErrPolicyAlreadyExists, policyName)
	}

	attrCopy := make([]PolicyAttribute, len(attrs))
	copy(attrCopy, attrs)

	b.policies.Put(&LoadBalancerPolicy{
		PolicyName:                  policyName,
		PolicyTypeName:              policyTypeName,
		LoadBalancerName:            name,
		Region:                      region,
		PolicyAttributeDescriptions: attrCopy,
	})

	return nil
}

// DeleteLoadBalancerPolicy removes a policy from a load balancer.
func (b *InMemoryBackend) DeleteLoadBalancerPolicy(ctx context.Context, name, policyName string) error {
	b.mu.Lock("DeleteLoadBalancerPolicy")
	defer b.mu.Unlock()

	region := getRegion(ctx, b.region)

	lb, ok := b.lbs.Get(lbTableKey(region, name))
	if !ok {
		return fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
	}

	k := policyTableKey(region, name, policyName)
	if !b.policies.Has(k) {
		// PolicyNotFound is not in DeleteLoadBalancerPolicy's real typed-error
		// switch (only InvalidConfigurationRequest/LoadBalancerNotFound --
		// deserializers.go's awsAwsquery_deserializeOpErrorDeleteLoadBalancerPolicy,
		// confirmed against the AWS API reference too); a real client would get
		// an untyped GenericAPIError instead of *types.PolicyNotFoundException.
		// gopherstack-5gfl: known mismatch, no confirmed correct remedy -- AWS
		// doesn't document whether this op is idempotent for a missing policy
		// the way DeleteLoadBalancer is, so this is left as-is rather than guessed.
		return fmt.Errorf("%w: %q", ErrPolicyNotFound, policyName)
	}

	// Reject deletion if the policy is currently attached to a listener. Real
	// AWS's DeleteLoadBalancerPolicy typed-error switch only recognizes
	// InvalidConfigurationRequest and LoadBalancerNotFound for this op (see
	// deserializers.go's awsAwsquery_deserializeOpErrorDeleteLoadBalancerPolicy);
	// a generic ValidationError here would not deserialize into
	// InvalidConfigurationRequestException on a real client.
	for _, l := range lb.Listeners {
		if slices.Contains(l.PolicyNames, policyName) {
			return fmt.Errorf(
				"%w: policy %q is still in use by listener on port %d",
				ErrInvalidConfiguration,
				policyName,
				l.LoadBalancerPort,
			)
		}
	}

	// Reject deletion if the policy is currently attached to a backend server.
	for _, bsd := range lb.BackendServerDescriptions {
		if slices.Contains(bsd.PolicyNames, policyName) {
			return fmt.Errorf(
				"%w: policy %q is still in use by backend server on port %d",
				ErrInvalidConfiguration,
				policyName,
				bsd.InstancePort,
			)
		}
	}

	b.policies.Delete(k)

	return nil
}

// DescribeLoadBalancerPolicies returns policies associated with the given load balancer,
// optionally filtered by policy names.
func (b *InMemoryBackend) DescribeLoadBalancerPolicies(
	ctx context.Context,
	name string,
	policyNames []string,
) ([]LoadBalancerPolicy, error) {
	b.mu.RLock("DescribeLoadBalancerPolicies")
	defer b.mu.RUnlock()

	region := getRegion(ctx, b.region)

	// When a load balancer name is given, validate it exists.
	if name != "" {
		if !b.lbs.Has(lbTableKey(region, name)) {
			return nil, fmt.Errorf("%w: %q", ErrLoadBalancerNotFound, name)
		}
	}

	filterNames := make(map[string]bool, len(policyNames))
	for _, n := range policyNames {
		filterNames[n] = true
	}

	if name == "" {
		samples := builtinSamplePolicies()
		result := make([]LoadBalancerPolicy, 0, len(samples))

		for _, p := range samples {
			if len(filterNames) > 0 && !filterNames[p.PolicyName] {
				continue
			}

			cp := p
			attrCopy := make([]PolicyAttribute, len(p.PolicyAttributeDescriptions))
			copy(attrCopy, p.PolicyAttributeDescriptions)
			cp.PolicyAttributeDescriptions = attrCopy
			result = append(result, cp)
		}

		sort.Slice(result, func(i, j int) bool { return result[i].PolicyName < result[j].PolicyName })

		return result, nil
	}

	lbPolicies := b.policiesByLB.Get(lbTableKey(region, name))
	result := make([]LoadBalancerPolicy, 0, len(lbPolicies))
	for _, p := range lbPolicies {
		if len(filterNames) > 0 && !filterNames[p.PolicyName] {
			continue
		}

		cp := *p
		attrCopy := make([]PolicyAttribute, len(p.PolicyAttributeDescriptions))
		copy(attrCopy, p.PolicyAttributeDescriptions)
		cp.PolicyAttributeDescriptions = attrCopy
		result = append(result, cp)
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].PolicyName < result[j].PolicyName
	})

	return result, nil
}

// builtinSamplePolicies returns the predefined Classic ELB SSL/cipher reference policies
// that AWS ships by default. These are returned by DescribeLoadBalancerPolicies when no
// LoadBalancerName is specified.
func builtinSamplePolicies() []LoadBalancerPolicy {
	return []LoadBalancerPolicy{
		{
			PolicyName:       "ELBSecurityPolicy-2016-08",
			PolicyTypeName:   policyTypeSSLNeg,
			LoadBalancerName: "",
			PolicyAttributeDescriptions: []PolicyAttribute{
				{AttributeName: attrRefSecurityPolicy, AttributeValue: "ELBSecurityPolicy-2016-08"},
				{AttributeName: attrProtocolTLS12, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS11, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS10, AttributeValue: boolStrFalse},
				{AttributeName: attrServerCipherOrder, AttributeValue: boolStrTrue},
			},
		},
		{
			PolicyName:       "ELBSecurityPolicy-TLS-1-2-2017-01",
			PolicyTypeName:   policyTypeSSLNeg,
			LoadBalancerName: "",
			PolicyAttributeDescriptions: []PolicyAttribute{
				{AttributeName: attrRefSecurityPolicy, AttributeValue: "ELBSecurityPolicy-TLS-1-2-2017-01"},
				{AttributeName: attrProtocolTLS12, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS11, AttributeValue: boolStrFalse},
				{AttributeName: attrProtocolTLS10, AttributeValue: boolStrFalse},
				{AttributeName: attrServerCipherOrder, AttributeValue: boolStrTrue},
			},
		},
		{
			PolicyName:     "ELBSample-ELBDefaultNegotiationPolicy",
			PolicyTypeName: policyTypeSSLNeg,
			PolicyAttributeDescriptions: []PolicyAttribute{
				{AttributeName: attrProtocolTLS12, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS11, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS10, AttributeValue: boolStrTrue},
				{AttributeName: attrServerCipherOrder, AttributeValue: boolStrFalse},
			},
		},
		{
			PolicyName:     "ELBSample-OpenSSLDefaultCipherPolicy",
			PolicyTypeName: policyTypeSSLNeg,
			PolicyAttributeDescriptions: []PolicyAttribute{
				{AttributeName: attrProtocolTLS12, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS11, AttributeValue: boolStrTrue},
				{AttributeName: attrProtocolTLS10, AttributeValue: boolStrTrue},
				{AttributeName: attrServerCipherOrder, AttributeValue: boolStrFalse},
			},
		},
	}
}

// builtinPolicyTypes returns the built-in Classic ELB policy type descriptions.
func builtinPolicyTypes() []PolicyTypeDescription {
	return []PolicyTypeDescription{
		{
			PolicyTypeName: policyTypeAppCookie,
			Description:    "Stickiness policy with sticky session lifetimes controlled by the application-generated cookie.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: "CookieName",
					AttributeType: attrTypeString,
					Cardinality:   "ONE",
					Description:   "The name of the application cookie used for stickiness.",
				},
			},
		},
		{
			PolicyTypeName: policyTypeLBCookie,
			Description:    "Stickiness policy with sticky session lifetimes controlled by the browser or an expiration period.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: "CookieExpirationPeriod",
					AttributeType: "Long",
					Cardinality:   cardinalityZeroOrOne,
					Description:   "The time period, in seconds, after which the cookie should be considered stale.",
				},
			},
		},
		{
			PolicyTypeName: "ProxyProtocolPolicyType",
			Description:    "Policy that enables Proxy Protocol on the load balancer.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: "ProxyProtocol", AttributeType: attrTypeBoolean,
					Cardinality: "ONE", DefaultValue: boolStrFalse,
					Description: "Enable or disable Proxy Protocol support.",
				},
			},
		},
		{
			PolicyTypeName: "PublicKeyPolicyType",
			Description:    "Policy that holds a public key for back-end server authentication.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: "PublicKey", AttributeType: attrTypeString,
					Cardinality: "ONE_OR_MORE",
					Description: "The public key used to authenticate back-end servers.",
				},
			},
		},
		{
			PolicyTypeName: "BackendServerAuthenticationPolicyType",
			Description: "Policy that enables authentication between the load balancer " +
				"and back-end instances.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: "PublicKeyPolicyName", AttributeType: "PolicyName",
					Cardinality: "ONE_OR_MORE",
					Description: "The policy name of a PublicKeyPolicy.",
				},
			},
		},
		{
			PolicyTypeName: policyTypeSSLNeg,
			Description: "Policy that configures front-end connections using the protocols " +
				"and ciphers available in the OpenSSL library.",
			PolicyAttributeTypeDescriptions: []PolicyAttributeTypeDescription{
				{
					AttributeName: attrProtocolTLS12, AttributeType: attrTypeBoolean,
					Cardinality: cardinalityZeroOrOne, DefaultValue: boolStrTrue,
				},
				{
					AttributeName: attrProtocolTLS11, AttributeType: attrTypeBoolean,
					Cardinality: cardinalityZeroOrOne, DefaultValue: boolStrFalse,
				},
				{
					AttributeName: attrProtocolTLS10, AttributeType: attrTypeBoolean,
					Cardinality: cardinalityZeroOrOne, DefaultValue: boolStrFalse,
				},
				{
					AttributeName: attrServerCipherOrder, AttributeType: attrTypeBoolean,
					Cardinality: cardinalityZeroOrOne, DefaultValue: boolStrFalse,
				},
				{
					AttributeName: attrRefSecurityPolicy, AttributeType: attrTypeString,
					Cardinality: cardinalityZeroOrOne,
					Description: "The reference security policy name. " +
						"Mutually exclusive with explicit protocol/cipher attributes.",
				},
			},
		},
	}
}

// DescribeLoadBalancerPolicyTypes returns the specified policy type descriptions.
// If policyTypeNames is non-empty, an error is returned for any unknown type name.
func (b *InMemoryBackend) DescribeLoadBalancerPolicyTypes(
	_ context.Context, policyTypeNames []string,
) ([]PolicyTypeDescription, error) {
	all := builtinPolicyTypes()

	if len(policyTypeNames) == 0 {
		return all, nil
	}

	byName := make(map[string]PolicyTypeDescription, len(all))
	for _, pt := range all {
		byName[pt.PolicyTypeName] = pt
	}

	result := make([]PolicyTypeDescription, 0, len(policyTypeNames))

	for _, typeName := range policyTypeNames {
		pt, ok := byName[typeName]
		if !ok {
			return nil, fmt.Errorf("%w: policy type %q not found", ErrPolicyTypeNotFound, typeName)
		}

		result = append(result, pt)
	}

	return result, nil
}
