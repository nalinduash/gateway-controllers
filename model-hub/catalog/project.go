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

// A definition is the single source for both template copies that exist
// today: the gateway's LlmProviderTemplate resource and the Platform API's
// template record. The functions here derive each one.

// GatewayTemplate is the gateway's LlmProviderTemplate resource.
type GatewayTemplate struct {
	APIVersion string              `yaml:"apiVersion" json:"apiVersion"`
	Kind       string              `yaml:"kind" json:"kind"`
	Metadata   GatewayMetadata     `yaml:"metadata" json:"metadata"`
	Spec       GatewayTemplateSpec `yaml:"spec" json:"spec"`
}

// GatewayMetadata names the resource.
type GatewayMetadata struct {
	Name string `yaml:"name" json:"name"`
}

// GatewayTemplateSpec is the template spec with its display name.
type GatewayTemplateSpec struct {
	DisplayName  string `yaml:"displayName" json:"displayName"`
	TemplateSpec `yaml:",inline"`
}

// ToGatewayTemplate derives the gateway resource. Nothing is dropped.
func ToGatewayTemplate(d ProviderDefinition) GatewayTemplate {
	return GatewayTemplate{
		APIVersion: "gateway.api-platform.wso2.com/v1",
		Kind:       "LlmProviderTemplate",
		Metadata:   GatewayMetadata{Name: d.Metadata.Name},
		Spec:       GatewayTemplateSpec{DisplayName: d.Spec.DisplayName, TemplateSpec: d.Spec.Template},
	}
}

// PlatformTemplate is the body Platform API takes on POST /llm-provider-templates.
// Platform API stores fewer fields than the gateway: no fallbacks, value maps,
// provider fields or the newer token kinds.
type PlatformTemplate struct {
	ID               string            `yaml:"id" json:"id"`
	DisplayName      string            `yaml:"displayName" json:"displayName"`
	Description      string            `yaml:"description,omitempty" json:"description,omitempty"`
	ManagedBy        string            `yaml:"managedBy" json:"managedBy"`
	Version          string            `yaml:"version" json:"version"`
	Metadata         *PlatformMetadata `yaml:"metadata,omitempty" json:"metadata,omitempty"`
	PlatformTokens   `yaml:",inline"`
	ResourceMappings *PlatformResourceMappings `yaml:"resourceMappings,omitempty" json:"resourceMappings,omitempty"`
}

// PlatformMetadata is the connection and branding block.
type PlatformMetadata struct {
	EndpointURL    string `yaml:"endpointUrl,omitempty" json:"endpointUrl,omitempty"`
	Auth           *Auth  `yaml:"auth,omitempty" json:"auth,omitempty"`
	LogoURL        string `yaml:"logoUrl,omitempty" json:"logoUrl,omitempty"`
	OpenAPISpecURL string `yaml:"openapiSpecUrl,omitempty" json:"openapiSpecUrl,omitempty"`
}

// PlatformTokens are the six extraction points Platform API stores.
type PlatformTokens struct {
	PromptTokens     *PlatformIdentifier `yaml:"promptTokens,omitempty" json:"promptTokens,omitempty"`
	CompletionTokens *PlatformIdentifier `yaml:"completionTokens,omitempty" json:"completionTokens,omitempty"`
	TotalTokens      *PlatformIdentifier `yaml:"totalTokens,omitempty" json:"totalTokens,omitempty"`
	RemainingTokens  *PlatformIdentifier `yaml:"remainingTokens,omitempty" json:"remainingTokens,omitempty"`
	RequestModel     *PlatformIdentifier `yaml:"requestModel,omitempty" json:"requestModel,omitempty"`
	ResponseModel    *PlatformIdentifier `yaml:"responseModel,omitempty" json:"responseModel,omitempty"`
}

// PlatformIdentifier is an extraction point without fallbacks.
type PlatformIdentifier struct {
	Location   string `yaml:"location" json:"location"`
	Identifier string `yaml:"identifier" json:"identifier"`
}

// PlatformResourceMappings are the per-path overrides.
type PlatformResourceMappings struct {
	Resources []PlatformResourceMapping `yaml:"resources" json:"resources"`
}

// PlatformResourceMapping overrides the six extraction points for one path.
type PlatformResourceMapping struct {
	Resource       string `yaml:"resource" json:"resource"`
	PlatformTokens `yaml:",inline"`
}

// ToPlatformTemplate derives the Platform API template body.
func ToPlatformTemplate(d ProviderDefinition) PlatformTemplate {
	s := d.Spec
	t := PlatformTemplate{
		ID:             d.Metadata.Name,
		DisplayName:    s.DisplayName,
		Description:    s.Description,
		ManagedBy:      s.Template.ManagedBy,
		Version:        s.Template.Version,
		PlatformTokens: toPlatformTokens(s.Template.TokenFields),
	}

	meta := PlatformMetadata{EndpointURL: s.DefaultURL, Auth: s.Auth}
	if s.Links != nil {
		meta.LogoURL = s.Links.LogoURL
		meta.OpenAPISpecURL = s.Links.OpenAPISpecURL
	}
	if meta != (PlatformMetadata{}) {
		t.Metadata = &meta
	}

	if s.Template.ResourceMappings != nil {
		mappings := &PlatformResourceMappings{}
		for _, r := range s.Template.ResourceMappings.Resources {
			mappings.Resources = append(mappings.Resources, PlatformResourceMapping{
				Resource:       r.Resource,
				PlatformTokens: toPlatformTokens(r.TokenFields),
			})
		}
		t.ResourceMappings = mappings
	}
	return t
}

func toPlatformTokens(f TokenFields) PlatformTokens {
	return PlatformTokens{
		PromptTokens:     plain(f.PromptTokens),
		CompletionTokens: plain(f.CompletionTokens),
		TotalTokens:      plain(f.TotalTokens),
		RemainingTokens:  plain(f.RemainingTokens),
		RequestModel:     plain(f.RequestModel),
		ResponseModel:    plain(f.ResponseModel),
	}
}

func plain(id *Identifier) *PlatformIdentifier {
	if id == nil {
		return nil
	}
	return &PlatformIdentifier{Location: id.Location, Identifier: id.Identifier}
}
