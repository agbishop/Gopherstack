package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	iamRoleName = "pgoload-role"

	// iamUserRotation bounds the number of distinct IAM users pgoload
	// cycles through so CreateUser/DeleteUser churn doesn't grow the
	// backend's user set without limit over a long run.
	iamUserRotation = 8

	iamAssumeRolePolicy = `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Allow","Principal":{"Service":"lambda.amazonaws.com"},"Action":"sts:AssumeRole"}]}`
	iamInlinePolicy = `{"Version":"2012-10-17","Statement":[` +
		`{"Effect":"Allow","Action":"s3:GetObject","Resource":"*"}]}`
)

// ensureIAMRole creates iamRoleName, tolerating one that already exists, and
// returns its ARN. Lambda and Step Functions setup reuse this role.
func ensureIAMRole(ctx context.Context, cl *iam.Client, log *slog.Logger) (string, error) {
	out, err := cl.CreateRole(ctx, &iam.CreateRoleInput{
		RoleName:                 aws.String(iamRoleName),
		AssumeRolePolicyDocument: aws.String(iamAssumeRolePolicy),
	})
	if err == nil {
		return aws.ToString(out.Role.Arn), nil
	}

	if _, ok := errors.AsType[*iamtypes.EntityAlreadyExistsException](err); !ok {
		return "", fmt.Errorf("create role %s: %w", iamRoleName, err)
	}

	log.InfoContext(ctx, "iam role already exists", "role", iamRoleName)

	got, err := cl.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(iamRoleName)})
	if err != nil {
		return "", fmt.Errorf("get existing role %s: %w", iamRoleName, err)
	}

	return aws.ToString(got.Role.Arn), nil
}

func iamUserName(workerID, i int) string {
	return fmt.Sprintf("pgoload-user-%d-%d", workerID, i%iamUserRotation)
}

// iamWorker repeatedly runs a mix of IAM operations, staggered by workerID,
// until ctx is done.
func iamWorker(ctx context.Context, cl *iam.Client, workerID int, c *opCounter, log *slog.Logger) {
	ops := []opFunc{
		func(ctx context.Context, workerID, i int) error { return iamCreateUserOp(ctx, cl, workerID, i) },
		func(ctx context.Context, _, _ int) error { return iamGetRoleOp(ctx, cl) },
		func(ctx context.Context, _, _ int) error { return iamListUsersOp(ctx, cl) },
		func(ctx context.Context, _, _ int) error { return iamListRolesOp(ctx, cl) },
		func(ctx context.Context, workerID, i int) error { return iamPutRolePolicyOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return iamDeleteUserOp(ctx, cl, workerID, i) },
	}

	runOpLoop(ctx, workerID, ops, c, "iam", log)
}

func iamCreateUserOp(ctx context.Context, cl *iam.Client, workerID, i int) error {
	_, err := cl.CreateUser(ctx, &iam.CreateUserInput{UserName: aws.String(iamUserName(workerID, i))})
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*iamtypes.EntityAlreadyExistsException](err); ok {
		return nil
	}

	return err
}

func iamGetRoleOp(ctx context.Context, cl *iam.Client) error {
	_, err := cl.GetRole(ctx, &iam.GetRoleInput{RoleName: aws.String(iamRoleName)})

	return err
}

func iamListUsersOp(ctx context.Context, cl *iam.Client) error {
	_, err := cl.ListUsers(ctx, &iam.ListUsersInput{})

	return err
}

func iamListRolesOp(ctx context.Context, cl *iam.Client) error {
	_, err := cl.ListRoles(ctx, &iam.ListRolesInput{})

	return err
}

func iamPutRolePolicyOp(ctx context.Context, cl *iam.Client, workerID, _ int) error {
	_, err := cl.PutRolePolicy(ctx, &iam.PutRolePolicyInput{
		RoleName:       aws.String(iamRoleName),
		PolicyName:     aws.String(fmt.Sprintf("pgoload-policy-%d", workerID)),
		PolicyDocument: aws.String(iamInlinePolicy),
	})

	return err
}

func iamDeleteUserOp(ctx context.Context, cl *iam.Client, workerID, i int) error {
	_, err := cl.DeleteUser(ctx, &iam.DeleteUserInput{UserName: aws.String(iamUserName(workerID, i))})
	if err == nil {
		return nil
	}

	if _, ok := errors.AsType[*iamtypes.NoSuchEntityException](err); ok {
		return nil
	}

	return err
}
