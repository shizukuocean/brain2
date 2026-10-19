package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"meow-launcher/internal/installerbundle"
)

func main() {
	runner := flag.String("runner", "", "absolute path to meow-product-installer runner")
	payload := flag.String("payload", "", "absolute payload directory")
	output := flag.String("output", "", "absolute output installer path")
	productID := flag.String("product", "", "canonical product_id")
	version := flag.String("version", "", "release version")
	entrypoint := flag.String("entrypoint", "", "payload-relative product executable")
	selfUpdate := flag.Bool("self-update", false, "build a Launcher self-update package")
	flag.Parse()
	manifest, err := installerbundle.Build(installerbundle.BuildOptions{
		RunnerPath: *runner, PayloadRoot: *payload, OutputPath: *output, ProductID: *productID,
		Version: *version, ExecutableRelativePath: *entrypoint, SelfUpdate: *selfUpdate,
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	encoded, _ := json.Marshal(manifest)
	fmt.Println(string(encoded))
}
