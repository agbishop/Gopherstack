package rds

import (
	"fmt"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/lockmetrics"
	"github.com/blackbirdworks/gopherstack/pkgs/store"
)

// NewInMemoryBackend creates a new InMemoryBackend with a background reconciler.
func NewInMemoryBackend(accountID, region string) *InMemoryBackend {
	b := &InMemoryBackend{
		registry:                store.NewRegistry(),
		instanceReadyAt:         make(map[string]time.Time),
		tags:                    make(map[string][]Tag),
		clusterRoles:            make(map[string][]DBClusterRole),
		instanceRoles:           make(map[string]map[string]string),
		events:                  make([]Event, 0),
		fisFailoverFaults:       make(map[string]time.Time),
		proxyTargets:            make(map[string][]DBProxyTarget),
		automatedBackups:        make(map[string]*DBInstanceAutomatedBackup),
		snapshotTenantDatabases: make(map[string][]*DBSnapshotTenantDatabase),
		clusterReadyAt:          make(map[string]time.Time),
		piMetrics:               make(map[string]map[string][]PIDataPoint),
		instanceLogFiles:        make(map[string][]DBLogFile),
		instanceLogContent:      make(map[string]map[string]string),
		accountID:               accountID,
		region:                  region,
		defaultCACertificateID:  defaultCACertificateID,
		mu:                      lockmetrics.New("rds"),
	}
	registerAllTables(b)

	return b
}

// Close stops the background reconciler goroutine and waits for any in-flight
// delayed lifecycle transitions to finish. Close is safe to call more than once.
func (b *InMemoryBackend) Close() {
	// Reconciler is now ephemeral and stops on its own.
}

func (b *InMemoryBackend) scheduleReconcilerLocked() {
	if b.reconcilerRunning {
		return
	}
	b.reconcilerRunning = true
	go func() {
		defer func() {
			b.mu.Lock("reconcilerExit")
			b.reconcilerRunning = false
			b.mu.Unlock()
		}()
		ticker := time.NewTicker(instanceTransitionDelay / reconcilerDivisor)
		defer ticker.Stop()
		for {
			<-ticker.C
			b.mu.Lock("runReconciler")
			b.reconcileInstancesLocked()
			if len(b.instanceReadyAt) == 0 && len(b.clusterReadyAt) == 0 {
				b.mu.Unlock()

				return
			}
			b.mu.Unlock()
		}
	}()
}

// Region returns the AWS region this backend is configured for.
func (b *InMemoryBackend) Region() string { return b.region }

// AccountID returns the AWS account ID this backend is configured for.
func (b *InMemoryBackend) AccountID() string { return b.accountID }

// Reset clears all backend state, returning it to a clean empty state.
func (b *InMemoryBackend) Reset() {
	b.mu.Lock("Reset")
	defer b.mu.Unlock()

	b.registry.ResetAll()
	b.instanceReadyAt = make(map[string]time.Time)
	b.tags = make(map[string][]Tag)
	b.clusterRoles = make(map[string][]DBClusterRole)
	b.instanceRoles = make(map[string]map[string]string)
	b.events = make([]Event, 0)
	b.fisFailoverFaults = make(map[string]time.Time)
	b.proxyTargets = make(map[string][]DBProxyTarget)
	b.automatedBackups = make(map[string]*DBInstanceAutomatedBackup)
	b.snapshotTenantDatabases = make(map[string][]*DBSnapshotTenantDatabase)
	b.clusterReadyAt = make(map[string]time.Time)
	b.piMetrics = make(map[string]map[string][]PIDataPoint)
	b.instanceLogFiles = make(map[string][]DBLogFile)
	b.instanceLogContent = make(map[string]map[string]string)
	// defaultCACertificateID is an account setting (ModifyCertificates), not a
	// resource collection: its clean-state value is the const default, not "".
	b.defaultCACertificateID = defaultCACertificateID
}

// SetDNSRegistrar wires a DNS server so RDS instance hostnames are auto-registered.
func (b *InMemoryBackend) SetDNSRegistrar(dns DNSRegistrar) {
	b.mu.Lock("SetDNSRegistrar")
	defer b.mu.Unlock()

	b.dnsRegistrar = dns
}

// enginePort returns the default port for the given database engine.
func enginePort(engine string) int {
	switch engine {
	case engineMySQL, engineMariaDB, engineAuroraMySQL:
		return mysqlPort
	default:
		return defaultPort
	}
}

func (b *InMemoryBackend) reconcileInstancesLocked() {
	now := time.Now()

	for id, readyAt := range b.instanceReadyAt {
		if !readyAt.IsZero() && now.After(readyAt) {
			if inst, ok := b.instances.Get(normalizeID(id)); ok {
				applyPendingModifications(inst)
				inst.DBInstanceStatus = instanceStatusAvailable
				b.publishInstanceEventLocked(id, "DB instance is now available")
			}
			delete(b.instanceReadyAt, id)
		}
	}

	for id, readyAt := range b.clusterReadyAt {
		if !readyAt.IsZero() && now.After(readyAt) {
			if c, ok := b.clusters.Get(normalizeID(id)); ok && c.Status == "rebooting" {
				c.Status = instanceStatusAvailable
				b.publishClusterEventLocked(id, "DB cluster is now available")
			}
			delete(b.clusterReadyAt, id)
		}
	}
}

func (b *InMemoryBackend) publishInstanceEventLocked(id, msg string) {
	event := Event{
		Message:          msg,
		SourceIdentifier: id,
		SourceType:       "db-instance",
		SourceArn:        b.rdsARN("db", id),
		CreatedAt:        time.Now(),
	}

	b.events = append(b.events, event)
	if len(b.events) > maxEvents {
		b.events = b.events[len(b.events)-maxEvents:]
	}
}

// rdsARN constructs the ARN for an RDS resource.
// The format is: arn:aws:rds:{region}:{accountID}:{resourceType}:{id}.
func (b *InMemoryBackend) rdsARN(resourceType, id string) string {
	return arn.Build("rds", b.region, b.accountID, fmt.Sprintf("%s:%s", resourceType, id))
}

// AddClusterInternal creates a DB cluster directly, bypassing normal validation. Used for seeding tests.
func (b *InMemoryBackend) AddClusterInternal(id, engine string) *DBCluster {
	b.mu.Lock("AddClusterInternal")
	defer b.mu.Unlock()

	c := &DBCluster{
		DBClusterIdentifier: id,
		DBClusterArn:        b.rdsARN("cluster", id),
		Engine:              engine,
		Status:              instanceStatusAvailable,
	}
	b.clusters.Put(c)
	cp := *c

	return &cp
}

// AddInstanceInternal creates a DB instance directly, bypassing normal validation. Used for seeding tests.
func (b *InMemoryBackend) AddInstanceInternal(id, engine string) *DBInstance {
	b.mu.Lock("AddInstanceInternal")
	defer b.mu.Unlock()

	inst := &DBInstance{
		DBInstanceIdentifier: id,
		DBInstanceArn:        b.rdsARN("db", id),
		Engine:               engine,
		DBInstanceStatus:     instanceStatusAvailable,
		DBInstanceClass:      defaultInstanceClass,
		AllocatedStorage:     defaultAllocatedStorage,
	}
	b.instances.Put(inst)
	cp := *inst

	return &cp
}

// AddEventSubscriptionInternal creates an event subscription directly. Used for seeding tests.
func (b *InMemoryBackend) AddEventSubscriptionInternal(name, snsTopicArn string) *EventSubscription {
	b.mu.Lock("AddEventSubscriptionInternal")
	defer b.mu.Unlock()

	sub := &EventSubscription{
		SubscriptionName: name,
		SnsTopicArn:      snsTopicArn,
		Status:           subscriptionStatusActive,
		SourceIDs:        []string{},
	}
	b.eventSubscriptions.Put(sub)
	cp := *sub
	cp.SourceIDs = make([]string, 0)

	return &cp
}

// AddBlueGreenDeploymentInternal creates a Blue/Green Deployment directly. Used for seeding tests.
func (b *InMemoryBackend) AddBlueGreenDeploymentInternal(name, source string) *BlueGreenDeployment {
	b.mu.Lock("AddBlueGreenDeploymentInternal")
	defer b.mu.Unlock()

	id := "bgd-" + name
	d := &BlueGreenDeployment{
		BlueGreenDeploymentIdentifier: id,
		BlueGreenDeploymentName:       name,
		Source:                        source,
		Status:                        blueGreenDeploymentStatusAvailable,
	}
	b.blueGreenDeployments.Put(d)
	cp := *d

	return &cp
}

// AddSecurityGroupInternal creates a DB security group directly. Used for seeding tests.
func (b *InMemoryBackend) AddSecurityGroupInternal(name, description string) *DBSecurityGroup {
	b.mu.Lock("AddSecurityGroupInternal")
	defer b.mu.Unlock()

	sg := &DBSecurityGroup{
		DBSecurityGroupName:        name,
		DBSecurityGroupDescription: description,
		IPRanges:                   []IPRange{},
	}
	b.dbSecurityGroups.Put(sg)
	cp := *sg
	cp.IPRanges = make([]IPRange, 0)

	return &cp
}
