// Read standard YAML using the repository's existing YAML dependency.
// Only non-secret supplier metadata is accepted and emitted.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/sunqirui1987/xhub/internal/providerconfig"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: providerconfig CONFIG.yaml")
		os.Exit(1)
	}
	f, err := os.Open(os.Args[1])
	if err != nil {
		fmt.Fprintln(os.Stderr, "Cannot open provider configuration")
		os.Exit(1)
	}
	defer f.Close()
	cfg, err := providerconfig.Load(f)
	if err != nil {
		fmt.Fprintln(os.Stderr, "Invalid provider configuration; use the documented fields and do not put secrets in YAML")
		os.Exit(1)
	}
	if err = json.NewEncoder(os.Stdout).Encode(cfg); err != nil {
		os.Exit(1)
	}
}
