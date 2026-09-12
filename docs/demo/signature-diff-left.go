package calc

import "errors"

type Config struct {
	Precision int
	RoundUp   bool
}

func Validate(s string) error {
	if len(s) == 0 {
		return errors.New("empty")
	}
	return nil
}

func Process(c Config, input string) string {
	if err := Validate(input); err != nil {
		return ""
	}
	result := "[" + input + "]"
	return result
}

func FormatOutput(s string) string {
	return "out: " + s
}
