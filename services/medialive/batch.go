package medialive

// --- Batch operations ---

func (b *InMemoryBackend) batchSetState(
	channelIDs, multiplexIDs []string,
	state string,
) *BatchResult {
	var result BatchResult
	for _, id := range channelIDs {
		ch, ok := b.channels.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		ch.State = state
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: ch.ARN, State: ch.State},
		)
	}
	for _, id := range multiplexIDs {
		mx, ok := b.multiplexes.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		mx.State = state
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: mx.ARN, State: mx.State},
		)
	}

	return &result
}

// BatchStart starts channels/multiplexes in bulk (BatchStartInput has no
// inputIds field in the real API).
func (b *InMemoryBackend) BatchStart(
	channelIDs, multiplexIDs []string,
) (*BatchResult, error) {
	b.mu.Lock("BatchStart")
	defer b.mu.Unlock()

	return b.batchSetState(channelIDs, multiplexIDs, stateRunning), nil
}

// BatchStop stops channels/multiplexes in bulk (BatchStopInput has no
// inputIds field in the real API).
func (b *InMemoryBackend) BatchStop(
	channelIDs, multiplexIDs []string,
) (*BatchResult, error) {
	b.mu.Lock("BatchStop")
	defer b.mu.Unlock()

	return b.batchSetState(channelIDs, multiplexIDs, stateIdle), nil
}

// batchDeleteInputSecurityGroups deletes each requested input security
// group, appending to result. Split out of BatchDelete to keep that
// function's cyclomatic complexity down now that it handles four resource
// kinds.
func (b *InMemoryBackend) batchDeleteInputSecurityGroups(
	result *BatchResult,
	inputSecurityGroupIDs []string,
) {
	for _, id := range inputSecurityGroupIDs {
		g, ok := b.inputSecurityGroups.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		b.inputSecurityGroups.Delete(id)
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: g.ARN, State: g.State},
		)
	}
}

// BatchDelete deletes channels/inputs/multiplexes/input-security-groups in
// bulk.
func (b *InMemoryBackend) BatchDelete(
	channelIDs, inputIDs, multiplexIDs, inputSecurityGroupIDs []string,
) (*BatchResult, error) {
	b.mu.Lock("BatchDelete")
	defer b.mu.Unlock()
	var result BatchResult
	for _, id := range channelIDs {
		ch, ok := b.channels.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		if ch.State == stateRunning {
			result.Failed = append(
				result.Failed,
				BatchFailedResult{ID: id, Arn: ch.ARN, Code: "CONFLICT"},
			)

			continue
		}
		b.channels.Delete(id)
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: ch.ARN, State: stateDeleting},
		)
	}
	for _, id := range inputIDs {
		inp, ok := b.inputs.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		b.inputs.Delete(id)
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: inp.ARN, State: stateDeleted},
		)
	}
	for _, id := range multiplexIDs {
		mx, ok := b.multiplexes.Get(id)
		if !ok {
			result.Failed = append(result.Failed, BatchFailedResult{ID: id, Code: batchErrNotFound})

			continue
		}
		if mx.State == stateRunning {
			result.Failed = append(
				result.Failed,
				BatchFailedResult{ID: id, Arn: mx.ARN, Code: "CONFLICT"},
			)

			continue
		}
		b.multiplexes.Delete(id)
		result.Successful = append(
			result.Successful,
			BatchSuccessfulResult{ID: id, Arn: mx.ARN, State: stateDeleted},
		)
	}
	b.batchDeleteInputSecurityGroups(&result, inputSecurityGroupIDs)

	return &result, nil
}
