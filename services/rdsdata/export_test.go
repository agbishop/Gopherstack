package rdsdata

import "time"

// SetTransactionTimes backdates a transaction's CreatedAt/LastActivityAt in
// the backend's default region, for Janitor expiry tests.
func SetTransactionTimes(b *InMemoryBackend, txID string, createdAt, lastActivityAt time.Time) {
	b.mu.Lock("SetTransactionTimes")
	defer b.mu.Unlock()

	if tx, ok := b.transactionsStore(b.defaultRegion).Get(txID); ok {
		tx.CreatedAt = createdAt
		tx.LastActivityAt = lastActivityAt
	}
}

// EngineHasTx reports whether the engine has a live *sql.Tx for txID.
func EngineHasTx(b *InMemoryBackend) func(txID string) bool {
	return func(txID string) bool {
		b.engine.mu.Lock()
		defer b.engine.mu.Unlock()

		_, ok := b.engine.txs[txID]

		return ok
	}
}

// ExecutedStatementCount returns the number of executed statements stored in the backend's default region.
func ExecutedStatementCount(b *InMemoryBackend) int {
	b.mu.RLock("ExecutedStatementCount")
	defer b.mu.RUnlock()

	return len(b.executedStatements[b.defaultRegion])
}

// TransactionCount returns the number of active transactions in the backend's default region.
func TransactionCount(b *InMemoryBackend) int {
	b.mu.RLock("TransactionCount")
	defer b.mu.RUnlock()

	tbl := b.transactions[b.defaultRegion]
	if tbl == nil {
		return 0
	}

	return tbl.Len()
}

// HandlerOpsLen returns the number of operations in GetSupportedOperations.
func HandlerOpsLen(h *Handler) int {
	return len(h.GetSupportedOperations())
}
