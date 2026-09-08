package cloudformation_test

import (
	"net/http"
	"net/url"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/cloudformation"
)

// ---- DescribeType backend (table-driven) -------------------------------------------

func TestDescribeType_Registered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup     func(*cloudformation.InMemoryBackend)
		check     func(*testing.T, *cloudformation.TypeDetails)
		name      string
		typeName  string
		typeArn   string
		versionID string
		wantErr   bool
	}{
		{
			name: "lookup by TypeName after RegisterType",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::MyService::Widget", "s3://pkg.zip")
			},
			typeName: "MyOrg::MyService::Widget",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.Equal(t, "MyOrg::MyService::Widget", d.TypeName)
				assert.Equal(t, "RESOURCE", d.Type)
				assert.Equal(t, "COMPLETE", d.Status)
				assert.Equal(t, "PRIVATE", d.Visibility)
				assert.True(t, d.IsDefaultVersion)
			},
		},
		{
			name: "lookup by ARN after RegisterType",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::Svc::Res", "s3://pkg.zip")
			},
			typeArn: "arn:aws:cloudformation:::type/resource/MyOrg::Svc::Res",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.Equal(t, "MyOrg::Svc::Res", d.TypeName)
				assert.NotEmpty(t, d.TypeArn)
			},
		},
		{
			name: "published type has PUBLIC visibility",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::Pub::Type", "s3://pkg.zip")
				_, _ = b.PublishType("MyOrg::Pub::Type")
			},
			typeName: "MyOrg::Pub::Type",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.Equal(t, "PUBLIC", d.Visibility)
			},
		},
		{
			name: "activated type IsActivated is true",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::Act::Type", "s3://pkg.zip")
				_, _ = b.ActivateType("MyOrg::Act::Type", "")
			},
			typeName: "MyOrg::Act::Type",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.True(t, d.IsActivated)
			},
		},
		{
			name:     "unknown type returns error",
			typeName: "Unknown::Type::Name",
			wantErr:  true,
		},
		{
			name:    "no typeName or arn returns error",
			wantErr: true,
		},
		{
			name: "specific versionID returned",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::Ver::Type", "s3://v1.zip")
				_, _ = b.RegisterType("MyOrg::Ver::Type", "s3://v2.zip")
			},
			typeName:  "MyOrg::Ver::Type",
			versionID: "00000001",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.Equal(t, "00000001", d.VersionID)
			},
		},
		{
			name: "default version updated by SetTypeDefaultVersion",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("MyOrg::Def::Type", "s3://v1.zip")
				_, _ = b.RegisterType("MyOrg::Def::Type", "s3://v2.zip")
				typeArn := "arn:aws:cloudformation:::type/resource/MyOrg::Def::Type"
				_ = b.SetTypeDefaultVersion(typeArn, "00000001")
			},
			typeName: "MyOrg::Def::Type",
			check: func(t *testing.T, d *cloudformation.TypeDetails) {
				t.Helper()
				assert.Equal(t, "00000001", d.DefaultVersionID)
				assert.True(t, d.IsDefaultVersion)
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			if tc.setup != nil {
				tc.setup(b)
			}
			details, err := b.DescribeType(tc.typeName, tc.typeArn, tc.versionID)
			if tc.wantErr {
				require.Error(t, err)

				return
			}
			require.NoError(t, err)
			require.NotNil(t, details)
			if tc.check != nil {
				tc.check(t, details)
			}
		})
	}
}

// ---- DescribeType HTTP handler (table-driven) -------------------------------------

func TestHandler_DescribeType_Registered(t *testing.T) {
	t.Parallel()

	tests := []struct {
		setup        func(*cloudformation.InMemoryBackend)
		formValues   url.Values
		name         string
		wantContains []string
		wantStatus   int
	}{
		{
			name: "registered type returned from backend",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("Acme::Network::VPC", "s3://schema.zip")
			},
			formValues: url.Values{
				"Action":   {"DescribeType"},
				"TypeName": {"Acme::Network::VPC"},
			},
			wantStatus:   http.StatusOK,
			wantContains: []string{"DescribeTypeResponse", "Acme::Network::VPC", "RESOURCE"},
		},
		{
			name: "unknown type falls back to schema handler",
			formValues: url.Values{
				"Action":   {"DescribeType"},
				"TypeName": {"AWS::S3::Bucket"},
			},
			wantStatus:   http.StatusOK,
			wantContains: []string{"DescribeTypeResponse", "AWS::S3::Bucket"},
		},
		{
			name: "published type shows PUBLIC visibility in response",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("Acme::Pub::Widget", "s3://schema.zip")
				_, _ = b.PublishType("Acme::Pub::Widget")
			},
			formValues: url.Values{
				"Action":   {"DescribeType"},
				"TypeName": {"Acme::Pub::Widget"},
			},
			wantStatus:   http.StatusOK,
			wantContains: []string{"PUBLIC"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHandler()
			if tc.setup != nil {
				tc.setup(h.Backend.(*cloudformation.InMemoryBackend))
			}
			resp := postFormValues(t, h, tc.formValues)
			assert.Equal(t, tc.wantStatus, resp.Status, "body: %s", resp.Body)
			for _, want := range tc.wantContains {
				assert.Contains(t, resp.Body, want)
			}
		})
	}
}

// ---- TypeVersioning (table-driven) -----------------------------------------------

func TestTypeVersioning_MultipleRegistrations(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name          string
		wantDefault   string
		registerCount int
		wantVersions  int
	}{
		{
			name:          "single registration yields one version",
			registerCount: 1,
			wantVersions:  1,
			wantDefault:   "00000001",
		},
		{
			name:          "two registrations yield two versions",
			registerCount: 2,
			wantVersions:  2,
			wantDefault:   "00000002",
		},
		{
			name:          "three registrations yield three versions",
			registerCount: 3,
			wantVersions:  3,
			wantDefault:   "00000003",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			for i := range tc.registerCount {
				_, err := b.RegisterType("Acme::Ver::Type", "s3://v"+string(rune('0'+i+1))+".zip")
				require.NoError(t, err)
			}

			versions, err := b.ListTypeVersions("Acme::Ver::Type", "")
			require.NoError(t, err)
			assert.Len(t, versions, tc.wantVersions)

			details, err := b.DescribeType("Acme::Ver::Type", "", "")
			require.NoError(t, err)
			assert.Equal(t, tc.wantDefault, details.DefaultVersionID)
		})
	}
}

// ---- ListTypeVersions multiple versions ------------------------------------------

func TestListTypeVersions_MultipleVersions(t *testing.T) {
	t.Parallel()

	b := newBackend()
	_, err := b.RegisterType("Acme::Multi::Ver", "s3://v1.zip")
	require.NoError(t, err)
	_, err = b.RegisterType("Acme::Multi::Ver", "s3://v2.zip")
	require.NoError(t, err)
	_, err = b.RegisterType("Acme::Multi::Ver", "s3://v3.zip")
	require.NoError(t, err)

	versions, err := b.ListTypeVersions("Acme::Multi::Ver", "")
	require.NoError(t, err)
	assert.Len(t, versions, 3)
	assert.Contains(t, versions, "00000001")
	assert.Contains(t, versions, "00000002")
	assert.Contains(t, versions, "00000003")
}

// ---- ListTypes visibility (table-driven) -----------------------------------------

func TestListTypes_Visibility(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(*cloudformation.InMemoryBackend)
		wantPrivate    []string
		wantPublic     []string
		wantNotPresent []string
	}{
		{
			name: "registered type is PRIVATE",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("Acme::Priv::Type", "s3://pkg.zip")
			},
			wantPrivate: []string{"Acme::Priv::Type"},
		},
		{
			name: "published type is PUBLIC",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("Acme::Pub::Type", "s3://pkg.zip")
				_, _ = b.PublishType("Acme::Pub::Type")
			},
			wantPublic: []string{"Acme::Pub::Type"},
		},
		{
			name: "deprecated type excluded from list",
			setup: func(b *cloudformation.InMemoryBackend) {
				_, _ = b.RegisterType("Acme::Dep::Type", "s3://pkg.zip")
				typeArn := "arn:aws:cloudformation:::type/resource/Acme::Dep::Type"
				_ = b.DeregisterType("Acme::Dep::Type", typeArn, "")
			},
			wantNotPresent: []string{"Acme::Dep::Type"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			b := newBackend()
			if tc.setup != nil {
				tc.setup(b)
			}
			types, err := b.ListTypes("")
			require.NoError(t, err)

			typeMap := make(map[string]cloudformation.TypeSummary, len(types))
			for _, typ := range types {
				typeMap[typ.TypeName] = typ
			}

			for _, want := range tc.wantPrivate {
				summary, ok := typeMap[want]
				assert.True(t, ok, "expected %q in type list", want)
				assert.Equal(t, "PRIVATE", summary.Visibility)
			}
			for _, want := range tc.wantPublic {
				summary, ok := typeMap[want]
				assert.True(t, ok, "expected %q in type list", want)
				assert.Equal(t, "PUBLIC", summary.Visibility)
			}
			for _, notWant := range tc.wantNotPresent {
				_, ok := typeMap[notWant]
				assert.False(t, ok, "expected %q to be excluded from type list", notWant)
			}
		})
	}
}

// ---- Handler: DescribeType for registered type ------------------------------------

func TestHandler_DescribeType_RegisteredVsBuiltin(t *testing.T) {
	t.Parallel()

	h := newHandler()

	// Register a type and verify it comes from the registry.
	postFormValues(t, h, url.Values{
		"Action":               {"RegisterType"},
		"TypeName":             {"Acme::Network::Router"},
		"SchemaHandlerPackage": {"s3://schema.zip"},
	})

	tests := []struct {
		name         string
		typeName     string
		wantContains []string
	}{
		{
			name:     "registered type served from registry",
			typeName: "Acme::Network::Router",
			wantContains: []string{
				"DescribeTypeResponse",
				"Acme::Network::Router",
				"RESOURCE",
				"COMPLETE",
			},
		},
		{
			name:     "builtin AWS type served from schema fallback",
			typeName: "AWS::Lambda::Function",
			wantContains: []string{
				"DescribeTypeResponse",
				"AWS::Lambda::Function",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			resp := postFormValues(t, h, url.Values{
				"Action":   {"DescribeType"},
				"TypeName": {tc.typeName},
			})
			resp.mustOK(t)
			for _, want := range tc.wantContains {
				assert.Contains(t, resp.Body, want)
			}
		})
	}
}
