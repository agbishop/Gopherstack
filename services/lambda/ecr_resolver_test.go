package lambda_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/lambda"
)

// fakeECRResolver implements lambda.ECRResolver, accepting only the URIs in allowed.
type fakeECRResolver struct {
	allowed map[string]bool
}

var _ lambda.ECRResolver = (*fakeECRResolver)(nil)

func (f *fakeECRResolver) ResolveImage(imageURI string) bool {
	return f.allowed[imageURI]
}

// TestLambda_CreateFunction_ImageURI_NoResolverAcceptsAny documents the
// default (unwired) behavior: with no ECRResolver set, CreateFunction
// accepts any Code.ImageUri unvalidated, matching every other cross-service
// resolver in this repo (e.g. networkmanager.EC2Resolver) and every
// existing lambda test that creates an Image function with a bare "x" URI.
func TestLambda_CreateFunction_ImageURI_NoResolverAcceptsAny(t *testing.T) {
	t.Parallel()

	h, _ := newInMemoryHandler(t)

	body := `{"FunctionName":"no-resolver-fn","PackageType":"Image",` +
		`"Code":{"ImageUri":"111122223333.dkr.ecr.us-east-1.amazonaws.com/nonexistent-repo:latest"},` +
		`"Role":"arn:aws:iam:::role/r"}`
	rec := callInMemoryHandler(t, h, http.MethodPost, "/2015-03-31/functions", body)
	require.Equal(t, http.StatusCreated, rec.Code)
}

// TestLambda_CreateFunction_ImageURI_ResolverRejectsUnknownImage is the
// regression test for gopherstack-vrpy: AWS's real CreateFunction validates
// Code.ImageUri against ECR at create time and rejects an image that does
// not exist with InvalidParameterValueException ("Source image <uri> does
// not exist. Provide a valid source image."), not only at pull time. Before
// the ECRResolver wiring, this backend accepted (and stored) any ImageUri
// regardless of whether a resolver said it existed.
func TestLambda_CreateFunction_ImageURI_ResolverRejectsUnknownImage(t *testing.T) {
	t.Parallel()

	h, bk := newInMemoryHandler(t)
	bk.SetECRResolver(&fakeECRResolver{allowed: map[string]bool{}})

	body := `{"FunctionName":"missing-image-fn","PackageType":"Image",` +
		`"Code":{"ImageUri":"111122223333.dkr.ecr.us-east-1.amazonaws.com/nonexistent-repo:latest"},` +
		`"Role":"arn:aws:iam:::role/r"}`
	rec := callInMemoryHandler(t, h, http.MethodPost, "/2015-03-31/functions", body)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValueException")
	assert.Contains(t, rec.Body.String(), "does not exist")

	_, err := bk.GetFunction("missing-image-fn")
	require.Error(t, err, "a rejected CreateFunction must not persist the function")
}

// TestLambda_CreateFunction_ImageURI_ResolverAcceptsKnownImage proves the
// resolver check does not false-positive: a wired resolver that reports the
// image exists must let CreateFunction through exactly as before.
func TestLambda_CreateFunction_ImageURI_ResolverAcceptsKnownImage(t *testing.T) {
	t.Parallel()

	h, bk := newInMemoryHandler(t)
	uri := "111122223333.dkr.ecr.us-east-1.amazonaws.com/real-repo:latest"
	bk.SetECRResolver(&fakeECRResolver{allowed: map[string]bool{uri: true}})

	body := `{"FunctionName":"real-image-fn","PackageType":"Image",` +
		`"Code":{"ImageUri":"` + uri + `"},"Role":"arn:aws:iam:::role/r"}`
	rec := callInMemoryHandler(t, h, http.MethodPost, "/2015-03-31/functions", body)
	require.Equal(t, http.StatusCreated, rec.Code)
}

// TestLambda_UpdateFunctionCode_ImageURI_ResolverRejectsUnknownImage covers
// the second call site AWS applies the same validation to: UpdateFunctionCode.
func TestLambda_UpdateFunctionCode_ImageURI_ResolverRejectsUnknownImage(t *testing.T) {
	t.Parallel()

	h, bk := newInMemoryHandler(t)
	createFunctionForTest(t, h, "update-missing-image-fn")

	bk.SetECRResolver(&fakeECRResolver{allowed: map[string]bool{}})

	rec := callInMemoryHandler(
		t, h, http.MethodPut,
		"/2015-03-31/functions/update-missing-image-fn/code",
		`{"ImageUri":"111122223333.dkr.ecr.us-east-1.amazonaws.com/nonexistent-repo:v2"}`,
	)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidParameterValueException")

	fn, err := bk.GetFunction("update-missing-image-fn")
	require.NoError(t, err)
	assert.Equal(t, "x", fn.ImageURI, "a rejected UpdateFunctionCode must not overwrite the stored ImageUri")
}
