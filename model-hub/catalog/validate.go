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
	"math"
	"net/http"
	"net/url"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Limits on contributed values. They keep a definition small enough to review
// and cheap for the gateway to evaluate.
const (
	maxDisplayName    = 128
	maxDescription    = 1023
	maxURL            = 2048
	maxIdentifier     = 256
	maxFallbacks      = 10
	maxValueMap       = 20
	maxProviderFields = 20
	maxResources      = 50
	maxModels         = 5000
)

var (
	versionRE         = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
	templateVersionRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)
	headerNameRE      = regexp.MustCompile(`^[A-Za-z0-9-]{1,128}$`)
	fieldNameRE       = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9]{0,63}$`)
	shortValueRE      = regexp.MustCompile(`^[a-zA-Z0-9_.-]{1,64}$`)
	// Model names are the pricing file keys and keep their case, because the
	// llm-cost policy looks them up exactly. They may hold "/" and ":".
	modelNameRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/@-]{0,199}$`)
)

// linkHosts are the hosts a definition's links may point at.
var linkHosts = []string{"raw.githubusercontent.com"}

var identifierLocations = []string{"payload", "header", "queryParam", "pathParam"}

var authTypes = []string{"apiKey"}

// Validate checks one provider version and returns every problem found.
// An empty result means the entry is Validated.
func Validate(p Provider) []error {
	v := &validator{}
	v.provider(p)
	v.models(p)
	v.icon(p.Icon)
	return v.errs
}

type validator struct {
	errs []error
}

func (v *validator) fail(format string, args ...any) {
	v.errs = append(v.errs, fmt.Errorf(format, args...))
}

func (v *validator) provider(p Provider) {
	d := p.Definition
	if d.APIVersion != APIVersion {
		v.fail("apiVersion must be %s", APIVersion)
	}
	if d.Kind != KindProvider {
		v.fail("kind must be %s", KindProvider)
	}

	name, version := d.Metadata.Name, d.Metadata.Version
	if !providerNameRE.MatchString(name) {
		v.fail("metadata.name must be lower case letters, digits and hyphens, at most 40 characters")
	}
	if !versionRE.MatchString(version) {
		v.fail("metadata.version must look like 1.0")
	}
	// The folder layout is <name>/v<version>; the file must agree with it.
	if p.Dir != "" {
		if filepath.Base(p.Dir) != "v"+version {
			v.fail("metadata.version %q does not match folder %q", version, filepath.Base(p.Dir))
		}
		if filepath.Base(filepath.Dir(p.Dir)) != name {
			v.fail("metadata.name %q does not match folder %q", name, filepath.Base(filepath.Dir(p.Dir)))
		}
	}

	s := d.Spec
	v.text("spec.displayName", s.DisplayName, maxDisplayName, true)
	v.text("spec.description", s.Description, maxDescription, true)
	if !slices.Contains(WireFormats, s.WireFormat) {
		v.fail("spec.wireFormat must be one of %s", strings.Join(WireFormats, ", "))
	}
	if s.Hosting != HostingHosted && s.Hosting != HostingSelfHosted {
		v.fail("spec.hosting must be %s or %s", HostingHosted, HostingSelfHosted)
	}
	if s.DefaultURL != "" {
		// A hosted provider is reached over the internet, so it must use TLS.
		// A self-hosted server often runs inside the user's network.
		schemes := []string{"https"}
		if s.Hosting == HostingSelfHosted {
			schemes = append(schemes, "http")
		}
		v.url("spec.defaultUrl", s.DefaultURL, schemes, nil)
	}
	if s.Auth != nil {
		if !slices.Contains(authTypes, s.Auth.Type) {
			v.fail("spec.auth.type must be one of %s", strings.Join(authTypes, ", "))
		}
		if !headerNameRE.MatchString(s.Auth.Header) {
			v.fail("spec.auth.header must be a header name")
		}
		v.text("spec.auth.valuePrefix", s.Auth.ValuePrefix, 32, false)
	}
	if s.Links != nil {
		if s.Links.LogoURL != "" {
			v.url("spec.links.logoUrl", s.Links.LogoURL, []string{"https"}, linkHosts)
		}
		if s.Links.OpenAPISpecURL != "" {
			v.url("spec.links.openapiSpecUrl", s.Links.OpenAPISpecURL, []string{"https"}, linkHosts)
		}
	}
	v.template(s.Template)
}

func (v *validator) template(t TemplateSpec) {
	if t.GroupID == "" || len(t.GroupID) > 40 || !shortValueRE.MatchString(t.GroupID) {
		v.fail("spec.template.groupId is required: letters, digits, '.', '_' or '-', at most 40 characters")
	}
	v.text("spec.template.managedBy", t.ManagedBy, 255, true)
	if !templateVersionRE.MatchString(t.Version) {
		v.fail("spec.template.version must look like v1.0")
	}

	// The gateway cannot count tokens or name the model without these.
	required := map[string]*Identifier{
		"promptTokens":     t.PromptTokens,
		"completionTokens": t.CompletionTokens,
		"requestModel":     t.RequestModel,
		"responseModel":    t.ResponseModel,
	}
	for _, field := range sortedKeys(required) {
		if required[field] == nil {
			v.fail("spec.template.%s is required", field)
		}
	}
	v.tokenFields("spec.template", t.TokenFields)

	if len(t.ProviderFields) > maxProviderFields {
		v.fail("spec.template.providerFields has more than %d entries", maxProviderFields)
	}
	for _, key := range sortedKeys(t.ProviderFields) {
		if !fieldNameRE.MatchString(key) {
			v.fail("spec.template.providerFields key %q must be a simple name", key)
		}
		id := t.ProviderFields[key]
		v.identifier("spec.template.providerFields."+key, &id)
	}
	if t.CacheAccounting != "" && t.CacheAccounting != "inclusive" && t.CacheAccounting != "additive" {
		v.fail("spec.template.cacheAccounting must be inclusive or additive")
	}
	if t.ResourceMappings != nil {
		if len(t.ResourceMappings.Resources) > maxResources {
			v.fail("spec.template.resourceMappings has more than %d resources", maxResources)
		}
		for i, r := range t.ResourceMappings.Resources {
			path := fmt.Sprintf("spec.template.resourceMappings.resources[%d]", i)
			if !strings.HasPrefix(r.Resource, "/") || len(r.Resource) > maxIdentifier {
				v.fail("%s.resource must be a path starting with /", path)
			}
			v.tokenFields(path, r.TokenFields)
		}
	}
}

func (v *validator) tokenFields(path string, f TokenFields) {
	fields := map[string]*Identifier{
		"promptTokens": f.PromptTokens, "completionTokens": f.CompletionTokens,
		"totalTokens": f.TotalTokens, "remainingTokens": f.RemainingTokens,
		"requestModel": f.RequestModel, "responseModel": f.ResponseModel,
		"cachedTokens": f.CachedTokens, "cacheWriteTokens": f.CacheWriteTokens,
		"cacheWrite1hTokens": f.CacheWrite1hTokens, "reasoningTokens": f.ReasoningTokens,
		"audioInputTokens": f.AudioInputTokens, "audioOutputTokens": f.AudioOutputTokens,
		"serviceTier": f.ServiceTier,
	}
	for _, name := range sortedKeys(fields) {
		if fields[name] != nil {
			v.identifier(path+"."+name, fields[name])
		}
	}
}

// identifier checks one extraction point. The gateway evaluates these on every
// response, so each expression is short and must suit its location.
func (v *validator) identifier(path string, id *Identifier) {
	if !slices.Contains(identifierLocations, id.Location) {
		v.fail("%s.location must be one of %s", path, strings.Join(identifierLocations, ", "))
		return
	}
	if len(id.FallbackIdentifiers) > maxFallbacks {
		v.fail("%s has more than %d fallbackIdentifiers", path, maxFallbacks)
	}
	for _, expr := range append([]string{id.Identifier}, id.FallbackIdentifiers...) {
		v.expression(path, id.Location, expr)
	}
	if len(id.ValueMap) > maxValueMap {
		v.fail("%s.valueMap has more than %d entries", path, maxValueMap)
	}
	for _, key := range sortedKeys(id.ValueMap) {
		if !shortValueRE.MatchString(key) || !shortValueRE.MatchString(id.ValueMap[key]) {
			v.fail("%s.valueMap entries must be short words", path)
		}
	}
}

func (v *validator) expression(path, location, expr string) {
	if expr == "" || len(expr) > maxIdentifier {
		v.fail("%s: identifier must be 1 to %d characters", path, maxIdentifier)
		return
	}
	switch location {
	case "payload":
		if !strings.HasPrefix(expr, "$") || strings.ContainsFunc(expr, unicode.IsSpace) {
			v.fail("%s: payload identifier %q must be a JSONPath starting with $", path, expr)
		}
	case "header", "queryParam":
		if !headerNameRE.MatchString(expr) {
			v.fail("%s: %s identifier %q must be a plain name", path, location, expr)
		}
	case "pathParam":
		// Go regular expressions run in linear time, so a pattern that
		// compiles cannot be made to backtrack.
		if _, err := regexp.Compile(expr); err != nil {
			v.fail("%s: pathParam identifier is not a valid regular expression", path)
		}
	}
}

func (v *validator) models(p Provider) {
	m := p.Models
	if m.APIVersion != APIVersion {
		v.fail("%s: apiVersion must be %s", ModelsFile, APIVersion)
	}
	if m.Kind != KindModelList {
		v.fail("%s: kind must be %s", ModelsFile, KindModelList)
	}
	if m.Provider != p.Definition.Metadata.Name {
		v.fail("%s: provider %q does not match metadata.name", ModelsFile, m.Provider)
	}
	if len(m.Models) > maxModels {
		v.fail("%s has more than %d models", ModelsFile, maxModels)
	}
	seen := map[string]bool{}
	for _, model := range m.Models {
		v.model(model, seen)
	}
}

func (v *validator) model(m Model, seen map[string]bool) {
	if !modelNameRE.MatchString(m.Name) || strings.Contains(m.Name, "..") || strings.Contains(m.Name, "//") {
		v.fail("model %q: name has characters that are not allowed", m.Name)
		return
	}
	if seen[m.Name] {
		v.fail("model %q is listed twice", m.Name)
	}
	seen[m.Name] = true

	switch m.Status {
	case StatusUnverified, StatusCrossChecked:
	case StatusVerified:
		// A verified price is one a person read on the provider's own page.
		if m.Source == "" || m.CheckedOn == "" {
			v.fail("model %q: a verified model needs source and checkedOn", m.Name)
		}
	default:
		v.fail("model %q: status must be %s, %s or %s", m.Name, StatusUnverified, StatusCrossChecked, StatusVerified)
	}
	if m.Source != "" {
		v.url(fmt.Sprintf("model %q source", m.Name), m.Source, []string{"https"}, nil)
	}
	for field, value := range map[string]string{"checkedOn": m.CheckedOn, "deprecationDate": m.DeprecationDate} {
		if value == "" {
			continue
		}
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			v.fail("model %q: %s must be a date like 2026-01-31", m.Name, field)
		}
	}
	if m.Successor != "" && !modelNameRE.MatchString(m.Successor) {
		v.fail("model %q: successor is not a valid model name", m.Name)
	}
	v.pricing(m)
}

func (v *validator) pricing(m Model) {
	if len(m.Pricing) == 0 {
		v.fail("model %q: pricing is required", m.Name)
		return
	}
	for _, key := range sortedKeys(m.Pricing) {
		value := m.Pricing[key]
		switch {
		case key == "provider":
			s, ok := value.(string)
			if !ok || !IsPricingProvider(s) {
				v.fail("model %q: pricing.provider %v is not a provider the llm-cost policy knows", m.Name, value)
			}
		case key == "mode":
			if s, ok := value.(string); !ok || !shortValueRE.MatchString(s) {
				v.fail("model %q: pricing.mode must be a short word", m.Name)
			}
		case slices.Contains(PricingMapFields, key):
			entries, ok := value.(map[string]any)
			if !ok || len(entries) > maxValueMap {
				v.fail("model %q: pricing.%s must be a small map of numbers", m.Name, key)
				continue
			}
			for _, sub := range sortedKeys(entries) {
				if !shortValueRE.MatchString(sub) || !isPrice(entries[sub]) {
					v.fail("model %q: pricing.%s.%s must be a non-negative number", m.Name, key, sub)
				}
			}
		case slices.Contains(PricingNumberFields, key):
			if !isPrice(value) {
				v.fail("model %q: pricing.%s must be a non-negative number", m.Name, key)
			}
		default:
			v.fail("model %q: pricing.%s is not a pricing field", m.Name, key)
		}
	}
	if _, ok := m.Pricing["provider"]; !ok {
		v.fail("model %q: pricing.provider is required", m.Name)
	}
}

// icon accepts PNG only, checked by content. An SVG can carry script.
func (v *validator) icon(icon []byte) {
	if icon == nil {
		return
	}
	if http.DetectContentType(icon) != "image/png" {
		v.fail("%s must be a PNG image", IconFile)
	}
}

func (v *validator) text(path, value string, max int, required bool) {
	if required && strings.TrimSpace(value) == "" {
		v.fail("%s is required", path)
		return
	}
	if len(value) > max {
		v.fail("%s is longer than %d characters", path, max)
	}
	if strings.ContainsFunc(value, func(r rune) bool { return unicode.IsControl(r) && r != '\n' }) {
		v.fail("%s contains control characters", path)
	}
}

// url checks a URL's form only. The Hub never fetches it.
func (v *validator) url(path, raw string, schemes, hosts []string) {
	u, err := url.Parse(raw)
	switch {
	case err != nil || len(raw) > maxURL:
		v.fail("%s is not a valid URL", path)
	case !slices.Contains(schemes, u.Scheme):
		v.fail("%s must use %s", path, strings.Join(schemes, " or "))
	case u.Host == "":
		v.fail("%s must name a host", path)
	case u.User != nil:
		v.fail("%s must not carry credentials", path)
	case hosts != nil && !slices.Contains(hosts, strings.ToLower(u.Hostname())):
		v.fail("%s must point at one of %s", path, strings.Join(hosts, ", "))
	}
}

func isPrice(value any) bool {
	var f float64
	switch n := value.(type) {
	case int:
		f = float64(n)
	case float64:
		f = n
	default:
		return false
	}
	return f >= 0 && !math.IsInf(f, 0) && !math.IsNaN(f)
}

// CompareVersions orders "X.Y" versions numerically: 1.10 is after 1.9.
func CompareVersions(a, b string) int {
	pa, pb := strings.SplitN(a, ".", 2), strings.SplitN(b, ".", 2)
	for i := range 2 {
		var x, y int
		if i < len(pa) {
			x, _ = strconv.Atoi(pa[i])
		}
		if i < len(pb) {
			y, _ = strconv.Atoi(pb[i])
		}
		if x != y {
			return x - y
		}
	}
	return 0
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
