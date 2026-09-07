package lightsail

// This file backs family G (6 ops: CreateKeyPair, DeleteKeyPair,
// DownloadDefaultKeyPair, GetKeyPair, GetKeyPairs, ImportKeyPair) and family
// H (6 ops: AllocateStaticIp, AttachStaticIp, DetachStaticIp, GetStaticIp,
// GetStaticIps, ReleaseStaticIp) -- both self-contained, instance-adjacent
// families with no complex sub-state-machine (PARITY.md's suggested
// implementation ordering, step 2).

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"sort"
	"time"

	"golang.org/x/crypto/ssh"

	"github.com/blackbirdworks/gopherstack/pkgs/page"
	"github.com/blackbirdworks/gopherstack/pkgs/tags"
)

const (
	rsaKeyBits             = 2048
	defaultKeyPairName     = "LightsailDefaultKeyPair"
	opTypeAllocateStaticIP = "AllocateStaticIp"
	opTypeAttachStaticIP   = "AttachStaticIp"
	opTypeDetachStaticIP   = "DetachStaticIp"
	opTypeReleaseStaticIP  = "ReleaseStaticIp"
	opTypeDeleteKeyPair    = "DeleteKeyPair"
	opTypeImportKeyPair    = "ImportKeyPair"
	opTypeCreateKeyPair    = "CreateKeyPair"
)

// generateRSAKeyPair generates a real RSA key pair (not a placeholder
// string) and returns its PEM-encoded private key, OpenSSH authorized-keys
// public key, and MD5 fingerprint -- matching services/ec2's own
// CreateKeyPair (key_pairs.go), the established precedent in this repo for
// an honestly-functional (if not AWS-issued) key pair rather than fabricated
// text.
func generateRSAKeyPair(name string) (string, string, string, error) {
	privKey, genErr := rsa.GenerateKey(rand.Reader, rsaKeyBits)
	if genErr != nil {
		return "", "", "", genErr
	}

	privDER := x509.MarshalPKCS1PrivateKey(privKey)
	privPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: privDER})

	pub, sshErr := ssh.NewPublicKey(&privKey.PublicKey)
	if sshErr != nil {
		return "", "", "", sshErr
	}

	fp := ssh.FingerprintLegacyMD5(pub)
	authorized := fmt.Sprintf("%s gopherstack-%s", trimNewline(string(ssh.MarshalAuthorizedKey(pub))), name)

	return string(privPEM), authorized, fp, nil
}

func trimNewline(s string) string {
	for len(s) > 0 && (s[len(s)-1] == '\n' || s[len(s)-1] == '\r') {
		s = s[:len(s)-1]
	}

	return s
}

// CreateKeyPair generates a new named key pair, returning the public
// KeyPair metadata plus its private/public key material -- returned in full
// exactly once (PARITY.md family G); this backend never stores the private
// material for a non-default key pair afterward.
func (b *InMemoryBackend) CreateKeyPair(
	name string,
	userTags map[string]string,
) (*KeyPair, *Operation, string, string, error) {
	priv, pub, fp, err := generateRSAKeyPair(name)
	if err != nil {
		return nil, nil, "", "", serviceError("generate key pair: " + err.Error())
	}

	b.mu.Lock("CreateKeyPair")
	defer b.mu.Unlock()

	if regErr := b.registerNameLocked(ResourceTypeKeyPair, name); regErr != nil {
		return nil, nil, "", "", regErr
	}

	kp := &KeyPair{
		Name: name, Arn: b.regionalARN(ResourceTypeKeyPair, newUUID()), SupportCode: newSupportCode(),
		Fingerprint: fp, CreatedAt: nowUTC(),
		Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		Tags:     tags.New("lightsail.keypair." + name + ".tags"),
	}
	kp.Tags.Merge(userTags)
	b.keyPairs.Put(kp)

	ops := b.newOperationsLocked(opTypeCreateKeyPair, ResourceTypeKeyPair, []string{name})

	return kp.clone(), &ops[0], priv, pub, nil
}

// ImportKeyPair registers a caller-supplied public key. This backend never
// receives (and so never stores) private key material for an imported key.
func (b *InMemoryBackend) ImportKeyPair(name, publicKeyBase64 string) (*Operation, error) {
	b.mu.Lock("ImportKeyPair")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeKeyPair, name); err != nil {
		return nil, err
	}

	fp := fingerprintFromMaterial(publicKeyBase64)
	kp := &KeyPair{
		Name: name, Arn: b.regionalARN(ResourceTypeKeyPair, newUUID()), SupportCode: newSupportCode(),
		Fingerprint: fp, CreatedAt: nowUTC(),
		Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		Tags:     tags.New("lightsail.keypair." + name + ".tags"),
	}
	b.keyPairs.Put(kp)

	ops := b.newOperationsLocked(opTypeImportKeyPair, ResourceTypeKeyPair, []string{name})

	return &ops[0], nil
}

// fingerprintFromMaterial derives a stable, deterministic MD5-shaped
// fingerprint string from raw key material -- a defensible placeholder
// since parsing an arbitrary caller-supplied public key format is out of
// scope; never claimed as AWS's own computed fingerprint.
const (
	// fingerprintHashMultiplier is an arbitrary small-prime multiplier for
	// the per-character rolling hash below -- a common odd-prime choice for
	// spreading hash output, not a meaningful quantity on its own.
	fingerprintHashMultiplier = 131
	fingerprintByteMask       = 0xff
	fingerprintByteShift1     = 8
	fingerprintByteShift2     = 16
	fingerprintByteShift3     = 24
)

func fingerprintFromMaterial(material string) string {
	sum := 0
	for _, c := range material {
		sum = sum*fingerprintHashMultiplier + int(c)
	}

	if sum < 0 {
		sum = -sum
	}

	return fmt.Sprintf(
		"%02x:%02x:%02x:%02x",
		sum&fingerprintByteMask,
		(sum>>fingerprintByteShift1)&fingerprintByteMask,
		(sum>>fingerprintByteShift2)&fingerprintByteMask,
		(sum>>fingerprintByteShift3)&fingerprintByteMask,
	)
}

// DeleteKeyPair deletes the named key pair.
func (b *InMemoryBackend) DeleteKeyPair(name string) (*Operation, error) {
	b.mu.Lock("DeleteKeyPair")
	defer b.mu.Unlock()

	kp, ok := b.keyPairs.Get(name)
	if !ok {
		return nil, notFoundError("KeyPair", name)
	}

	if kp.Tags != nil {
		kp.Tags.Close()
	}

	b.keyPairs.Delete(name)
	b.unregisterNameLocked(name)

	ops := b.newOperationsLocked(opTypeDeleteKeyPair, ResourceTypeKeyPair, []string{name})

	return &ops[0], nil
}

// DownloadDefaultKeyPair returns this account/region's singleton default
// key pair, lazily creating it on first call -- the one KeyPair whose
// private material stays retrievable indefinitely (PARITY.md 4.1), unlike
// every other CreateKeyPair-created key.
func (b *InMemoryBackend) DownloadDefaultKeyPair() (time.Time, string, string, error) {
	b.mu.Lock("DownloadDefaultKeyPair")
	defer b.mu.Unlock()

	if b.defaultKeyPair == nil {
		priv, pub, fp, genErr := generateRSAKeyPair(defaultKeyPairName)
		if genErr != nil {
			return time.Time{}, "", "", serviceError("generate default key pair: " + genErr.Error())
		}

		b.defaultKeyPair = &KeyPair{
			Name: defaultKeyPairName, Arn: b.regionalARN(ResourceTypeKeyPair, newUUID()),
			SupportCode: newSupportCode(), Fingerprint: fp, CreatedAt: nowUTC(),
			Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		}
		b.defaultKeyPairPrivateKey = priv
		b.defaultKeyPairPublicKey = pub
	}

	return b.defaultKeyPair.CreatedAt, b.defaultKeyPairPrivateKey, b.defaultKeyPairPublicKey, nil
}

// defaultKeyPairPrivateKeyLocked lazily creates (if needed) and returns the
// default key pair's private key. Callers must hold b.mu.
func (b *InMemoryBackend) defaultKeyPairPrivateKeyLocked() string {
	if b.defaultKeyPair == nil {
		priv, pub, fp, genErr := generateRSAKeyPair(defaultKeyPairName)
		if genErr != nil {
			return ""
		}

		b.defaultKeyPair = &KeyPair{
			Name: defaultKeyPairName, Arn: b.regionalARN(ResourceTypeKeyPair, newUUID()),
			SupportCode: newSupportCode(), Fingerprint: fp, CreatedAt: nowUTC(),
			Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
		}
		b.defaultKeyPairPrivateKey = priv
		b.defaultKeyPairPublicKey = pub
	}

	return b.defaultKeyPairPrivateKey
}

// GetKeyPair returns the named key pair's public metadata.
func (b *InMemoryBackend) GetKeyPair(name string) (*KeyPair, error) {
	b.mu.RLock("GetKeyPair")
	defer b.mu.RUnlock()

	kp, ok := b.keyPairs.Get(name)
	if !ok {
		return nil, notFoundError("KeyPair", name)
	}

	return kp.clone(), nil
}

// GetKeyPairs returns every key pair, optionally including the default key
// pair, paginated.
func (b *InMemoryBackend) GetKeyPairs(includeDefault bool, token string) (page.Page[*KeyPair], error) {
	b.mu.RLock("GetKeyPairs")
	defer b.mu.RUnlock()

	all := b.keyPairs.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*KeyPair, 0, len(all)+1)
	if includeDefault && b.defaultKeyPair != nil {
		out = append(out, b.defaultKeyPair.clone())
	}

	for _, kp := range all {
		out = append(out, kp.clone())
	}

	return paginateGeneric(out, token)
}

// AllocateStaticIP allocates a new, unattached static IP.
func (b *InMemoryBackend) AllocateStaticIP(name string) (*Operation, error) {
	b.mu.Lock("AllocateStaticIp")
	defer b.mu.Unlock()

	if err := b.registerNameLocked(ResourceTypeStaticIP, name); err != nil {
		return nil, err
	}

	sip := &StaticIP{
		Name: name, Arn: b.regionalARN(ResourceTypeStaticIP, newUUID()), SupportCode: newSupportCode(),
		IPAddress: publicIPForName(name, 0), CreatedAt: nowUTC(),
		Location: ResourceLocation{RegionName: b.region, AvailabilityZone: availabilityZoneA(b.region)},
	}
	b.staticIPs.Put(sip)

	ops := b.newOperationsLocked(opTypeAllocateStaticIP, ResourceTypeStaticIP, []string{name})

	return &ops[0], nil
}

// AttachStaticIP attaches the named static IP to instanceName.
func (b *InMemoryBackend) AttachStaticIP(name, instanceName string) (*Operation, error) {
	b.mu.Lock("AttachStaticIp")
	defer b.mu.Unlock()

	sip, ok := b.staticIPs.Get(name)
	if !ok {
		return nil, notFoundError("StaticIp", name)
	}

	inst, ok := b.instances.Get(instanceName)
	if !ok {
		return nil, notFoundError("Instance", instanceName)
	}

	sip.IsAttached = true
	sip.AttachedTo = instanceName
	inst.IsStaticIP = true
	inst.PublicIPAddress = sip.IPAddress

	ops := b.newOperationsLocked(opTypeAttachStaticIP, ResourceTypeStaticIP, []string{name})

	return &ops[0], nil
}

// DetachStaticIP detaches the named static IP from whatever instance it is
// attached to.
func (b *InMemoryBackend) DetachStaticIP(name string) (*Operation, error) {
	b.mu.Lock("DetachStaticIp")
	defer b.mu.Unlock()

	sip, ok := b.staticIPs.Get(name)
	if !ok {
		return nil, notFoundError("StaticIp", name)
	}

	if inst, instOK := b.instances.Get(sip.AttachedTo); instOK {
		inst.IsStaticIP = false
		inst.PublicIPAddress = publicIPForName(inst.Name, inst.PublicIPGeneration)
	}

	sip.IsAttached = false
	sip.AttachedTo = ""

	ops := b.newOperationsLocked(opTypeDetachStaticIP, ResourceTypeStaticIP, []string{name})

	return &ops[0], nil
}

// GetStaticIP returns the named static IP.
func (b *InMemoryBackend) GetStaticIP(name string) (*StaticIP, error) {
	b.mu.RLock("GetStaticIp")
	defer b.mu.RUnlock()

	sip, ok := b.staticIPs.Get(name)
	if !ok {
		return nil, notFoundError("StaticIp", name)
	}

	return sip.clone(), nil
}

// GetStaticIPs returns every static IP, paginated.
func (b *InMemoryBackend) GetStaticIPs(token string) (page.Page[*StaticIP], error) {
	b.mu.RLock("GetStaticIps")
	defer b.mu.RUnlock()

	all := b.staticIPs.All()
	sort.Slice(all, func(i, j int) bool { return all[i].Name < all[j].Name })

	out := make([]*StaticIP, len(all))
	for i, v := range all {
		out[i] = v.clone()
	}

	return paginateGeneric(out, token)
}

// ReleaseStaticIP releases (deletes) the named static IP.
func (b *InMemoryBackend) ReleaseStaticIP(name string) (*Operation, error) {
	b.mu.Lock("ReleaseStaticIp")
	defer b.mu.Unlock()

	sip, ok := b.staticIPs.Get(name)
	if !ok {
		return nil, notFoundError("StaticIp", name)
	}

	if inst, instOK := b.instances.Get(sip.AttachedTo); instOK {
		inst.IsStaticIP = false
		inst.PublicIPAddress = publicIPForName(inst.Name, inst.PublicIPGeneration)
	}

	b.staticIPs.Delete(name)
	b.unregisterNameLocked(name)

	ops := b.newOperationsLocked(opTypeReleaseStaticIP, ResourceTypeStaticIP, []string{name})

	return &ops[0], nil
}
