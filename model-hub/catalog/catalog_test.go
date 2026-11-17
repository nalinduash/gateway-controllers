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

package catalog

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"testing"
)

// These tests run against the definitions committed in ../providers.

const providersDir = "../providers"

// templateFiles maps each built-in Hub entry to its file in testdata/templates.
var templateFiles = map[string]string{
	"openai":          "openai-template.yaml",
	"anthropic":       "anthropic-template.yaml",
	"gemini":          "gemini-template.yaml",
	"mistralai":       "mistral-template.yaml",
	"awsbedrock":      "awsbedrock-template.yaml",
	"azure-openai":    "azureopenai-template.yaml",
	"azureai-foundry": "azureaifoundry-template.yaml",
	"typesafe":        "typesafe-template.yaml",
}

func loadRepo(t *testing.T) []Provider {
	t.Helper()
	providers, problems := Load(providersDir)
	if len(problems) > 0 {
		t.Fatalf("load problems: %v", problems)
	}
	return providers
}

func byName(t *testing.T, providers []Provider, name string) Provider {
	t.Helper()
	for _, p := range providers {
		if p.Definition.Metadata.Name == name {
			return p
		}
	}
	t.Fatalf("provider %s not found", name)
	return Provider{}
}

// normalize turns any value into plain JSON types so values decoded from YAML
// (int) and from JSON (float64) compare equal.
func normalize(t *testing.T, value any) any {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out any
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestRepoDefinitionsAreValid(t *testing.T) {
	providers := loadRepo(t)
	if len(providers) != 9 {
		t.Errorf("got %d providers, want the 8 built-ins and vllm", len(providers))
	}
	for _, p := range providers {
		if errs := Validate(p); len(errs) > 0 {
			t.Errorf("%s: %v", p.Dir, errs)
		}
	}
}

// The pricing file generated from the Hub must equal the gateway's pricing
// file: every entry, every field, every value.
func TestGeneratedPricingMatchesGatewayFile(t *testing.T) {
	generated, err := GeneratePricing(loadRepo(t))
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile("../testdata/model_prices.json")
	if err != nil {
		t.Fatal(err)
	}
	var want map[string]any
	if err := json.Unmarshal(data, &want); err != nil {
		t.Fatal(err)
	}

	got := normalize(t, generated).(map[string]any)
	if len(got) != len(want) {
		t.Errorf("generated %d entries, gateway file has %d", len(got), len(want))
	}
	for key, entry := range want {
		if !reflect.DeepEqual(got[key], entry) {
			t.Errorf("%s: generated %v, want %v", key, got[key], entry)
		}
	}
}

// Projecting a definition gives back the gateway's built-in template exactly.
func TestGatewayProjectionMatchesBuiltInTemplates(t *testing.T) {
	providers := loadRepo(t)
	for name, file := range templateFiles {
		t.Run(name, func(t *testing.T) {
			var want map[string]any
			readFixture(t, filepath.Join("gateway", file), &want)
			got := ToGatewayTemplate(byName(t, providers, name).Definition)
			if !reflect.DeepEqual(normalize(t, got), normalize(t, want)) {
				t.Errorf("projection differs from the gateway template\n got: %v\nwant: %v", normalize(t, got), normalize(t, want))
			}
		})
	}
}

// platformDrift lists fields where Platform API's built-in template has fallen
// behind the gateway's copy. The definitions follow the gateway copy, so these
// are fixed in Platform API once its templates are generated from the Hub.
var platformDrift = map[string][]string{
	// The gateway copy names it MistralAI; Platform API says Mistral.
	"mistralai": {"displayName"},
	// The gateway copy also accepts URL-encoded model IDs (%3A).
	"awsbedrock": {"requestModel", "responseModel"},
	// Same character class, written differently.
	"gemini": {"requestModel"},
}

// Projecting a definition gives back Platform API's built-in template. Two
// differences are intended: Platform API assigns groupId itself (it is
// read-only in its API), and the Hub adds a description.
func TestPlatformProjectionMatchesBuiltInTemplates(t *testing.T) {
	providers := loadRepo(t)
	for name, file := range templateFiles {
		t.Run(name, func(t *testing.T) {
			var fixture struct {
				Metadata struct {
					Name string `yaml:"name"`
				} `yaml:"metadata"`
				Spec map[string]any `yaml:"spec"`
			}
			readFixture(t, filepath.Join("platform", file), &fixture)
			want := normalize(t, fixture.Spec).(map[string]any)
			delete(want, "groupId")
			want["id"] = fixture.Metadata.Name

			got := normalize(t, ToPlatformTemplate(byName(t, providers, name).Definition)).(map[string]any)
			delete(got, "description")
			for _, key := range platformDrift[name] {
				if reflect.DeepEqual(got[key], want[key]) {
					t.Errorf("%s no longer drifts; remove it from platformDrift", key)
				}
				delete(got, key)
				delete(want, key)
			}
			for _, key := range sortedKeys(want) {
				if !reflect.DeepEqual(got[key], want[key]) {
					t.Errorf("%s: projection has %v, Platform API template has %v", key, got[key], want[key])
				}
			}
			for key := range got {
				if _, ok := want[key]; !ok {
					t.Errorf("%s: projection has %v, Platform API template has none", key, got[key])
				}
			}
		})
	}
}

// Every pricing field the llm-cost policy reads must be one the validator
// accepts, or a valid price could be rejected.
func TestPricingFieldsCoverLLMCostPolicy(t *testing.T) {
	source, err := os.ReadFile("../../policies/llm-cost/pricing.go")
	if err != nil {
		t.Skip("llm-cost policy not found next to model-hub")
	}
	tags := regexp.MustCompile(`json:"([a-z0-9_]+)`).FindAllStringSubmatch(string(source), -1)
	if len(tags) == 0 {
		t.Fatal("no json tags found in pricing.go")
	}
	for _, tag := range tags {
		field := tag[1]
		known := field == "provider" || field == "mode" ||
			slices.Contains(PricingNumberFields, field) || slices.Contains(PricingMapFields, field)
		if !known {
			t.Errorf("llm-cost reads pricing field %q, which the validator rejects", field)
		}
	}
}

func readFixture(t *testing.T, name string, out any) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../testdata/templates", name))
	if err != nil {
		t.Fatal(err)
	}
	if err := UnmarshalYAML(data, out); err != nil {
		t.Fatal(err)
	}
}
