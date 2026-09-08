package ecs

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

const (
	containerHealthStatusUnknown = "UNKNOWN"
	containerHealthStatusHealthy = "HEALTHY"
)

// buildContainerArn constructs a container ARN from a task ARN.
func buildContainerArn(taskArn string) string {
	return "arn:aws:ecs:" + strings.TrimPrefix(
		taskArn,
		"arn:aws:ecs:",
	) + "/container/" + uuid.NewString()
}

// buildNetworkBindingsForContainer converts a container definition's port mappings
// to NetworkBinding records, defaulting the protocol to "tcp" when unset.
func buildNetworkBindingsForContainer(cd ContainerDefinition) []NetworkBinding {
	if len(cd.PortMappings) == 0 {
		return nil
	}

	bindings := make([]NetworkBinding, 0, len(cd.PortMappings))

	for _, pm := range cd.PortMappings {
		if pm.ContainerPort == 0 {
			continue
		}

		proto := pm.Protocol
		if proto == "" {
			proto = transportTCP
		}

		hostPort := pm.HostPort
		if hostPort == 0 {
			hostPort = pm.ContainerPort
		}

		bindings = append(bindings, NetworkBinding{
			BindIP:        "0.0.0.0",
			ContainerPort: pm.ContainerPort,
			HostPort:      hostPort,
			Protocol:      proto,
		})
	}

	return bindings
}

// buildContainerFromDef builds a Container from a ContainerDefinition.
func buildContainerFromDef(taskArn string, initialStatus string, cd ContainerDefinition) Container {
	c := Container{
		ContainerArn: buildContainerArn(taskArn),
		TaskArn:      taskArn,
		Name:         cd.Name,
		Image:        cd.Image,
		LastStatus:   initialStatus,
	}

	if cd.CPU != 0 {
		c.CPU = strconv.Itoa(cd.CPU)
	}

	if cd.Memory != 0 {
		c.Memory = strconv.Itoa(cd.Memory)
	}

	if cd.MemoryReservation != 0 {
		c.MemoryReservation = strconv.Itoa(cd.MemoryReservation)
	}

	if cd.HealthCheck != nil {
		c.HealthStatus = containerHealthStatusUnknown
	}

	c.NetworkBindings = buildNetworkBindingsForContainer(cd)

	return c
}

// buildContainersForTask creates the initial Container slice from a task's
// ContainerDefinitions. Status matches the task's initial status.
func buildContainersForTask(task *Task, td *TaskDefinition) []Container {
	containers := make([]Container, 0, len(td.ContainerDefinitions))

	for _, cd := range td.ContainerDefinitions {
		containers = append(containers, buildContainerFromDef(task.TaskArn, task.LastStatus, cd))
	}

	return containers
}

// syncContainerStatuses updates Container.LastStatus to match the task's LastStatus.
// Called after RunTask completes (success or failure) to keep containers in sync.
func syncContainerStatuses(task *Task, exitCode *int) {
	for i := range task.Containers {
		task.Containers[i].LastStatus = task.LastStatus

		if task.LastStatus == statusStopped && exitCode != nil {
			task.Containers[i].ExitCode = exitCode
		}

		if task.LastStatus == statusRunning {
			task.Containers[i].RuntimeID = uuid.NewString()[:12]

			if task.Containers[i].HealthStatus == containerHealthStatusUnknown {
				task.Containers[i].HealthStatus = containerHealthStatusHealthy
			}
		}
	}
}

// setContainerExitCode records exitCode on the named container within task,
// if present. Used when a container's own exit is observed directly (see
// markTaskStoppedByContainerExit), unlike the StopTask path where no
// per-container exit code is known.
func setContainerExitCode(task *Task, containerName string, exitCode int) {
	for i := range task.Containers {
		if task.Containers[i].Name == containerName {
			task.Containers[i].ExitCode = &exitCode

			return
		}
	}
}

const (
	// eniIDLen is the length of a UUID-derived suffix used as ENI attachment IDs.
	eniIDLen = 36

	// eniSubnetSuffixLen is the number of task ARN chars used as the subnet ID suffix.
	eniSubnetSuffixLen = 8

	// ipOctetMod is the modulus applied to UUID bytes to form private IP octets.
	ipOctetMod = 256

	// ipPrivateClass is the first octet of simulated Fargate private IPs (10.x.y.z).
	ipPrivateClass = 10
)

// safeLastN returns the last n bytes of s, padding by repetition if shorter.
func safeLastN(s string, n int) string {
	if len(s) >= n {
		return s[len(s)-n:]
	}

	// pad by repeating until long enough
	for len(s) < n {
		s += s
	}

	return s[len(s)-n:]
}

// newFargateTaskAttachment builds a simulated Fargate ENI attachment for a task ARN.
// Each task gets a unique ENI ID, MAC address, and private IP derived from a random UUID
// so that callers can distinguish attachments across tasks.
func newFargateTaskAttachment(taskArn string) TaskAttachment {
	id := uuid.NewString()

	// Derive a stable but unique 12-hex-char "MAC" from the first 12 chars of the UUID
	// (no dashes). Format as colon-separated pairs.
	macRaw := id[:8] + id[9:13] // 12 hex chars from UUID without dashes
	mac := fmt.Sprintf("%s:%s:%s:%s:%s:%s",
		macRaw[0:2], macRaw[2:4], macRaw[4:6],
		macRaw[6:8], macRaw[8:10], macRaw[10:12],
	)

	// Derive a unique /16 private IP: 10.x.y.z where x.y come from the last two
	// bytes of the attachment UUID (gives 65536 distinct IPs before collision).
	ipSuffix := id[len(id)-9:] // last 9 chars of UUID: "xxxxxxxx-" → no, use raw hex
	_ = ipSuffix
	eniShort := id[:8] // 8-char prefix as eni ID suffix
	eniID := "eni-" + eniShort

	// Use the task ARN suffix as a stable subnet hint (all tasks in the same cluster
	// share a synthetic subnet derived from that cluster's tasks).
	subnetID := "subnet-" + safeLastN(taskArn, eniSubnetSuffixLen)

	// Build a unique private IP from the attachment UUID bytes.
	octet3 := int(id[0]) % ipOctetMod
	octet4 := int(id[1]) % ipOctetMod
	privateIP := fmt.Sprintf("%d.0.%d.%d", ipPrivateClass, octet3, octet4)
	privateDNS := fmt.Sprintf("ip-%d-0-%d-%d.ec2.internal", ipPrivateClass, octet3, octet4)

	return TaskAttachment{
		ID:     safeLastN(taskArn, eniIDLen),
		Type:   "ElasticNetworkInterface",
		Status: "ATTACHED",
		Details: []KeyValuePair{
			{Name: "subnetId", Value: subnetID},
			{Name: "networkInterfaceId", Value: eniID},
			{Name: "macAddress", Value: mac},
			{Name: "privateDnsName", Value: privateDNS},
			{Name: "privateIPv4Address", Value: privateIP},
		},
	}
}
