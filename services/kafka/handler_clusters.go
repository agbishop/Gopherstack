package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"net/http"
	"strings"

	"github.com/labstack/echo/v5"
)

// createClusterInput is CreateClusterInput (api_op_CreateCluster.go,
// kafka@v1.57.2). ConfigurationInfo/EncryptionInfo/EnhancedMonitoring/
// LoggingInfo/OpenMonitoring/Rebalancing/StorageMode are real, wire-confirmed
// members (awsRestjson1_serializeOpDocumentCreateClusterInput) that were
// previously not even parsed here, so any caller-supplied value was silently
// dropped at creation time.
type createClusterInput struct {
	Tags                 map[string]string     `json:"tags,omitempty"`
	ClientAuthentication *ClientAuthentication `json:"clientAuthentication,omitempty"`
	ConfigurationInfo    *ConfigurationInfo    `json:"configurationInfo,omitempty"`
	EncryptionInfo       *EncryptionInfo       `json:"encryptionInfo,omitempty"`
	LoggingInfo          *LoggingInfo          `json:"loggingInfo,omitempty"`
	OpenMonitoring       *OpenMonitoring       `json:"openMonitoring,omitempty"`
	Rebalancing          *Rebalancing          `json:"rebalancing,omitempty"`
	ClusterName          string                `json:"clusterName"`
	KafkaVersion         string                `json:"kafkaVersion"`
	EnhancedMonitoring   string                `json:"enhancedMonitoring,omitempty"`
	StorageMode          string                `json:"storageMode,omitempty"`
	BrokerNodeGroupInfo  BrokerNodeGroupInfo   `json:"brokerNodeGroupInfo"`
	NumberOfBrokerNodes  int32                 `json:"numberOfBrokerNodes"`
}

func (in *createClusterInput) options() ClusterCreateOptions {
	return ClusterCreateOptions{
		ConfigurationInfo:  in.ConfigurationInfo,
		EncryptionInfo:     in.EncryptionInfo,
		LoggingInfo:        in.LoggingInfo,
		OpenMonitoring:     in.OpenMonitoring,
		Rebalancing:        in.Rebalancing,
		EnhancedMonitoring: in.EnhancedMonitoring,
		StorageMode:        in.StorageMode,
	}
}

type createClusterOutput struct {
	ClusterArn  string `json:"clusterArn"`
	ClusterName string `json:"clusterName"`
	State       string `json:"state"`
}

// brokerSoftwareInfo represents the current broker software information.
// Field-diffed against deserializers.go's
// awsRestjson1_deserializeDocumentBrokerSoftwareInfo: types.BrokerSoftwareInfo
// has 3 real members, not 1 -- ConfigurationArn/ConfigurationRevision were
// previously dropped even though the cluster's ConfigurationInfo (the same
// data) is already tracked.
type brokerSoftwareInfo struct {
	ConfigurationArn      string `json:"configurationArn,omitempty"`
	KafkaVersion          string `json:"kafkaVersion"`
	ConfigurationRevision int64  `json:"configurationRevision,omitempty"`
}

// clusterInfoV1 is the V1 cluster response shape (DescribeCluster / ListClusters).
//
// Field-diffed against kafka@v1.57.2 deserializers.go's
// awsRestjson1_deserializeDocumentClusterInfo switch: the real types.ClusterInfo
// has no top-level KafkaVersion or ConfigurationInfo member (KafkaVersion only
// appears nested under CurrentBrokerSoftwareInfo; ConfigurationInfo is a
// MutableClusterInfo/ClusterOperation-only field, not emitted here by AWS) --
// both were previously fabricated on this DTO and are not modeled at all.
type clusterInfoV1 struct {
	Tags                      map[string]string     `json:"tags,omitempty"`
	CurrentBrokerSoftwareInfo *brokerSoftwareInfo   `json:"currentBrokerSoftwareInfo,omitempty"`
	ClientAuthentication      *ClientAuthentication `json:"clientAuthentication,omitempty"`
	EncryptionInfo            *EncryptionInfo       `json:"encryptionInfo,omitempty"`
	OpenMonitoring            *OpenMonitoring       `json:"openMonitoring,omitempty"`
	LoggingInfo               *LoggingInfo          `json:"loggingInfo,omitempty"`
	StateInfo                 *StateInfo            `json:"stateInfo,omitempty"`
	Rebalancing               *Rebalancing          `json:"rebalancing,omitempty"`
	ClusterArn                string                `json:"clusterArn"`
	ClusterName               string                `json:"clusterName"`
	State                     string                `json:"state"`
	CurrentVersion            string                `json:"currentVersion"`
	ActiveOperationArn        string                `json:"activeOperationArn,omitempty"`
	EnhancedMonitoring        string                `json:"enhancedMonitoring,omitempty"`
	StorageMode               string                `json:"storageMode,omitempty"`
	CreationTime              string                `json:"creationTime,omitempty"`
	ZookeeperConnectString    string                `json:"zookeeperConnectString,omitempty"`
	ZookeeperConnectStringTLS string                `json:"zookeeperConnectStringTls,omitempty"`
	BrokerNodeGroupInfo       BrokerNodeGroupInfo   `json:"brokerNodeGroupInfo"`
	NumberOfBrokerNodes       int32                 `json:"numberOfBrokerNodes"`
}

type describeClusterOutput struct {
	ClusterInfo *clusterInfoV1 `json:"clusterInfo"`
}

type listClustersOutput struct {
	NextToken       string           `json:"nextToken,omitempty"`
	ClusterInfoList []*clusterInfoV1 `json:"clusterInfoList"`
}

// provisionedClusterInfo is the V2 "provisioned" arm (types.Provisioned).
// Field-diffed against deserializers.go's
// awsRestjson1_deserializeDocumentProvisioned switch: unlike ClusterInfo(V1),
// the real type has no State member at all (State lives one level up, on the
// V2 response's top-level Cluster) and, like V1, no KafkaVersion or
// ConfigurationInfo member -- all three were previously fabricated here.
type provisionedClusterInfo struct {
	CurrentBrokerSoftwareInfo *brokerSoftwareInfo   `json:"currentBrokerSoftwareInfo,omitempty"`
	ClientAuthentication      *ClientAuthentication `json:"clientAuthentication,omitempty"`
	EncryptionInfo            *EncryptionInfo       `json:"encryptionInfo,omitempty"`
	OpenMonitoring            *OpenMonitoring       `json:"openMonitoring,omitempty"`
	LoggingInfo               *LoggingInfo          `json:"loggingInfo,omitempty"`
	Rebalancing               *Rebalancing          `json:"rebalancing,omitempty"`
	EnhancedMonitoring        string                `json:"enhancedMonitoring,omitempty"`
	StorageMode               string                `json:"storageMode,omitempty"`
	ZookeeperConnectString    string                `json:"zookeeperConnectString,omitempty"`
	ZookeeperConnectStringTLS string                `json:"zookeeperConnectStringTls,omitempty"`
	BrokerNodeGroupInfo       BrokerNodeGroupInfo   `json:"brokerNodeGroupInfo"`
	NumberOfBrokerNodes       int32                 `json:"numberOfBrokerNodes"`
}

// clusterInfoV2 is the real top-level types.Cluster (DescribeClusterV2 /
// ListClustersV2). Field-diffed against
// awsRestjson1_deserializeDocumentCluster: ActiveOperationArn/CreationTime/
// StateInfo are real members this DTO previously dropped even though the
// backend already tracks all three (see toClusterInfoV1, which already
// emitted them correctly on the V1 sibling).
type clusterInfoV2 struct {
	Tags               map[string]string       `json:"tags,omitempty"`
	Provisioned        *provisionedClusterInfo `json:"provisioned,omitempty"`
	Serverless         *ServerlessClusterInfo  `json:"serverless,omitempty"`
	StateInfo          *StateInfo              `json:"stateInfo,omitempty"`
	ClusterArn         string                  `json:"clusterArn"`
	ClusterName        string                  `json:"clusterName"`
	ClusterType        string                  `json:"clusterType"`
	State              string                  `json:"state"`
	CurrentVersion     string                  `json:"currentVersion,omitempty"`
	ActiveOperationArn string                  `json:"activeOperationArn,omitempty"`
	CreationTime       string                  `json:"creationTime,omitempty"`
}

type describeClusterV2Output struct {
	ClusterInfo *clusterInfoV2 `json:"clusterInfo"`
}

type listClustersV2Output struct {
	NextToken       string           `json:"nextToken,omitempty"`
	ClusterInfoList []*clusterInfoV2 `json:"clusterInfoList"`
}

// getBootstrapBrokersOutput mirrors GetBootstrapBrokersOutput. Field-diffed
// against aws-sdk-go-v2/service/kafka@v1.49.0's deserializers.go switch on
// awsRestjson1_deserializeOpDocumentGetBootstrapBrokersOutput: three of the
// "Public"-suffixed field names were wrong (real MSK puts "Public" BEFORE the
// auth-method suffix, e.g. "bootstrapBrokerStringPublicTls" not
// "...TlsPublic") and VpcConnectivityTLS had the wrong casing
// ("...VpcConnectivityTls", lowercase "ls") -- a real SDK client's JSON
// unmarshal silently drops any key it doesn't recognize (see the
// deserializer's `default: _, _ = key, value` case), so all four fields were
// unreachable by a real aws-sdk-go-v2 client before this fix.
type getBootstrapBrokersOutput struct {
	BootstrapBrokerString                     string `json:"bootstrapBrokerString,omitempty"`
	BootstrapBrokerStringTLS                  string `json:"bootstrapBrokerStringTls,omitempty"`
	BootstrapBrokerStringSaslScram            string `json:"bootstrapBrokerStringSaslScram,omitempty"`
	BootstrapBrokerStringSaslIam              string `json:"bootstrapBrokerStringSaslIam,omitempty"`
	BootstrapBrokerStringTLSPublic            string `json:"bootstrapBrokerStringPublicTls,omitempty"`
	BootstrapBrokerStringSaslScramPublic      string `json:"bootstrapBrokerStringPublicSaslScram,omitempty"`
	BootstrapBrokerStringSaslIamPublic        string `json:"bootstrapBrokerStringPublicSaslIam,omitempty"`
	BootstrapBrokerStringVpcConnectivityTLS   string `json:"bootstrapBrokerStringVpcConnectivityTls,omitempty"`
	BootstrapBrokerStringVpcConnectivityScram string `json:"bootstrapBrokerStringVpcConnectivitySaslScram,omitempty"`
	BootstrapBrokerStringVpcConnectivityIam   string `json:"bootstrapBrokerStringVpcConnectivitySaslIam,omitempty"`
}

type serverlessVpcConfigInput struct {
	SubnetIDs        []string `json:"subnetIds,omitempty"`
	SecurityGroupIDs []string `json:"securityGroupIds,omitempty"`
}

type serverlessAuthInput struct {
	Sasl *SaslSettings `json:"sasl,omitempty"`
}

type serverlessInput struct {
	ClientAuthentication *serverlessAuthInput       `json:"clientAuthentication,omitempty"`
	VpcConfigs           []serverlessVpcConfigInput `json:"vpcConfigs,omitempty"`
}

type createClusterV2Input struct {
	Tags        map[string]string `json:"tags,omitempty"`
	Provisioned *provisionedInput `json:"provisioned,omitempty"`
	Serverless  *serverlessInput  `json:"serverless,omitempty"`
	ClusterName string            `json:"clusterName"`
}

// provisionedInput is types.ProvisionedRequest (types.go:1362). Same
// previously-dropped-members bug as createClusterInput above.
type provisionedInput struct {
	ClientAuthentication *ClientAuthentication `json:"clientAuthentication,omitempty"`
	ConfigurationInfo    *ConfigurationInfo    `json:"configurationInfo,omitempty"`
	EncryptionInfo       *EncryptionInfo       `json:"encryptionInfo,omitempty"`
	LoggingInfo          *LoggingInfo          `json:"loggingInfo,omitempty"`
	OpenMonitoring       *OpenMonitoring       `json:"openMonitoring,omitempty"`
	Rebalancing          *Rebalancing          `json:"rebalancing,omitempty"`
	KafkaVersion         string                `json:"kafkaVersion"`
	EnhancedMonitoring   string                `json:"enhancedMonitoring,omitempty"`
	StorageMode          string                `json:"storageMode,omitempty"`
	BrokerNodeGroupInfo  BrokerNodeGroupInfo   `json:"brokerNodeGroupInfo"`
	NumberOfBrokerNodes  int32                 `json:"numberOfBrokerNodes"`
}

func (in *provisionedInput) options() ClusterCreateOptions {
	return ClusterCreateOptions{
		ConfigurationInfo:  in.ConfigurationInfo,
		EncryptionInfo:     in.EncryptionInfo,
		LoggingInfo:        in.LoggingInfo,
		OpenMonitoring:     in.OpenMonitoring,
		Rebalancing:        in.Rebalancing,
		EnhancedMonitoring: in.EnhancedMonitoring,
		StorageMode:        in.StorageMode,
	}
}

type createClusterV2Output struct {
	ClusterArn  string `json:"clusterArn"`
	ClusterName string `json:"clusterName"`
	ClusterType string `json:"clusterType"`
}

func (h *Handler) handleCreateCluster(ctx context.Context, c *echo.Context, body []byte) error {
	var in createClusterInput
	if err := json.Unmarshal(body, &in); err != nil {
		return h.writeError(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"invalid request body: "+err.Error(),
		)
	}

	cluster, err := h.Backend.CreateCluster(ctx,
		in.ClusterName,
		in.KafkaVersion,
		in.NumberOfBrokerNodes,
		in.BrokerNodeGroupInfo,
		in.ClientAuthentication,
		in.Tags,
		in.options(),
	)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, createClusterOutput{
		ClusterArn:  cluster.ClusterArn,
		ClusterName: cluster.ClusterName,
		State:       cluster.State,
	})
}

func (h *Handler) handleCreateClusterV2(ctx context.Context, c *echo.Context, body []byte) error {
	var in createClusterV2Input
	if err := json.Unmarshal(body, &in); err != nil {
		return h.writeError(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"invalid request body: "+err.Error(),
		)
	}

	if in.Provisioned != nil && in.Serverless != nil {
		return h.writeError(
			c,
			http.StatusBadRequest,
			"BadRequestException",
			"only one of provisioned or serverless may be specified",
		)
	}

	if in.Serverless != nil {
		srv := in.Serverless
		serverlessInfo := &ServerlessClusterInfo{}
		if srv.ClientAuthentication != nil {
			serverlessInfo.ClientAuthentication = &ServerlessClientAuthentication{
				Sasl: srv.ClientAuthentication.Sasl,
			}
		}
		for _, vc := range srv.VpcConfigs {
			serverlessInfo.VpcConfigs = append(serverlessInfo.VpcConfigs, ServerlessVpcConfig(vc))
		}

		cluster, err := h.Backend.CreateServerlessCluster(
			ctx,
			in.ClusterName,
			serverlessInfo,
			in.Tags,
		)
		if err != nil {
			return h.writeBackendError(c, err)
		}

		return c.JSON(http.StatusOK, createClusterV2Output{
			ClusterArn:  cluster.ClusterArn,
			ClusterName: cluster.ClusterName,
			ClusterType: ClusterTypeServerless,
		})
	}

	var brokerInfo BrokerNodeGroupInfo

	var kafkaVersion string

	var numBrokers int32

	var clientAuth *ClientAuthentication

	var createOpts ClusterCreateOptions

	if in.Provisioned != nil {
		brokerInfo = in.Provisioned.BrokerNodeGroupInfo
		kafkaVersion = in.Provisioned.KafkaVersion
		numBrokers = in.Provisioned.NumberOfBrokerNodes
		clientAuth = in.Provisioned.ClientAuthentication
		createOpts = in.Provisioned.options()
	}

	cluster, err := h.Backend.CreateCluster(ctx,
		in.ClusterName,
		kafkaVersion,
		numBrokers,
		brokerInfo,
		clientAuth,
		in.Tags,
		createOpts,
	)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, createClusterV2Output{
		ClusterArn:  cluster.ClusterArn,
		ClusterName: cluster.ClusterName,
		ClusterType: ClusterTypeProvisioned,
	})
}

func (h *Handler) handleListClusters(ctx context.Context, c *echo.Context) error {
	clusters := h.Backend.ListClusters(ctx)
	nameFilter := c.Request().URL.Query().Get("clusterNameFilter")
	all := make([]*clusterInfoV1, 0, len(clusters))

	for _, cl := range clusters {
		if nameFilter != "" && !strings.HasPrefix(cl.ClusterName, nameFilter) {
			continue
		}

		all = append(all, toClusterInfoV1(cl))
	}

	token := c.Request().URL.Query().Get("nextToken")
	offset := decodeKafkaPageToken(token)

	offset = min(offset, len(all))

	page := all[offset:]
	pageSize := kafkaPageSize(c)

	var nextToken string

	if len(page) > pageSize {
		page = page[:pageSize]
		nextToken = encodeKafkaPageToken(offset + pageSize)
	}

	return c.JSON(http.StatusOK, listClustersOutput{ClusterInfoList: page, NextToken: nextToken})
}

func (h *Handler) handleListClustersV2(ctx context.Context, c *echo.Context) error {
	clusters := h.Backend.ListClusters(ctx)
	query := c.Request().URL.Query()
	nameFilter := query.Get("clusterNameFilter")
	typeFilter := query.Get("clusterTypeFilter")
	all := make([]*clusterInfoV2, 0, len(clusters))

	for _, cl := range clusters {
		if nameFilter != "" && !strings.HasPrefix(cl.ClusterName, nameFilter) {
			continue
		}

		if typeFilter != "" && !strings.EqualFold(cl.ClusterType, typeFilter) {
			continue
		}

		all = append(all, toClusterInfoV2(cl))
	}

	token := c.Request().URL.Query().Get("nextToken")
	offset := decodeKafkaPageToken(token)

	offset = min(offset, len(all))

	page := all[offset:]
	pageSize := kafkaPageSize(c)

	var nextToken string

	if len(page) > pageSize {
		page = page[:pageSize]
		nextToken = encodeKafkaPageToken(offset + pageSize)
	}

	return c.JSON(http.StatusOK, listClustersV2Output{ClusterInfoList: page, NextToken: nextToken})
}

func (h *Handler) handleDescribeCluster(
	ctx context.Context,
	c *echo.Context,
	clusterArn string,
) error {
	cluster, err := h.Backend.DescribeCluster(ctx, clusterArn)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, describeClusterOutput{ClusterInfo: toClusterInfoV1(cluster)})
}

func (h *Handler) handleDescribeClusterV2(
	ctx context.Context,
	c *echo.Context,
	clusterArn string,
) error {
	cluster, err := h.Backend.DescribeCluster(ctx, clusterArn)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, describeClusterV2Output{ClusterInfo: toClusterInfoV2(cluster)})
}

func (h *Handler) handleDeleteCluster(
	ctx context.Context,
	c *echo.Context,
	clusterArn string,
) error {
	if err := h.Backend.DeleteCluster(ctx, clusterArn); err != nil {
		return h.writeBackendError(c, err)
	}

	return c.NoContent(http.StatusOK)
}

func (h *Handler) handleGetBootstrapBrokers(
	ctx context.Context,
	c *echo.Context,
	clusterArn string,
) error {
	cluster, err := h.Backend.DescribeCluster(ctx, clusterArn)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, bootstrapBrokersFor(cluster))
}

// bootstrapBrokersFor builds the bootstrap broker response based on cluster auth settings.
func bootstrapBrokersFor(cl *Cluster) getBootstrapBrokersOutput {
	out := getBootstrapBrokersOutput{
		BootstrapBrokerString:    "localhost:9092",
		BootstrapBrokerStringTLS: "localhost:9094",
	}

	auth := cl.ClientAuthentication
	if auth == nil {
		return out
	}

	addSaslBrokers(auth.Sasl, &out)

	ci := cl.BrokerNodeGroupInfo.ConnectivityInfo
	if ci == nil {
		return out
	}

	addPublicBrokers(auth, ci.PublicAccess, &out)
	addVpcConnectivityBrokers(ci.VpcConnectivity, &out)

	return out
}

// addSaslBrokers populates SASL broker endpoints when SASL authentication is configured.
func addSaslBrokers(sasl *SaslSettings, out *getBootstrapBrokersOutput) {
	if sasl == nil {
		return
	}

	if sasl.Scram != nil && sasl.Scram.Enabled {
		out.BootstrapBrokerStringSaslScram = "localhost:9096"
	}

	if sasl.Iam != nil && sasl.Iam.Enabled {
		out.BootstrapBrokerStringSaslIam = "localhost:9098"
	}
}

// addPublicBrokers populates public access broker endpoints when public access is enabled.
func addPublicBrokers(
	auth *ClientAuthentication,
	pa *PublicAccess,
	out *getBootstrapBrokersOutput,
) {
	if pa == nil || pa.Type != "SERVICE_PROVIDED_EIPS" {
		return
	}

	out.BootstrapBrokerStringTLSPublic = "localhost:9194"

	if auth.Sasl == nil {
		return
	}

	if auth.Sasl.Scram != nil && auth.Sasl.Scram.Enabled {
		out.BootstrapBrokerStringSaslScramPublic = "localhost:9196"
	}

	if auth.Sasl.Iam != nil && auth.Sasl.Iam.Enabled {
		out.BootstrapBrokerStringSaslIamPublic = "localhost:9198"
	}
}

// addVpcConnectivityBrokers populates VPC connectivity broker endpoints.
func addVpcConnectivityBrokers(vc *VpcConnectivity, out *getBootstrapBrokersOutput) {
	if vc == nil || vc.ClientAuthentication == nil {
		return
	}

	vca := vc.ClientAuthentication

	if vca.TLS != nil && vca.TLS.Enabled {
		out.BootstrapBrokerStringVpcConnectivityTLS = "localhost:9294"
	}

	if vca.Sasl == nil {
		return
	}

	if vca.Sasl.Scram != nil && vca.Sasl.Scram.Enabled {
		out.BootstrapBrokerStringVpcConnectivityScram = "localhost:9296"
	}

	if vca.Sasl.Iam != nil && vca.Sasl.Iam.Enabled {
		out.BootstrapBrokerStringVpcConnectivityIam = "localhost:9298"
	}
}

// brokerSoftwareInfoFor returns a brokerSoftwareInfo for the given Kafka
// version and the cluster's currently-applied configuration, or nil if the
// version is empty. configInfo mirrors ConfigurationArn/ConfigurationRevision
// -- previously dropped even though the cluster already tracks it.
func brokerSoftwareInfoFor(kafkaVersion string, configInfo *ConfigurationInfo) *brokerSoftwareInfo {
	if kafkaVersion == "" {
		return nil
	}

	info := &brokerSoftwareInfo{KafkaVersion: kafkaVersion}
	if configInfo != nil {
		info.ConfigurationArn = configInfo.Arn
		info.ConfigurationRevision = configInfo.Revision
	}

	return info
}

// toClusterInfoV1 converts a Cluster to the V1 cluster info shape.
func toClusterInfoV1(cl *Cluster) *clusterInfoV1 {
	return &clusterInfoV1{
		ClusterArn:                cl.ClusterArn,
		ClusterName:               cl.ClusterName,
		State:                     cl.State,
		CurrentVersion:            cl.CurrentVersion,
		BrokerNodeGroupInfo:       cl.BrokerNodeGroupInfo,
		NumberOfBrokerNodes:       cl.NumberOfBrokerNodes,
		ClientAuthentication:      cl.ClientAuthentication,
		EncryptionInfo:            cl.EncryptionInfo,
		OpenMonitoring:            cl.OpenMonitoring,
		LoggingInfo:               cl.LoggingInfo,
		StateInfo:                 cl.StateInfo,
		ActiveOperationArn:        cl.ActiveOperationArn,
		EnhancedMonitoring:        cl.EnhancedMonitoring,
		StorageMode:               cl.StorageMode,
		CreationTime:              cl.CreationTime,
		ZookeeperConnectString:    zookeeperConnectStringFor(cl.ClusterArn, zkPortPlaintext),
		ZookeeperConnectStringTLS: zookeeperConnectStringFor(cl.ClusterArn, zkPortTLS),
		Tags:                      maps.Clone(cl.Tags),
		CurrentBrokerSoftwareInfo: brokerSoftwareInfoFor(cl.KafkaVersion, cl.ConfigurationInfo),
		Rebalancing:               cl.Rebalancing,
	}
}

// ZooKeeper connect string ports: 2181 is the standard plaintext port, 2182
// the TLS port, per MSK's documented ZooKeeper endpoint convention.
const (
	zkPortPlaintext = 2181
	zkPortTLS       = 2182
)

// zookeeperConnectStringFor synthesises a ZooKeeper connect string from the
// cluster ARN. This mirrors the legacy ZooKeeper endpoint format used by
// older MSK clusters; there is no real per-broker ZooKeeper state in this
// in-memory emulator to draw the value from instead.
func zookeeperConnectStringFor(clusterArn string, port int) string {
	if clusterArn == "" {
		return ""
	}

	// Synthesise a deterministic-looking ZK endpoint from the ARN suffix.
	// Format: z-1.<cluster-id>.kafka.<region>.amazonaws.com:<port>,...
	clusterID, region := parseClusterIDAndRegion(clusterArn)

	return fmt.Sprintf(
		"z-1.%s.kafka.%s.amazonaws.com:%d,"+
			"z-2.%s.kafka.%s.amazonaws.com:%d,"+
			"z-3.%s.kafka.%s.amazonaws.com:%d",
		clusterID, region, port,
		clusterID, region, port,
		clusterID, region, port,
	)
}

// parseClusterIDAndRegion extracts the cluster ID and region from an ARN.
func parseClusterIDAndRegion(clusterArn string) (string, string) {
	const (
		minARNSlashParts  = 2
		arnColonFields    = 6
		arnRegionPosition = 4
	)

	clusterID := "unknown"
	region := "us-east-1"

	parts := strings.Split(clusterArn, "/")
	if len(parts) >= minARNSlashParts {
		clusterID = parts[len(parts)-1]
	}

	arnParts := strings.SplitN(clusterArn, ":", arnColonFields)
	if len(arnParts) >= arnRegionPosition {
		region = arnParts[3]
	}

	return clusterID, region
}

// toClusterInfoV2 converts a Cluster to the V2 cluster info shape.
func toClusterInfoV2(cl *Cluster) *clusterInfoV2 {
	clusterType := cl.ClusterType
	if clusterType == "" {
		clusterType = ClusterTypeProvisioned
	}

	info := &clusterInfoV2{
		ClusterArn:         cl.ClusterArn,
		ClusterName:        cl.ClusterName,
		ClusterType:        clusterType,
		State:              cl.State,
		CurrentVersion:     cl.CurrentVersion,
		ActiveOperationArn: cl.ActiveOperationArn,
		CreationTime:       cl.CreationTime,
		StateInfo:          cl.StateInfo,
		Tags:               maps.Clone(cl.Tags),
	}

	if clusterType == ClusterTypeServerless {
		info.Serverless = cl.Serverless
	} else {
		info.Provisioned = &provisionedClusterInfo{
			BrokerNodeGroupInfo:       cl.BrokerNodeGroupInfo,
			NumberOfBrokerNodes:       cl.NumberOfBrokerNodes,
			ClientAuthentication:      cl.ClientAuthentication,
			EncryptionInfo:            cl.EncryptionInfo,
			OpenMonitoring:            cl.OpenMonitoring,
			LoggingInfo:               cl.LoggingInfo,
			EnhancedMonitoring:        cl.EnhancedMonitoring,
			StorageMode:               cl.StorageMode,
			CurrentBrokerSoftwareInfo: brokerSoftwareInfoFor(cl.KafkaVersion, cl.ConfigurationInfo),
			ZookeeperConnectString:    zookeeperConnectStringFor(cl.ClusterArn, zkPortPlaintext),
			ZookeeperConnectStringTLS: zookeeperConnectStringFor(cl.ClusterArn, zkPortTLS),
			Rebalancing:               cl.Rebalancing,
		}
	}

	return info
}
