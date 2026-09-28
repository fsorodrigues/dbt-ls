package utils

// Map transforms a slice of type T into a slice of type R
func Map[T any, R any](collection []T, iteratee func(T) R) []R {
	result := make([]R, len(collection))
	for i, item := range collection {
		result[i] = iteratee(item)
	}
	return result
}
