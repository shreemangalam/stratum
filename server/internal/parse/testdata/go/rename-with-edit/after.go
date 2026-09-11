package main

func computeSum(items []int) int {
	sum := 0
	for _, v := range items {
		sum += v * 2
	}
	return sum
}
