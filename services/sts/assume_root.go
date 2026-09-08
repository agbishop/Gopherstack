package sts

import (
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// rootSessionName is the SessionName recorded for AssumeRoot-issued sessions.
const rootSessionName = "root"

// approvedRootTaskPolicies returns the AWS-approved task policy ARNs for AssumeRoot (Gap #7).
func approvedRootTaskPolicies() []string {
	return []string{
		"arn:aws:iam::aws:policy/root-task/IAMAuditRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/IAMCreateRootUserPassword",
		"arn:aws:iam::aws:policy/root-task/IAMDeleteRootUserCredentials",
		"arn:aws:iam::aws:policy/root-task/S3UnlockBucketPolicy",
		"arn:aws:iam::aws:policy/root-task/SQSUnlockQueuePolicy",
	}
}

// validateApprovedRootTaskPolicy checks that TaskPolicyArn is in the AWS-approved set.
func validateApprovedRootTaskPolicy(taskPolicyArn string) error {
	if slices.Contains(approvedRootTaskPolicies(), taskPolicyArn) {
		return nil
	}

	return fmt.Errorf("%w: TaskPolicyArn %q is not in the approved set", ErrValidation, taskPolicyArn)
}

// extractAccountFromPrincipal returns the account portion of an ARN or the principal itself
// if it looks like a 12-digit account ID.
func extractAccountFromPrincipal(principal string) string {
	if len(principal) == 12 && allDigits(principal) {
		return principal
	}

	parts := strings.SplitN(principal, ":", arnComponentCount)
	if len(parts) >= arnComponentCount {
		return parts[4]
	}

	return principal
}

// allDigits reports whether every character in s is an ASCII digit.
func allDigits(s string) bool {
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}

	return true
}

// AssumeRoot generates short-term privileged credentials for a member account root.
// TaskPolicyArn must be in the AWS-approved set; TargetPrincipal must be a 12-digit account ID.
func (b *InMemoryBackend) AssumeRoot(input *AssumeRootInput) (*AssumeRootResponse, error) {
	b.cntAssumeRoot.Add(1)

	if input.TargetPrincipal == "" {
		return nil, ErrMissingTargetPrincipal
	}

	if input.TaskPolicyArn == "" {
		return nil, ErrMissingTaskPolicyArn
	}

	if err := validateApprovedRootTaskPolicy(input.TaskPolicyArn); err != nil {
		return nil, err
	}

	// TargetPrincipal must be a 12-digit member account ID.
	account := extractAccountFromPrincipal(input.TargetPrincipal)
	if !accountIDRe.MatchString(account) {
		return nil, fmt.Errorf("%w: got %q", ErrInvalidTargetPrincipal, input.TargetPrincipal)
	}

	duration := input.DurationSeconds
	if duration == 0 {
		duration = MaxRootDurationSeconds
	}

	if duration != MaxRootDurationSeconds {
		return nil, fmt.Errorf(
			"%w: DurationSeconds must be exactly %d for AssumeRoot",
			ErrInvalidDuration, MaxRootDurationSeconds,
		)
	}

	creds, err := generateCredentialSet()
	if err != nil {
		return nil, err
	}

	expiration := time.Now().UTC().Add(time.Duration(duration) * time.Second)
	assumedRoleArn := arn.Build("sts", "", account, "assumed-root")

	// SourceIdentity is not a request parameter for AssumeRoot; it persists
	// from the caller's own session, per AWS's documented behavior.
	var sourceIdentity string
	if input.CallerSession != nil {
		sourceIdentity = input.CallerSession.SourceIdentity
	}

	session := &SessionInfo{
		Expiration:      expiration,
		AssumedRoleArn:  assumedRoleArn,
		AccountID:       account,
		SessionName:     rootSessionName,
		AccessKeyID:     creds.AccessKeyID,
		SecretAccessKey: creds.SecretAccessKey,
		SessionToken:    creds.SessionToken,
		AssumedRoleID:   account + ":" + rootSessionName,
		SourceIdentity:  sourceIdentity,
		IsAssumedRole:   true,
	}

	b.storeSession(session)

	return &AssumeRootResponse{
		Xmlns: STSNamespace,
		AssumeRootResult: AssumeRootResult{
			Credentials: Credentials{
				AccessKeyID:     creds.AccessKeyID,
				SecretAccessKey: creds.SecretAccessKey,
				SessionToken:    creds.SessionToken,
				Expiration:      expiration.Format(time.RFC3339),
			},
			SourceIdentity: sourceIdentity,
		},
		ResponseMetadata: ResponseMetadata{RequestID: uuid.NewString()},
	}, nil
}
