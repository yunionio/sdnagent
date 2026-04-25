package tc

import "sort"

func Sort[T IComparable](a []T) {
	sort.Slice(a, func(i, j int) bool {
		return a[i].Compare(a[j]) < 0
	})
}

func Split[T IComparable](a, b []T, baseCmp bool) (aNoB []T, aAndB []T, bAndA []T, bNoA []T) {
	i, j := 0, 0
	for i < len(a) && j < len(b) {
		var cmp int
		if baseCmp {
			cmp = a[i].CompareBase(b[j])
		} else {
			cmp = a[i].Compare(b[j])
		}
		if cmp == 0 {
			aAndB = append(aAndB, a[i])
			bAndA = append(bAndA, b[j])
			i++
			j++
		} else if cmp < 0 {
			aNoB = append(aNoB, a[i])
			i++
		} else {
			bNoA = append(bNoA, b[j])
			j++
		}
	}
	for i < len(a) {
		aNoB = append(aNoB, a[i])
		i++
	}
	for j < len(b) {
		bNoA = append(bNoA, b[j])
		j++
	}
	return
}
