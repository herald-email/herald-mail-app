package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/herald-email/herald-mail-app/internal/testmail"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string) error {
	fs := flag.NewFlagSet("testmail-sanitize", flag.ContinueOnError)
	inputPath := fs.String("in", "", "raw quarantine .eml file to sanitize")
	outputPath := fs.String("out", "", "sanitized output .eml file")
	validatePath := fs.String("validate", "", "corpus directory to validate")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *validatePath != "" {
		if err := testmail.ValidateCorpus(*validatePath); err != nil {
			return fmt.Errorf("validate corpus: %w", err)
		}
	}
	if *inputPath == "" && *outputPath == "" {
		if *validatePath != "" {
			return nil
		}
		return fmt.Errorf("usage: testmail-sanitize -in reports/quarantine/mail/raw.eml -out internal/testmail/testdata/corpus/<case>/message.eml [-validate internal/testmail/testdata/corpus]")
	}
	if *inputPath == "" || *outputPath == "" {
		return fmt.Errorf("-in and -out must be provided together")
	}

	raw, err := os.ReadFile(*inputPath)
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}
	sanitized := testmail.SanitizeBytes(raw)
	if err := testmail.ValidateSanitizedBytes(*outputPath, sanitized); err != nil {
		return fmt.Errorf("sanitized output failed validation: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(*outputPath), 0o755); err != nil {
		return fmt.Errorf("create output directory: %w", err)
	}
	if err := os.WriteFile(*outputPath, sanitized, 0o644); err != nil {
		return fmt.Errorf("write output: %w", err)
	}
	return nil
}
