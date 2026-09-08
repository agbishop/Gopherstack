package azurearm

// SubscriptionBody returns the wire response body for
// GET /subscriptions/{sub} and as the single entry of GET /subscriptions'
// "value" array.
func SubscriptionBody(subscriptionID, displayName string) map[string]any {
	return map[string]any{
		"id":             "/subscriptions/" + subscriptionID,
		"subscriptionId": subscriptionID,
		"displayName":    displayName,
		"state":          "Enabled",
		"tenantId":       subscriptionID,
	}
}

// TenantBody returns the wire response body for one entry of GET /tenants'
// "value" array.
func TenantBody(tenantID string) map[string]any {
	return map[string]any{
		"id":             "/tenants/" + tenantID,
		"tenantId":       tenantID,
		"displayName":    "gopherstack",
		"defaultDomain":  "gopherstack.onmicrosoft.com",
		"tenantCategory": "Home",
	}
}

// RegisterProvider marks namespace as registered for subscriptionID.
func (b *InMemoryBackend) RegisterProvider(subscriptionID, namespace string) {
	b.mu.Lock("RegisterProvider")
	defer b.mu.Unlock()

	if b.registeredProviders[subscriptionID] == nil {
		b.registeredProviders[subscriptionID] = make(map[string]bool)
	}

	b.registeredProviders[subscriptionID][namespace] = true
}

// IsProviderRegistered reports whether namespace has been registered for
// subscriptionID.
func (b *InMemoryBackend) IsProviderRegistered(subscriptionID, namespace string) bool {
	b.mu.RLock("IsProviderRegistered")
	defer b.mu.RUnlock()

	return b.registeredProviders[subscriptionID][namespace]
}

// ProviderRegistrationState returns "Registered" or "NotRegistered" for
// namespace under subscriptionID, the value ARM's /providers/{ns} response
// carries in its "registrationState" field.
func (b *InMemoryBackend) ProviderRegistrationState(subscriptionID, namespace string) string {
	if b.IsProviderRegistered(subscriptionID, namespace) {
		return "Registered"
	}

	return "NotRegistered"
}

// ProviderBody returns the wire response body for GET
// /subscriptions/{sub}/providers/{ns} and each entry of the
// GET /subscriptions/{sub}/providers list.
func ProviderBody(subscriptionID, namespace, registrationState string, types []ResourceTypeDef) map[string]any {
	resourceTypes := make([]map[string]any, 0, len(types))

	for _, t := range types {
		resourceTypes = append(resourceTypes, map[string]any{
			"resourceType": t.Type,
			"apiVersions":  t.APIVersions,
		})
	}

	return map[string]any{
		"id":                "/subscriptions/" + subscriptionID + "/providers/" + namespace,
		"namespace":         namespace,
		"registrationState": registrationState,
		"resourceTypes":     resourceTypes,
	}
}
