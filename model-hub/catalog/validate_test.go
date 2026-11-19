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
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// validProvider is a small, valid self-hosted entry that each case breaks in
// one way.
func validProvider() Provider {
	id := func(location, expr string) *Identifier { return &Identifier{Location: location, Identifier: expr} }
	return Provider{
		Definition: ProviderDefinition{
			APIVersion: APIVersion,
			Kind:       KindProvider,
			Metadata:   Metadata{Name: "example", Version: "1.0"},
			Spec: ProviderSpec{
				DisplayName: "Example",
				Description: "An example provider.",
				WireFormat:  "openai",
				Hosting:     HostingHosted,
				DefaultURL:  "https://api.example.com/v1",
				Auth:        &Auth{Type: "apiKey", Header: "Authorization", ValuePrefix: "Bearer "},
				Template: TemplateSpec{
					GroupID:   "example",
					ManagedBy: "organization",
					Version:   "v1.0",
					TokenFields: TokenFields{
						PromptTokens:     id("payload", "$.usage.prompt_tokens"),
						CompletionTokens: id("payload", "$.usage.completion_tokens"),
						RequestModel:     id("payload", "$.model"),
						ResponseModel:    id("payload", "$.model"),
					},
				},
			},
		},
		Models: ModelList{
			APIVersion: APIVersion,
			Kind:       KindModelList,
			Provider:   "example",
			Models: []Model{{
				Name:    "example-1",
				Status:  StatusUnverified,
				Pricing: map[string]any{"provider": "openai", "input_cost_per_token": 1e-06, "max_tokens": 8192},
			}},
		},
	}
}

func TestValidateAcceptsValidProvider(t *testing.T) {
	if errs := Validate(validProvider()); len(errs) > 0 {
		t.Fatalf("valid provider rejected: %v", errs)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		break_ func(p *Provider)
		want   string
	}{
		{"upper case name", func(p *Provider) { p.Definition.Metadata.Name = "Example" }, "metadata.name"},
		{"name with a path", func(p *Provider) { p.Definition.Metadata.Name = "../x" }, "metadata.name"},
		{"bad version", func(p *Provider) { p.Definition.Metadata.Version = "v1" }, "metadata.version"},
		{"unknown wire format", func(p *Provider) { p.Definition.Spec.WireFormat = "soap" }, "wireFormat"},
		{"unknown hosting", func(p *Provider) { p.Definition.Spec.Hosting = "cloud" }, "hosting"},
		{"hosted over http", func(p *Provider) { p.Definition.Spec.DefaultURL = "http://api.example.com" }, "must use https"},
		{"url with credentials", func(p *Provider) { p.Definition.Spec.DefaultURL = "https://user:pw@api.example.com" }, "credentials"},
		{"logo on another host", func(p *Provider) {
			p.Definition.Spec.Links = &Links{LogoURL: "https://evil.example.com/logo.png"}
		}, "must point at"},
		{"missing display name", func(p *Provider) { p.Definition.Spec.DisplayName = " " }, "displayName is required"},
		{"control characters", func(p *Provider) { p.Definition.Spec.Description = "a\x1bb" }, "control characters"},
		{"description too long", func(p *Provider) { p.Definition.Spec.Description = strings.Repeat("a", 1024) }, "longer than"},
		{"unknown auth type", func(p *Provider) { p.Definition.Spec.Auth.Type = "basic" }, "auth.type"},
		{"bad auth header", func(p *Provider) { p.Definition.Spec.Auth.Header = "X Bad" }, "auth.header"},
		{"missing prompt tokens", func(p *Provider) { p.Definition.Spec.Template.PromptTokens = nil }, "promptTokens is required"},
		{"bad template version", func(p *Provider) { p.Definition.Spec.Template.Version = "1.0" }, "template.version"},
		{"bad location", func(p *Provider) { p.Definition.Spec.Template.RequestModel.Location = "body" }, "location must be"},
		{"payload path without $", func(p *Provider) {
			p.Definition.Spec.Template.PromptTokens.Identifier = "usage.prompt_tokens"
		}, "JSONPath"},
		{"identifier too long", func(p *Provider) {
			p.Definition.Spec.Template.PromptTokens.Identifier = "$." + strings.Repeat("a", 300)
		}, "1 to 256"},
		{"bad path regex", func(p *Provider) {
			p.Definition.Spec.Template.RequestModel = &Identifier{Location: "pathParam", Identifier: "model/(["}
		}, "regular expression"},
		{"too many fallbacks", func(p *Provider) {
			p.Definition.Spec.Template.PromptTokens.FallbackIdentifiers = make([]string, 11)
			for i := range 11 {
				p.Definition.Spec.Template.PromptTokens.FallbackIdentifiers[i] = "$.a"
			}
		}, "fallbackIdentifiers"},
		{"bad cache accounting", func(p *Provider) { p.Definition.Spec.Template.CacheAccounting = "both" }, "cacheAccounting"},
		{"resource without slash", func(p *Provider) {
			p.Definition.Spec.Template.ResourceMappings = &ResourceMappings{Resources: []ResourceMapping{{Resource: "responses"}}}
		}, "resource must be a path"},
		{"models for another provider", func(p *Provider) { p.Models.Provider = "other" }, "does not match"},
		{"model name with ..", func(p *Provider) { p.Models.Models[0].Name = "a/../b" }, "not allowed"},
		{"duplicate model", func(p *Provider) { p.Models.Models = append(p.Models.Models, p.Models.Models[0]) }, "listed twice"},
		{"unknown status", func(p *Provider) { p.Models.Models[0].Status = "trusted" }, "status must be"},
		{"verified without source", func(p *Provider) { p.Models.Models[0].Status = StatusVerified }, "needs source"},
		{"bad date", func(p *Provider) { p.Models.Models[0].DeprecationDate = "next year" }, "date like"},
		{"unknown pricing field", func(p *Provider) { p.Models.Models[0].Pricing["free_tokens"] = 1 }, "not a pricing field"},
		{"negative price", func(p *Provider) { p.Models.Models[0].Pricing["input_cost_per_token"] = -1.0 }, "non-negative"},
		{"price as text", func(p *Provider) { p.Models.Models[0].Pricing["input_cost_per_token"] = "cheap" }, "non-negative"},
		{"unknown pricing provider", func(p *Provider) { p.Models.Models[0].Pricing["provider"] = "acme" }, "llm-cost"},
		{"missing pricing provider", func(p *Provider) { delete(p.Models.Models[0].Pricing, "provider") }, "provider is required"},
		{"svg icon", func(p *Provider) { p.Icon = []byte(`<svg xmlns="http://www.w3.org/2000/svg"><script/></svg>`) }, "PNG"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := validProvider()
			c.break_(&p)
			errs := Validate(p)
			for _, err := range errs {
				if strings.Contains(err.Error(), c.want) {
					return
				}
			}
			t.Errorf("want an error containing %q, got %v", c.want, errs)
		})
	}
}

func TestSelfHostedMayOmitURLOrUseHTTP(t *testing.T) {
	p := validProvider()
	p.Definition.Spec.Hosting = HostingSelfHosted
	p.Definition.Spec.DefaultURL = ""
	if errs := Validate(p); len(errs) > 0 {
		t.Errorf("self-hosted without URL rejected: %v", errs)
	}
	p.Definition.Spec.DefaultURL = "http://vllm.internal:8000/v1"
	if errs := Validate(p); len(errs) > 0 {
		t.Errorf("self-hosted over http rejected: %v", errs)
	}
}

// writeVersion writes a provider version folder holding the given files.
func writeVersion(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "example", "v1.0")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func validFiles(t *testing.T) map[string]string {
	t.Helper()
	p := validProvider()
	def, err := MarshalYAML(p.Definition)
	if err != nil {
		t.Fatal(err)
	}
	models, err := MarshalJSON(p.Models)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]string{ProviderFile: string(def), ModelsFile: string(models)}
}

func TestLoadProviderReadsValidFolder(t *testing.T) {
	p, err := LoadProvider(writeVersion(t, validFiles(t)))
	if err != nil {
		t.Fatal(err)
	}
	if errs := Validate(p); len(errs) > 0 {
		t.Errorf("loaded provider is invalid: %v", errs)
	}
}

func TestLoadProviderRejects(t *testing.T) {
	cases := map[string]func(files map[string]string){
		"unknown field": func(f map[string]string) {
			f[ProviderFile] = strings.Replace(f[ProviderFile], "spec:", "spec:\n  certified: true", 1)
		},
		"two documents":      func(f map[string]string) { f[ProviderFile] += "---\nkind: Other\n" },
		"missing models":     func(f map[string]string) { delete(f, ModelsFile) },
		"oversized provider": func(f map[string]string) { f[ProviderFile] += "#" + strings.Repeat("x", MaxProviderFileBytes) },
	}
	for name, breakFiles := range cases {
		t.Run(name, func(t *testing.T) {
			files := validFiles(t)
			breakFiles(files)
			if _, err := LoadProvider(writeVersion(t, files)); err == nil {
				t.Error("want an error, got none")
			}
		})
	}
}

func TestLoadReportsBadFolderNames(t *testing.T) {
	root := t.TempDir()
	for _, dir := range []string{"Bad_Name/v1.0", "good/latest"} {
		if err := os.MkdirAll(filepath.Join(root, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	providers, problems := Load(root)
	if len(providers) != 0 || len(problems) != 2 {
		t.Errorf("got %d providers and problems %v, want 0 and 2", len(providers), problems)
	}
}

func TestFolderMustMatchMetadata(t *testing.T) {
	p := validProvider()
	p.Dir = filepath.Join("providers", "example", "v2.0")
	errs := Validate(p)
	if len(errs) != 1 || !strings.Contains(errs[0].Error(), "does not match folder") {
		t.Errorf("got %v, want one folder mismatch", errs)
	}
}

func TestGeneratePricingRejectsDuplicateModels(t *testing.T) {
	a, b := validProvider(), validProvider()
	b.Definition.Metadata.Name = "other"
	if _, err := GeneratePricing([]Provider{a, b}); err == nil {
		t.Error("want an error for a model listed by two providers")
	}
}

func TestLatestKeepsHighestVersion(t *testing.T) {
	versions := []string{"1.0", "1.9", "1.10"}
	var providers []Provider
	for _, v := range versions {
		p := validProvider()
		p.Definition.Metadata.Version = v
		providers = append(providers, p)
	}
	latest := Latest(providers)
	if len(latest) != 1 || latest[0].Definition.Metadata.Version != "1.10" {
		t.Errorf("got %v", latest)
	}
	if CompareVersions("1.10", "1.9") <= 0 {
		t.Error("1.10 must sort after 1.9")
	}
}
