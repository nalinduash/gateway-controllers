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
	"errors"
	"fmt"
)

// Certification levels, weakest first. Only Validated can be earned today:
// Conformant needs the conformance suite and Certified needs live runs.
const (
	LevelValidated  = "validated"
	LevelConformant = "conformant"
	LevelCertified  = "certified"
)

// Bundle is what the publish step hands to the Hub API: every published
// provider version and the models of the latest versions. The pipeline sets
// the certification level; a definition file can never claim one.
type Bundle struct {
	GeneratedAt string           `json:"generatedAt"`
	Providers   []BundleProvider `json:"providers"`
	Models      []BundleModel    `json:"models"`
}

// BundleProvider is one published provider version.
type BundleProvider struct {
	Name             string           `json:"name"`
	Version          string           `json:"version"`
	IsLatest         bool             `json:"isLatest"`
	DisplayName      string           `json:"displayName"`
	Description      string           `json:"description"`
	WireFormat       string           `json:"wireFormat"`
	Hosting          string           `json:"hosting"`
	DefaultURL       string           `json:"defaultUrl,omitempty"`
	Auth             *Auth            `json:"auth,omitempty"`
	Links            *Links           `json:"links,omitempty"`
	Certification    Certification    `json:"certification"`
	ModelCount       int              `json:"modelCount"`
	IconFile         string           `json:"iconFile,omitempty"`
	Definition       string           `json:"definition"`
	GatewayTemplate  string           `json:"gatewayTemplate"`
	PlatformTemplate PlatformTemplate `json:"platformTemplate"`
}

// Certification records the level an entry holds and when it was earned.
type Certification struct {
	Level string `json:"level"`
	Since string `json:"since"`
}

// BundleModel is one model of a provider's latest version.
type BundleModel struct {
	Provider string `json:"provider"`
	Model
}

// IconFileName is where the publish step writes a provider version's icon.
func IconFileName(name, version string) string {
	return fmt.Sprintf("%s-%s.png", name, version)
}

// BuildBundle publishes providers that pass validation. Any invalid entry stops
// the build, so a broken definition is never half-published.
func BuildBundle(providers []Provider, generatedAt string) (Bundle, error) {
	var problems []error
	for _, p := range providers {
		for _, err := range Validate(p) {
			problems = append(problems, fmt.Errorf("%s: %w", p.Dir, err))
		}
	}
	if _, err := GeneratePricing(providers); err != nil {
		problems = append(problems, err)
	}
	if len(problems) > 0 {
		return Bundle{}, errors.Join(problems...)
	}

	latest := map[string]string{}
	for _, p := range Latest(providers) {
		latest[p.Definition.Metadata.Name] = p.Definition.Metadata.Version
	}

	b := Bundle{GeneratedAt: generatedAt, Providers: []BundleProvider{}, Models: []BundleModel{}}
	for _, p := range providers {
		d := p.Definition
		gateway, err := MarshalYAML(ToGatewayTemplate(d))
		if err != nil {
			return Bundle{}, err
		}
		entry := BundleProvider{
			Name:             d.Metadata.Name,
			Version:          d.Metadata.Version,
			IsLatest:         latest[d.Metadata.Name] == d.Metadata.Version,
			DisplayName:      d.Spec.DisplayName,
			Description:      d.Spec.Description,
			WireFormat:       d.Spec.WireFormat,
			Hosting:          d.Spec.Hosting,
			DefaultURL:       d.Spec.DefaultURL,
			Auth:             d.Spec.Auth,
			Links:            d.Spec.Links,
			Certification:    Certification{Level: LevelValidated, Since: generatedAt},
			ModelCount:       len(p.Models.Models),
			Definition:       string(p.Raw),
			GatewayTemplate:  string(gateway),
			PlatformTemplate: ToPlatformTemplate(d),
		}
		if p.Icon != nil {
			entry.IconFile = IconFileName(d.Metadata.Name, d.Metadata.Version)
		}
		b.Providers = append(b.Providers, entry)

		if entry.IsLatest {
			for _, m := range p.Models.Models {
				b.Models = append(b.Models, BundleModel{Provider: d.Metadata.Name, Model: m})
			}
		}
	}
	return b, nil
}
