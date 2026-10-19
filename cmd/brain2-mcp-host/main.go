package main

import (
	"fmt"
	"os"

	"meow-launcher/internal/brain2"
)

func main() {
	mode, target, err := targetFromArgs(os.Args[1:])
	if err == nil && mode == "registry" {
		err = brain2.ServeMCPRegistry(target, os.Stdin, os.Stdout)
	} else if err == nil {
		err = brain2.ServeMCP(target, os.Stdin, os.Stdout)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "Brain2 MCP failed:", err)
		os.Exit(1)
	}
}

func targetFromArgs(args []string) (string, string, error) {
	for i := 0; i < len(args); i++ {
		if args[i] == "--registry" && i+1 < len(args) {
			return "registry", args[i+1], nil
		}
		if args[i] == "--root" && i+1 < len(args) {
			return "root", args[i+1], nil
		}
	}
	return "", "", fmt.Errorf("brain2-mcp-host requires --registry <Launcher registry path>")
}
