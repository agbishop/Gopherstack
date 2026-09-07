package glacier

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/labstack/echo/v5"
)

// vaultLocation returns the location path for a vault creation response.
func vaultLocation(accountID, vaultName string) string {
	return fmt.Sprintf("/%s/vaults/%s", accountID, vaultName)
}

// validateVaultName returns an error if the vault name is empty, too long, or contains
// characters outside the set allowed by AWS Glacier: [a-zA-Z0-9._-].
func validateVaultName(name string) error {
	if len(name) == 0 || len(name) > maxVaultNameLen {
		return fmt.Errorf("%w: length must be 1-%d", ErrInvalidVaultName, maxVaultNameLen)
	}

	for i := range len(name) {
		c := name[i]
		if (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') &&
			c != '.' && c != '_' && c != '-' {
			return fmt.Errorf("%w: invalid character 0x%02x at position %d", ErrInvalidVaultName, c, i)
		}
	}

	return nil
}

func (h *Handler) handleCreateVault(c *echo.Context, vaultName string) error {
	if err := validateVaultName(vaultName); err != nil {
		return h.writeError(c, http.StatusBadRequest, "InvalidParameterValueException", err.Error())
	}

	v, err := h.Backend.CreateVault(h.AccountID, h.DefaultRegion, vaultName)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	c.Response().Header().Set("Location", vaultLocation(h.AccountID, vaultName))
	c.Response().Header().Set("X-Amzn-Requestid", "glacier-create-vault")

	return c.JSON(http.StatusCreated, createVaultResponse{
		Location: vaultLocation(h.AccountID, v.VaultName),
	})
}

func (h *Handler) handleDescribeVault(c *echo.Context, vaultName string) error {
	v, err := h.Backend.DescribeVault(h.AccountID, h.DefaultRegion, vaultName)
	if err != nil {
		return h.writeBackendError(c, err)
	}

	return c.JSON(http.StatusOK, toDescribeVaultResponse(v))
}

func (h *Handler) handleDeleteVault(c *echo.Context, vaultName string) error {
	if err := h.Backend.DeleteVault(h.AccountID, h.DefaultRegion, vaultName); err != nil {
		return h.writeBackendError(c, err)
	}

	return c.NoContent(http.StatusNoContent)
}

func (h *Handler) handleListVaults(c *echo.Context, accountID string) error {
	resolved := h.resolveAccountID(accountID)
	vaults := h.Backend.ListVaults(resolved, h.DefaultRegion)
	items := make([]describeVaultResponse, 0, len(vaults))

	for _, v := range vaults {
		items = append(items, toDescribeVaultResponse(v))
	}

	// Support `marker` pagination: start listing after this vault name.
	marker := c.QueryParam("marker")
	if marker != "" {
		marker = decodeMarker(marker)
	}

	if marker != "" {
		start := 0

		for start < len(items) && items[start].VaultName != marker {
			start++
		}

		if start < len(items) {
			items = items[start+1:]
		} else {
			items = items[:0]
		}
	}

	// Support `limit` to cap the number of results returned. AWS: 1-50, default 10.
	limitStr := c.QueryParam("limit")

	n := defaultListVaultsLimit

	if limitStr != "" {
		var err error

		n, err = strconv.Atoi(limitStr)
		if err != nil || n < minListLimit || n > maxListVaultsLimit {
			return h.writeError(
				c,
				http.StatusBadRequest,
				"InvalidParameterValueException",
				fmt.Sprintf(
					"%v: must be between %d and %d",
					ErrLimitOutOfRange,
					minListLimit,
					maxListVaultsLimit,
				),
			)
		}
	}

	var nextMarker *string

	if n < len(items) {
		last := encodeMarker(items[n-1].VaultName)
		nextMarker = &last
		items = items[:n]
	}

	return c.JSON(http.StatusOK, listVaultsResponse{
		Marker:    nextMarker,
		VaultList: items,
	})
}

// toDescribeVaultResponse converts a vault to a describe vault response.
//
// NumberOfArchives/SizeInBytes report the as-of-last-inventory snapshot, not
// the live counters -- LastInventoryDate empty means no inventory has ever
// run, so both stay nil/omitted rather than reporting a live count as if it
// were an inventory result (gopherstack-zpo5).
func toDescribeVaultResponse(v *Vault) describeVaultResponse {
	resp := describeVaultResponse{
		VaultARN:          v.VaultARN,
		VaultName:         v.VaultName,
		CreationDate:      v.CreationDate,
		LastInventoryDate: v.LastInventoryDate,
	}

	if v.LastInventoryDate != "" {
		numArchives := v.NumberOfArchivesAtLastInventory
		sizeBytes := v.SizeInBytesAtLastInventory
		resp.NumberOfArchives = &numArchives
		resp.SizeInBytes = &sizeBytes
	}

	return resp
}
