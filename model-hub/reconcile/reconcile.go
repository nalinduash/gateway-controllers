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

// Package reconcile compares the gateway's pricing file with LiteLLM's and
// sorts every model into a bucket. It only reports: it never changes a price,
// never deletes a model and never picks a side when prices differ.
package reconcile

import (
	"math"
	"slices"
	"sort"
	"strings"

	"github.com/wso2/gateway-controllers/model-hub/catalog"
)

// providerAliases maps LiteLLM provider names to ours. Our names are kept,
// because the llm-cost policy picks its fee logic from them.
var providerAliases = map[string]string{
	"bedrock_converse":       "bedrock",
	"text-completion-openai": "openai",
	"hosted_vllm":            "vllm",
}

// firstWave are the providers (in our names) whose LiteLLM-only models are
// worth importing: the 8 built-in templates and vLLM.
var firstWave = []string{"openai", "anthropic", "gemini", "mistral", "bedrock", "azure", "azure_text", "azure_ai", "typesafe", "vllm"}

// Report is the result of one comparison.
type Report struct {
	// Agree are models whose prices match. This is "cross-checked", weaker
	// than verified: both files could carry the same old value.
	Agree []string `json:"agree"`
	// Differ are models whose prices disagree. A person decides each one.
	Differ []Difference `json:"differ"`
	// OnlyLiteLLM are first-wave models we do not have, to import unverified.
	OnlyLiteLLM []Candidate `json:"onlyLiteLLM"`
	// OtherProvidersSkipped counts LiteLLM-only models outside the first wave.
	OtherProvidersSkipped int `json:"otherProvidersSkipped"`
	// OnlyOurs are models LiteLLM does not list: check whether they retired.
	OnlyOurs []string `json:"onlyOurs"`
	// Deprecations are retirement dates LiteLLM records for our models.
	Deprecations []Deprecation `json:"deprecations"`
	// ProviderNames are models whose provider name differs only by spelling.
	ProviderNames []NameDifference `json:"providerNames"`
}

// Difference is one model whose prices disagree.
type Difference struct {
	Model  string      `json:"model"`
	Fields []FieldDiff `json:"fields"`
}

// FieldDiff is one price field that disagrees.
type FieldDiff struct {
	Field  string   `json:"field"`
	Ours   *float64 `json:"ours"`
	Theirs *float64 `json:"theirs"`
	// ChangePercent is how far LiteLLM's value is from ours. It is absent when
	// one side has no value.
	ChangePercent *float64 `json:"changePercent,omitempty"`
	// NeedsApproval is set when the change is above the threshold or one side
	// is missing.
	NeedsApproval bool `json:"needsApproval"`
}

// Candidate is a LiteLLM-only model.
type Candidate struct {
	Model    string `json:"model"`
	Provider string `json:"provider"`
}

// Deprecation is a retirement date LiteLLM records.
type Deprecation struct {
	Model     string `json:"model"`
	Date      string `json:"date"`
	Source    string `json:"source,omitempty"`
	Successor string `json:"successor,omitempty"`
}

// NameDifference is a model whose provider name is spelled differently.
type NameDifference struct {
	Model  string `json:"model"`
	Ours   string `json:"ours"`
	Theirs string `json:"theirs"`
}

// Compare sorts every model into a bucket. ours is model_prices.json, litellm
// is LiteLLM's model_prices_and_context_window.json. A price change above
// thresholdPercent is marked as needing approval.
func Compare(ours, litellm map[string]map[string]any, thresholdPercent float64) Report {
	r := Report{Agree: []string{}, Differ: []Difference{}, OnlyLiteLLM: []Candidate{}, OnlyOurs: []string{},
		Deprecations: []Deprecation{}, ProviderNames: []NameDifference{}}

	for _, model := range sortedKeys(ours) {
		entry := ours[model]
		theirs, ok := litellm[model]
		if !ok || theirs["litellm_provider"] == nil {
			r.OnlyOurs = append(r.OnlyOurs, model)
			continue
		}

		ourProvider, _ := entry["provider"].(string)
		theirProvider, _ := theirs["litellm_provider"].(string)
		if ourProvider != theirProvider {
			r.ProviderNames = append(r.ProviderNames, NameDifference{Model: model, Ours: ourProvider, Theirs: theirProvider})
		}
		if date, _ := theirs["deprecation_date"].(string); date != "" {
			source, _ := theirs["deprecation_source"].(string)
			successor, _ := theirs["successor_model"].(string)
			r.Deprecations = append(r.Deprecations, Deprecation{Model: model, Date: date, Source: source, Successor: successor})
		}

		if diffs := comparePrices(entry, theirs, thresholdPercent); len(diffs) > 0 {
			r.Differ = append(r.Differ, Difference{Model: model, Fields: diffs})
		} else {
			r.Agree = append(r.Agree, model)
		}
	}

	for _, model := range sortedKeys(litellm) {
		if _, ok := ours[model]; ok {
			continue
		}
		provider, _ := litellm[model]["litellm_provider"].(string)
		if provider == "" {
			continue // an alias or the sample spec, not a model
		}
		provider = normalize(provider)
		if isFirstWave(provider) {
			r.OnlyLiteLLM = append(r.OnlyLiteLLM, Candidate{Model: model, Provider: provider})
		} else {
			r.OtherProvidersSkipped++
		}
	}
	return r
}

// comparePrices compares every price field we carry. A field only LiteLLM
// carries is not compared: the llm-cost policy would not read it.
func comparePrices(ours, theirs map[string]any, thresholdPercent float64) []FieldDiff {
	var diffs []FieldDiff
	for _, field := range sortedKeys(ours) {
		if !isPriceField(field) {
			continue
		}
		a, okA := number(ours[field])
		b, okB := number(theirs[field])
		if !okA {
			continue
		}
		if !okB {
			diffs = append(diffs, FieldDiff{Field: field, Ours: &a, NeedsApproval: true})
			continue
		}
		if equal(a, b) {
			continue
		}
		d := FieldDiff{Field: field, Ours: &a, Theirs: &b}
		if a != 0 {
			// Rounded to 0.01%, so float noise cannot cross the threshold.
			change := math.Round(math.Abs(b-a)/a*10000) / 100
			d.ChangePercent = &change
			d.NeedsApproval = change > thresholdPercent
		} else {
			d.NeedsApproval = true
		}
		diffs = append(diffs, d)
	}
	return diffs
}

func isPriceField(field string) bool {
	return slices.Contains(catalog.PricingNumberFields, field) && !strings.HasPrefix(field, "max_")
}

func number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int:
		return float64(n), true
	}
	return 0, false
}

// equal compares two prices, ignoring float noise from decimal conversion.
func equal(a, b float64) bool {
	if a == b {
		return true
	}
	return math.Abs(a-b) <= 1e-9*math.Max(math.Abs(a), math.Abs(b))
}

func normalize(provider string) string {
	if ours, ok := providerAliases[provider]; ok {
		return ours
	}
	return provider
}

func isFirstWave(provider string) bool {
	return slices.Contains(firstWave, provider) || provider == "vertex_ai" || strings.HasPrefix(provider, "vertex_ai-")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
