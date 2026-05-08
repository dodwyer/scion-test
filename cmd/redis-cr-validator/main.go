package main

import (
	"fmt"
	"io"
	"os"

	"github.com/dodwyer/scion-test/internal/validator"
)

func main() {
	input, code := openInput(os.Args[1:])
	if code != 0 {
		os.Exit(code)
	}

	data, err := io.ReadAll(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: reading input: %v\n", err)
		os.Exit(2)
	}

	errs, parseErr := validator.ParseAndValidate(data)
	if parseErr != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", parseErr)
		os.Exit(2)
	}

	if len(errs) > 0 {
		for _, e := range errs {
			fmt.Fprintln(os.Stderr, e.Error())
		}
		os.Exit(1)
	}
}

// openInput returns the reader to validate and an exit code (0 = ok, 2 = fatal).
func openInput(args []string) (io.Reader, int) {
	if len(args) == 0 {
		stat, err := os.Stdin.Stat()
		if err != nil || (stat.Mode()&os.ModeCharDevice) != 0 {
			fmt.Fprintln(os.Stderr, "Usage: redis-cr-validator <file.yaml>")
			fmt.Fprintln(os.Stderr, "       redis-cr-validator -   (read from stdin)")
			return nil, 2
		}
		return os.Stdin, 0
	}
	if args[0] == "-" {
		return os.Stdin, 0
	}
	f, err := os.Open(args[0])
	if err != nil {
		fmt.Fprintf(os.Stderr, "ERROR: %v\n", err)
		return nil, 2
	}
	return f, 0
}
