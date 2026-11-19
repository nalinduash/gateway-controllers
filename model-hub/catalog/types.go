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

// Package catalog reads, validates and publishes the Model Hub definitions:
// one ProviderDefinition and one ModelList per provider version.
package catalog

// APIVersion is the only apiVersion a definition file may declare.
const APIVersion = "modelhub.api-platform.wso2.com/v1alpha1"

// Kinds of definition file.
const (
	KindProvider  = "ProviderDefinition"
	KindModelList = "ModelList"
)

// Hosting types.
const (
	HostingHosted     = "hosted"
	HostingSelfHosted = "self-hosted"
)

// WireFormats are the request formats the gateway already handles. A provider
// on any other format needs a transformer policy, not a definition.
var WireFormats = []string{"openai", "anthropic", "gemini", "mistral", "bedrock", "azure-openai", "azure-ai-foundry", "typesafe"}

// Model verification statuses, weakest first.
const (
	StatusUnverified   = "unverified"    // imported, nobody has checked it
	StatusCrossChecked = "cross-checked" // our price agrees with LiteLLM's
	StatusVerified     = "verified"      // a person checked the provider's pricing page
)

// ProviderDefinition is provider.yaml: the existing LlmProviderTemplate spec
// plus the catalog fields the Hub needs.
type ProviderDefinition struct {
	APIVersion string       `yaml:"apiVersion" json:"apiVersion"`
	Kind       string       `yaml:"kind" json:"kind"`
	Metadata   Metadata     `yaml:"metadata" json:"metadata"`
	Spec       ProviderSpec `yaml:"spec" json:"spec"`
}

// Metadata names a provider version.
type Metadata struct {
	Name    string `yaml:"name" json:"name"`
	Version string `yaml:"version" json:"version"`
}

// ProviderSpec holds the catalog fields and the template.
type ProviderSpec struct {
	DisplayName string       `yaml:"displayName" json:"displayName"`
	Description string       `yaml:"description" json:"description"`
	WireFormat  string       `yaml:"wireFormat" json:"wireFormat"`
	Hosting     string       `yaml:"hosting" json:"hosting"`
	DefaultURL  string       `yaml:"defaultUrl,omitempty" json:"defaultUrl,omitempty"`
	Auth        *Auth        `yaml:"auth,omitempty" json:"auth,omitempty"`
	Links       *Links       `yaml:"links,omitempty" json:"links,omitempty"`
	Template    TemplateSpec `yaml:"template" json:"template"`
}

// Auth describes how the gateway sends the provider key.
type Auth struct {
	Type        string `yaml:"type" json:"type"`
	Header      string `yaml:"header,omitempty" json:"header,omitempty"`
	ValuePrefix string `yaml:"valuePrefix,omitempty" json:"valuePrefix,omitempty"`
}

// Links are optional pointers that consoles show to users. The Hub never
// fetches them.
type Links struct {
	LogoURL        string `yaml:"logoUrl,omitempty" json:"logoUrl,omitempty"`
	OpenAPISpecURL string `yaml:"openapiSpecUrl,omitempty" json:"openapiSpecUrl,omitempty"`
}

// TemplateSpec is the gateway's LlmProviderTemplate spec, field for field,
// without displayName (taken from ProviderSpec).
type TemplateSpec struct {
	GroupID          string `yaml:"groupId" json:"groupId"`
	ManagedBy        string `yaml:"managedBy" json:"managedBy"`
	Version          string `yaml:"version" json:"version"`
	TokenFields      `yaml:",inline"`
	ProviderFields   map[string]Identifier `yaml:"providerFields,omitempty" json:"providerFields,omitempty"`
	CacheAccounting  string                `yaml:"cacheAccounting,omitempty" json:"cacheAccounting,omitempty"`
	ResourceMappings *ResourceMappings     `yaml:"resourceMappings,omitempty" json:"resourceMappings,omitempty"`
}

// TokenFields are the named extraction points shared by the template and its
// per-resource overrides.
type TokenFields struct {
	PromptTokens       *Identifier `yaml:"promptTokens,omitempty" json:"promptTokens,omitempty"`
	CompletionTokens   *Identifier `yaml:"completionTokens,omitempty" json:"completionTokens,omitempty"`
	TotalTokens        *Identifier `yaml:"totalTokens,omitempty" json:"totalTokens,omitempty"`
	RemainingTokens    *Identifier `yaml:"remainingTokens,omitempty" json:"remainingTokens,omitempty"`
	RequestModel       *Identifier `yaml:"requestModel,omitempty" json:"requestModel,omitempty"`
	ResponseModel      *Identifier `yaml:"responseModel,omitempty" json:"responseModel,omitempty"`
	CachedTokens       *Identifier `yaml:"cachedTokens,omitempty" json:"cachedTokens,omitempty"`
	CacheWriteTokens   *Identifier `yaml:"cacheWriteTokens,omitempty" json:"cacheWriteTokens,omitempty"`
	CacheWrite1hTokens *Identifier `yaml:"cacheWrite1hTokens,omitempty" json:"cacheWrite1hTokens,omitempty"`
	ReasoningTokens    *Identifier `yaml:"reasoningTokens,omitempty" json:"reasoningTokens,omitempty"`
	AudioInputTokens   *Identifier `yaml:"audioInputTokens,omitempty" json:"audioInputTokens,omitempty"`
	AudioOutputTokens  *Identifier `yaml:"audioOutputTokens,omitempty" json:"audioOutputTokens,omitempty"`
	ServiceTier        *Identifier `yaml:"serviceTier,omitempty" json:"serviceTier,omitempty"`
}

// Identifier says where the gateway reads one value.
type Identifier struct {
	Location            string            `yaml:"location" json:"location"`
	Identifier          string            `yaml:"identifier" json:"identifier"`
	FallbackIdentifiers []string          `yaml:"fallbackIdentifiers,omitempty" json:"fallbackIdentifiers,omitempty"`
	ValueMap            map[string]string `yaml:"valueMap,omitempty" json:"valueMap,omitempty"`
}

// ResourceMappings overrides token fields for some request paths.
type ResourceMappings struct {
	Resources []ResourceMapping `yaml:"resources" json:"resources"`
}

// ResourceMapping overrides token fields for one request path.
type ResourceMapping struct {
	Resource    string `yaml:"resource" json:"resource"`
	TokenFields `yaml:",inline"`
}

// ModelList is models.json: every model a provider version serves.
type ModelList struct {
	APIVersion string  `yaml:"apiVersion" json:"apiVersion"`
	Kind       string  `yaml:"kind" json:"kind"`
	Provider   string  `yaml:"provider" json:"provider"`
	Models     []Model `yaml:"models" json:"models"`
}

// Model is one model entry. Pricing holds the entry exactly as it appears in
// the gateway's model_prices.json, so the pricing file can be generated back.
type Model struct {
	Name            string         `yaml:"name" json:"name"`
	Status          string         `yaml:"status" json:"status"`
	Source          string         `yaml:"source,omitempty" json:"source,omitempty"`
	CheckedOn       string         `yaml:"checkedOn,omitempty" json:"checkedOn,omitempty"`
	DeprecationDate string         `yaml:"deprecationDate,omitempty" json:"deprecationDate,omitempty"`
	Successor       string         `yaml:"successor,omitempty" json:"successor,omitempty"`
	Pricing         map[string]any `yaml:"pricing" json:"pricing"`
}

// Provider is one provider version read from disk.
type Provider struct {
	Definition ProviderDefinition
	Models     ModelList
	// Raw is provider.yaml as written, served by the definition endpoint.
	Raw []byte
	// Icon is the icon.png bytes, or nil when the provider has none.
	Icon []byte
	// Dir is the version folder, used in error messages only.
	Dir string
}
