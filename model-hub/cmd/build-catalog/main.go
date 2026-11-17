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

// Command build-catalog is the publish step: it validates every definition and
// writes the bundle the Hub API serves, plus the generated pricing file.
//
//	go run ./cmd/build-catalog -providers providers -out dist
//
// It writes dist/catalog.json, dist/model_prices.json and dist/icons/*.png.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/wso2/gateway-controllers/model-hub/catalog"
)

func main() {
	root := flag.String("providers", "providers", "folder holding <name>/v<X.Y>/ definitions")
	out := flag.String("out", "dist", "folder to write the bundle to")
	flag.Parse()
	if err := run(*root, *out); err != nil {
		log.Fatal(err)
	}
}

func run(root, out string) error {
	providers, problems := catalog.Load(root)
	if len(problems) > 0 {
		return fmt.Errorf("cannot publish: %v", problems)
	}
	bundle, err := catalog.BuildBundle(providers, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return fmt.Errorf("cannot publish:\n%w", err)
	}
	pricing, err := catalog.GeneratePricing(providers)
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(out, "icons"), 0o755); err != nil {
		return err
	}
	for _, p := range providers {
		if p.Icon == nil {
			continue
		}
		name := catalog.IconFileName(p.Definition.Metadata.Name, p.Definition.Metadata.Version)
		if err := os.WriteFile(filepath.Join(out, "icons", name), p.Icon, 0o644); err != nil {
			return err
		}
	}
	if err := writeJSON(filepath.Join(out, "catalog.json"), bundle); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(out, "model_prices.json"), pricing); err != nil {
		return err
	}
	fmt.Printf("published %d provider versions and %d models to %s\n", len(bundle.Providers), len(bundle.Models), out)
	return nil
}

func writeJSON(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
