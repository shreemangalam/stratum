package main

func FormatOutput(data string) string {
	return "[" + data + "]"
}

func TransformData(input string) string {
	return input + "_transformed"
}

func CheckInput(s string) bool {
	return len(s) > 0 && s != ""
}
