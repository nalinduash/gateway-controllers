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

// Command seed is the one-time import that created the first definitions from
// the 8 built-in templates and the gateway's pricing file. Every model comes in
// as unverified. It is kept so the import can be repeated and reviewed.
//
//	go run ./cmd/seed -api-platform ../../api-platform -out providers
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/wso2/gateway-controllers/model-hub/catalog"
)

// builtin maps one built-in template to its Hub entry.
type builtin struct {
	name        string // template metadata.name, also the Hub name
	file        string // template file name in both template folders
	specsDir    string // folder under llm-provider-specs holding icon.png
	wireFormat  string
	description string
	pricing     []string // pricing file provider values whose models it lists
}

var builtins = []builtin{
	{"openai", "openai-template.yaml", "openai", "openai",
		"OpenAI's hosted models through the Chat Completions and Responses APIs.", []string{"openai"}},
	{"anthropic", "anthropic-template.yaml", "anthropic", "anthropic",
		"Anthropic's Claude models through the Messages API.", []string{"anthropic"}},
	{"gemini", "gemini-template.yaml", "gemini", "gemini",
		"Google's Gemini models through Google AI Studio and Vertex AI.", []string{"gemini", "vertex_ai*"}},
	{"mistralai", "mistral-template.yaml", "mistral", "mistral",
		"Mistral AI's hosted models.", []string{"mistral"}},
	{"awsbedrock", "awsbedrock-template.yaml", "awsbedrock", "bedrock",
		"Models hosted on Amazon Bedrock through the Converse and InvokeModel APIs.", []string{"bedrock"}},
	{"azure-openai", "azureopenai-template.yaml", "azure-openai", "azure-openai",
		"OpenAI models deployed in an Azure OpenAI resource.", []string{"azure", "azure_text"}},
	{"azureai-foundry", "azureaifoundry-template.yaml", "azureai-foundry", "azure-ai-foundry",
		"Models deployed in Azure AI Foundry.", []string{"azure_ai"}},
	{"typesafe", "typesafe-template.yaml", "typesafe", "typesafe",
		"TypeSafe's evaluation models.", []string{"typesafe"}},
}

const header = `# --------------------------------------------------------------------
# Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
#
# WSO2 LLC. licenses this file to you under the Apache License,
# Version 2.0 (the "License"); you may not use this file except
# in compliance with the License.
# You may obtain a copy of the License at
#
# http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing,
# software distributed under the License is distributed on an
# "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
# KIND, either express or implied.  See the License for the
# specific language governing permissions and limitations
# under the License.
# --------------------------------------------------------------------

`

func main() {
	apiPlatform := flag.String("api-platform", "", "path to an api-platform checkout")
	out := flag.String("out", "providers", "folder to write the definitions to")
	flag.Parse()
	if *apiPlatform == "" {
		log.Fatal("-api-platform is required")
	}
	if err := run(*apiPlatform, *out); err != nil {
		log.Fatal(err)
	}
}

func run(apiPlatform, out string) error {
	gatewayDir := filepath.Join(apiPlatform, "gateway", "gateway-controller", "default-llm-provider-templates")
	platformDir := filepath.Join(apiPlatform, "platform-api", "resources", "default-llm-provider-templates")
	pricing, err := readPricing(filepath.Join(apiPlatform, "gateway", "configs", "llm-pricing", "model_prices.json"))
	if err != nil {
		return err
	}

	used := map[string]bool{}
	for _, b := range builtins {
		def, err := definitionFor(b, gatewayDir, platformDir)
		if err != nil {
			return fmt.Errorf("%s: %w", b.name, err)
		}
		models := modelsFor(b, pricing, used)
		icon, err := os.ReadFile(filepath.Join(apiPlatform, "llm-provider-specs", b.specsDir, "icon.png"))
		if err != nil {
			return fmt.Errorf("%s: %w", b.name, err)
		}
		if err := write(filepath.Join(out, b.name, "v"+def.Metadata.Version), def, models, icon); err != nil {
			return err
		}
		fmt.Printf("%s: %d models\n", b.name, len(models.Models))
	}

	for key := range pricing {
		if !used[key] {
			fmt.Printf("not imported (no provider owns it): %s\n", key)
		}
	}
	return nil
}

func readPricing(path string) (map[string]map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	out := map[string]map[string]any{}
	return out, json.Unmarshal(data, &out)
}

// definitionFor merges the two template copies: the token fields from the
// gateway copy, which is the richer one, and the connection details from the
// Platform API copy, which the gateway copy does not carry.
func definitionFor(b builtin, gatewayDir, platformDir string) (catalog.ProviderDefinition, error) {
	var gw catalog.GatewayTemplate
	if err := readYAML(filepath.Join(gatewayDir, b.file), &gw); err != nil {
		return catalog.ProviderDefinition{}, err
	}
	var pf struct {
		Spec struct {
			Metadata catalog.PlatformMetadata `yaml:"metadata"`
		} `yaml:"spec"`
	}
	if err := readYAML(filepath.Join(platformDir, b.file), &pf); err != nil {
		return catalog.ProviderDefinition{}, err
	}
	meta := pf.Spec.Metadata

	def := catalog.ProviderDefinition{
		APIVersion: catalog.APIVersion,
		Kind:       catalog.KindProvider,
		Metadata:   catalog.Metadata{Name: b.name, Version: "1.0"},
		Spec: catalog.ProviderSpec{
			DisplayName: gw.Spec.DisplayName,
			Description: b.description,
			WireFormat:  b.wireFormat,
			Hosting:     catalog.HostingHosted,
			DefaultURL:  meta.EndpointURL,
			Auth:        meta.Auth,
			Template:    gw.Spec.TemplateSpec,
		},
	}
	if meta.LogoURL != "" || meta.OpenAPISpecURL != "" {
		def.Spec.Links = &catalog.Links{LogoURL: meta.LogoURL, OpenAPISpecURL: meta.OpenAPISpecURL}
	}
	return def, nil
}

func modelsFor(b builtin, pricing map[string]map[string]any, used map[string]bool) catalog.ModelList {
	list := catalog.ModelList{APIVersion: catalog.APIVersion, Kind: catalog.KindModelList, Provider: b.name}
	keys := make([]string, 0, len(pricing))
	for k := range pricing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		provider, _ := pricing[key]["provider"].(string)
		if !owns(b, provider) {
			continue
		}
		used[key] = true
		list.Models = append(list.Models, catalog.Model{
			Name:    key,
			Status:  catalog.StatusUnverified,
			Pricing: pricing[key],
		})
	}
	return list
}

func owns(b builtin, provider string) bool {
	for _, p := range b.pricing {
		if p == provider {
			return true
		}
		if p == "vertex_ai*" && (provider == "vertex_ai" || strings.HasPrefix(provider, "vertex_ai-")) {
			return true
		}
	}
	return false
}

func write(dir string, def catalog.ProviderDefinition, models catalog.ModelList, icon []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if err := writeYAML(filepath.Join(dir, catalog.ProviderFile), def); err != nil {
		return err
	}
	if err := writeJSON(filepath.Join(dir, catalog.ModelsFile), models); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, catalog.IconFile), icon, 0o644)
}

func readYAML(path string, out any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return catalog.UnmarshalYAML(data, out)
}

func writeYAML(path string, value any) error {
	data, err := catalog.MarshalYAML(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, append([]byte(header), data...), 0o644)
}

func writeJSON(path string, value any) error {
	data, err := catalog.MarshalJSON(value)
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}
