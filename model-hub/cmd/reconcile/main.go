/*
 * Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
 *
 * WSO2 LLC. licenses this file to you under the Apache License,
 * Version 2.0 (the "License"); you may not use this file except
 * in compliance with the License.
 * You may obtain a copy of the License at
 *
 * http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied.  See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

// Command reconcile compares the gateway's pricing file with LiteLLM's and
// writes a report for a maintainer. It never writes the pricing file.
//
//	go run ./cmd/reconcile -ours testdata/model_prices.json \
//	    -litellm ../../litellm/model_prices_and_context_window.json -out dist
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/wso2/gateway-controllers/model-hub/reconcile"
)

// maxPricingFileBytes bounds what is read into memory. LiteLLM's file is about 4 MB.
const maxPricingFileBytes = 64 << 20

func main() {
	ours := flag.String("ours", "", "the gateway's model_prices.json")
	litellm := flag.String("litellm", "", "LiteLLM's model_prices_and_context_window.json")
	out := flag.String("out", "dist", "folder to write reconcile-report.md and reconcile-report.json to")
	threshold := flag.Float64("threshold", 20, "a price change above this percent needs approval")
	flag.Parse()
	if *ours == "" || *litellm == "" {
		log.Fatal("-ours and -litellm are required")
	}
	if err := run(*ours, *litellm, *out, *threshold); err != nil {
		log.Fatal(err)
	}
}

func run(oursPath, litellmPath, out string, threshold float64) error {
	ours, err := readPricing(oursPath)
	if err != nil {
		return err
	}
	litellm, err := readPricing(litellmPath)
	if err != nil {
		return err
	}
	report := reconcile.Compare(ours, litellm, threshold)

	if err := os.MkdirAll(out, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(report, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "reconcile-report.json"), append(data, '\n'), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(out, "reconcile-report.md"), []byte(report.Markdown(threshold)), 0o644); err != nil {
		return err
	}
	fmt.Printf("agree %d, differ %d, only LiteLLM %d (+%d other providers), only ours %d, deprecations %d\n",
		len(report.Agree), len(report.Differ), len(report.OnlyLiteLLM), report.OtherProvidersSkipped,
		len(report.OnlyOurs), len(report.Deprecations))
	return nil
}

// readPricing reads a pricing file. Entries that are not objects (LiteLLM keeps
// a few aliases as strings) are skipped.
func readPricing(path string) (map[string]map[string]any, error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if info.Size() > maxPricingFileBytes {
		return nil, fmt.Errorf("%s is larger than %d bytes", path, maxPricingFileBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	out := make(map[string]map[string]any, len(raw))
	for key, msg := range raw {
		var entry map[string]any
		if json.Unmarshal(msg, &entry) == nil && entry != nil {
			out[key] = entry
		}
	}
	return out, nil
}
