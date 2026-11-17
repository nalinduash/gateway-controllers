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
	"fmt"
	"slices"
	"strings"
)

// PricingProviders are the provider values the llm-cost policy picks its fee
// logic from. A model keeps the value exactly; renaming one changes the cost.
var PricingProviders = []string{
	"openai", "anthropic", "gemini", "mistral", "bedrock",
	"azure", "azure_text", "azure_ai", "typesafe",
}

// IsPricingProvider reports whether value is a known pricing provider. The
// vertex_ai family shares the Gemini calculator under several names.
func IsPricingProvider(value string) bool {
	return slices.Contains(PricingProviders, value) || value == "vertex_ai" || strings.HasPrefix(value, "vertex_ai-")
}

// PricingMapFields hold a small map of numbers instead of one number.
var PricingMapFields = []string{"provider_specific_entry", "search_context_cost_per_query"}

// PricingNumberFields are the numeric fields of model_prices.json. They follow
// LiteLLM's names, which the gateway's pricing file already uses.
var PricingNumberFields = []string{
	"max_input_tokens", "max_output_tokens", "max_tokens",
	"cache_creation_input_audio_token_cost", "cache_creation_input_token_cost",
	"cache_creation_input_token_cost_above_1hr", "cache_creation_input_token_cost_above_1hr_above_200k_tokens",
	"cache_creation_input_token_cost_above_200k_tokens",
	"cache_read_input_audio_token_cost", "cache_read_input_image_token_cost", "cache_read_input_token_cost",
	"cache_read_input_token_cost_above_200k_tokens", "cache_read_input_token_cost_above_200k_tokens_priority",
	"cache_read_input_token_cost_above_272k_tokens", "cache_read_input_token_cost_above_272k_tokens_priority",
	"cache_read_input_token_cost_flex", "cache_read_input_token_cost_per_audio_token",
	"cache_read_input_token_cost_priority",
	"code_interpreter_cost_per_session",
	"input_cost_per_audio_per_second", "input_cost_per_audio_per_second_above_128k_tokens",
	"input_cost_per_audio_token", "input_cost_per_audio_token_priority",
	"input_cost_per_character", "input_cost_per_character_above_128k_tokens",
	"input_cost_per_image", "input_cost_per_image_above_128k_tokens", "input_cost_per_image_token",
	"input_cost_per_pixel", "input_cost_per_query", "input_cost_per_second",
	"input_cost_per_token", "input_cost_per_token_above_128k_tokens",
	"input_cost_per_token_above_200k_tokens", "input_cost_per_token_above_200k_tokens_priority",
	"input_cost_per_token_above_272k_tokens", "input_cost_per_token_above_272k_tokens_priority",
	"input_cost_per_token_batch_requests", "input_cost_per_token_batches",
	"input_cost_per_token_flex", "input_cost_per_token_priority",
	"input_cost_per_video_per_second", "input_cost_per_video_per_second_above_128k_tokens",
	"input_cost_per_video_per_second_above_15s_interval", "input_cost_per_video_per_second_above_8s_interval",
	"ocr_cost_per_page",
	"output_cost_per_audio_token", "output_cost_per_character", "output_cost_per_character_above_128k_tokens",
	"output_cost_per_image", "output_cost_per_image_token", "output_cost_per_pixel",
	"output_cost_per_reasoning_token", "output_cost_per_second",
	"output_cost_per_token", "output_cost_per_token_above_128k_tokens",
	"output_cost_per_token_above_200k_tokens", "output_cost_per_token_above_200k_tokens_priority",
	"output_cost_per_token_above_272k_tokens", "output_cost_per_token_above_272k_tokens_priority",
	"output_cost_per_token_batches", "output_cost_per_token_flex", "output_cost_per_token_priority",
	"output_cost_per_video_per_second",
	"web_search_cost_per_request",
}

// GeneratePricing builds model_prices.json from the latest version of each
// provider. Two providers listing the same model is an error, because the
// pricing file can hold only one entry per name.
func GeneratePricing(providers []Provider) (map[string]map[string]any, error) {
	out := map[string]map[string]any{}
	owner := map[string]string{}
	for _, p := range Latest(providers) {
		name := p.Definition.Metadata.Name
		for _, m := range p.Models.Models {
			if other, ok := owner[m.Name]; ok {
				return nil, fmt.Errorf("model %q is listed by both %s and %s", m.Name, other, name)
			}
			owner[m.Name] = name
			out[m.Name] = m.Pricing
		}
	}
	return out, nil
}

// Latest keeps the highest version of each provider. Input must be sorted by
// name, then version, as Load returns it.
func Latest(providers []Provider) []Provider {
	var out []Provider
	for i, p := range providers {
		last := i == len(providers)-1 || providers[i+1].Definition.Metadata.Name != p.Definition.Metadata.Name
		if last {
			out = append(out, p)
		}
	}
	return out
}
