package apigatewaymanagementapi

// messageRing is a fixed-capacity circular buffer of PostedMessage values.
// Push is O(1); when full, the oldest entry is overwritten. Snapshot returns
// a flat slice in arrival order so callers can iterate as if it were a list.
type messageRing struct {
	items []PostedMessage
	head  int
	size  int
}

func newMessageRing(capacity int) *messageRing {
	if capacity <= 0 {
		capacity = 1
	}

	return &messageRing{items: make([]PostedMessage, capacity)}
}

func (r *messageRing) len() int { return r.size }

// push appends m, evicting the oldest entry once the buffer is full.
func (r *messageRing) push(m PostedMessage) {
	r.items[r.head] = m
	r.head = (r.head + 1) % len(r.items)

	if r.size < len(r.items) {
		r.size++
	}
}

// snapshot returns a copy of the messages in arrival order.
func (r *messageRing) snapshot() []PostedMessage {
	out := make([]PostedMessage, r.size)

	if r.size == 0 {
		return out
	}

	start := (r.head - r.size + len(r.items)) % len(r.items)
	for i := range r.size {
		out[i] = r.items[(start+i)%len(r.items)]
	}

	return out
}

// reset empties the buffer in O(1) without releasing the backing array.
func (r *messageRing) reset() {
	r.head = 0
	r.size = 0
}
