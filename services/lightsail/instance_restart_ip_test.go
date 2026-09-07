package lightsail_test

import (
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	lightsailsdk "github.com/aws/aws-sdk-go-v2/service/lightsail"
	"github.com/stretchr/testify/require"
)

// TestStartInstance_PublicIPOnRestart proves StartInstance reassigns a
// dynamic public IP on a stopped->running transition, per
// api_op_StartInstance.go's doc comment ("Lightsail assigns a new public IP
// address"), while an attached static IP survives the same transition
// (gopherstack-i2s6).
func TestStartInstance_PublicIPOnRestart(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		attachStatic bool
	}{
		{name: "dynamic IP changes"},
		{name: "static IP unchanged", attachStatic: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			client := newTestClient(t)
			ctx := t.Context()
			const instName = "restart-target"

			_, err := client.CreateInstances(ctx, &lightsailsdk.CreateInstancesInput{
				InstanceNames: []string{instName}, AvailabilityZone: aws.String("us-east-1a"),
				BlueprintId: aws.String("amazon_linux_2023"), BundleId: aws.String("nano_3_0"),
			})
			require.NoError(t, err)

			waitForInstanceState(t, client, instName, "running")

			if tc.attachStatic {
				_, err = client.AllocateStaticIp(
					ctx,
					&lightsailsdk.AllocateStaticIpInput{StaticIpName: aws.String("restart-ip")},
				)
				require.NoError(t, err)

				_, err = client.AttachStaticIp(ctx, &lightsailsdk.AttachStaticIpInput{
					StaticIpName: aws.String("restart-ip"), InstanceName: aws.String(instName),
				})
				require.NoError(t, err)
			}

			before, err := client.GetInstance(ctx, &lightsailsdk.GetInstanceInput{InstanceName: aws.String(instName)})
			require.NoError(t, err)
			beforeIP := aws.ToString(before.Instance.PublicIpAddress)
			require.NotEmpty(t, beforeIP)

			_, err = client.StopInstance(ctx, &lightsailsdk.StopInstanceInput{InstanceName: aws.String(instName)})
			require.NoError(t, err)
			waitForInstanceState(t, client, instName, "stopped")

			_, err = client.StartInstance(ctx, &lightsailsdk.StartInstanceInput{InstanceName: aws.String(instName)})
			require.NoError(t, err)
			waitForInstanceState(t, client, instName, "running")

			after, err := client.GetInstance(ctx, &lightsailsdk.GetInstanceInput{InstanceName: aws.String(instName)})
			require.NoError(t, err)
			afterIP := aws.ToString(after.Instance.PublicIpAddress)

			if tc.attachStatic {
				require.Equal(t, beforeIP, afterIP, "a static IP must survive a stop/start cycle")
			} else {
				require.NotEqual(t, beforeIP, afterIP, "a dynamic IP must change on a stopped->running transition")
			}
		})
	}
}

func waitForInstanceState(t *testing.T, client *lightsailsdk.Client, instName, want string) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := client.GetInstanceState(
			t.Context(),
			&lightsailsdk.GetInstanceStateInput{InstanceName: aws.String(instName)},
		)

		return err == nil && aws.ToString(out.State.Name) == want
	}, defaultAsyncWait, defaultAsyncPoll, "instance never reached "+want)
}
