package metrics

type sampleRing[T any] struct {
	samples []T
	head    int
	count   int
}

func (r *sampleRing[T]) push(sample T) {
	r.samples[r.head] = sample
	r.head = (r.head + 1) % len(r.samples)
	if r.count < len(r.samples) {
		r.count++
	}
}

// lastN returns up to n samples, ordered from newest to oldest.
func (r sampleRing[T]) lastN(n int) []T {
	available := min(n, r.count)
	result := make([]T, available)
	for i := range available {
		idx := (r.head - 1 - i + len(r.samples)) % len(r.samples)
		result[i] = r.samples[idx]
	}
	return result
}

// Helpers

func newSampleRing[T any](size int) sampleRing[T] {
	return sampleRing[T]{samples: make([]T, max(size, 1))}
}
