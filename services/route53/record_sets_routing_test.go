package route53_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53"
)

func int64ptr(v int64) *int64 {
	p := new(int64)
	*p = v

	return p
}

func TestWeightedRouting(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // fieldalignment: test struct, cosmetic only
		name      string
		weight    *int64
		wantErr   bool
		wantErrIs string
	}{
		{
			name:    "weight_100_accepted",
			weight:  int64ptr(100),
			wantErr: false,
		},
		{
			name:    "weight_0_accepted", // stop-traffic record — previously rejected
			weight:  int64ptr(0),
			wantErr: false,
		},
		{
			name:    "weight_255_accepted",
			weight:  int64ptr(255),
			wantErr: false,
		},
		{
			name:      "weight_256_rejected",
			weight:    int64ptr(256),
			wantErr:   true,
			wantErrIs: "InvalidChangeBatch",
		},
		{
			name:      "weight_negative_rejected",
			weight:    int64ptr(-1),
			wantErr:   true,
			wantErrIs: "InvalidChangeBatch",
		},
		{
			name:    "nil_weight_no_routing_policy_accepted",
			weight:  nil,
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()
			hz, err := b.CreateHostedZone("example.com", "ref-wt-"+tt.name, "", false, "", "", "")
			require.NoError(t, err)

			setID := ""
			if tt.weight != nil {
				setID = "id1" // SetIdentifier required for routing-policy records
			}

			changes := []route53.Change{
				{
					Action: route53.ChangeActionCreate,
					ResourceRecordSet: route53.ResourceRecordSet{
						Name:          "host.example.com.",
						Type:          "A",
						TTL:           300,
						Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
						SetIdentifier: setID,
						Weight:        tt.weight,
					},
				},
			}

			_, err = b.ChangeResourceRecordSets(hz.ID, changes)
			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErrIs)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestWeightZero_RoundTrip(t *testing.T) {
	t.Parallel()

	// Weight=0 must survive a write and be readable back as 0.
	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch><Changes><Change>
    <Action>CREATE</Action>
    <ResourceRecordSet>
      <Name>w0.example.com</Name><Type>A</Type><TTL>300</TTL>
      <SetIdentifier>stop</SetIdentifier>
      <Weight>0</Weight>
      <ResourceRecords><ResourceRecord><Value>1.2.3.4</Value></ResourceRecord></ResourceRecords>
    </ResourceRecordSet>
  </Change></Changes></ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Read back and verify <Weight>0</Weight> is present in the response.
	rec = send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "<Weight>0</Weight>",
		"Weight=0 must be emitted in list response")
}

func TestRoutingPolicyMutualExclusion(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		rrs     route53.ResourceRecordSet
		wantErr bool
	}{
		{
			name: "latency_routing_accepted",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "us-east-1",
				Region:        "us-east-1",
				Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
		},
		{
			name: "failover_primary_accepted",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "primary",
				Failover:      route53.FailoverPrimary,
				Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
		},
		{
			name: "failover_secondary_accepted",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "secondary",
				Failover:      route53.FailoverSecondary,
				Records:       []route53.ResourceRecord{{Value: "2.3.4.5"}},
			},
		},
		{
			name: "multivalue_accepted",
			rrs: route53.ResourceRecordSet{
				Name:             "host.example.com.",
				Type:             "A",
				TTL:              300,
				SetIdentifier:    "mv1",
				MultiValueAnswer: true,
				Records:          []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
		},
		{
			name: "weight_and_failover_rejected",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "id1",
				Weight:        int64ptr(10),
				Failover:      route53.FailoverPrimary,
				Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
			wantErr: true,
		},
		{
			name: "weight_and_latency_rejected",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "id1",
				Weight:        int64ptr(50),
				Region:        "us-east-1",
				Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
			wantErr: true,
		},
		{
			name: "no_policy_no_set_id_accepted",
			rrs: route53.ResourceRecordSet{
				Name:    "plain.example.com.",
				Type:    "A",
				TTL:     300,
				Records: []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
		},
		{
			name: "set_id_without_policy_rejected",
			rrs: route53.ResourceRecordSet{
				Name:          "host.example.com.",
				Type:          "A",
				TTL:           300,
				SetIdentifier: "orphan",
				Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
			wantErr: true,
		},
		{
			name: "policy_without_set_id_rejected",
			rrs: route53.ResourceRecordSet{
				Name:    "host.example.com.",
				Type:    "A",
				TTL:     300,
				Region:  "us-east-1",
				Records: []route53.ResourceRecord{{Value: "1.2.3.4"}},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()
			hz, err := b.CreateHostedZone("example.com", "ref-rp-"+tt.name, "", false, "", "", "")
			require.NoError(t, err)

			changes := []route53.Change{
				{Action: route53.ChangeActionCreate, ResourceRecordSet: tt.rrs},
			}

			_, err = b.ChangeResourceRecordSets(hz.ID, changes)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestWeightedRecordsCoexist(t *testing.T) {
	t.Parallel()

	b := route53.NewInMemoryBackend()
	hz, err := b.CreateHostedZone("example.com", "ref-wcoexist", "", false, "", "", "")
	require.NoError(t, err)

	// Create three weighted records for the same name+type.
	cases := []struct { //nolint:govet // fieldalignment: test struct, cosmetic only
		setID  string
		weight int64
		ip     string
	}{
		{"east", 100, "10.0.0.1"},
		{"west", 50, "10.0.0.2"},
		{"stop", 0, "10.0.0.3"}, // Weight=0: valid stop-traffic record
	}

	for _, c := range cases {
		changes := []route53.Change{
			{
				Action: route53.ChangeActionCreate,
				ResourceRecordSet: route53.ResourceRecordSet{
					Name:          "api.example.com.",
					Type:          "A",
					TTL:           300,
					SetIdentifier: c.setID,
					Weight:        int64ptr(c.weight),
					Records:       []route53.ResourceRecord{{Value: c.ip}},
				},
			},
		}
		_, err = b.ChangeResourceRecordSets(hz.ID, changes)
		require.NoError(t, err, "weight=%d record must be accepted", c.weight)
	}

	pg, err := b.ListResourceRecordSets(hz.ID, "", "", "", 100)
	require.NoError(t, err)

	weightedCount := 0
	for _, rrs := range pg.Records {
		if rrs.Name == "api.example.com." {
			weightedCount++
			assert.NotNil(t, rrs.Weight, "Weight must be non-nil for weighted records")
		}
	}
	assert.Equal(t, 3, weightedCount, "all three weighted records must be present")
}

func TestGeoRoutingAccepted(t *testing.T) {
	t.Parallel()

	tests := []struct { //nolint:govet // fieldalignment: test struct, cosmetic only
		name    string
		wantErr bool
		geo     *route53.GeoLocation
	}{
		{
			name:    "country_routing",
			geo:     &route53.GeoLocation{CountryCode: "US"},
			wantErr: false,
		},
		{
			name:    "continent_routing",
			geo:     &route53.GeoLocation{ContinentCode: "NA"},
			wantErr: false,
		},
		{
			name: "country_and_subdivision",
			geo: &route53.GeoLocation{
				CountryCode:     "US",
				SubdivisionCode: "CA",
			},
			wantErr: false,
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			b := route53.NewInMemoryBackend()
			hz, err := b.CreateHostedZone("example.com", fmt.Sprintf("ref-geo-%d", i), "", false, "", "", "")
			require.NoError(t, err)

			changes := []route53.Change{
				{
					Action: route53.ChangeActionCreate,
					ResourceRecordSet: route53.ResourceRecordSet{
						Name:          fmt.Sprintf("geo%d.example.com.", i),
						Type:          "A",
						TTL:           300,
						SetIdentifier: fmt.Sprintf("geo-id-%d", i),
						GeoLocation:   tt.geo,
						Records:       []route53.ResourceRecord{{Value: "1.2.3.4"}},
					},
				},
			}
			_, err = b.ChangeResourceRecordSets(hz.ID, changes)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestChangeResourceRecordSets_RoutingPolicyMutualExclusion(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	// Weight + Failover together is invalid.
	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>host.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>id1</SetIdentifier>
          <Weight>10</Weight>
          <Failover>PRIMARY</Failover>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestChangeResourceRecordSets_RoutingPolicyRequiresSetIdentifier(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>host.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <Weight>10</Weight>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
}

func TestChangeResourceRecordSets_MultiValueAnswer(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>multi.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>multi-1</SetIdentifier>
          <MultiValueAnswer>true</MultiValueAnswer>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	require.Equal(t, http.StatusOK, rec.Code)

	// Verify MultiValueAnswer appears in list response.
	rec = send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "MultiValueAnswer")
}

func TestListGeoLocations(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	rec := send(t, h, http.MethodGet, "/2013-04-01/geolocation", "")
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Contains(t, rec.Body.String(), "ListGeoLocationsResponse")
	assert.Contains(t, rec.Body.String(), "Africa")
	assert.Contains(t, rec.Body.String(), "Europe")
}

func TestGetGeoLocation(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	tests := []struct {
		name        string
		query       string
		wantContain string
		wantCode    int
	}{
		{
			name:        "valid_continent",
			query:       "?continentcode=EU",
			wantCode:    http.StatusOK,
			wantContain: "Europe",
		},
		{
			name:        "valid_country",
			query:       "?countrycode=US",
			wantCode:    http.StatusOK,
			wantContain: "United States",
		},
		{
			name:        "not_found",
			query:       "?continentcode=XX",
			wantCode:    http.StatusNotFound,
			wantContain: "<Code>NoSuchGeoLocation</Code>",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := send(t, h, http.MethodGet, "/2013-04-01/geolocation"+tt.query, "")
			assert.Equal(t, tt.wantCode, rec.Code)
			if tt.wantContain != "" {
				assert.Contains(t, rec.Body.String(), tt.wantContain)
			}
		})
	}
}

func TestGeoProximityLocation_RequiresOneField(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	// No AWSRegion/Coordinates/LocalZoneGroup — must fail.
	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>geo.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>reg1</SetIdentifier>
          <GeoProximityLocation>
            <Bias>0</Bias>
          </GeoProximityLocation>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost,
		"/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidInput")
}

func TestGeoProximityLocation_BiasTooLarge(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>geo.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>reg1</SetIdentifier>
          <GeoProximityLocation>
            <AWSRegion>us-east-1</AWSRegion>
            <Bias>100</Bias>
          </GeoProximityLocation>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost,
		"/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidInput")
}

func TestGeoProximityLocation_ValidAWSRegion(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>geo.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>reg1</SetIdentifier>
          <GeoProximityLocation>
            <AWSRegion>us-east-1</AWSRegion>
            <Bias>10</Bias>
          </GeoProximityLocation>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost,
		"/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusOK, rec.Code)
}

func TestGeoProximityLocation_InvalidCoordinateLatitude(t *testing.T) {
	t.Parallel()

	h := newHandler(t)

	rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, rec.Code)
	zoneID := extractZoneID(t, rec.Body.String())

	body := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>geo.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <SetIdentifier>reg1</SetIdentifier>
          <GeoProximityLocation>
            <Coordinates>
              <Latitude>91.5</Latitude>
              <Longitude>0.0</Longitude>
            </Coordinates>
          </GeoProximityLocation>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	rec = send(t, h, http.MethodPost,
		"/2013-04-01/hostedzone/"+zoneID+"/rrset", body)
	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Contains(t, rec.Body.String(), "InvalidInput")
}

func TestRoutingPolicy_Weighted(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	createRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, createRec.Code)
	zoneID := extractZoneID(t, createRec.Body.String())

	changeXML := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>primary-us-east</SetIdentifier>
          <Weight>70</Weight>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.1.1.1</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>secondary-us-west</SetIdentifier>
          <Weight>30</Weight>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>2.2.2.2</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	changeRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", changeXML)
	require.Equal(t, http.StatusOK, changeRec.Code)

	listRec := send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, listRec.Code)
	body := listRec.Body.String()

	assert.Contains(t, body, "primary-us-east")
	assert.Contains(t, body, "secondary-us-west")
	assert.Contains(t, body, "1.1.1.1")
	assert.Contains(t, body, "2.2.2.2")
	assert.Contains(t, body, "<Weight>70</Weight>")
	assert.Contains(t, body, "<Weight>30</Weight>")
}

func TestRoutingPolicy_Failover(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	createRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, createRec.Code)
	zoneID := extractZoneID(t, createRec.Body.String())

	// Create health check first.
	hcRec := send(t, h, http.MethodPost, "/2013-04-01/healthcheck", createHealthCheckXML)
	require.Equal(t, http.StatusCreated, hcRec.Code)
	hcID := extractHealthCheckID(t, hcRec.Body.String())

	changeXML := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>api.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>primary</SetIdentifier>
          <Failover>PRIMARY</Failover>
          <HealthCheckId>` + hcID + `</HealthCheckId>
          <TTL>60</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>10.0.1.1</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>api.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>secondary</SetIdentifier>
          <Failover>SECONDARY</Failover>
          <TTL>60</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>10.0.2.1</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	changeRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", changeXML)
	require.Equal(t, http.StatusOK, changeRec.Code)

	listRec := send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, listRec.Code)
	body := listRec.Body.String()

	assert.Contains(t, body, "PRIMARY")
	assert.Contains(t, body, "SECONDARY")
	assert.Contains(t, body, hcID)
	assert.Contains(t, body, "10.0.1.1")
	assert.Contains(t, body, "10.0.2.1")
}

func TestRoutingPolicy_GeoLocation(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	createRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, createRec.Code)
	zoneID := extractZoneID(t, createRec.Body.String())

	changeXML := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>geo.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>us-record</SetIdentifier>
          <GeoLocation>
            <CountryCode>US</CountryCode>
          </GeoLocation>
          <TTL>60</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.2.3.4</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	changeRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", changeXML)
	require.Equal(t, http.StatusOK, changeRec.Code)

	listRec := send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, listRec.Code)
	body := listRec.Body.String()

	assert.Contains(t, body, "us-record")
	assert.Contains(t, body, "US")
	assert.Contains(t, body, "1.2.3.4")
}

func TestRoutingPolicy_LatencyBased(t *testing.T) {
	t.Parallel()

	h := newHandler(t)
	createRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
	require.Equal(t, http.StatusCreated, createRec.Code)
	zoneID := extractZoneID(t, createRec.Body.String())

	changeXML := `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>latency.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>us-east-1</SetIdentifier>
          <Region>us-east-1</Region>
          <TTL>60</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>3.4.5.6</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

	changeRec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", changeXML)
	require.Equal(t, http.StatusOK, changeRec.Code)

	listRec := send(t, h, http.MethodGet, "/2013-04-01/hostedzone/"+zoneID+"/rrset", "")
	require.Equal(t, http.StatusOK, listRec.Code)
	body := listRec.Body.String()

	assert.Contains(t, body, "us-east-1")
	assert.Contains(t, body, "3.4.5.6")
}
