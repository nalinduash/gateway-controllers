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
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"

	"go.yaml.in/yaml/v3"
)

// File names inside a provider version folder.
const (
	ProviderFile = "provider.yaml"
	ModelsFile   = "models.yaml"
	IconFile     = "icon.png"
)

// Size ceilings for each file. A definition that needs more is a mistake.
const (
	MaxProviderFileBytes = 256 << 10
	MaxModelsFileBytes   = 4 << 20
	MaxIconBytes         = 256 << 10
)

var (
	providerNameRE  = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)
	versionFolderRE = regexp.MustCompile(`^v[0-9]+\.[0-9]+$`)
)

// Load reads every provider version under root (root/<name>/v<X.Y>/).
// A folder that cannot be read or parsed is reported in the returned errors
// and skipped, so one bad contribution does not hide the others.
func Load(root string) ([]Provider, []error) {
	var providers []Provider
	var problems []error

	names, err := os.ReadDir(root)
	if err != nil {
		return nil, []error{fmt.Errorf("read providers folder: %w", err)}
	}
	for _, name := range names {
		if !name.IsDir() {
			continue
		}
		if !providerNameRE.MatchString(name.Name()) {
			problems = append(problems, fmt.Errorf("%s: provider folder name must be lower case letters, digits and hyphens", name.Name()))
			continue
		}
		versions, err := os.ReadDir(filepath.Join(root, name.Name()))
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", name.Name(), err))
			continue
		}
		for _, version := range versions {
			if !version.IsDir() {
				continue
			}
			if !versionFolderRE.MatchString(version.Name()) {
				problems = append(problems, fmt.Errorf("%s/%s: version folder must look like v1.0", name.Name(), version.Name()))
				continue
			}
			p, err := LoadProvider(filepath.Join(root, name.Name(), version.Name()))
			if err != nil {
				problems = append(problems, err)
				continue
			}
			providers = append(providers, p)
		}
	}

	sort.Slice(providers, func(i, j int) bool {
		a, b := providers[i].Definition.Metadata, providers[j].Definition.Metadata
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		return CompareVersions(a.Version, b.Version) < 0
	})
	return providers, problems
}

// LoadProvider reads one provider version folder.
func LoadProvider(dir string) (Provider, error) {
	p := Provider{Dir: dir}

	raw, err := readLimited(filepath.Join(dir, ProviderFile), MaxProviderFileBytes)
	if err != nil {
		return p, fmt.Errorf("%s: %w", dir, err)
	}
	if err := decodeStrict(raw, &p.Definition); err != nil {
		return p, fmt.Errorf("%s/%s: %w", dir, ProviderFile, err)
	}
	p.Raw = raw

	modelsRaw, err := readLimited(filepath.Join(dir, ModelsFile), MaxModelsFileBytes)
	if err != nil {
		return p, fmt.Errorf("%s: %w", dir, err)
	}
	if err := decodeStrict(modelsRaw, &p.Models); err != nil {
		return p, fmt.Errorf("%s/%s: %w", dir, ModelsFile, err)
	}

	icon, err := readLimited(filepath.Join(dir, IconFile), MaxIconBytes)
	switch {
	case err == nil:
		p.Icon = icon
	case !errors.Is(err, os.ErrNotExist):
		return p, fmt.Errorf("%s: %w", dir, err)
	}
	return p, nil
}

// readLimited reads a regular file, refusing links and files over max bytes.
func readLimited(path string, max int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", filepath.Base(path))
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > max {
		return nil, fmt.Errorf("%s is larger than %d bytes", filepath.Base(path), max)
	}
	return data, nil
}

// decodeStrict parses one YAML document and rejects unknown fields, so a typo
// or an injected field never passes silently.
func decodeStrict(data []byte, out any) error {
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := dec.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("file must hold exactly one YAML document")
	}
	return nil
}
