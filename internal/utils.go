package utils

// MapFunc creates an array of values by running each element in the collection through the `iteratee`.
// For example, if you want to get square of an array of numbers, we can use this as:
//
//	result = MapFunc([]int{2, 3}, func(n int) int {
//			return n*n
//	})
//
// Then result will be [4, 9].
func MapFunc[T any, R any](xs []T, iteratee func(v T) R) []R {
	var result []R

	for _, x := range xs {
		result = append(result, iteratee(x))
	}

	return result
}
