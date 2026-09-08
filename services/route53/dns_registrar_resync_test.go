package route53_test

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/blackbirdworks/gopherstack/services/route53"
)

// TestChangeResourceRecordSets_DNSRegistrarResync covers cases the simple
// create/delete DNSRegistrar tests in record_sets_test.go miss: a name with
// more than one live record set (weighted routing) sharing the embedded
// DNS server's per-hostname registration.
func TestChangeResourceRecordSets_DNSRegistrarResync(t *testing.T) {
	t.Parallel()

	t.Run("upsert_replaces_stale_dns_value", func(t *testing.T) {
		t.Parallel()

		registrar := &mockDNSRegistrar{registered: make(map[string]bool)}
		backend := route53.NewInMemoryBackend()
		backend.SetDNSRegistrar(registrar)
		h := route53.NewHandler(backend)

		rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
		require.Equal(t, http.StatusCreated, rec.Code)

		zoneID := extractZoneID(t, rec.Body.String())

		const createXML = `<?xml version="1.0" encoding="UTF-8"?>
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

		rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", createXML)
		require.Equal(t, http.StatusOK, rec.Code)
		require.Equal(t, []string{"1.2.3.4"}, registrar.records["www.example.com."])

		const upsertXML = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>UPSERT</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>5.6.7.8</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

		rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", upsertXML)
		require.Equal(t, http.StatusOK, rec.Code)

		assert.Equal(t, []string{"5.6.7.8"}, registrar.records["www.example.com."],
			"UPSERT must replace the DNS registration's values, not append to the old ones")
	})

	t.Run("delete_one_of_two_weighted_keeps_sibling_registered", func(t *testing.T) {
		t.Parallel()

		registrar := &mockDNSRegistrar{registered: make(map[string]bool)}
		backend := route53.NewInMemoryBackend()
		backend.SetDNSRegistrar(registrar)
		h := route53.NewHandler(backend)

		rec := send(t, h, http.MethodPost, "/2013-04-01/hostedzone", createZoneXML)
		require.Equal(t, http.StatusCreated, rec.Code)

		zoneID := extractZoneID(t, rec.Body.String())

		const createXML = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>CREATE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>a</SetIdentifier>
          <Weight>50</Weight>
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
          <SetIdentifier>b</SetIdentifier>
          <Weight>50</Weight>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>2.2.2.2</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

		rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", createXML)
		require.Equal(t, http.StatusOK, rec.Code)
		require.True(t, registrar.registered["www.example.com."])

		const deleteAXML = `<?xml version="1.0" encoding="UTF-8"?>
<ChangeResourceRecordSetsRequest xmlns="https://route53.amazonaws.com/doc/2013-04-01/">
  <ChangeBatch>
    <Changes>
      <Change>
        <Action>DELETE</Action>
        <ResourceRecordSet>
          <Name>www.example.com</Name>
          <Type>A</Type>
          <SetIdentifier>a</SetIdentifier>
          <Weight>50</Weight>
          <TTL>300</TTL>
          <ResourceRecords>
            <ResourceRecord><Value>1.1.1.1</Value></ResourceRecord>
          </ResourceRecords>
        </ResourceRecordSet>
      </Change>
    </Changes>
  </ChangeBatch>
</ChangeResourceRecordSetsRequest>`

		rec = send(t, h, http.MethodPost, "/2013-04-01/hostedzone/"+zoneID+"/rrset", deleteAXML)
		require.Equal(t, http.StatusOK, rec.Code)

		assert.True(t, registrar.registered["www.example.com."],
			"deleting one weighted record set must not deregister a sibling record set sharing the same name")
		assert.Equal(t, []string{"2.2.2.2"}, registrar.records["www.example.com."],
			"the surviving weighted record's value must still be registered")
	})
}
