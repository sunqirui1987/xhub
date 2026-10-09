// Command pricedata regenerates internal/catalog/publicdata/pricedata.json from
// the configured price feed.
//
// Run it when the feed changes a price or adds a model, then rebuild the
// gateway:
//
//	go run ./cmd/pricedata
//	go build ./cmd/gateway
//
// The conversion lives in internal/catalog, so this generator and the runtime
// reload in the console produce identical rows. There is deliberately no second
// implementation to drift from.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/sunqirui1987/xhub/internal/catalog"
	"github.com/sunqirui1987/xhub/internal/logx"
)

func main() {
	url := flag.String("url", catalog.MarketURL, "market feed to read")
	out := flag.String("out", "internal/catalog/publicdata/pricedata.json", "file to write")
	flag.Parse()

	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	doc, err := catalog.FetchMarket(ctx, *url)
	if err != nil {
		fmt.Fprintf(os.Stderr, "pricedata: %v\n", err)
		os.Exit(1)
	}
	raw, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "pricedata: %v\n", err)
		os.Exit(1)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "pricedata: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(*out, append(raw, '\n'), 0o644); err != nil {
		fmt.Fprintf(os.Stderr, "pricedata: %v\n", err)
		os.Exit(1)
	}
	logx.Info("pricedata wrote %s models=%d providers=%d", *out, len(doc.Models), len(doc.Providers))
	fmt.Printf("%s: %d models, %d providers\n", *out, len(doc.Models), len(doc.Providers))
}
