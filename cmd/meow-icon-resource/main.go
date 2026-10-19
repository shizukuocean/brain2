package main

import (
	"errors"
	"flag"
	"fmt"
	"image/png"
	"os"
	"path/filepath"

	"github.com/tc-hib/winres"
)

func main() {
	input := flag.String("input", "", "source PNG path")
	output := flag.String("output", "", "target Windows amd64 .syso path")
	flag.Parse()
	if err := buildResource(*input, *output); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func buildResource(input, output string) error {
	if input == "" || output == "" {
		return errors.New("input and output are required")
	}

	source, err := os.Open(filepath.Clean(input))
	if err != nil {
		return fmt.Errorf("open application icon: %w", err)
	}
	image, decodeErr := png.Decode(source)
	closeErr := source.Close()
	if decodeErr != nil {
		return fmt.Errorf("decode application icon: %w", decodeErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close application icon: %w", closeErr)
	}

	icon, err := winres.NewIconFromResizedImage(image, []int{16, 24, 32, 48, 64, 128, 256})
	if err != nil {
		return fmt.Errorf("resize application icon: %w", err)
	}
	resources := winres.ResourceSet{}
	if err := resources.SetIcon(winres.Name("APPICON"), icon); err != nil {
		return fmt.Errorf("set application icon: %w", err)
	}

	target := filepath.Clean(output)
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create resource directory: %w", err)
	}
	file, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return fmt.Errorf("create application resource: %w", err)
	}
	if err := resources.WriteObject(file, winres.ArchAMD64); err != nil {
		_ = file.Close()
		_ = os.Remove(target)
		return fmt.Errorf("write application resource: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(target)
		return fmt.Errorf("close application resource: %w", err)
	}
	return nil
}
