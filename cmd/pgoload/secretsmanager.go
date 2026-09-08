package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/secretsmanager"
	smtypes "github.com/aws/aws-sdk-go-v2/service/secretsmanager/types"
)

// secretsCount is how many secrets pgoload provisions once during setup and
// then reads/updates/recreates in rotation, bounding the backend's secret
// set for the whole run. Deliberately not a multiple of len(ops) in
// secretsManagerWorker (5) for the same reason as ssmParamRotation: it
// keeps every secret index reachable by every operation instead of a fixed
// subset always landing on the same op (see ssm.go).
const secretsCount = 8

func secretName(idx int) string {
	return fmt.Sprintf("pgoload-secret-%d", idx)
}

// secretIndex spreads workers across secretsCount using their workerID as a
// phase offset. Indexing by i alone would put every worker's Nth iteration
// on the same secret at roughly the same wall-clock time (all i counters
// advance in lockstep), maximizing collisions between concurrent
// Get/Put/Recreate calls; the offset spreads that load instead.
func secretIndex(workerID, i int) int {
	return (workerID + i) % secretsCount
}

// ensureSecrets creates secretsCount secrets once, tolerating any that
// already exist from a prior run.
func ensureSecrets(ctx context.Context, cl *secretsmanager.Client, log *slog.Logger) error {
	for idx := range secretsCount {
		_, err := cl.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
			Name:         aws.String(secretName(idx)),
			SecretString: aws.String("s3cr3t-0"),
		})
		if err == nil {
			continue
		}

		if _, ok := errors.AsType[*smtypes.ResourceExistsException](err); ok {
			continue
		}

		return fmt.Errorf("create secret %s: %w", secretName(idx), err)
	}

	log.InfoContext(ctx, "secretsmanager secrets ready", "count", secretsCount)

	return nil
}

// secretsManagerWorker repeatedly runs a mix of Secrets Manager operations,
// staggered by workerID, until ctx is done.
func secretsManagerWorker(
	ctx context.Context,
	cl *secretsmanager.Client,
	workerID int,
	c *opCounter,
	log *slog.Logger,
) {
	ops := []opFunc{
		func(ctx context.Context, workerID, i int) error { return secretsGetSecretValueOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return secretsGetSecretValueOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return secretsGetSecretValueOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return secretsPutSecretValueOp(ctx, cl, workerID, i) },
		func(ctx context.Context, workerID, i int) error { return secretsPutSecretValueOp(ctx, cl, workerID, i) },
		func(ctx context.Context, _, _ int) error { return secretsListSecretsOp(ctx, cl) },
		func(ctx context.Context, workerID, i int) error { return secretsRecreateOp(ctx, cl, workerID, i) },
	}

	runOpLoop(ctx, workerID, ops, c, "secretsmanager", log)
}

func secretsGetSecretValueOp(ctx context.Context, cl *secretsmanager.Client, workerID, i int) error {
	_, err := cl.GetSecretValue(ctx, &secretsmanager.GetSecretValueInput{
		SecretId: aws.String(secretName(secretIndex(workerID, i))),
	})

	return err
}

func secretsPutSecretValueOp(ctx context.Context, cl *secretsmanager.Client, workerID, i int) error {
	_, err := cl.PutSecretValue(ctx, &secretsmanager.PutSecretValueInput{
		SecretId:     aws.String(secretName(secretIndex(workerID, i))),
		SecretString: aws.String(fmt.Sprintf("s3cr3t-%d", i)),
	})

	return err
}

func secretsListSecretsOp(ctx context.Context, cl *secretsmanager.Client) error {
	_, err := cl.ListSecrets(ctx, &secretsmanager.ListSecretsInput{})

	return err
}

// secretsRecreateOp deletes and immediately recreates one secret, exercising
// the full delete/create wire paths without ever colliding with an existing
// name (delete always runs first).
func secretsRecreateOp(ctx context.Context, cl *secretsmanager.Client, workerID, i int) error {
	name := secretName(secretIndex(workerID, i))

	if _, err := cl.DeleteSecret(ctx, &secretsmanager.DeleteSecretInput{
		SecretId:                   aws.String(name),
		ForceDeleteWithoutRecovery: aws.Bool(true),
	}); err != nil {
		return fmt.Errorf("delete secret %s: %w", name, err)
	}

	if _, err := cl.CreateSecret(ctx, &secretsmanager.CreateSecretInput{
		Name:         aws.String(name),
		SecretString: aws.String("s3cr3t-recreated"),
	}); err != nil {
		return fmt.Errorf("recreate secret %s: %w", name, err)
	}

	return nil
}
