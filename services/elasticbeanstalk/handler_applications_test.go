package elasticbeanstalk_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandler_CreateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantXML    string
		wantStatus int
	}{
		{
			name:       "success",
			body:       "Version=2010-12-01&Action=CreateApplication&ApplicationName=my-app&Description=My+App",
			wantStatus: http.StatusOK,
			wantXML:    "CreateApplicationResponse",
		},
		{
			name:       "missing application name",
			body:       "Version=2010-12-01&Action=CreateApplication",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			rec := postEBForm(t, h, tt.body)

			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantXML != "" {
				assert.Contains(t, rec.Body.String(), tt.wantXML)
			}
		})
	}
}

func TestHandler_DescribeApplications(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	// Create two applications.
	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=app-a")
	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=app-b")

	tests := []struct {
		name       string
		body       string
		wantApp    string
		wantStatus int
	}{
		{
			name:       "list all",
			body:       "Version=2010-12-01&Action=DescribeApplications",
			wantStatus: http.StatusOK,
		},
		{
			name:       "filter by name",
			body:       "Version=2010-12-01&Action=DescribeApplications&ApplicationNames.member.1=app-a",
			wantStatus: http.StatusOK,
			wantApp:    "app-a",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := postEBForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantApp != "" {
				assert.Contains(t, rec.Body.String(), tt.wantApp)
			}
		})
	}
}

// TestHandler_DescribeApplications_IncludesConfigurationTemplates verifies
// DescribeApplications includes configuration template names.
func TestHandler_DescribeApplications_IncludesConfigurationTemplates(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		templatesBefore []string
		contains        []string
		absent          []string
	}{
		{
			// Real AWS: "Creates an application that has one configuration
			// template named default" -- CreateApplication auto-provisions
			// a "Default" template, so the list is never actually empty.
			name:     "no explicit templates — auto-created Default template only",
			contains: []string{"<ApplicationName>myapp</ApplicationName>", "<member>Default</member>"},
			absent:   []string{"<member>tmpl1</member>"},
		},
		{
			name:            "one template — name included alongside Default",
			templatesBefore: []string{"tmpl1"},
			contains:        []string{"<member>tmpl1</member>", "<member>Default</member>"},
		},
		{
			name:            "two templates — both names included alongside Default",
			templatesBefore: []string{"tmpl1", "tmpl2"},
			contains:        []string{"<member>tmpl1</member>", "<member>tmpl2</member>", "<member>Default</member>"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=myapp")

			for _, name := range tt.templatesBefore {
				postEBForm(t, h, "Version=2010-12-01&Action=CreateConfigurationTemplate"+
					"&ApplicationName=myapp&TemplateName="+name)
			}

			rec := postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications")
			require.Equal(t, http.StatusOK, rec.Code)

			for _, s := range tt.contains {
				assert.Contains(t, rec.Body.String(), s)
			}

			for _, s := range tt.absent {
				assert.NotContains(t, rec.Body.String(), s)
			}
		})
	}
}

// TestHandler_DescribeApplications_SortedByName verifies alphabetic sort order.
func TestHandler_DescribeApplications_SortedByName(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	for _, name := range []string{"zapp", "aapp", "mapp"} {
		rec := postEBForm(t, h,
			"Version=2010-12-01&Action=CreateApplication&ApplicationName="+name)
		require.Equal(t, http.StatusOK, rec.Code)
	}

	rec := postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications")
	require.Equal(t, http.StatusOK, rec.Code)

	body := rec.Body.String()
	posA := indexOfFirst(body, "aapp")
	posM := indexOfFirst(body, "mapp")
	posZ := indexOfFirst(body, "zapp")

	assert.Less(t, posA, posM, "aapp should come before mapp")
	assert.Less(t, posM, posZ, "mapp should come before zapp")
}

func TestHandler_DeleteApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		setupName  string
		deleteName string
		wantStatus int
	}{
		{
			name:       "delete existing",
			setupName:  "del-app",
			deleteName: "del-app",
			wantStatus: http.StatusOK,
		},
		{
			name:       "delete nonexistent",
			deleteName: "nonexistent",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.setupName != "" {
				postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName="+tt.setupName)
			}

			rec := postEBForm(t, h, "Version=2010-12-01&Action=DeleteApplication&ApplicationName="+tt.deleteName)
			assert.Equal(t, tt.wantStatus, rec.Code)
		})
	}
}

// TestHandler_DeleteApplication_CascadesToRelatedResources verifies that deleting an
// application removes its environments, versions, and configuration templates.
func TestHandler_DeleteApplication_CascadesToRelatedResources(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=my-app")
	postEBForm(t, h,
		"Version=2010-12-01&Action=CreateEnvironment&ApplicationName=my-app&EnvironmentName=env1")
	postEBForm(t, h,
		"Version=2010-12-01&Action=CreateApplicationVersion&ApplicationName=my-app&VersionLabel=v1")
	postEBForm(t, h,
		"Version=2010-12-01&Action=CreateConfigurationTemplate"+
			"&ApplicationName=my-app&TemplateName=tmpl1")

	assert.Equal(t, 1, h.Backend.EnvironmentCount())
	assert.Equal(t, 1, h.Backend.AppVersionCount())
	// 2 = the auto-created "Default" template plus the explicit tmpl1.
	assert.Equal(t, 2, h.Backend.ConfigTemplateCount())

	rec := postEBForm(t, h,
		"Version=2010-12-01&Action=DeleteApplication&ApplicationName=my-app&TerminateEnvByForce=true")
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 0, h.Backend.ApplicationCount())
	assert.Equal(t, 0, h.Backend.EnvironmentCount())
	assert.Equal(t, 0, h.Backend.AppVersionCount())
	assert.Equal(t, 0, h.Backend.ConfigTemplateCount())
}

// TestHandler_DeleteApplication_RefusesRunningEnvironment locks real AWS's
// DeleteApplication doc comment: "You cannot delete an application that has
// a running environment." Passing TerminateEnvByForce=true terminates the
// environment first and lets the delete proceed.
func TestHandler_DeleteApplication_RefusesRunningEnvironment(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=eb-app")
	postEBForm(t, h,
		"Version=2010-12-01&Action=CreateEnvironment&ApplicationName=eb-app&EnvironmentName=eb-env")
	require.Equal(t, 1, h.Backend.EnvironmentCount())

	rec := postEBForm(t, h, "Version=2010-12-01&Action=DeleteApplication&ApplicationName=eb-app")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "running environment")
	assert.Equal(t, 1, h.Backend.ApplicationCount())
	assert.Equal(t, 1, h.Backend.EnvironmentCount())

	rec = postEBForm(t, h,
		"Version=2010-12-01&Action=DeleteApplication&ApplicationName=eb-app&TerminateEnvByForce=true")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, h.Backend.ApplicationCount())
	assert.Equal(t, 0, h.Backend.EnvironmentCount())
}

func TestHandler_UpdateApplication(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		body       string
		wantXML    string
		wantStatus int
		setup      bool
	}{
		{
			name:       "success",
			setup:      true,
			body:       "Version=2010-12-01&Action=UpdateApplication&ApplicationName=my-app&Description=updated",
			wantStatus: http.StatusOK,
			wantXML:    "UpdateApplicationResponse",
		},
		{
			name:       "missing application name",
			body:       "Version=2010-12-01&Action=UpdateApplication&Description=updated",
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "not found",
			body:       "Version=2010-12-01&Action=UpdateApplication&ApplicationName=nonexistent",
			wantStatus: http.StatusBadRequest,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()

			if tt.setup {
				postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=my-app")
			}

			rec := postEBForm(t, h, tt.body)
			assert.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantXML != "" {
				assert.Contains(t, rec.Body.String(), tt.wantXML)
			}
		})
	}
}

// TestHandler_UpdateApplicationResourceLifecycle_SurfacedOnDescribe verifies that a
// lifecycle service role set via UpdateApplicationResourceLifecycle is
// readable back through DescribeApplications -- the backend stores it on
// Application, and it must not be a write-only field.
func TestHandler_UpdateApplicationResourceLifecycle_SurfacedOnDescribe(t *testing.T) {
	t.Parallel()

	h := newTestHandler()
	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=lc-app")

	rec := postEBForm(t, h,
		"Version=2010-12-01&Action=UpdateApplicationResourceLifecycle&ApplicationName=lc-app"+
			"&ResourceLifecycleConfig.ServiceRole=arn:aws:iam::123456789012:role/eb-lifecycle")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications&ApplicationNames.member.1=lc-app")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "<ResourceLifecycleConfig>")
	assert.Contains(t, body, "<ServiceRole>arn:aws:iam::123456789012:role/eb-lifecycle</ServiceRole>")
}

// TestHandler_CreateApplication_DateCreatedPresent verifies that DateCreated is
// present in both create and describe application responses.
func TestHandler_CreateApplication_DateCreatedPresent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		setup  string
		action string
	}{
		{
			name:   "create response includes DateCreated",
			action: "Version=2010-12-01&Action=CreateApplication&ApplicationName=ts-app",
		},
		{
			name:   "describe response includes DateCreated",
			setup:  "Version=2010-12-01&Action=CreateApplication&ApplicationName=ts-app",
			action: "Version=2010-12-01&Action=DescribeApplications",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newTestHandler()
			if tt.setup != "" {
				postEBForm(t, h, tt.setup)
			}

			rec := postEBForm(t, h, tt.action)
			require.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "<DateCreated>")
		})
	}
}

// TestHandler_CreateApplication_AutoCreatesDefaultConfigurationTemplate locks
// real AWS's documented CreateApplication behavior: "Creates an application
// that has one configuration template named default" (see the API doc
// example response, which renders it capitalized as "Default"). Before this
// was implemented, a freshly created application had zero configuration
// templates -- a state no real Elastic Beanstalk application can be in.
func TestHandler_CreateApplication_AutoCreatesDefaultConfigurationTemplate(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=defapp")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<member>Default</member>")
	assert.Equal(t, 1, h.Backend.ConfigTemplateCount())

	// The Default template must be independently describable, not merely
	// listed on the application.
	descRec := postEBForm(t, h,
		"Version=2010-12-01&Action=DescribeConfigurationSettings&ApplicationName=defapp&TemplateName=Default")
	require.Equal(t, http.StatusOK, descRec.Code)
	assert.Contains(t, descRec.Body.String(), "<TemplateName>Default</TemplateName>")
}

// TestHandler_CreateApplicationVersion_AutoCreateApplication_AlsoGetsDefaultTemplate
// verifies that an application implicitly created via
// CreateApplicationVersion's AutoCreateApplication=true carries the same
// auto-provisioned Default template as one created via CreateApplication
// directly -- both go through the same underlying "create application"
// state transition in real AWS.
func TestHandler_CreateApplicationVersion_AutoCreateApplication_AlsoGetsDefaultTemplate(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	rec := postEBForm(t, h,
		"Version=2010-12-01&Action=CreateApplicationVersion"+
			"&ApplicationName=auto-app&VersionLabel=v1&AutoCreateApplication=true")
	require.Equal(t, http.StatusOK, rec.Code)

	assert.Equal(t, 1, h.Backend.ConfigTemplateCount())

	descRec := postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications")
	require.Equal(t, http.StatusOK, descRec.Code)
	assert.Contains(t, descRec.Body.String(), "<member>Default</member>")
}

// TestHandler_DeleteApplication_CascadesDefaultTemplate verifies the
// auto-created Default template is not a ghost row surviving application
// deletion -- it must be cleaned up by the same cascade-delete path as any
// explicitly created template.
func TestHandler_DeleteApplication_CascadesDefaultTemplate(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=defapp2")
	require.Equal(t, 1, h.Backend.ConfigTemplateCount())

	rec := postEBForm(t, h, "Version=2010-12-01&Action=DeleteApplication&ApplicationName=defapp2")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, 0, h.Backend.ConfigTemplateCount())
}

// TestHandler_DescribeApplications_IncludesVersions locks that
// ApplicationDescription.Versions (real AWS: "The names of the versions for
// this application") lists application version labels -- this field
// existed on the wire type but was never populated.
func TestHandler_DescribeApplications_IncludesVersions(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=verapp")

	// No versions yet: Versions must not claim a nonexistent version label.
	rec := postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotContains(t, rec.Body.String(), "<member>v1</member>")

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplicationVersion&ApplicationName=verapp&VersionLabel=v1")
	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplicationVersion&ApplicationName=verapp&VersionLabel=v2")

	rec = postEBForm(t, h, "Version=2010-12-01&Action=DescribeApplications")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "<Versions><member>v1</member><member>v2</member></Versions>")
}

// TestHandler_UpdateApplication_IncludesVersionsAndTemplates verifies
// UpdateApplication's response -- like CreateApplication and
// DescribeApplications -- surfaces the application's current
// ConfigurationTemplates and Versions rather than always rendering them
// empty.
func TestHandler_UpdateApplication_IncludesVersionsAndTemplates(t *testing.T) {
	t.Parallel()

	h := newTestHandler()

	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplication&ApplicationName=updapp")
	postEBForm(t, h, "Version=2010-12-01&Action=CreateApplicationVersion&ApplicationName=updapp&VersionLabel=v1")

	rec := postEBForm(t, h, "Version=2010-12-01&Action=UpdateApplication&ApplicationName=updapp&Description=new")
	require.Equal(t, http.StatusOK, rec.Code)
	body := rec.Body.String()
	assert.Contains(t, body, "<member>Default</member>")
	assert.Contains(t, body, "<member>v1</member>")
}
