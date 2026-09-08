package outposts

import (
	"github.com/blackbirdworks/gopherstack/pkgs/awstime"
)

func toAddressWire(a *Address) *addressWire {
	if a == nil {
		return nil
	}

	return &addressWire{
		AddressLine1:       a.AddressLine1,
		AddressLine2:       a.AddressLine2,
		AddressLine3:       a.AddressLine3,
		City:               a.City,
		ContactName:        a.ContactName,
		ContactPhoneNumber: a.ContactPhoneNumber,
		CountryCode:        a.CountryCode,
		DistrictOrCounty:   a.DistrictOrCounty,
		Municipality:       a.Municipality,
		PostalCode:         a.PostalCode,
		StateOrRegion:      a.StateOrRegion,
	}
}

func fromAddressWire(w *addressWire) *Address {
	if w == nil {
		return nil
	}

	return &Address{
		AddressLine1:       w.AddressLine1,
		AddressLine2:       w.AddressLine2,
		AddressLine3:       w.AddressLine3,
		City:               w.City,
		ContactName:        w.ContactName,
		ContactPhoneNumber: w.ContactPhoneNumber,
		CountryCode:        w.CountryCode,
		DistrictOrCounty:   w.DistrictOrCounty,
		Municipality:       w.Municipality,
		PostalCode:         w.PostalCode,
		StateOrRegion:      w.StateOrRegion,
	}
}

func toRackPropsWire(r *RackPhysicalProperties) *rackPhysicalPropertiesWire {
	if r == nil {
		return nil
	}

	return &rackPhysicalPropertiesWire{
		FiberOpticCableType:       r.FiberOpticCableType,
		MaximumSupportedWeightLbs: r.MaximumSupportedWeightLbs,
		OpticalStandard:           r.OpticalStandard,
		PowerConnector:            r.PowerConnector,
		PowerDrawKva:              r.PowerDrawKva,
		PowerFeedDrop:             r.PowerFeedDrop,
		PowerPhase:                r.PowerPhase,
		UplinkCount:               r.UplinkCount,
		UplinkGbps:                r.UplinkGbps,
	}
}

func fromRackPropsWire(w *rackPhysicalPropertiesWire) *RackPhysicalProperties {
	if w == nil {
		return nil
	}

	return &RackPhysicalProperties{
		FiberOpticCableType:       w.FiberOpticCableType,
		MaximumSupportedWeightLbs: w.MaximumSupportedWeightLbs,
		OpticalStandard:           w.OpticalStandard,
		PowerConnector:            w.PowerConnector,
		PowerDrawKva:              w.PowerDrawKva,
		PowerFeedDrop:             w.PowerFeedDrop,
		PowerPhase:                w.PowerPhase,
		UplinkCount:               w.UplinkCount,
		UplinkGbps:                w.UplinkGbps,
	}
}

func toOutpostWire(o *Outpost) outpostWire {
	out := outpostWire{
		AvailabilityZone:      o.AvailabilityZone,
		AvailabilityZoneId:    o.AvailabilityZoneID,
		Description:           o.Description,
		LifeCycleStatus:       o.LifeCycleStatus,
		Name:                  o.Name,
		OutpostArn:            o.ARN,
		OutpostId:             o.ID,
		OwnerId:               o.OwnerID,
		SiteArn:               o.SiteARN,
		SiteId:                o.SiteID,
		SupportedHardwareType: o.SupportedHardwareType,
	}

	if o.Tags != nil {
		out.Tags = o.Tags.Clone()
	}

	return out
}

func toSiteWire(s *Site) siteWire {
	out := siteWire{
		AccountId:              s.AccountID,
		Description:            s.Description,
		Name:                   s.Name,
		Notes:                  s.Notes,
		RackPhysicalProperties: toRackPropsWire(s.RackPhysicalProperties),
		SiteArn:                s.ARN,
		SiteId:                 s.ID,
	}

	if s.OperatingAddress != nil {
		out.OperatingAddressCity = s.OperatingAddress.City
		out.OperatingAddressCountryCode = s.OperatingAddress.CountryCode
		out.OperatingAddressStateOrRegion = s.OperatingAddress.StateOrRegion
	}

	if s.Tags != nil {
		out.Tags = s.Tags.Clone()
	}

	return out
}

func toSubscriptionWire(s Subscription) subscriptionWire {
	out := subscriptionWire{
		Currency:              s.Currency,
		OrderIds:              cloneStrs(s.OrderIDs),
		SubscriptionId:        s.SubscriptionID,
		SubscriptionStatus:    s.SubscriptionStatus,
		SubscriptionType:      s.SubscriptionType,
		MonthlyRecurringPrice: s.MonthlyRecurringPrice,
		UpfrontPrice:          s.UpfrontPrice,
	}

	if !s.BeginDate.IsZero() {
		out.BeginDate = awstime.Epoch(s.BeginDate)
	}

	if !s.EndDate.IsZero() {
		out.EndDate = awstime.Epoch(s.EndDate)
	}

	return out
}

func toLineItemWire(li LineItem) lineItemWire {
	return lineItemWire{
		CatalogItemId:      li.CatalogItemID,
		LineItemId:         li.LineItemID,
		PreviousLineItemId: li.PreviousLineItemID,
		PreviousOrderId:    li.PreviousOrderID,
		Status:             li.Status,
		Quantity:           li.Quantity,
	}
}

func toOrderWire(o *Order) orderWire {
	out := orderWire{
		OrderId:               o.ID,
		OrderType:             o.OrderType,
		OutpostId:             o.OutpostID,
		PaymentOption:         o.PaymentOption,
		PaymentTerm:           o.PaymentTerm,
		QuoteIdentifier:       o.QuoteIdentifier,
		QuoteOptionIdentifier: o.QuoteOptionIdentifier,
		Status:                o.Status,
		LineItems:             make([]lineItemWire, 0, len(o.LineItems)),
	}

	for _, li := range o.LineItems {
		out.LineItems = append(out.LineItems, toLineItemWire(li))
	}

	if !o.OrderSubmissionDate.IsZero() {
		out.OrderSubmissionDate = awstime.Epoch(o.OrderSubmissionDate)
	}

	if !o.OrderFulfilledDate.IsZero() {
		out.OrderFulfilledDate = awstime.Epoch(o.OrderFulfilledDate)
	}

	return out
}

// lineItemStatusCounts tallies an order's LineItems by Status, for
// OrderSummary.LineItemCountsByStatus.
func lineItemStatusCounts(items []LineItem) map[string]int32 {
	if len(items) == 0 {
		return nil
	}

	counts := make(map[string]int32, len(items))
	for _, li := range items {
		counts[li.Status]++
	}

	return counts
}

func toOrderSummaryWire(o *Order) orderSummaryWire {
	out := orderSummaryWire{
		LineItemCountsByStatus: lineItemStatusCounts(o.LineItems),
		OrderId:                o.ID,
		OrderType:              o.OrderType,
		OutpostId:              o.OutpostID,
		Status:                 o.Status,
	}

	if !o.OrderSubmissionDate.IsZero() {
		out.OrderSubmissionDate = awstime.Epoch(o.OrderSubmissionDate)
	}

	if !o.OrderFulfilledDate.IsZero() {
		out.OrderFulfilledDate = awstime.Epoch(o.OrderFulfilledDate)
	}

	return out
}

func toQuoteCapacityWire(c QuoteCapacity) quoteCapacityWire {
	return quoteCapacityWire(c)
}

func toQuoteConstraintWire(c QuoteConstraint) quoteConstraintWire {
	return quoteConstraintWire(c)
}

func toPricingOptionWire(p PricingOption) pricingOptionWire {
	return pricingOptionWire{
		PricingType: quotePricingTypeSubscription,
		SubscriptionPricingDetails: &subscriptionPricingDetailsWire{
			Currency:              p.Currency,
			PaymentOption:         p.PaymentOption,
			PaymentTerm:           p.PaymentTerm,
			MonthlyRecurringPrice: p.MonthlyRecurringPrice,
			UpfrontPrice:          p.UpfrontPrice,
		},
	}
}

func toQuoteOptionWire(q *Quote) quoteOptionWire {
	capacities := make([]quoteCapacityWire, 0, len(q.RequestedCapacities))
	for _, c := range q.RequestedCapacities {
		capacities = append(capacities, toQuoteCapacityWire(c))
	}

	pricing := make([]pricingOptionWire, 0, len(q.PricingOptions))
	for _, p := range q.PricingOptions {
		pricing = append(pricing, toPricingOptionWire(p))
	}

	return quoteOptionWire{
		QuoteOptionIdentifier: q.QuoteOptionID,
		Capacities:            capacities,
		PricingOptions:        pricing,
		Specifications:        []struct{}{},
		CapacitySummary: &capacitySummaryWire{
			CapacityChange:     capacities,
			ExistingCapacities: []quoteCapacityWire{},
			FinalCapacities:    capacities,
		},
	}
}

func toQuoteWireBase(q *Quote) quoteWire {
	out := quoteWire{
		AccountId:               q.AccountID,
		CountryCode:             q.CountryCode,
		Description:             q.Description,
		OutpostArn:              q.OutpostARN,
		QuoteId:                 q.ID,
		QuoteStatus:             q.Status,
		StatusMessage:           q.StatusMessage,
		SubmittedOrderId:        q.SubmittedOrderID,
		RequestedPaymentOptions: cloneStrs(q.RequestedPaymentOptions),
		RequestedPaymentTerms:   cloneStrs(q.RequestedPaymentTerms),

		RequestedCapacities: make([]quoteCapacityWire, 0, len(q.RequestedCapacities))}
	for _, c := range q.RequestedCapacities {
		out.RequestedCapacities = append(out.RequestedCapacities, toQuoteCapacityWire(c))
	}

	out.RequestedConstraints = make([]quoteConstraintWire, 0, len(q.RequestedConstraints))
	for _, c := range q.RequestedConstraints {
		out.RequestedConstraints = append(out.RequestedConstraints, toQuoteConstraintWire(c))
	}

	if len(q.PricingOptions) > 0 || q.QuoteOptionID != "" {
		out.QuoteOptions = []quoteOptionWire{toQuoteOptionWire(q)}
	}

	if !q.CreatedDate.IsZero() {
		out.CreatedDate = awstime.Epoch(q.CreatedDate)
	}

	if !q.ExpirationDate.IsZero() {
		out.ExpirationDate = awstime.Epoch(q.ExpirationDate)
	}

	return out
}

// toQuoteWire builds the full Quote response shape (CreateQuote/GetQuote/UpdateQuote).
func toQuoteWire(q *Quote) quoteWire {
	out := toQuoteWireBase(q)

	out.OrderingRequirements = make([]orderingRequirementWire, 0, len(q.OrderingRequirements))
	for _, r := range q.OrderingRequirements {
		out.OrderingRequirements = append(out.OrderingRequirements, orderingRequirementWire(r))
	}

	return out
}

// toQuoteSummaryWire builds the QuoteSummary response shape (ListQuotes),
// which omits OrderingRequirements -- see wire.go's quoteWire doc comment.
func toQuoteSummaryWire(q *Quote) quoteWire {
	return toQuoteWireBase(q)
}

func toInstanceTypeCapacityWire(c InstanceTypeCapacity) instanceTypeCapacityWire {
	return instanceTypeCapacityWire(c)
}

func toInstancesToExcludeWire(e *InstancesToExclude) *instancesToExcludeWire {
	if e == nil {
		return nil
	}

	return &instancesToExcludeWire{
		AccountIds: cloneStrs(e.AccountIDs),
		Instances:  cloneStrs(e.Instances),
		Services:   cloneStrs(e.Services),
	}
}

func fromInstancesToExcludeWire(w *instancesToExcludeWire) *InstancesToExclude {
	if w == nil {
		return nil
	}

	return &InstancesToExclude{
		AccountIDs: cloneStrs(w.AccountIds),
		Instances:  cloneStrs(w.Instances),
		Services:   cloneStrs(w.Services),
	}
}

func toCapacityTaskFailureWire(f *CapacityTaskFailure) *capacityTaskFailureWire {
	if f == nil {
		return nil
	}

	return &capacityTaskFailureWire{Reason: f.Reason, Type: f.Type}
}

func toCapacityTaskResponse(t *CapacityTask) capacityTaskResponse {
	out := capacityTaskResponse{
		AssetId:                       t.AssetID,
		CapacityTaskId:                t.ID,
		CapacityTaskStatus:            t.Status,
		DryRun:                        t.DryRun,
		Failed:                        toCapacityTaskFailureWire(t.Failed),
		InstancesToExclude:            toInstancesToExcludeWire(t.InstancesToExclude),
		OrderId:                       t.OrderID,
		OutpostId:                     t.OutpostID,
		TaskActionOnBlockingInstances: t.TaskActionOnBlockingInstances,

		RequestedInstancePools: make([]instanceTypeCapacityWire, 0, len(t.RequestedInstancePools))}
	for _, p := range t.RequestedInstancePools {
		out.RequestedInstancePools = append(out.RequestedInstancePools, toInstanceTypeCapacityWire(p))
	}

	if !t.CreationDate.IsZero() {
		out.CreationDate = awstime.Epoch(t.CreationDate)
	}

	if !t.LastModifiedDate.IsZero() {
		out.LastModifiedDate = awstime.Epoch(t.LastModifiedDate)
	}

	if !t.CompletionDate.IsZero() {
		out.CompletionDate = awstime.Epoch(t.CompletionDate)
	}

	return out
}

func toCapacityTaskSummaryWire(t *CapacityTask) capacityTaskSummaryWire {
	out := capacityTaskSummaryWire{
		AssetId:            t.AssetID,
		CapacityTaskId:     t.ID,
		CapacityTaskStatus: t.Status,
		OrderId:            t.OrderID,
		OutpostId:          t.OutpostID,
	}

	if !t.CreationDate.IsZero() {
		out.CreationDate = awstime.Epoch(t.CreationDate)
	}

	if !t.LastModifiedDate.IsZero() {
		out.LastModifiedDate = awstime.Epoch(t.LastModifiedDate)
	}

	if !t.CompletionDate.IsZero() {
		out.CompletionDate = awstime.Epoch(t.CompletionDate)
	}

	return out
}

func toAssetInfoWire(a *Asset) assetInfoWire {
	out := assetInfoWire{
		AssetId:       a.ID,
		AssetType:     a.AssetType,
		RackId:        a.RackID,
		AssetLocation: &assetLocationWire{RackElevation: new(a.RackElevation)},
	}

	if a.ComputeAttributes != nil {
		ca := a.ComputeAttributes
		out.ComputeAttributes = &computeAttributesWire{
			HostId:           ca.HostID,
			State:            ca.State,
			InstanceFamilies: cloneStrs(ca.InstanceFamilies),
			MaxVcpus:         ca.MaxVcpus,
		}

		out.ComputeAttributes.InstanceTypeCapacities = make(
			[]instanceTypeCapacityWire,
			0,
			len(ca.InstanceTypeCapacities),
		)
		for _, c := range ca.InstanceTypeCapacities {
			out.ComputeAttributes.InstanceTypeCapacities = append(
				out.ComputeAttributes.InstanceTypeCapacities, toInstanceTypeCapacityWire(c),
			)
		}
	}

	return out
}

func toEC2CapacityWire(c EC2Capacity) ec2CapacityWire {
	return ec2CapacityWire(c)
}

func toCatalogItemWire(c CatalogItem) catalogItemWire {
	out := catalogItemWire{
		CatalogItemId:       c.ID,
		ItemStatus:          c.ItemStatus,
		SupportedStorage:    cloneStrs(c.SupportedStorage),
		SupportedUplinkGbps: append([]int32(nil), c.SupportedUplinkGbps...),
		PowerKva:            c.PowerKva,
		WeightLbs:           c.WeightLbs,

		EC2Capacities: make([]ec2CapacityWire, 0, len(c.EC2Capacities))}
	for _, ec := range c.EC2Capacities {
		out.EC2Capacities = append(out.EC2Capacities, toEC2CapacityWire(ec))
	}

	return out
}

func toInstanceTypeItemWire(it OrderableInstanceType) instanceTypeItemWire {
	return instanceTypeItemWire{InstanceType: it.InstanceType, VCPUs: new(it.VCPUs)}
}

func toDetailedInstanceTypeItemWire(it OrderableInstanceType) detailedInstanceTypeItemWire {
	out := detailedInstanceTypeItemWire{
		InstanceType:       it.InstanceType,
		NetworkPerformance: it.NetworkPerformance,
		MemoryInMib:        it.MemoryInMib,
		VCPUs:              new(it.VCPUs),

		FormFactorConfigs: []formFactorConfigWire{
			{FormFactor: it.FormFactor, OutpostGeneration: it.OutpostGeneration},
		}}

	return out
}

func toConnectionDetailsWire(c *Connection) *connectionDetailsWire {
	if c == nil {
		return nil
	}

	return &connectionDetailsWire{
		AllowedIps:          cloneStrs(c.AllowedIps),
		ClientPublicKey:     c.ClientPublicKey,
		ClientTunnelAddress: c.ClientTunnelAddress,
		ServerEndpoint:      c.ServerEndpoint,
		ServerPublicKey:     c.ServerPublicKey,
		ServerTunnelAddress: c.ServerTunnelAddress,
	}
}
