package elbv2

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awserr"
)

var (
	// ErrLoadBalancerNotFound is returned when the requested load balancer does not exist.
	ErrLoadBalancerNotFound = awserr.New("LoadBalancerNotFound", awserr.ErrNotFound)
	// ErrTargetGroupNotFound is returned when the requested target group does not exist.
	ErrTargetGroupNotFound = awserr.New("TargetGroupNotFound", awserr.ErrNotFound)
	// ErrListenerNotFound is returned when the requested listener does not exist.
	ErrListenerNotFound = awserr.New("ListenerNotFound", awserr.ErrNotFound)
	// ErrRuleNotFound is returned when the requested rule does not exist.
	ErrRuleNotFound = awserr.New("RuleNotFound", awserr.ErrNotFound)
	// ErrTrustStoreNotFound is returned when the requested trust store does not exist.
	ErrTrustStoreNotFound = awserr.New("TrustStoreNotFound", awserr.ErrNotFound)
	// ErrLoadBalancerAlreadyExists is returned when a load balancer with that name already exists.
	ErrLoadBalancerAlreadyExists = awserr.New("DuplicateLoadBalancerName", awserr.ErrAlreadyExists)
	// ErrTargetGroupAlreadyExists is returned when a target group with that name already exists.
	ErrTargetGroupAlreadyExists = awserr.New("DuplicateTargetGroupName", awserr.ErrAlreadyExists)
	// ErrTrustStoreAlreadyExists is returned when a trust store with that name already exists.
	ErrTrustStoreAlreadyExists = awserr.New("DuplicateTrustStoreName", awserr.ErrAlreadyExists)
	// ErrInvalidParameter is returned when a request parameter is invalid or missing.
	ErrInvalidParameter = awserr.New("ValidationError", awserr.ErrInvalidParameter)
	// ErrUnknownAction is returned when the requested action is not recognized.
	ErrUnknownAction = awserr.New("InvalidAction", awserr.ErrInvalidParameter)
	// ErrDuplicateRulePriority is returned when two rules have the same priority.
	// AWS's real error code for this condition is "PriorityInUse" (PriorityInUseException),
	// not "DuplicatePriority" — verified against aws-sdk-go-v2/service/elasticloadbalancingv2/types.
	ErrDuplicateRulePriority = awserr.New("PriorityInUse", awserr.ErrInvalidParameter)
	// ErrOperationNotPermitted is returned when the operation is not allowed (e.g. deleting default rule).
	ErrOperationNotPermitted = awserr.New("OperationNotPermitted", awserr.ErrInvalidParameter)
	// ErrDuplicateListener is returned when a listener on the same port already exists.
	ErrDuplicateListener = awserr.New("DuplicateListener", awserr.ErrAlreadyExists)
	// ErrTargetGroupInUse is returned when attempting to delete a target group that is still referenced.
	ErrTargetGroupInUse = awserr.New("ResourceInUse", awserr.ErrInvalidParameter)
	// ErrInvalidConfigurationRequest is returned when a configuration is invalid for the LB type.
	ErrInvalidConfigurationRequest = awserr.New(
		"InvalidConfigurationRequest",
		awserr.ErrInvalidParameter,
	)
	// ErrResourcePolicyNotFound is returned when no resource policy is set for a resource.
	ErrResourcePolicyNotFound = awserr.New("ResourceNotFound", awserr.ErrNotFound)
	// ErrTrustStoreAssociationNotFound is returned when a shared trust store association does not exist.
	ErrTrustStoreAssociationNotFound = awserr.New("AssociationNotFound", awserr.ErrNotFound)
	// ErrRevocationIDNotFound is returned when the requested revocation ID does not exist
	// on the trust store (GetTrustStoreRevocationContent).
	ErrRevocationIDNotFound = awserr.New("RevocationIdNotFound", awserr.ErrNotFound)
	// ErrCertificateNotFound is returned when a listener's CertificateArn does not
	// resolve against ACM or IAM. Modeled on CreateListener, ModifyListener and
	// AddListenerCertificates (CertificateNotFoundException) -- NOT on
	// CreateLoadBalancer, which does not accept certificates.
	ErrCertificateNotFound = awserr.New("CertificateNotFound", awserr.ErrNotFound)
	// ErrInvalidSecurityGroup is returned when a security group passed to
	// CreateLoadBalancer does not exist. Modeled on
	// CreateLoadBalancer's InvalidSecurityGroupException.
	ErrInvalidSecurityGroup = awserr.New("InvalidSecurityGroup", awserr.ErrInvalidParameter)
	// ErrSubnetNotFound is returned when a subnet passed to CreateLoadBalancer does not
	// exist. Modeled on CreateLoadBalancer's SubnetNotFoundException ("The specified
	// subnet does not exist") -- NOT InvalidSubnetException, whose doc comment reads
	// "The specified subnet is out of available addresses" (a capacity condition, not
	// an existence check); verified in aws-sdk-go-v2/service/elasticloadbalancingv2
	// types/errors.go.
	ErrSubnetNotFound = awserr.New("SubnetNotFound", awserr.ErrNotFound)
)
