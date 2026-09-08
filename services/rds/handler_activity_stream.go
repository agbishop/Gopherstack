package rds

import (
	"encoding/xml"
	"net/url"
	"strings"
)

func (h *Handler) handleStartActivityStream(vals url.Values) (any, error) {
	resourceARN := vals.Get("ResourceArn")
	kmsKeyID := vals.Get("KmsKeyId")
	mode := vals.Get("Mode")

	// The resource ARN encodes the cluster identifier at the end
	clusterID := arnToClusterID(resourceARN)

	cluster, err := h.Backend.StartActivityStream(clusterID, kmsKeyID, mode)
	if err != nil {
		return nil, err
	}

	return startActivityStreamResponse{
		Xmlns:             rdsXMLNS,
		KinesisStreamName: cluster.ActivityStreamKinesisStreamName,
		KMSKeyID:          cluster.ActivityStreamKMSKeyID,
		Status:            cluster.ActivityStreamStatus,
		Mode:              cluster.ActivityStreamMode,
		ApplyImmediately:  true,
	}, nil
}

func (h *Handler) handleStopActivityStream(vals url.Values) (any, error) {
	resourceARN := vals.Get("ResourceArn")
	clusterID := arnToClusterID(resourceARN)

	cluster, err := h.Backend.StopActivityStream(clusterID)
	if err != nil {
		return nil, err
	}

	return stopActivityStreamResponse{
		Xmlns:             rdsXMLNS,
		KinesisStreamName: cluster.ActivityStreamKinesisStreamName,
		KMSKeyID:          cluster.ActivityStreamKMSKeyID,
		Status:            cluster.ActivityStreamStatus,
	}, nil
}

func (h *Handler) handleModifyActivityStream(vals url.Values) (any, error) {
	resourceARN := vals.Get("ResourceArn")
	auditPolicy := vals.Get("AuditPolicyState")
	// arnToClusterID's name is a misnomer here: this op's ARN is instance-shaped,
	// not cluster-shaped -- see InMemoryBackend.ModifyActivityStream's landmine
	// (gopherstack-mial: reconciling needs activity-stream state on DBInstance too).
	clusterID := arnToClusterID(resourceARN)

	cluster, err := h.Backend.ModifyActivityStream(clusterID, auditPolicy)
	if err != nil {
		return nil, err
	}

	return modifyActivityStreamResponse{
		Xmlns:             rdsXMLNS,
		KinesisStreamName: cluster.ActivityStreamKinesisStreamName,
		KMSKeyID:          cluster.ActivityStreamKMSKeyID,
		Mode:              cluster.ActivityStreamMode,
		Status:            cluster.ActivityStreamStatus,
		PolicyStatus:      cluster.ActivityStreamAuditPolicy,
	}, nil
}

// arnToClusterID extracts the cluster identifier from an ARN like
// "arn:aws:rds:us-east-1:123456789012:cluster:my-cluster".
func arnToClusterID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}

	return arn
}

type startActivityStreamResponse struct {
	XMLName           xml.Name `xml:"StartActivityStreamResponse"`
	Xmlns             string   `xml:"xmlns,attr"`
	KinesisStreamName string   `xml:"StartActivityStreamResult>KinesisStreamName,omitempty"`
	KMSKeyID          string   `xml:"StartActivityStreamResult>KmsKeyId,omitempty"`
	Status            string   `xml:"StartActivityStreamResult>Status,omitempty"`
	Mode              string   `xml:"StartActivityStreamResult>Mode,omitempty"`
	ApplyImmediately  bool     `xml:"StartActivityStreamResult>ApplyImmediately,omitempty"`
}

type stopActivityStreamResponse struct {
	XMLName           xml.Name `xml:"StopActivityStreamResponse"`
	Xmlns             string   `xml:"xmlns,attr"`
	KinesisStreamName string   `xml:"StopActivityStreamResult>KinesisStreamName,omitempty"`
	KMSKeyID          string   `xml:"StopActivityStreamResult>KmsKeyId,omitempty"`
	Status            string   `xml:"StopActivityStreamResult>Status,omitempty"`
}

// modifyActivityStreamResponse mirrors aws-sdk-go-v2's ModifyActivityStreamOutput,
// which has no "AuditPolicy" member — the real wire field for the audit
// policy's lock state is "PolicyStatus" (types.ActivityStreamPolicyStatus).
// A prior version of this response invented an "AuditPolicy" element that
// does not exist on the real SDK's output and omitted the real
// KinesisStreamName/Mode members.
type modifyActivityStreamResponse struct {
	XMLName           xml.Name `xml:"ModifyActivityStreamResponse"`
	Xmlns             string   `xml:"xmlns,attr"`
	KinesisStreamName string   `xml:"ModifyActivityStreamResult>KinesisStreamName,omitempty"`
	KMSKeyID          string   `xml:"ModifyActivityStreamResult>KmsKeyId,omitempty"`
	Mode              string   `xml:"ModifyActivityStreamResult>Mode,omitempty"`
	Status            string   `xml:"ModifyActivityStreamResult>Status,omitempty"`
	PolicyStatus      string   `xml:"ModifyActivityStreamResult>PolicyStatus,omitempty"`
}
