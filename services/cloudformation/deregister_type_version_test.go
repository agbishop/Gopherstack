package cloudformation_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDeregisterType_VersionSemantics locks in a parity fix: DeregisterType
// previously took only an Arn and unconditionally deprecated the whole type,
// silently ignoring VersionId (gopherstack-sk32). Real semantics
// (cloudformation@v1.76.1 api_op_DeregisterType.go doc comment): deregistering
// a non-default version deprecates only that version; deregistering the last
// active version deprecates the whole type along with it; deregistering the
// default version while other active versions exist is rejected.
func TestDeregisterType_VersionSemantics(t *testing.T) {
	t.Parallel()

	t.Run("non-default version deregistered leaves type and default version live", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.RegisterType("Acme::Multi::A", "s3://v1.zip")
		require.NoError(t, err)
		_, err = b.RegisterType("Acme::Multi::A", "s3://v2.zip")
		require.NoError(t, err)
		typeArn := "arn:aws:cloudformation:::type/resource/Acme::Multi::A"

		err = b.DeregisterType("Acme::Multi::A", typeArn, "00000001")
		require.NoError(t, err)

		live, err := b.ListTypeVersions("Acme::Multi::A", "")
		require.NoError(t, err)
		assert.Equal(t, []string{"00000002"}, live)

		deprecated, err := b.ListTypeVersions("Acme::Multi::A", "DEPRECATED")
		require.NoError(t, err)
		assert.Equal(t, []string{"00000001"}, deprecated)

		details, err := b.DescribeType("Acme::Multi::A", "", "")
		require.NoError(t, err)
		assert.Equal(t, "LIVE", details.DeprecatedStatus, "type as a whole must stay live")

		oldVersion, err := b.DescribeType("", typeArn, "00000001")
		require.NoError(t, err)
		assert.Equal(t, "DEPRECATED", oldVersion.DeprecatedStatus)
	})

	t.Run("deregistering the only version deprecates the whole type", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.RegisterType("Acme::Solo::B", "s3://v1.zip")
		require.NoError(t, err)
		typeArn := "arn:aws:cloudformation:::type/resource/Acme::Solo::B"

		err = b.DeregisterType("Acme::Solo::B", typeArn, "00000001")
		require.NoError(t, err)

		details, err := b.DescribeType("Acme::Solo::B", "", "")
		require.NoError(t, err)
		assert.Equal(t, "DEPRECATED", details.DeprecatedStatus)

		types, err := b.ListTypes("")
		require.NoError(t, err)
		for _, ty := range types {
			assert.NotEqual(t, "Acme::Solo::B", ty.TypeName, "deregistered type must not be listed")
		}
	})

	t.Run("deregistering default version while other version active is rejected", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.RegisterType("Acme::Multi::C", "s3://v1.zip")
		require.NoError(t, err)
		_, err = b.RegisterType("Acme::Multi::C", "s3://v2.zip")
		require.NoError(t, err)
		typeArn := "arn:aws:cloudformation:::type/resource/Acme::Multi::C"

		// 00000002 is the default version after two RegisterType calls.
		err = b.DeregisterType("Acme::Multi::C", typeArn, "00000002")
		require.Error(t, err)

		live, err := b.ListTypeVersions("Acme::Multi::C", "")
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"00000001", "00000002"}, live, "no version should be deprecated on rejection")
	})

	t.Run("deregistering whole type also deprecates its versions", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.RegisterType("Acme::Multi::F", "s3://v1.zip")
		require.NoError(t, err)
		_, err = b.RegisterType("Acme::Multi::F", "s3://v2.zip")
		require.NoError(t, err)
		typeArn := "arn:aws:cloudformation:::type/resource/Acme::Multi::F"

		err = b.DeregisterType("Acme::Multi::F", typeArn, "")
		require.NoError(t, err)

		live, err := b.ListTypeVersions("Acme::Multi::F", "")
		require.NoError(t, err)
		assert.Empty(t, live, "no version should remain live once the whole type is deregistered")
	})

	t.Run("unknown version id is rejected", func(t *testing.T) {
		t.Parallel()
		b := newBackend()
		_, err := b.RegisterType("Acme::Solo::D", "s3://v1.zip")
		require.NoError(t, err)
		typeArn := "arn:aws:cloudformation:::type/resource/Acme::Solo::D"

		err = b.DeregisterType("Acme::Solo::D", typeArn, "99999999")
		require.Error(t, err)
	})
}

// TestDeregisterType_HandlerRejectsDefaultVersionWithOthersActive checks the
// wire-level error code for the reject case: CFNRegistryException
// (cloudformation@v1.76.1 deserializers.go
// awsAwsquery_deserializeOpErrorDeregisterType models CFNRegistryException and
// TypeNotFoundException only).
func TestDeregisterType_HandlerRejectsDefaultVersionWithOthersActive(t *testing.T) {
	t.Parallel()

	h := newHandler()

	postForm(t, h, url.Values{
		"Action":               []string{"RegisterType"},
		"TypeName":             []string{"Acme::Handler::E"},
		"SchemaHandlerPackage": []string{"s3://v1.zip"},
	}.Encode())
	postForm(t, h, url.Values{
		"Action":               []string{"RegisterType"},
		"TypeName":             []string{"Acme::Handler::E"},
		"SchemaHandlerPackage": []string{"s3://v2.zip"},
	}.Encode())

	rec := postForm(t, h, url.Values{
		"Action":    []string{"DeregisterType"},
		"TypeName":  []string{"Acme::Handler::E"},
		"VersionId": []string{"00000002"},
	}.Encode())
	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "CFNRegistryException")

	// The non-default version can still be deregistered on its own.
	rec = postForm(t, h, url.Values{
		"Action":    []string{"DeregisterType"},
		"TypeName":  []string{"Acme::Handler::E"},
		"VersionId": []string{"00000001"},
	}.Encode())
	require.Equal(t, http.StatusOK, rec.Code)
}
