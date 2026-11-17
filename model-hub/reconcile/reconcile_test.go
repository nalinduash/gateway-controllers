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

package reconcile

import (
	"reflect"
	"strings"
	"testing"
)

type entry = map[string]any

var ours = map[string]entry{
	"same":              {"provider": "openai", "input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06, "max_tokens": 100},
	"small-shift":       {"provider": "openai", "input_cost_per_token": 1e-06},
	"big-shift":         {"provider": "anthropic", "input_cost_per_token": 1e-06},
	"dropped":           {"provider": "openai", "input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06},
	"retired":           {"provider": "mistral", "input_cost_per_token": 1e-06},
	"claude-on-bedrock": {"provider": "bedrock", "input_cost_per_token": 3e-06},
}

var litellm = map[string]entry{
	// Different max_tokens and an extra field we do not carry: still agree.
	"same":              {"litellm_provider": "openai", "input_cost_per_token": 1e-06, "output_cost_per_token": 2e-06, "max_tokens": 999, "fedramp_uplift_multiplier": 1.1},
	"small-shift":       {"litellm_provider": "openai", "input_cost_per_token": 1.1e-06},
	"big-shift":         {"litellm_provider": "anthropic", "input_cost_per_token": 2e-06, "deprecation_date": "2027-01-01", "deprecation_source": "https://example.com/deprecations", "successor_model": "big-shift-2"},
	"dropped":           {"litellm_provider": "openai", "input_cost_per_token": 1e-06},
	"claude-on-bedrock": {"litellm_provider": "bedrock_converse", "input_cost_per_token": 3e-06},
	"new-openai":        {"litellm_provider": "openai", "input_cost_per_token": 1e-06},
	"new-vertex":        {"litellm_provider": "vertex_ai-language-models", "input_cost_per_token": 1e-06},
	"new-vllm":          {"litellm_provider": "hosted_vllm", "input_cost_per_token": 0},
	"new-other":         {"litellm_provider": "fireworks_ai", "input_cost_per_token": 1e-06},
	"alias":             {"input_cost_per_token": 1e-06},
}

func TestCompareSortsIntoBuckets(t *testing.T) {
	r := Compare(ours, litellm, 20)

	check := func(name string, got, want any) {
		t.Helper()
		if !reflect.DeepEqual(got, want) {
			t.Errorf("%s: got %v, want %v", name, got, want)
		}
	}
	check("agree", r.Agree, []string{"claude-on-bedrock", "same"})
	check("only ours", r.OnlyOurs, []string{"retired"})
	check("only LiteLLM", r.OnlyLiteLLM, []Candidate{
		{Model: "new-openai", Provider: "openai"},
		{Model: "new-vertex", Provider: "vertex_ai-language-models"},
		{Model: "new-vllm", Provider: "vllm"},
	})
	check("other providers", r.OtherProvidersSkipped, 1)
	check("deprecations", r.Deprecations, []Deprecation{
		{Model: "big-shift", Date: "2027-01-01", Source: "https://example.com/deprecations", Successor: "big-shift-2"},
	})
	check("provider names", r.ProviderNames, []NameDifference{{Model: "claude-on-bedrock", Ours: "bedrock", Theirs: "bedrock_converse"}})

	differ := map[string][]FieldDiff{}
	for _, d := range r.Differ {
		differ[d.Model] = d.Fields
	}
	if len(differ) != 3 {
		t.Fatalf("got differ %v, want big-shift, dropped and small-shift", r.Differ)
	}
	if f := differ["small-shift"][0]; f.NeedsApproval || *f.ChangePercent < 9.9 || *f.ChangePercent > 10.1 {
		t.Errorf("a 10%% change under a 20%% threshold: %+v", f)
	}
	if f := differ["big-shift"][0]; !f.NeedsApproval {
		t.Errorf("a 100%% change must need approval: %+v", f)
	}
	if f := differ["dropped"][0]; f.Field != "output_cost_per_token" || f.Theirs != nil || !f.NeedsApproval {
		t.Errorf("a price LiteLLM lacks must need approval: %+v", f)
	}
}

// The comparison must never change its inputs: it reports, a person decides.
func TestCompareDoesNotModifyInputs(t *testing.T) {
	before := len(ours["same"])
	Compare(ours, litellm, 20)
	if len(ours["same"]) != before || ours["big-shift"]["input_cost_per_token"] != 1e-06 {
		t.Error("Compare changed our pricing data")
	}
}

func TestMarkdownCarriesAttributionAndCounts(t *testing.T) {
	md := Compare(ours, litellm, 20).Markdown(20)
	for _, want := range []string{"MIT-licensed", "| Prices agree | 2 |", "| Prices differ | 3 |", "`big-shift` | input_cost_per_token | 1e-06 | 2e-06 | 100% | yes |"} {
		if !strings.Contains(md, want) {
			t.Errorf("report is missing %q", want)
		}
	}
}
