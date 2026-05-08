package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dodwyer/scion-test/internal/validator"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stderr))
}

func run(args []string, stdin *os.File, stderr io.Writer) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: redis-cr-validator [path|-]")
		return 2
	}

	data, err := readInput(args, stdin)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: %v\n", err)
		return 2
	}

	errs, err := validator.ParseAndValidate(data)
	if err != nil {
		fmt.Fprintf(stderr, "ERROR: parse error: %v\n", err)
		return 2
	}
	if len(errs) > 0 {
		for _, validationErr := range errs {
			fmt.Fprintf(stderr, "ERROR: %s: %s\n", validationErr.Field, validationErr.Reason)
		}
		return 1
	}
	return 0
}

func readInput(args []string, stdin *os.File) ([]byte, error) {
	if len(args) == 1 && args[0] != "-" {
		return os.ReadFile(args[0])
	}
	if len(args) == 1 && args[0] == "-" {
		return io.ReadAll(stdin)
	}

	info, err := stdin.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return nil, fmt.Errorf("no input provided")
	}
	return io.ReadAll(stdin)
}
