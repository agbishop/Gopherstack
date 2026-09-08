package transfer

import (
	"fmt"
	"maps"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
)

// CreateConnectorInput holds all fields for CreateConnector.
type CreateConnectorInput struct {
	SftpConfig         *ConnectorSftpConfig
	As2Config          *ConnectorAs2Config
	Tags               map[string]string
	URL                string
	AccessRole         string
	LoggingRole        string
	SecurityPolicyName string
	IPAddressType      string
}

// CreateConnector creates a Transfer connector. URL is required.
func (b *InMemoryBackend) CreateConnector(
	url, accessRole string,
	sftpConfig *ConnectorSftpConfig,
	as2Config *ConnectorAs2Config,
	tags map[string]string,
) (*Connector, error) {
	return b.CreateConnectorFull(&CreateConnectorInput{
		URL:        url,
		AccessRole: accessRole,
		SftpConfig: sftpConfig,
		As2Config:  as2Config,
		Tags:       tags,
	})
}

// CreateConnectorFull creates a Transfer connector with full configuration.
func (b *InMemoryBackend) CreateConnectorFull(in *CreateConnectorInput) (*Connector, error) {
	if in.URL == "" {
		return nil, fmt.Errorf("%w: Url is required", ErrValidation)
	}

	b.mu.Lock("CreateConnector")
	defer b.mu.Unlock()

	connectorID := "c-" + uuid.NewString()[:20]

	merged := make(map[string]string, len(in.Tags))
	maps.Copy(merged, in.Tags)

	c := &Connector{
		ConnectorID:        connectorID,
		URL:                in.URL,
		AccessRole:         in.AccessRole,
		SftpConfig:         in.SftpConfig,
		As2Config:          in.As2Config,
		LoggingRole:        in.LoggingRole,
		SecurityPolicyName: in.SecurityPolicyName,
		IPAddressType:      in.IPAddressType,
		CreatedAt:          time.Now(),
		Tags:               merged,
		AccountID:          b.accountID,
		Region:             b.region,
	}
	b.connectors.Put(c)
	b.initTagsStore(arn.Build("transfer", b.region, b.accountID, "connector/"+connectorID), merged)

	return cloneConnector(c), nil
}

// DeleteConnector removes a connector by ID.
func (b *InMemoryBackend) DeleteConnector(connectorID string) error {
	b.mu.Lock("DeleteConnector")
	defer b.mu.Unlock()

	if !b.connectors.Has(connectorID) {
		return fmt.Errorf("%w: connector %s not found", ErrConnectorNotFound, connectorID)
	}

	b.connectors.Delete(connectorID)
	delete(b.tagsStore, connectorARN(b.accountID, b.region, connectorID))

	return nil
}

// DescribeConnector returns a connector by ID.
func (b *InMemoryBackend) DescribeConnector(connectorID string) (*Connector, error) {
	b.mu.RLock("DescribeConnector")
	defer b.mu.RUnlock()

	c, ok := b.connectors.Get(connectorID)
	if !ok {
		return nil, fmt.Errorf("%w: connector %s not found", ErrConnectorNotFound, connectorID)
	}

	return cloneConnector(c), nil
}

// ListConnectors returns all connectors sorted by connectorID.
func (b *InMemoryBackend) ListConnectors() []*Connector {
	b.mu.RLock("ListConnectors")
	defer b.mu.RUnlock()

	all := b.connectors.All()
	out := make([]*Connector, 0, len(all))

	for _, c := range all {
		out = append(out, cloneConnector(c))
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].ConnectorID < out[j].ConnectorID
	})

	return out
}

// UpdateConnectorInput holds all optional fields for UpdateConnector.
type UpdateConnectorInput struct {
	SftpConfig            *ConnectorSftpConfig
	As2Config             *ConnectorAs2Config
	ConnectorID           string
	URL                   string
	AccessRole            string
	LoggingRole           string
	SecurityPolicyName    string
	IPAddressType         string
	SetLoggingRole        bool
	SetSecurityPolicyName bool
	SetIPAddressType      bool
}

// UpdateConnector updates mutable fields on a connector.
func (b *InMemoryBackend) UpdateConnector(
	connectorID, url, accessRole string,
	sftpConfig *ConnectorSftpConfig,
	as2Config *ConnectorAs2Config,
) (*Connector, error) {
	return b.UpdateConnectorFull(&UpdateConnectorInput{
		ConnectorID: connectorID,
		URL:         url,
		AccessRole:  accessRole,
		SftpConfig:  sftpConfig,
		As2Config:   as2Config,
	})
}

// UpdateConnectorFull updates all mutable fields on a connector.
func (b *InMemoryBackend) UpdateConnectorFull(in *UpdateConnectorInput) (*Connector, error) {
	b.mu.Lock("UpdateConnector")
	defer b.mu.Unlock()

	c, ok := b.connectors.Get(in.ConnectorID)
	if !ok {
		return nil, fmt.Errorf("%w: connector %s not found", ErrConnectorNotFound, in.ConnectorID)
	}

	if in.URL != "" {
		c.URL = in.URL
	}

	if in.AccessRole != "" {
		c.AccessRole = in.AccessRole
	}

	if in.SftpConfig != nil {
		c.SftpConfig = in.SftpConfig
	}

	if in.As2Config != nil {
		c.As2Config = in.As2Config
	}

	if in.SetLoggingRole {
		c.LoggingRole = in.LoggingRole
	}

	if in.SetSecurityPolicyName {
		c.SecurityPolicyName = in.SecurityPolicyName
	}

	if in.SetIPAddressType {
		c.IPAddressType = in.IPAddressType
	}

	return cloneConnector(c), nil
}

// StartFileFileTransferResult persists a transfer record and returns the transferID.
func (b *InMemoryBackend) StartFileFileTransferResult(connectorID string, files []string) string {
	b.mu.Lock("StartFileFileTransferResult")
	defer b.mu.Unlock()

	transferID := uuid.NewString()

	b.transferRecords.Put(&FileTransferResult{
		TransferID:  transferID,
		ConnectorID: connectorID,
		Status:      "QUEUED",
		Files:       files,
		CreatedAt:   time.Now(),
	})

	return transferID
}

// ListFileFileTransferResults returns all transfer records for a connector.
func (b *InMemoryBackend) ListFileFileTransferResults(connectorID string) []*FileTransferResult {
	b.mu.RLock("ListFileFileTransferResults")
	defer b.mu.RUnlock()

	var out []*FileTransferResult

	for _, r := range b.transferRecords.All() {
		if connectorID == "" || r.ConnectorID == connectorID {
			cp := *r
			out = append(out, &cp)
		}
	}

	sort.Slice(out, func(i, j int) bool {
		return out[i].TransferID < out[j].TransferID
	})

	return out
}

// GetFileTransferResult returns the transfer record identified by connectorID
// and transferID, or nil if no such transfer exists. ListFileTransferResults
// (the real op) is scoped to exactly one transfer -- both ConnectorId and
// TransferId are required members -- unlike ListFileFileTransferResults above,
// which lists every transfer for a connector for internal/persistence use.
func (b *InMemoryBackend) GetFileTransferResult(connectorID, transferID string) *FileTransferResult {
	b.mu.RLock("GetFileTransferResult")
	defer b.mu.RUnlock()

	r, ok := b.transferRecords.Get(transferID)
	if !ok || r.ConnectorID != connectorID {
		return nil
	}

	cp := *r

	return &cp
}

// StartAsyncOperationRecord persists an async operation record and returns the operationID.
func (b *InMemoryBackend) StartAsyncOperationRecord(connectorID, opType string) string {
	b.mu.Lock("StartAsyncOperationRecord")
	defer b.mu.Unlock()

	opID := uuid.NewString()

	b.asyncOperations.Put(&AsyncOperationRecord{
		ID:          opID,
		ConnectorID: connectorID,
		Status:      "QUEUED",
		Type:        opType,
		CreatedAt:   time.Now(),
	})

	return opID
}

// AddConnectorInternal seeds a connector for testing purposes.
func (b *InMemoryBackend) AddConnectorInternal(connectorID, url string) {
	b.mu.Lock("AddConnectorInternal")
	defer b.mu.Unlock()

	b.connectors.Put(&Connector{
		ConnectorID: connectorID,
		URL:         url,
		CreatedAt:   time.Now(),
		Tags:        make(map[string]string),
		AccountID:   b.accountID,
		Region:      b.region,
	})
}
