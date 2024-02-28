package sliceutils

import (
	"golang.org/x/exp/maps"
)

// Merge slices, returns a slice without duplicates, the order is not guaranteed
func UniqueMergeSlices[T comparable](slice []T, elems ...T) []T {
	items := make(map[T]struct{}, len(slice)+len(elems))
	for _, item := range slice {
		if _, ok := items[item]; !ok {
			items[item] = struct{}{}
		}
	}
	for _, item := range elems {
		if _, ok := items[item]; !ok {
			items[item] = struct{}{}
		}
	}

	return maps.Keys(items)
}

// Returns true if the slices are equal, does not take into account the order
func IsSlicesEqual[T comparable](s1, s2 []T) bool {
	if len(s1) != len(s2) {
		return false
	}
	if s1 == nil && s2 == nil {
		return true
	}
	diff := make(map[T]int, len(s1))
	for _, _x := range s1 {
		diff[_x]++
	}
	for _, _y := range s2 {
		if _, ok := diff[_y]; !ok {
			return false
		}
		diff[_y]--
		if diff[_y] == 0 {
			delete(diff, _y)
		}
	}
	return len(diff) == 0
}

func RemoveDuplicates[T comparable](slice []T) []T {
	items := make(map[T]struct{}, len(slice))
	for _, item := range slice {
		if _, ok := items[item]; !ok {
			items[item] = struct{}{}
		}
	}
	return maps.Keys(items)
}

func RemoveDuplicatesFunc[T any](slice []T, equals func(T, T) bool) []T {
	unique := make([]T, 0, len(slice))
	for i := range slice {
		skip := false
		for j := range unique {
			if equals(slice[i], unique[j]) {
				skip = true
				break
			}
		}
		if !skip {
			unique = append(unique, slice[i])
		}
	}

	return unique
}

func Compare[T comparable](slice []T, element T) bool {
	for i := range slice {
		if slice[i] == element {
			return true
		}
	}
	return false
}

func CompareFunc[T any, E any](slice []T, element E, compareFunc func(T, E) bool) bool {
	for i := range slice {
		if compareFunc(slice[i], element) {
			return true
		}
	}
	return false
}
