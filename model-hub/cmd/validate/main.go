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

// Command validate checks every definition under a providers folder and exits
// non-zero when any is invalid. A pull request runs it before review.
//
//	go run ./cmd/validate -providers providers
package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/wso2/gateway-controllers/model-hub/catalog"
)

func main() {
	root := flag.String("providers", "providers", "folder holding <name>/v<X.Y>/ definitions")
	flag.Parse()

	failed := 0
	providers, problems := catalog.Load(*root)
	for _, err := range problems {
		fmt.Fprintln(os.Stderr, "ERROR", err)
		failed++
	}
	for _, p := range providers {
		errs := catalog.Validate(p)
		for _, err := range errs {
			fmt.Fprintf(os.Stderr, "ERROR %s: %v\n", p.Dir, err)
		}
		if len(errs) > 0 {
			failed++
			continue
		}
		fmt.Printf("ok    %s v%s (%d models)\n", p.Definition.Metadata.Name, p.Definition.Metadata.Version, len(p.Models.Models))
	}
	// Each model name may appear once across providers, or the generated
	// pricing file would be ambiguous.
	if _, err := catalog.GeneratePricing(providers); err != nil {
		fmt.Fprintln(os.Stderr, "ERROR", err)
		failed++
	}

	if failed > 0 {
		fmt.Fprintf(os.Stderr, "%d problem(s) found\n", failed)
		os.Exit(1)
	}
}
