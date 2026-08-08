package slicesx

// Unique returns in with duplicate elements removed, preserving first-seen order.
func Unique[T comparable](in []T) []T {
	if len(in) == 0 {
		return nil
	}
	seen := make(map[T]struct{}, len(in))
	out := make([]T, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// AppendIfMissing appends v to slice when v is not already present.
func AppendIfMissing[T comparable](slice []T, v T) []T {
	for _, existing := range slice {
		if existing == v {
			return slice
		}
	}
	return append(slice, v)
}
