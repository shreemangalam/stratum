package calc

import (
	"errors"
	"log"
)

type Config struct {
	Precision int
	RoundUp   bool
	Verbose   bool
}

func FormatOutput(s string) string {
	return "out: " + s
}

func CheckInput(s string) error {
	if len(s) == 0 {
		return errors.New("empty")
	}
	if len(s) > 1024 {
		return errors.New("too long")
	}
	return nil
}

func Process(c Config, input string) string {
	if err := CheckInput(input); err != nil {
		log.Printf("invalid: %v", err)
		return ""
	}
	result := "[" + input + "]"
	return result
}
