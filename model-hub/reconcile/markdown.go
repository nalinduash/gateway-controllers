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
	"fmt"
	"strconv"
	"strings"
)

// Markdown renders the report for a maintainer to review, the way a pull
// request is reviewed.
func (r Report) Markdown(thresholdPercent float64) string {
	var b strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&b, format, args...) }

	w("# Pricing reconciliation report\n\n")
	w("Compares the gateway's `model_prices.json` with LiteLLM's `model_prices_and_context_window.json`.\n")
	w("Nothing here was applied: every change goes through review like any other code.\n\n")
	w("LiteLLM's pricing data is MIT-licensed (Copyright (c) 2023 Berri AI). Any value imported from it\n")
	w("must keep that attribution.\n\n")

	w("| Bucket | Models | What to do |\n| --- | --- | --- |\n")
	w("| Prices agree | %d | Mark cross-checked. Weaker than verified: both files could share an old value. |\n", len(r.Agree))
	w("| Prices differ | %d | Never pick a side. Open the provider's pricing page, set the value, record the link and date. |\n", len(r.Differ))
	w("| Only in LiteLLM (first wave) | %d | Import as unverified. |\n", len(r.OnlyLiteLLM))
	w("| Only in LiteLLM (other providers) | %d | Not imported. |\n", r.OtherProvidersSkipped)
	w("| Only in ours | %d | Keep. Check whether the provider retired them. |\n", len(r.OnlyOurs))
	w("| Deprecation dates | %d | Copy the date and its source link. |\n", len(r.Deprecations))
	w("| Provider spelled differently | %d | Keep our name; llm-cost picks its fees from it. |\n\n", len(r.ProviderNames))

	w("## Prices differ\n\n")
	w("A change above %s%% needs a person's approval, however confident the source looks.\n\n", trim(thresholdPercent))
	if len(r.Differ) == 0 {
		w("None.\n\n")
	} else {
		w("| Model | Field | Ours | LiteLLM | Change | Needs approval |\n| --- | --- | --- | --- | --- | --- |\n")
		for _, d := range r.Differ {
			for _, f := range d.Fields {
				change := "-"
				if f.ChangePercent != nil {
					change = trim(*f.ChangePercent) + "%"
				}
				w("| `%s` | %s | %s | %s | %s | %s |\n", d.Model, f.Field, value(f.Ours), value(f.Theirs), change, yes(f.NeedsApproval))
			}
		}
		w("\n")
	}

	w("## Deprecation dates\n\n")
	if len(r.Deprecations) == 0 {
		w("None.\n\n")
	} else {
		w("| Model | Date | Successor | Source |\n| --- | --- | --- | --- |\n")
		for _, d := range r.Deprecations {
			w("| `%s` | %s | %s | %s |\n", d.Model, d.Date, orDash(d.Successor), orDash(d.Source))
		}
		w("\n")
	}

	w("## Only in ours\n\n")
	list(&b, r.OnlyOurs)

	w("## Only in LiteLLM (first wave)\n\n")
	if len(r.OnlyLiteLLM) == 0 {
		w("None.\n\n")
	} else {
		w("| Model | Provider |\n| --- | --- |\n")
		for _, c := range r.OnlyLiteLLM {
			w("| `%s` | %s |\n", c.Model, c.Provider)
		}
		w("\n")
	}

	w("## Provider spelled differently\n\n")
	if len(r.ProviderNames) == 0 {
		w("None.\n")
	} else {
		w("| Model | Ours | LiteLLM |\n| --- | --- | --- |\n")
		for _, n := range r.ProviderNames {
			w("| `%s` | %s | %s |\n", n.Model, n.Ours, n.Theirs)
		}
	}
	return b.String()
}

func list(b *strings.Builder, items []string) {
	if len(items) == 0 {
		b.WriteString("None.\n\n")
		return
	}
	for _, item := range items {
		fmt.Fprintf(b, "- `%s`\n", item)
	}
	b.WriteString("\n")
}

func value(v *float64) string {
	if v == nil {
		return "missing"
	}
	return strconv.FormatFloat(*v, 'g', -1, 64)
}

func trim(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

func yes(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}
