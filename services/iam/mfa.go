package iam

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"sort"
	"time"

	"github.com/blackbirdworks/gopherstack/pkgs/arn"
	"github.com/blackbirdworks/gopherstack/pkgs/page"
)

// EnableMFADevice links a virtual MFA device to a user.
// Returns an error if the device is already enabled (double-enable rejected).
func (b *InMemoryBackend) EnableMFADevice(userName, serialNumber, authCode1, authCode2 string) error {
	b.mu.RLock("EnableMFADevice-check")
	_, userExists := b.users.Get(userName)
	dev, deviceExists := b.virtualMFADevices.Get(serialNumber)
	b.mu.RUnlock()

	if !userExists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if !deviceExists {
		return fmt.Errorf("%w: virtual MFA device %q not found", ErrMFADeviceNotFound, serialNumber)
	}

	if dev.Status == MFAStatusEnabled {
		return fmt.Errorf(
			"%w: virtual MFA device %q is already enabled",
			ErrMFADeviceAlreadyEnabled, serialNumber,
		)
	}

	if err := b.validateAuthCodes(authCode1, authCode2); err != nil {
		return err
	}

	b.mu.Lock("EnableMFADevice")
	b.comp().mfaUserLinks[serialNumber] = userName
	b.mu.Unlock()

	// Update device status to Active. Must run after releasing b.mu above:
	// setMFADeviceStatus takes its own b.mu.Lock, and lockmetrics.RWMutex (like
	// sync.RWMutex) is not reentrant.
	_ = b.setMFADeviceStatus(serialNumber, MFAStatusEnabled)

	return nil
}

// DeactivateMFADevice unlinks a virtual MFA device from a user.
// Returns an error if the device is not currently enabled.
func (b *InMemoryBackend) DeactivateMFADevice(userName, serialNumber string) error {
	b.mu.RLock("DeactivateMFADevice-check")
	_, userExists := b.users.Get(userName)
	dev, deviceExists := b.virtualMFADevices.Get(serialNumber)
	b.mu.RUnlock()

	if !userExists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if !deviceExists {
		return fmt.Errorf("%w: virtual MFA device %q not found", ErrMFADeviceNotFound, serialNumber)
	}

	if dev.Status != MFAStatusEnabled {
		return fmt.Errorf(
			"%w: virtual MFA device %q is not currently enabled",
			ErrInvalidAction, serialNumber,
		)
	}

	b.mu.Lock("DeactivateMFADevice")
	delete(b.comp().mfaUserLinks, serialNumber)
	b.mu.Unlock()

	// Must run after releasing b.mu above: see EnableMFADevice's matching note.
	_ = b.setMFADeviceStatus(serialNumber, MFAStatusDeactivated)

	return nil
}

// GetMFADeviceOwner returns the user name that owns the given MFA device, or "".
func (b *InMemoryBackend) GetMFADeviceOwner(serialNumber string) string {
	b.mu.RLock("GetMFADeviceOwner")
	defer b.mu.RUnlock()

	return b.comp().mfaUserLinks[serialNumber]
}

// ListMFADevicesForUser returns all MFA devices assigned to a user.
func (b *InMemoryBackend) ListMFADevicesForUser(
	userName, marker string,
	maxItems int,
) (page.Page[VirtualMFADevice], error) {
	b.mu.RLock("ListMFADevicesForUser")
	defer b.mu.RUnlock()

	if _, exists := b.users.Get(userName); !exists {
		return page.Page[VirtualMFADevice]{}, fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	c := b.comp()

	var devices []VirtualMFADevice

	for serial, owner := range c.mfaUserLinks {
		if owner == userName {
			if dev, ok := b.virtualMFADevices.Get(serial); ok {
				devices = append(devices, *dev)
			}
		}
	}

	sort.Slice(devices, func(i, j int) bool { return devices[i].SerialNumber < devices[j].SerialNumber })

	return page.New(devices, marker, maxItems, iamDefaultMaxItems), nil
}

// GetVirtualMFADevice returns the virtual MFA device with the given serial number
// along with the user name it is currently assigned to (empty if unassigned).
// It returns ErrUserNotFound (mapped to NoSuchEntity) when no such device exists.
func (b *InMemoryBackend) GetVirtualMFADevice(serialNumber string) (VirtualMFADevice, string, error) {
	b.mu.RLock("GetVirtualMFADevice")
	defer b.mu.RUnlock()

	dev, exists := b.virtualMFADevices.Get(serialNumber)
	if !exists {
		return VirtualMFADevice{}, "", fmt.Errorf("%w: virtual MFA device %q not found", ErrUserNotFound, serialNumber)
	}

	owner := b.comp().mfaUserLinks[serialNumber]

	return *dev, owner, nil
}

// ResyncMFADevice resynchronizes the named virtual MFA device for a user.
// AWS validates that the user exists and that the MFA device is associated
// with that user; the resync itself stores no additional state (no TOTP
// validation is performed in the mock). It returns ErrUserNotFound
// (NoSuchEntity) when the user or association is missing.
func (b *InMemoryBackend) ResyncMFADevice(userName, serialNumber, authCode1, authCode2 string) error {
	b.mu.RLock("ResyncMFADevice-check")
	_, userExists := b.users.Get(userName)
	_, deviceExists := b.virtualMFADevices.Get(serialNumber)
	owner := b.comp().mfaUserLinks[serialNumber]
	b.mu.RUnlock()

	if !userExists {
		return fmt.Errorf("%w: user %q not found", ErrUserNotFound, userName)
	}

	if !deviceExists {
		return fmt.Errorf("%w: virtual MFA device %q not found", ErrUserNotFound, serialNumber)
	}

	if owner != userName {
		return fmt.Errorf(
			"%w: virtual MFA device %q is not associated with user %q",
			ErrUserNotFound,
			serialNumber,
			userName,
		)
	}

	if err := b.validateAuthCodes(authCode1, authCode2); err != nil {
		return err
	}

	return nil
}

// mfaSeedBytes is the number of random bytes used for MFA seed generation.
const mfaSeedBytes = 20

// mfaQRCodeBytes is the number of random bytes used for fake QR code PNG generation.
const mfaQRCodeBytes = 32

// CreateVirtualMFADeviceFull creates a virtual MFA device with QR code and seed data.
func (b *InMemoryBackend) CreateVirtualMFADeviceFull(
	virtualMFADeviceName, path string,
) (*VirtualMFADevice, error) {
	if virtualMFADeviceName == "" {
		return nil, fmt.Errorf("%w: VirtualMFADeviceName must not be empty", ErrInvalidInput)
	}

	p := normPath(path)
	serialNumber := arn.Build("iam", "", b.accountID, "mfa"+p+virtualMFADeviceName)

	b.mu.Lock("CreateVirtualMFADeviceFull")
	defer b.mu.Unlock()

	if _, exists := b.virtualMFADevices.Get(serialNumber); exists {
		return nil, fmt.Errorf("%w: virtual MFA device %q already exists", ErrUserAlreadyExists, virtualMFADeviceName)
	}

	// Generate a fake 20-byte seed and encode as base64 (mock base32 approximation).
	seedBytes := make([]byte, mfaSeedBytes)
	_, _ = rand.Read(seedBytes)
	base32Seed := base64.StdEncoding.EncodeToString(seedBytes)

	// Generate a minimal fake QR code PNG (random bytes base64-encoded).
	qrBytes := make([]byte, mfaQRCodeBytes)
	_, _ = rand.Read(qrBytes)
	qrPNG := base64.StdEncoding.EncodeToString(qrBytes)

	device := VirtualMFADevice{
		SerialNumber:         serialNumber,
		VirtualMFADeviceName: virtualMFADeviceName,
		Path:                 p,
		CreateDate:           time.Now().UTC(),
		Status:               MFAStatusNotAssigned,
		Base32StringSeed:     base32Seed,
		QRCodePNG:            qrPNG,
	}

	b.virtualMFADevices.Put(&device)

	return &device, nil
}

func (b *InMemoryBackend) validateAuthCodes(code1, code2 string) error {
	if len(code1) != 6 || len(code2) != 6 {
		return fmt.Errorf("%w: codes must be 6 digits", ErrInvalidAuthenticationCode)
	}
	for _, c := range code1 {
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: codes must be numeric", ErrInvalidAuthenticationCode)
		}
	}
	for _, c := range code2 {
		if c < '0' || c > '9' {
			return fmt.Errorf("%w: codes must be numeric", ErrInvalidAuthenticationCode)
		}
	}
	if code1 == code2 {
		return fmt.Errorf("%w: codes must be distinct", ErrInvalidAuthenticationCode)
	}

	return nil
}

// ListVirtualMFADevices returns a paginated list of virtual MFA devices.
func (b *InMemoryBackend) ListVirtualMFADevices(marker string, maxItems int) (page.Page[VirtualMFADevice], error) {
	b.mu.RLock("ListVirtualMFADevices")
	defer b.mu.RUnlock()

	devices := make([]VirtualMFADevice, 0, b.virtualMFADevices.Len())
	for _, d := range b.virtualMFADevices.All() {
		devices = append(devices, *d)
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].SerialNumber < devices[j].SerialNumber
	})

	return page.New(devices, marker, maxItems, iamDefaultMaxItems), nil
}

// DeleteVirtualMFADevice deletes a virtual MFA device by its serial number.
func (b *InMemoryBackend) DeleteVirtualMFADevice(serialNumber string) error {
	b.mu.Lock("DeleteVirtualMFADevice")
	defer b.mu.Unlock()

	dev, exists := b.virtualMFADevices.Get(serialNumber)
	if !exists {
		return fmt.Errorf("%w: virtual MFA device %q not found", ErrUserNotFound, serialNumber)
	}

	if dev.Status == MFAStatusEnabled {
		return fmt.Errorf(
			"%w: virtual MFA device %q must be deactivated before it can be deleted",
			ErrDeleteConflict, serialNumber,
		)
	}

	b.virtualMFADevices.Delete(serialNumber)

	return nil
}

// setMFADeviceStatus updates a virtual MFA device's Status field.
// Caller must NOT hold b.mu (acquires write lock).
func (b *InMemoryBackend) setMFADeviceStatus(serialNumber, status string) error {
	b.mu.Lock("setMFADeviceStatus")
	defer b.mu.Unlock()

	dev, exists := b.virtualMFADevices.Get(serialNumber)
	if !exists {
		return fmt.Errorf("%w: virtual MFA device %q not found", ErrMFADeviceNotFound, serialNumber)
	}

	dev.Status = status
	b.virtualMFADevices.Put(dev)

	return nil
}

// CreateVirtualMFADevice creates a virtual MFA device.
func (b *InMemoryBackend) CreateVirtualMFADevice(virtualMFADeviceName, path string) (*VirtualMFADevice, error) {
	if virtualMFADeviceName == "" {
		return nil, fmt.Errorf("%w: VirtualMFADeviceName must not be empty", ErrInvalidInput)
	}

	p := normPath(path)
	serialNumber := arn.Build("iam", "", b.accountID, "mfa"+p+virtualMFADeviceName)

	b.mu.Lock("CreateVirtualMFADevice")
	defer b.mu.Unlock()

	if _, exists := b.virtualMFADevices.Get(serialNumber); exists {
		return nil, fmt.Errorf("%w: virtual MFA device %q already exists", ErrUserAlreadyExists, virtualMFADeviceName)
	}

	device := VirtualMFADevice{
		SerialNumber:         serialNumber,
		VirtualMFADeviceName: virtualMFADeviceName,
		Path:                 p,
		CreateDate:           time.Now().UTC(),
		Status:               MFAStatusNotAssigned,
	}

	b.virtualMFADevices.Put(&device)

	return &device, nil
}
