package kafka

import (
	"context"
	"fmt"
	"slices"
	"time"
)

// CreateCluster creates a new MSK cluster. opts is variadic so existing
// positional call sites (in-package tests, services/cloudformation's
// composed call) keep their arity; only the wire-shape fix needs the extra
// data.
func (b *InMemoryBackend) CreateCluster(
	ctx context.Context,
	name, kafkaVersion string,
	numBrokers int32,
	brokerInfo BrokerNodeGroupInfo,
	clientAuth *ClientAuthentication,
	tags map[string]string,
	opts ...ClusterCreateOptions,
) (*Cluster, error) {
	var createOpts ClusterCreateOptions
	if len(opts) > 0 {
		createOpts = opts[0]
	}

	if name == "" {
		return nil, fmt.Errorf("clusterName is required: %w", ErrValidation)
	}

	if numBrokers < 1 {
		return nil, fmt.Errorf("numberOfBrokerNodes must be at least 1: %w", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateCluster")
	defer b.mu.Unlock()

	if len(b.clustersByName.Get(region+"|"+name)) > 0 {
		return nil, ErrAlreadyExists
	}

	if regionClusters := b.clustersByRegion.Get(region); len(regionClusters) >= maxClustersPerRegion {
		victim := regionClusters[0]
		b.clusters.Delete(victim.ClusterArn)
	}

	clusterArn := b.clusterARN(region, name)
	safeInfo := BrokerNodeGroupInfo{
		BrokerAZDistribution: brokerInfo.BrokerAZDistribution,
		InstanceType:         brokerInfo.InstanceType,
		ClientSubnets:        append([]string(nil), brokerInfo.ClientSubnets...),
		SecurityGroups:       append([]string(nil), brokerInfo.SecurityGroups...),
		ZoneIDs:              append([]string(nil), brokerInfo.ZoneIDs...),
		StorageInfo:          brokerInfo.StorageInfo,
		ConnectivityInfo:     brokerInfo.ConnectivityInfo,
	}
	if brokerInfo.StorageInfo != nil {
		safeInfo.StorageInfo = cloneStorageInfo(brokerInfo.StorageInfo)
	}
	if brokerInfo.ConnectivityInfo != nil {
		safeInfo.ConnectivityInfo = cloneConnectivityInfo(brokerInfo.ConnectivityInfo)
	}
	cluster := &Cluster{
		ClusterArn:           clusterArn,
		ClusterName:          name,
		ClusterType:          ClusterTypeProvisioned,
		KafkaVersion:         kafkaVersion,
		NumberOfBrokerNodes:  numBrokers,
		BrokerNodeGroupInfo:  safeInfo,
		ClientAuthentication: cloneClientAuth(clientAuth),
		State:                ClusterStateCreating,
		CurrentVersion:       DefaultClusterVersion,
		Tags:                 nonNilTagsCopy(tags),
		CreationTime:         time.Now().UTC().Format(time.RFC3339),
		EncryptionInfo:       cloneEncryptionInfo(createOpts.EncryptionInfo),
		OpenMonitoring:       cloneOpenMonitoring(createOpts.OpenMonitoring),
		LoggingInfo:          cloneLoggingInfo(createOpts.LoggingInfo),
		Rebalancing:          cloneRebalancing(createOpts.Rebalancing),
		EnhancedMonitoring:   createOpts.EnhancedMonitoring,
		StorageMode:          createOpts.StorageMode,
	}
	if createOpts.ConfigurationInfo != nil {
		ci := *createOpts.ConfigurationInfo
		cluster.ConfigurationInfo = &ci
	}
	b.clusters.Put(cluster)

	return cloneCluster(cluster), nil
}

// CreateServerlessCluster creates a new MSK Serverless cluster.
func (b *InMemoryBackend) CreateServerlessCluster(
	ctx context.Context,
	name string,
	serverless *ServerlessClusterInfo,
	tags map[string]string,
) (*Cluster, error) {
	if name == "" {
		return nil, fmt.Errorf("clusterName is required: %w", ErrValidation)
	}

	region := getRegion(ctx, b.region)

	b.mu.Lock("CreateServerlessCluster")
	defer b.mu.Unlock()

	if len(b.clustersByName.Get(region+"|"+name)) > 0 {
		return nil, ErrAlreadyExists
	}

	if regionClusters := b.clustersByRegion.Get(region); len(regionClusters) >= maxClustersPerRegion {
		victim := regionClusters[0]
		b.clusters.Delete(victim.ClusterArn)
	}

	clusterArn := b.clusterARN(region, name)
	cluster := &Cluster{
		ClusterArn:     clusterArn,
		ClusterName:    name,
		ClusterType:    ClusterTypeServerless,
		State:          ClusterStateCreating,
		CurrentVersion: DefaultClusterVersion,
		Tags:           nonNilTagsCopy(tags),
		Serverless:     cloneServerless(serverless),
		CreationTime:   time.Now().UTC().Format(time.RFC3339),
	}
	b.clusters.Put(cluster)

	return cloneCluster(cluster), nil
}

// DescribeCluster retrieves a cluster by ARN, advancing CREATING→ACTIVE on first poll.
func (b *InMemoryBackend) DescribeCluster(_ context.Context, clusterArn string) (*Cluster, error) {
	b.mu.Lock("DescribeCluster")
	defer b.mu.Unlock()

	c, ok := b.clusters.Get(clusterArn)
	if !ok {
		return nil, ErrNotFound
	}

	if c.State == ClusterStateCreating {
		c.pollCount++
		if c.pollCount >= 1 {
			c.State = ClusterStateActive
		}
	}

	return cloneCluster(c), nil
}

// ListClusters returns all MSK clusters in the request's region sorted by name.
func (b *InMemoryBackend) ListClusters(ctx context.Context) []*Cluster {
	region := getRegion(ctx, b.region)

	b.mu.RLock("ListClusters")
	defer b.mu.RUnlock()

	clusters := b.clustersByRegion.Get(region)
	out := make([]*Cluster, 0, len(clusters))
	for _, c := range clusters {
		out = append(out, cloneCluster(c))
	}

	slices.SortFunc(out, func(a, b *Cluster) int {
		if a.ClusterName < b.ClusterName {
			return -1
		}
		if a.ClusterName > b.ClusterName {
			return 1
		}

		return 0
	})

	return out
}

// DeleteCluster deletes a cluster by ARN, cascading to its SCRAM secrets,
// topics, cluster policy, VPC connections and channels. VPC connections and
// channels previously survived cluster deletion as ghost rows: both are
// stored keyed by their own ARN (VpcConnectionArn / ChannelArn) with a
// TargetClusterArn/ClusterArn back-reference, but nothing removed them here,
// so DescribeVpcConnection/DescribeChannel and the global ListVpcConnections
// kept returning rows pointing at a deleted cluster.
func (b *InMemoryBackend) DeleteCluster(_ context.Context, clusterArn string) error {
	b.mu.Lock("DeleteCluster")
	defer b.mu.Unlock()

	if !b.clusters.Has(clusterArn) {
		return ErrNotFound
	}

	b.clusters.Delete(clusterArn)
	delete(b.scramSecrets, clusterArn)
	delete(b.clusterPolicies, clusterArn)

	// Remove all topics/VPC connections/channels belonging to this cluster.
	// Each index's slice is cloned before deleting from it, since Table.Delete
	// mutates the index's backing group in place and would otherwise corrupt
	// this loop's own iteration.
	for _, t := range slices.Clone(b.topicsByCluster.Get(clusterArn)) {
		b.topics.Delete(topicKey(t.ClusterArn, t.TopicName))
	}

	for _, v := range slices.Clone(b.vpcConnectionsByCluster.Get(clusterArn)) {
		b.vpcConnections.Delete(v.VpcConnectionArn)
	}

	for _, ch := range slices.Clone(b.channelsByCluster.Get(clusterArn)) {
		b.channels.Delete(ch.ChannelArn)
	}

	return nil
}

// AddClusterInternal creates a cluster directly for testing purposes.
func (b *InMemoryBackend) AddClusterInternal(name, kafkaVersion string) *Cluster {
	b.mu.Lock("AddClusterInternal")
	defer b.mu.Unlock()

	clusterArn := b.clusterARN(b.region, name)
	cluster := &Cluster{
		ClusterArn:          clusterArn,
		ClusterName:         name,
		ClusterType:         ClusterTypeProvisioned,
		KafkaVersion:        kafkaVersion,
		NumberOfBrokerNodes: defaultBrokerCount,
		State:               ClusterStateActive,
		CurrentVersion:      DefaultClusterVersion,
		Tags:                make(map[string]string),
		CreationTime:        time.Now().UTC().Format(time.RFC3339),
	}
	b.clusters.Put(cluster)

	return cloneCluster(cluster)
}

// cloneCluster creates a deep copy of a cluster.
func cloneCluster(c *Cluster) *Cluster {
	clone := &Cluster{
		ClusterArn:          c.ClusterArn,
		ClusterName:         c.ClusterName,
		ClusterType:         c.ClusterType,
		KafkaVersion:        c.KafkaVersion,
		State:               c.State,
		CurrentVersion:      c.CurrentVersion,
		ActiveOperationArn:  c.ActiveOperationArn,
		EnhancedMonitoring:  c.EnhancedMonitoring,
		StorageMode:         c.StorageMode,
		CreationTime:        c.CreationTime,
		NumberOfBrokerNodes: c.NumberOfBrokerNodes,
		Tags:                nonNilTagsCopy(c.Tags),
		BrokerNodeGroupInfo: BrokerNodeGroupInfo{
			BrokerAZDistribution: c.BrokerNodeGroupInfo.BrokerAZDistribution,
			InstanceType:         c.BrokerNodeGroupInfo.InstanceType,
			ClientSubnets:        append([]string(nil), c.BrokerNodeGroupInfo.ClientSubnets...),
			SecurityGroups:       append([]string(nil), c.BrokerNodeGroupInfo.SecurityGroups...),
			ZoneIDs:              append([]string(nil), c.BrokerNodeGroupInfo.ZoneIDs...),
		},
	}

	if c.BrokerNodeGroupInfo.StorageInfo != nil {
		clone.BrokerNodeGroupInfo.StorageInfo = cloneStorageInfo(c.BrokerNodeGroupInfo.StorageInfo)
	}

	if c.BrokerNodeGroupInfo.ConnectivityInfo != nil {
		clone.BrokerNodeGroupInfo.ConnectivityInfo = cloneConnectivityInfo(c.BrokerNodeGroupInfo.ConnectivityInfo)
	}

	clone.ClientAuthentication = cloneClientAuth(c.ClientAuthentication)
	clone.EncryptionInfo = cloneEncryptionInfo(c.EncryptionInfo)
	clone.OpenMonitoring = cloneOpenMonitoring(c.OpenMonitoring)
	clone.LoggingInfo = cloneLoggingInfo(c.LoggingInfo)
	clone.Serverless = cloneServerless(c.Serverless)
	clone.Rebalancing = cloneRebalancing(c.Rebalancing)

	if c.StateInfo != nil {
		si := *c.StateInfo
		clone.StateInfo = &si
	}

	if c.ConfigurationInfo != nil {
		ci := *c.ConfigurationInfo
		clone.ConfigurationInfo = &ci
	}

	return clone
}

// cloneStorageInfo deep-copies a StorageInfo.
func cloneStorageInfo(s *StorageInfo) *StorageInfo {
	if s == nil {
		return nil
	}

	clone := &StorageInfo{}
	if s.EbsStorageInfo != nil {
		ebs := &EBSStorageInfo{VolumeSize: s.EbsStorageInfo.VolumeSize}
		if s.EbsStorageInfo.ProvisionedThroughput != nil {
			pt := *s.EbsStorageInfo.ProvisionedThroughput
			ebs.ProvisionedThroughput = &pt
		}
		clone.EbsStorageInfo = ebs
	}

	return clone
}

// cloneConnectivityInfo deep-copies a ConnectivityInfo.
func cloneConnectivityInfo(ci *ConnectivityInfo) *ConnectivityInfo {
	if ci == nil {
		return nil
	}

	clone := &ConnectivityInfo{}
	if ci.PublicAccess != nil {
		pa := *ci.PublicAccess
		clone.PublicAccess = &pa
	}

	if ci.VpcConnectivity != nil {
		clone.VpcConnectivity = cloneVpcConnectivity(ci.VpcConnectivity)
	}

	return clone
}

// cloneVpcConnectivity deep-copies a VpcConnectivity.
func cloneVpcConnectivity(src *VpcConnectivity) *VpcConnectivity {
	vc := &VpcConnectivity{}

	if src.ClientAuthentication == nil {
		return vc
	}

	ca := &VpcConnectivityClientAuthentication{}
	srcCA := src.ClientAuthentication

	if srcCA.Sasl != nil {
		sasl := &VpcConnectivitySasl{}

		if srcCA.Sasl.Iam != nil {
			iam := *srcCA.Sasl.Iam
			sasl.Iam = &iam
		}

		if srcCA.Sasl.Scram != nil {
			scram := *srcCA.Sasl.Scram
			sasl.Scram = &scram
		}

		ca.Sasl = sasl
	}

	if srcCA.TLS != nil {
		tls := *srcCA.TLS
		ca.TLS = &tls
	}

	vc.ClientAuthentication = ca

	return vc
}

// cloneEncryptionInfo deep-copies an EncryptionInfo.
func cloneEncryptionInfo(ei *EncryptionInfo) *EncryptionInfo {
	if ei == nil {
		return nil
	}

	clone := &EncryptionInfo{}
	if ei.EncryptionAtRest != nil {
		ear := *ei.EncryptionAtRest
		clone.EncryptionAtRest = &ear
	}

	if ei.EncryptionInTransit != nil {
		eit := *ei.EncryptionInTransit
		clone.EncryptionInTransit = &eit
	}

	return clone
}

// cloneOpenMonitoring deep-copies an OpenMonitoring.
func cloneOpenMonitoring(om *OpenMonitoring) *OpenMonitoring {
	if om == nil {
		return nil
	}

	clone := &OpenMonitoring{}
	if om.Prometheus != nil {
		p := &PrometheusInfo{}
		if om.Prometheus.JmxExporter != nil {
			jmx := *om.Prometheus.JmxExporter
			p.JmxExporter = &jmx
		}
		if om.Prometheus.NodeExporter != nil {
			ne := *om.Prometheus.NodeExporter
			p.NodeExporter = &ne
		}
		clone.Prometheus = p
	}

	return clone
}

// cloneLoggingInfo deep-copies a LoggingInfo.
func cloneLoggingInfo(li *LoggingInfo) *LoggingInfo {
	if li == nil {
		return nil
	}

	clone := &LoggingInfo{}
	if li.BrokerLogs != nil {
		bl := &BrokerLogs{}
		if li.BrokerLogs.CloudWatchLogs != nil {
			cwl := *li.BrokerLogs.CloudWatchLogs
			bl.CloudWatchLogs = &cwl
		}
		if li.BrokerLogs.Firehose != nil {
			fh := *li.BrokerLogs.Firehose
			bl.Firehose = &fh
		}
		if li.BrokerLogs.S3 != nil {
			s3 := *li.BrokerLogs.S3
			bl.S3 = &s3
		}
		clone.BrokerLogs = bl
	}

	return clone
}

// cloneRebalancing copies a Rebalancing.
func cloneRebalancing(r *Rebalancing) *Rebalancing {
	if r == nil {
		return nil
	}

	clone := *r

	return &clone
}

// cloneServerless deep-copies a ServerlessClusterInfo.
func cloneServerless(s *ServerlessClusterInfo) *ServerlessClusterInfo {
	if s == nil {
		return nil
	}

	clone := &ServerlessClusterInfo{}
	if s.ClientAuthentication != nil {
		ca := &ServerlessClientAuthentication{}
		if s.ClientAuthentication.Sasl != nil {
			ca.Sasl = cloneSasl(s.ClientAuthentication.Sasl)
		}
		clone.ClientAuthentication = ca
	}

	if len(s.VpcConfigs) > 0 {
		clone.VpcConfigs = make([]ServerlessVpcConfig, len(s.VpcConfigs))
		for i, vc := range s.VpcConfigs {
			clone.VpcConfigs[i] = ServerlessVpcConfig{
				SubnetIDs:        append([]string(nil), vc.SubnetIDs...),
				SecurityGroupIDs: append([]string(nil), vc.SecurityGroupIDs...),
			}
		}
	}

	return clone
}

// cloneClientAuth deep-copies a ClientAuthentication value.
func cloneClientAuth(auth *ClientAuthentication) *ClientAuthentication {
	if auth == nil {
		return nil
	}

	authCopy := &ClientAuthentication{}

	if auth.Sasl != nil {
		authCopy.Sasl = cloneSasl(auth.Sasl)
	}

	if auth.TLS != nil {
		tlsCopy := TLSSettings{
			Enabled:                     auth.TLS.Enabled,
			CertificateAuthorityArnList: append([]string(nil), auth.TLS.CertificateAuthorityArnList...),
		}
		authCopy.TLS = &tlsCopy
	}

	if auth.Unauthenticated != nil {
		ua := *auth.Unauthenticated
		authCopy.Unauthenticated = &ua
	}

	return authCopy
}

// cloneSasl deep-copies a SaslSettings value.
func cloneSasl(s *SaslSettings) *SaslSettings {
	if s == nil {
		return nil
	}

	c := *s

	if s.Scram != nil {
		scramCopy := *s.Scram
		c.Scram = &scramCopy
	}

	if s.Iam != nil {
		iamCopy := *s.Iam
		c.Iam = &iamCopy
	}

	return &c
}
