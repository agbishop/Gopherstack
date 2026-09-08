package elb_test

import (
	"context"
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/elb"
)

// fakeEC2Resolver is a test double for elb.EC2Resolver backed by static sets,
// standing in for the real services/ec2 backend cli.go wires in.
type fakeEC2Resolver struct {
	securityGroups map[string]bool
	subnets        map[string]bool
	instances      map[string]bool
}

func (f *fakeEC2Resolver) SecurityGroupExists(id string) bool { return f.securityGroups[id] }
func (f *fakeEC2Resolver) SubnetExists(id string) bool        { return f.subnets[id] }
func (f *fakeEC2Resolver) InstanceExists(id string) bool      { return f.instances[id] }

// fakeCertResolver is a test double for elb.CertificateResolver backed by a
// static set, standing in for the real services/acm and services/iam
// backends cli.go wires in.
type fakeCertResolver struct {
	certs map[string]bool
}

func (f *fakeCertResolver) ResolveCertificate(_ context.Context, certARN string) bool {
	return f.certs[certARN]
}

func TestApplySecurityGroupsToLoadBalancer_EC2Resolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		resolver   elb.EC2Resolver
		name       string
		wantStatus int
	}{
		{
			name:       "no_resolver_wired_accepts_any_id",
			resolver:   nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "known_security_group_accepted",
			resolver:   &fakeEC2Resolver{securityGroups: map[string]bool{"sg-real": true}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_security_group_rejected",
			resolver:   &fakeEC2Resolver{securityGroups: map[string]bool{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			if tt.resolver != nil {
				backend.SetEC2Resolver(tt.resolver)
			}

			h := elb.NewHandler(backend)
			mustCreateVPCLB(t, h, "sg-resolver-lb")

			rec := doELB(t, h, url.Values{
				"Action":                  {"ApplySecurityGroupsToLoadBalancer"},
				"Version":                 {"2012-06-01"},
				"LoadBalancerName":        {"sg-resolver-lb"},
				"SecurityGroups.member.1": {"sg-real"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestAttachLoadBalancerToSubnets_EC2Resolver(t *testing.T) {
	t.Parallel()

	tests := []struct {
		resolver   elb.EC2Resolver
		name       string
		wantStatus int
	}{
		{
			name:       "no_resolver_wired_accepts_any_id",
			resolver:   nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "known_subnet_accepted",
			resolver:   &fakeEC2Resolver{subnets: map[string]bool{"subnet-00001": true, "subnet-real": true}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_subnet_rejected",
			resolver:   &fakeEC2Resolver{subnets: map[string]bool{"subnet-00001": true}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			if tt.resolver != nil {
				backend.SetEC2Resolver(tt.resolver)
			}

			h := elb.NewHandler(backend)
			mustCreateVPCLB(t, h, "subnet-resolver-lb")

			rec := doELB(t, h, url.Values{
				"Action":           {"AttachLoadBalancerToSubnets"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"subnet-resolver-lb"},
				"Subnets.member.1": {"subnet-real"},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestRegisterInstancesWithLoadBalancer_EC2Resolver(t *testing.T) {
	t.Parallel()

	const knownInstance = "i-0123456789abcdef0"

	tests := []struct {
		resolver   elb.EC2Resolver
		name       string
		wantStatus int
	}{
		{
			name:       "no_resolver_wired_accepts_any_id",
			resolver:   nil,
			wantStatus: http.StatusOK,
		},
		{
			name:       "known_instance_accepted",
			resolver:   &fakeEC2Resolver{instances: map[string]bool{knownInstance: true}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_instance_rejected",
			resolver:   &fakeEC2Resolver{instances: map[string]bool{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			if tt.resolver != nil {
				backend.SetEC2Resolver(tt.resolver)
			}

			h := elb.NewHandler(backend)
			mustCreateLB(t, h, "instance-resolver-lb")

			rec := doELB(t, h, url.Values{
				"Action":                        {"RegisterInstancesWithLoadBalancer"},
				"Version":                       {"2012-06-01"},
				"LoadBalancerName":              {"instance-resolver-lb"},
				"Instances.member.1.InstanceId": {knownInstance},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestSetLoadBalancerListenerSSLCertificate_CertificateResolver(t *testing.T) {
	t.Parallel()

	const knownCert = "arn:aws:acm:us-east-1:123456789012:certificate/known"

	tests := []struct {
		resolver    elb.CertificateResolver
		name        string
		initialCert string
		certID      string
		wantStatus  int
	}{
		{
			name:        "no_resolver_wired_accepts_any_cert",
			resolver:    nil,
			initialCert: "arn:aws:acm:us-east-1:123456789012:certificate/unknown",
			certID:      "arn:aws:acm:us-east-1:123456789012:certificate/unknown",
			wantStatus:  http.StatusOK,
		},
		{
			name:        "known_certificate_accepted",
			resolver:    &fakeCertResolver{certs: map[string]bool{knownCert: true}},
			initialCert: knownCert,
			certID:      knownCert,
			wantStatus:  http.StatusOK,
		},
		{
			name:        "unknown_certificate_rejected",
			resolver:    &fakeCertResolver{certs: map[string]bool{knownCert: true}},
			initialCert: knownCert,
			certID:      "arn:aws:acm:us-east-1:123456789012:certificate/unknown",
			wantStatus:  http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			if tt.resolver != nil {
				backend.SetCertificateResolver(tt.resolver)
			}

			h := elb.NewHandler(backend)
			mustCreateLB(t, h, "cert-resolver-lb")
			createRec := doELB(t, h, url.Values{
				"Action":                              {"CreateLoadBalancerListeners"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"cert-resolver-lb"},
				"Listeners.member.1.Protocol":         {"HTTPS"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"8443"},
				"Listeners.member.1.SSLCertificateId": {tt.initialCert},
			})
			require.Equal(t, http.StatusOK, createRec.Code)

			rec := doELB(t, h, url.Values{
				"Action":           {"SetLoadBalancerListenerSSLCertificate"},
				"Version":          {"2012-06-01"},
				"LoadBalancerName": {"cert-resolver-lb"},
				"LoadBalancerPort": {"443"},
				"SSLCertificateId": {tt.certID},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestCreateLoadBalancerListeners_CertificateResolver(t *testing.T) {
	t.Parallel()

	const knownCert = "arn:aws:iam::123456789012:server-certificate/known"

	tests := []struct {
		resolver   elb.CertificateResolver
		name       string
		wantStatus int
	}{
		{
			name:       "known_certificate_accepted",
			resolver:   &fakeCertResolver{certs: map[string]bool{knownCert: true}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_certificate_rejected",
			resolver:   &fakeCertResolver{certs: map[string]bool{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			backend.SetCertificateResolver(tt.resolver)
			h := elb.NewHandler(backend)
			mustCreateLB(t, h, "createlisteners-cert-lb")

			rec := doELB(t, h, url.Values{
				"Action":                              {"CreateLoadBalancerListeners"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"createlisteners-cert-lb"},
				"Listeners.member.1.Protocol":         {"HTTPS"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"8443"},
				"Listeners.member.1.SSLCertificateId": {knownCert},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

func TestCreateLoadBalancer_InlineHTTPSCertificateResolver(t *testing.T) {
	t.Parallel()

	const knownCert = "arn:aws:acm:us-east-1:123456789012:certificate/known"

	tests := []struct {
		resolver   elb.CertificateResolver
		name       string
		wantStatus int
	}{
		{
			name:       "known_certificate_accepted",
			resolver:   &fakeCertResolver{certs: map[string]bool{knownCert: true}},
			wantStatus: http.StatusOK,
		},
		{
			name:       "unknown_certificate_rejected",
			resolver:   &fakeCertResolver{certs: map[string]bool{}},
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			backend := newBackend()
			backend.SetCertificateResolver(tt.resolver)
			h := elb.NewHandler(backend)

			rec := doELB(t, h, url.Values{
				"Action":                              {"CreateLoadBalancer"},
				"Version":                             {"2012-06-01"},
				"LoadBalancerName":                    {"create-inline-https-lb"},
				"AvailabilityZones.member.1":          {"us-east-1a"},
				"Listeners.member.1.Protocol":         {"HTTPS"},
				"Listeners.member.1.LoadBalancerPort": {"443"},
				"Listeners.member.1.InstancePort":     {"8443"},
				"Listeners.member.1.SSLCertificateId": {knownCert},
			})
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}
