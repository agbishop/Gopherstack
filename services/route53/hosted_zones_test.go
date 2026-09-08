package route53_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53"
)

// TestCreateHostedZone_RequiresNameAndCallerReference verifies that
// CreateHostedZone rejects requests missing Name or CallerReference.
// Real AWS returns 400 InvalidInput for both cases; the emulator had the
// backend validation but lacked handler-level parity tests.
func TestCreateHostedZone_RequiresNameAndCallerReference(t *testing.T) {
	t.Parallel()

	const path = "/2013-04-01/hostedzone"

	tests := []struct {
		body     string
		name     string
		wantCode int
	}{
		{
			name: "missing_zone_name_rejected",
			body: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">` +
				`<CallerReference>ref-no-name</CallerReference>` +
				`</CreateHostedZoneRequest>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name: "missing_caller_reference_rejected",
			body: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">` +
				`<Name>example.com</Name>` +
				`</CreateHostedZoneRequest>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name: "valid_request_accepted",
			body: `<?xml version="1.0" encoding="UTF-8"?>` +
				`<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">` +
				`<Name>parity-test.com</Name>` +
				`<CallerReference>ref-parity-ok</CallerReference>` +
				`</CreateHostedZoneRequest>`,
			wantCode: http.StatusCreated,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t)
			rec := send(t, h, http.MethodPost, path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code,
				"CreateHostedZone status for case %q", tt.name)

			if tt.wantCode == http.StatusBadRequest {
				assert.Contains(t, rec.Body.String(), "InvalidInput",
					"expected InvalidInput error code")
			}
		})
	}
}

// TestCreateHostedZone_CallerReferenceIdempotency verifies that
// reusing the same CallerReference returns the existing zone rather than
// creating a duplicate. Real AWS guarantees this idempotency behavior.
func TestCreateHostedZone_CallerReferenceIdempotency(t *testing.T) {
	t.Parallel()

	const path = "/2013-04-01/hostedzone"

	body := `<?xml version="1.0" encoding="UTF-8"?>` +
		`<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">` +
		`<Name>idem-test.com</Name>` +
		`<CallerReference>ref-idem-1</CallerReference>` +
		`</CreateHostedZoneRequest>`

	h := newHandler(t)

	rec1 := send(t, h, http.MethodPost, path, body)
	assert.Equal(t, http.StatusCreated, rec1.Code, "first create should succeed")

	zoneID := extractZoneID(t, rec1.Body.String())

	rec2 := send(t, h, http.MethodPost, path, body)
	assert.Equal(t, http.StatusCreated, rec2.Code,
		"second create with same CallerReference should return existing zone")

	zoneID2 := extractZoneID(t, rec2.Body.String())
	assert.Equal(t, zoneID, zoneID2,
		"same CallerReference should return the same zone ID both times")
}

func TestDeleteHostedZone_DeregistersDNS(t *testing.T) {
	t.Parallel()

	const addRecordXML = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	tests := []struct {
		name     string
		hostname string
	}{
		{name: "deregisters_dns_on_zone_delete", hostname: "www.example.com."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			registered := make(map[string]bool)
			reg := &mockDNSRegistrar{registered: registered}
			backend := route53.NewInMemoryBackend()
			backend.SetDNSRegistrar(reg)
			h := route53.NewHandler(backend)

			rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
			require.Equal(t, http.StatusCreated, rec.Code)

			zoneID := extractZoneID(t, rec.Body.String())

			send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", addRecordXML)
			require.True(t, reg.registered[tt.hostname])

			// Delete the record first — AWS rejects deletion of non-empty zones.
			const deleteRecordXML = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>DELETE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`
			delRRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", deleteRecordXML)
			require.Equal(t, http.StatusOK, delRRec.Code)

			delRec := send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
			require.Equal(t, http.StatusOK, delRec.Code)
			assert.False(t, reg.registered[tt.hostname])
		})
	}
}

func TestHostedZoneAlreadyExists_HTTP(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	const body1 = `<?xml version="1.0" encoding="UTF-8"?>
<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Name>example.com</Name>
  <CallerReference>shared-ref</CallerReference>
</CreateHostedZoneRequest>`

	const body2 = `<?xml version="1.0" encoding="UTF-8"?>
<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Name>different.com</Name>
  <CallerReference>shared-ref</CallerReference>
</CreateHostedZoneRequest>`

	first := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", body1)
	require.Equal(t, http.StatusCreated, first.Code)

	second := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", body2)
	assert.Equal(t, http.StatusConflict, second.Code)
	assert.Contains(t, second.Body.String(), "HostedZoneAlreadyExists")
}

func TestDeleteHostedZone_NotEmpty(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		records  string
		wantBody string
		wantCode int
	}{
		{
			name: "zone_with_A_record_rejected",
			records: `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch><Changes><Change>
    <Action>CREATE</Action>
    <ResourceRecordSet>
      <Name>www.example.com</Name><Type>A</Type><TTL>300</TTL>
      <ResourceRecords><ResourceRecord><Value>1.2.3.4</Value></ResourceRecord></ResourceRecords>
    </ResourceRecordSet>
  </Change></Changes></ChangeBatch>
</ChangeResourceRecordSetsRequest>`,
			wantCode: http.StatusBadRequest,
			wantBody: "HostedZoneNotEmpty",
		},
		{
			name: "zone_with_TXT_record_rejected",
			records: `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch><Changes><Change>
    <Action>CREATE</Action>
    <ResourceRecordSet>
      <Name>_dmarc.example.com</Name><Type>TXT</Type><TTL>300</TTL>
      <ResourceRecords><ResourceRecord><Value>"v=DMARC1"</Value></ResourceRecord></ResourceRecords>
    </ResourceRecordSet>
  </Change></Changes></ChangeBatch>
</ChangeResourceRecordSetsRequest>`,
			wantCode: http.StatusBadRequest,
			wantBody: "HostedZoneNotEmpty",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t)

			rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
			require.Equal(t, http.StatusCreated, rec.Code)
			zoneID := extractZoneID(t, rec.Body.String())

			// Add records to make the zone non-empty.
			rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", tt.records)
			require.Equal(t, http.StatusOK, rec.Code)

			// Delete should fail with HostedZoneNotEmpty.
			rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
			assert.Equal(t, tt.wantCode, rec.Code)
			assert.Contains(t, rec.Body.String(), tt.wantBody)
		})
	}
}

func TestDeleteHostedZone_RejectedWhileDNSSECEnabled(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	zoneID := createZoneForOpsTest(t, h)
	createKSKForOpsTest(t, h, zoneID, "main-key")

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/enable-dnssec", "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "HostedZoneNotEmpty")

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/disable-dnssec", "")
	require.Equal(t, http.StatusOK, rec.Code)

	rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestDeleteHostedZone_EmptyZoneSucceeds(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
	}{
		{name: "empty_zone_deleted"},
		{name: "empty_zone_deleted_second"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t)

			rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
			require.Equal(t, http.StatusCreated, rec.Code)
			zoneID := extractZoneID(t, rec.Body.String())

			rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
			assert.Equal(t, http.StatusOK, rec.Code)
			assert.Contains(t, rec.Body.String(), "DeleteHostedZoneResponse")
		})
	}
}

func TestDeleteHostedZone_AfterRecordsRemoved_Succeeds(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	// Add a record.
	addBody := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch><Changes><Change>
    <Action>CREATE</Action>
    <ResourceRecordSet>
      <Name>host.example.com</Name><Type>A</Type><TTL>60</TTL>
      <ResourceRecords><ResourceRecord><Value>10.0.0.1</Value></ResourceRecord></ResourceRecords>
    </ResourceRecordSet>
  </Change></Changes></ChangeBatch>
</ChangeResourceRecordSetsRequest>`
	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", addBody)
	require.Equal(t, http.StatusOK, rec.Code)

	// Delete fails while record exists.
	rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
	require.Equal(t, http.StatusBadRequest, rec.Code)

	// Remove the record.
	delBody := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch><Changes><Change>
    <Action>DELETE</Action>
    <ResourceRecordSet>
      <Name>host.example.com</Name><Type>A</Type><TTL>60</TTL>
      <ResourceRecords><ResourceRecord><Value>10.0.0.1</Value></ResourceRecord></ResourceRecords>
    </ResourceRecordSet>
  </Change></Changes></ChangeBatch>
</ChangeResourceRecordSetsRequest>`
	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", delBody)
	require.Equal(t, http.StatusOK, rec.Code)

	// Now delete succeeds.
	rec = send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestCreateHostedZone_DuplicateCallerReference(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		ref     string
		name2   string
		comment string
	}{
		{
			name:    "same_ref_same_params_returns_existing",
			ref:     "unique-caller-ref-hz-1",
			name2:   "example.com",
			comment: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()

			first, err := b.CreateHostedZone("example.com", tt.ref, "first", false, "", "", "")
			require.NoError(t, err)

			// Same CallerReference *and* identical other parameters is a safe
			// retry: AWS returns the original zone.
			second, err := b.CreateHostedZone(tt.name2, tt.ref, tt.comment, false, "", "", "")
			require.NoError(t, err)

			assert.Equal(t, first.ID, second.ID,
				"duplicate CallerReference with identical params must return the same zone ID")
			assert.Equal(t, first.Name, second.Name,
				"original zone name must be preserved")
		})
	}
}

// TestCreateHostedZone_DuplicateCallerReference_DifferentParams verifies
// real AWS behavior: reusing a CallerReference with a *different* Name (or
// Comment/PrivateZone) is NOT idempotent — it returns HostedZoneAlreadyExists
// (409), since the CallerReference is now ambiguous between two distinct
// requested zones. A prior version of this test asserted the request was
// "still idempotent" for a different name, which is not how Route 53 behaves.
func TestCreateHostedZone_DuplicateCallerReference_DifferentParams(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref  string
	}{
		{name: "same_ref_different_name_rejected", ref: "unique-caller-ref-hz-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()

			_, err := b.CreateHostedZone("example.com", tt.ref, "first", false, "", "", "")
			require.NoError(t, err)

			_, err = b.CreateHostedZone("other.com", tt.ref, "second", false, "", "", "")
			require.Error(t, err)
			assert.ErrorIs(t, err, route53.ErrHostedZoneAlreadyExists)
		})
	}
}

func TestCreateHostedZone_UniqueCallerReference_CreatesNew(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ref1 string
		ref2 string
	}{
		{name: "different_refs_create_two_zones", ref1: "ref-a", ref2: "ref-b"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()

			z1, err := b.CreateHostedZone("example.com", tt.ref1, "", false, "", "", "")
			require.NoError(t, err)

			z2, err := b.CreateHostedZone("example.com", tt.ref2, "", false, "", "", "")
			require.NoError(t, err)

			assert.NotEqual(t, z1.ID, z2.ID,
				"distinct CallerReferences must create distinct zones")
		})
	}
}

func TestCreateHostedZone_DuplicateCallerReference_HTTPRoundTrip(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	const ref = "idempotent-hz-ref"
	body := `<?xml version="1.0" encoding="UTF-8"?>
<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Name>example.com</Name>
  <CallerReference>` + ref + `</CallerReference>
</CreateHostedZoneRequest>`

	rec1 := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", body)
	require.Equal(t, http.StatusCreated, rec1.Code)
	id1 := extractZoneID(t, rec1.Body.String())

	rec2 := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", body)
	require.Equal(t, http.StatusCreated, rec2.Code)
	id2 := extractZoneID(t, rec2.Body.String())

	assert.Equal(t, id1, id2,
		"second CreateHostedZone with same CallerReference must return the same zone ID")
}

func TestDeleteEmptyZone_WithDefaultNSSOA_Succeeds(t *testing.T) {
	t.Parallel()

	// A zone with only NS+SOA (the defaults) must be deletable.
	// This regression test ensures the non-empty check skips default records.
	tests := []struct {
		name string
		ref  string
	}{
		{name: "delete_zone_with_only_defaults", ref: "ref-del-1"},
		{name: "delete_second_zone", ref: "ref-del-2"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()
			hz, err := b.CreateHostedZone("example.com", tt.ref, "", false, "", "", "")
			require.NoError(t, err)

			err = b.DeleteHostedZone(hz.ID)
			assert.NoError(t, err, "zone with only default NS+SOA must be deletable")
		})
	}
}

func TestPrivateZone(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		private bool
	}{
		{name: "public_zone_flag_false", private: false},
		{name: "private_zone_flag_true", private: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()
			hz, err := b.CreateHostedZone("example.com", "ref-pvt-"+tt.name, "", tt.private, "", "", "")
			require.NoError(t, err)
			assert.Equal(t, tt.private, hz.PrivateZone)

			got, err := b.GetHostedZone(hz.ID)
			require.NoError(t, err)
			assert.Equal(t, tt.private, got.PrivateZone)
		})
	}
}

func TestZoneCount(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		wantCount int
		creates   int
	}{
		{
			name:      "single_create",
			creates:   1,
			wantCount: 1,
		},
		{
			name:      "three_creates",
			creates:   3,
			wantCount: 3,
		},
		{
			name:      "zero_creates",
			creates:   0,
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()

			for i := range tt.creates {
				_, err := b.CreateHostedZone("example.com", "ref-"+string(rune('A'+i)), "", false, "", "", "")
				require.NoError(t, err)
			}

			assert.Equal(t, tt.wantCount, route53.ZoneCount(b))
		})
	}
}

func TestUpdateHostedZoneComment(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<UpdateHostedZoneCommentRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Comment>updated comment</Comment>
</UpdateHostedZoneCommentRequest>`

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID, body)
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "UpdateHostedZoneCommentResponse")

	// Verify comment persisted.
	rec = send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "updated comment")
}

func TestUpdateHostedZoneComment_NotFound(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	body := `<?xml version="1.0" encoding="UTF-8"?>
<UpdateHostedZoneCommentRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Comment>nope</Comment>
</UpdateHostedZoneCommentRequest>`

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/ZNONEXISTENT", body)
	assert.Equal(t, http.StatusNotFound, rec.Code)
}

func TestGetHostedZoneLimit(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	rec = send(t, h, http.MethodGet, "/2013-04-01/hostedzonelimit/"+zoneID+"/MAX_RRSETS_BY_ZONE", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "GetHostedZoneLimitResponse")
	assert.Contains(t, rec.Body.String(), "MAX_RRSETS_BY_ZONE")
}

func TestListHostedZonesByVPC(t *testing.T) {
	t.Parallel()

	b := route53.NewInMemoryBackend()

	hz, err := b.CreateHostedZone("private.example.com", "ref", "", true, "", "", "")
	require.NoError(t, err)

	require.NoError(t, b.AssociateVPCWithHostedZone(hz.ID, "vpc-123", "us-east-1"))

	p, err := b.ListHostedZonesByVPC("vpc-123", "", "", 0)
	require.NoError(t, err)
	require.Len(t, p.Data, 1)
	assert.Equal(t, hz.ID, p.Data[0].ID)

	// Different VPC — no results.
	p, err = b.ListHostedZonesByVPC("vpc-other", "", "", 0)
	require.NoError(t, err)
	assert.Empty(t, p.Data)
}

func TestRoute53Handler(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name         string
		method       string
		path         string
		body         string
		wantContains []string
		wantCode     int
	}{
		{
			name:         "CreateHostedZone",
			method:       http.MethodPost,
			path:         "/2013-04-01/hostedzone",
			body:         createZoneXML,
			wantCode:     http.StatusCreated,
			wantContains: []string{"example.com", "INSYNC"},
		},
		{
			name:   "CreateHostedZone_MissingName",
			method: http.MethodPost,
			path:   "/2013-04-01/hostedzone",
			body: `<?xml version="1.0"?>
<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Name></Name>
  <CallerReference>ref-1</CallerReference>
</CreateHostedZoneRequest>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:   "CreateHostedZone_MissingCallerRef",
			method: http.MethodPost,
			path:   "/2013-04-01/hostedzone",
			body: `<?xml version="1.0"?>
<CreateHostedZoneRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <Name>example.com</Name>
  <CallerReference></CallerReference>
</CreateHostedZoneRequest>`,
			wantCode: http.StatusBadRequest,
		},
		{
			name:         "ListHostedZones_Empty",
			method:       http.MethodGet,
			path:         "/2013-04-01/hostedzone",
			wantCode:     http.StatusOK,
			wantContains: []string{"ListHostedZonesResponse"},
		},
		{
			name:     "GetHostedZone_NotFound",
			method:   http.MethodGet,
			path:     "/2013-04-01/hostedzone/ZNONEXISTENT",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "DeleteHostedZone_NotFound",
			method:   http.MethodDelete,
			path:     "/2013-04-01/hostedzone/ZNONEXISTENT",
			wantCode: http.StatusNotFound,
		},
		{
			name:     "ListResourceRecordSets_NotFound",
			method:   http.MethodGet,
			path:     "/2013-04-01/hostedzone/ZNONEXISTENT/rrset",
			wantCode: http.StatusNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			h := newHandler(t)
			rec := send(t, h, tt.method, tt.path, tt.body)
			assert.Equal(t, tt.wantCode, rec.Code)

			for _, s := range tt.wantContains {
				assert.Contains(t, rec.Body.String(), s)
			}
		})
	}
}

func TestRoute53Handler_DeleteHostedZone(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)

	zoneID := extractZoneID(t, rec.Body.String())
	delRec := send(t, h, http.MethodDelete, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusOK, delRec.Code)

	// Zone should no longer be found.
	getRec := send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID, "")
	assert.Equal(t, http.StatusNotFound, getRec.Code)
}
