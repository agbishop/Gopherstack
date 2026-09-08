package codeconnections

import (
	"context"
	"sort"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

type createConnectionInput struct {
	ConnectionName string `json:"ConnectionName"`
	HostArn        string `json:"HostArn"`
	ProviderType   string `json:"ProviderType"`
	Tags           []tag  `json:"Tags"`
}

type createConnectionOutput struct {
	ConnectionArn string `json:"ConnectionArn"`
	Tags          []tag  `json:"Tags,omitempty"`
}

func (h *Handler) handleCreateConnection(
	ctx context.Context,
	in *createConnectionInput,
) (*createConnectionOutput, error) {
	conn, err := h.Backend.CreateConnection(
		ctx,
		in.ConnectionName,
		in.ProviderType,
		in.HostArn,
		tagsFromArray(in.Tags),
	)
	if err != nil {
		return nil, err
	}

	return &createConnectionOutput{
		ConnectionArn: conn.ConnectionArn,
		Tags:          tagsToSortedArray(conn.Tags),
	}, nil
}

type getConnectionInput struct {
	ConnectionArn string `json:"ConnectionArn"`
}

// connectionItem is the wire shape of the real Connection type
// (aws-sdk-go-v2/service/codeconnections@v1.13.4 types.Connection), used by
// both GetConnection (nested under "Connection") and ListConnections (as
// array items). Connection has no Tags member at all (confirmed against
// awsAwsjson10_deserializeDocumentConnection's case switch) -- Tags is only
// ever returned by CreateConnectionOutput.
type connectionItem struct {
	ConnectionName   string `json:"ConnectionName"`
	ConnectionArn    string `json:"ConnectionArn"`
	HostArn          string `json:"HostArn,omitempty"`
	OwnerAccountID   string `json:"OwnerAccountId"`
	ProviderType     string `json:"ProviderType"`
	ConnectionStatus string `json:"ConnectionStatus"`
}

type getConnectionOutput struct {
	Connection connectionItem `json:"Connection"`
}

func (h *Handler) handleGetConnection(
	ctx context.Context,
	in *getConnectionInput,
) (*getConnectionOutput, error) {
	conn, err := h.Backend.GetConnection(ctx, in.ConnectionArn)
	if err != nil {
		return nil, err
	}

	return &getConnectionOutput{Connection: connToItem(conn)}, nil
}

type listConnectionsInput struct {
	NextToken          *string `json:"NextToken"`
	MaxResults         *int32  `json:"MaxResults"`
	HostArnFilter      string  `json:"HostArnFilter"`
	ProviderTypeFilter string  `json:"ProviderTypeFilter"`
}

type listConnectionsOutput struct {
	NextToken   *string          `json:"NextToken,omitempty"`
	Connections []connectionItem `json:"Connections"`
}

func (h *Handler) handleListConnections(
	ctx context.Context,
	in *listConnectionsInput,
) (*listConnectionsOutput, error) {
	conns := h.Backend.ListConnections(ctx, in.ProviderTypeFilter, in.HostArnFilter)

	// Sort for stable pagination. ConnectionName is not unique (CreateConnection
	// has no ResourceAlreadyExistsException for a duplicate name, see
	// connections.go), so ConnectionArn (always unique) breaks ties -- without
	// it, two connections sharing a name have no total order between them.
	sort.Slice(conns, func(i, j int) bool {
		if conns[i].ConnectionName != conns[j].ConnectionName {
			return conns[i].ConnectionName < conns[j].ConnectionName
		}

		return conns[i].ConnectionArn < conns[j].ConnectionArn
	})

	all := make([]connectionItem, 0, len(conns))
	for _, conn := range conns {
		all = append(all, connToItem(conn))
	}

	var limit int
	if in.MaxResults != nil && *in.MaxResults > 0 {
		limit = int(*in.MaxResults)
	}

	token := ""
	if in.NextToken != nil {
		token = *in.NextToken
	}

	p := page.New(all, token, limit, ccDefaultPageSize)

	var nextToken *string
	if p.Next != "" {
		nextToken = &p.Next
	}

	return &listConnectionsOutput{Connections: p.Data, NextToken: nextToken}, nil
}

type deleteConnectionInput struct {
	ConnectionArn string `json:"ConnectionArn"`
}

func (h *Handler) handleDeleteConnection(
	ctx context.Context,
	in *deleteConnectionInput,
) (*emptyOutput, error) {
	if err := h.Backend.DeleteConnection(ctx, in.ConnectionArn); err != nil {
		return nil, err
	}

	return &emptyOutput{}, nil
}

func connToItem(conn *Connection) connectionItem {
	return connectionItem{
		ConnectionName:   conn.ConnectionName,
		ConnectionArn:    conn.ConnectionArn,
		ProviderType:     conn.ProviderType,
		ConnectionStatus: conn.Status,
		OwnerAccountID:   conn.OwnerAccountID,
		HostArn:          conn.HostArn,
	}
}
