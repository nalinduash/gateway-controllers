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
	"strings"
	"testing"
)

func TestBuildBundleFromRepo(t *testing.T) {
	b, err := BuildBundle(loadRepo(t), "2026-11-17T00:00:00Z")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Providers) != 9 || len(b.Models) != 841 {
		t.Fatalf("got %d providers and %d models, want 9 and 841", len(b.Providers), len(b.Models))
	}
	for _, p := range b.Providers {
		if p.Certification.Level != LevelValidated || !p.IsLatest {
			t.Errorf("%s: level %q latest %v", p.Name, p.Certification.Level, p.IsLatest)
		}
		if !strings.Contains(p.GatewayTemplate, "kind: LlmProviderTemplate") {
			t.Errorf("%s: gateway template missing", p.Name)
		}
		if p.PlatformTemplate.ID != p.Name {
			t.Errorf("%s: platform template id %q", p.Name, p.PlatformTemplate.ID)
		}
		if (p.IconFile == "") != (p.Name == "vllm") {
			t.Errorf("%s: icon file %q", p.Name, p.IconFile)
		}
	}
}

func TestBuildBundleRefusesInvalidEntry(t *testing.T) {
	bad := validProvider()
	bad.Definition.Spec.WireFormat = "soap"
	if _, err := BuildBundle([]Provider{validProvider(), bad}, "now"); err == nil {
		t.Error("want the build to stop on an invalid entry")
	}
}

// Only the latest version's models are listed, and both versions are published.
func TestBuildBundleListsModelsOfLatestVersion(t *testing.T) {
	old, current := validProvider(), validProvider()
	current.Definition.Metadata.Version = "1.1"
	current.Models.Models[0].Name = "example-2"
	b, err := BuildBundle([]Provider{old, current}, "now")
	if err != nil {
		t.Fatal(err)
	}
	if len(b.Providers) != 2 || b.Providers[0].IsLatest || !b.Providers[1].IsLatest {
		t.Errorf("got providers %+v", b.Providers)
	}
	if len(b.Models) != 1 || b.Models[0].Name != "example-2" {
		t.Errorf("got models %+v", b.Models)
	}
}
