# Model Hub definitions

The Model Hub is a public, versioned catalog of the LLM providers and models the AI Gateway
supports, built the way Policy Hub is built for policies. This folder holds the definitions and the
tools that check and publish them.

A definition is data, never code. The Hub describes and tests a provider; it never sits on the
request path. A Hub entry becomes the `LlmProviderTemplate` resource that exists today.

## Layout

```
model-hub/
  providers/<name>/v<X.Y>/
    provider.yaml   ProviderDefinition: the template spec plus catalog fields
    models.json     ModelList: the provider's models and their prices
    icon.png        optional, PNG only
  catalog/          loading, validation, projections and the publish bundle
  reconcile/        the pricing comparison with LiteLLM
  cmd/              validate, build-catalog, reconcile, seed
  testdata/         snapshots of today's templates and pricing file
```

## Add a provider

A provider whose API speaks a wire format the gateway already handles (`openai`, `anthropic`,
`gemini`, `mistral`, `bedrock`, `azure-openai`, `azure-ai-foundry`, `typesafe`) is added with
definition files only. [`providers/vllm`](providers/vllm/v1.0/provider.yaml) is the example: an
OpenAI-compatible, self-hosted server. A new wire format needs a transformer policy first, which goes
through Policy Hub review.

1. Create `providers/<name>/v1.0/`. The name is lower case letters, digits and hyphens.
2. Write `provider.yaml`:

   | Field | Meaning |
   | --- | --- |
   | `spec.wireFormat` | The request format the provider speaks |
   | `spec.hosting` | `hosted`, or `self-hosted` for a server the user runs |
   | `spec.defaultUrl` | The provider's base URL. `https` for hosted providers. Leave it out when every user has their own (Azure, Bedrock, vLLM) |
   | `spec.auth` | How the gateway sends the key |
   | `spec.links` | Optional logo and OpenAPI links, on `raw.githubusercontent.com` only |
   | `spec.template` | The `LlmProviderTemplate` spec: where the gateway reads token counts and the model name |

3. Write `models.json`. A self-hosted provider may list no models.
4. Run the validator.

## Add or change a model

Each entry in `models.json` keeps the model's name and `pricing` exactly as they appear in the
gateway's `model_prices.json`, because the `llm-cost` policy looks the name up as written and picks
its fee logic from `pricing.provider`. Field names follow the pricing file, which matches LiteLLM's.

| Field | Meaning |
| --- | --- |
| `status` | `unverified` (imported), `cross-checked` (agrees with LiteLLM) or `verified` (a person read the provider's pricing page) |
| `source`, `checkedOn` | The pricing page and the date it was read. Required for `verified` |
| `deprecationDate`, `successor` | When the provider retires the model, and what replaces it |

The 841 models in the first import came from the gateway's pricing file and are all `unverified`.

## Certification levels

| Level | Earned when | Available |
| --- | --- | --- |
| Validated | The files are well formed and pass the validator | Yes |
| Conformant | The conformance suite passes against recorded replies | Not yet |
| Certified | The conformance suite passes against the real provider | Not yet |

The publish step sets the level. A definition file cannot claim one.

## Commands

Run from this folder. Go is the only requirement.

```bash
# Check every definition, as a pull request would.
go run ./cmd/validate

# Publish: write dist/catalog.json (what the Hub API serves), dist/model_prices.json and the icons.
go run ./cmd/build-catalog -out dist

# Compare the gateway's pricing file with LiteLLM's and write a report. Nothing is changed.
go run ./cmd/reconcile -ours testdata/model_prices.json \
    -litellm <litellm>/model_prices_and_context_window.json -out dist

# Run the tests.
go test ./...
```

## What the checks guard against

| Risk | Check |
| --- | --- |
| A typo or an injected field | Unknown fields are rejected |
| A harmful URL | `https` only for hosted providers, no credentials in URLs, links on one allowed host |
| A slow or harmful expression | Identifiers are at most 256 characters, payload paths start with `$`, path patterns must compile (Go regular expressions run in linear time) |
| A script in an icon | Icons must be PNG, checked by content |
| A path outside the folder | Folder names are checked, and links and files that are not regular files are refused |
| An oversized file | Size limits on every file |
| A price that changes cost reporting | Unknown pricing fields, negative prices and unknown provider names are rejected |

## Pricing reconciliation

`reconcile` sorts every model into a bucket: prices agree, prices differ, only in LiteLLM, only in
ours, and deprecation dates. It never writes the pricing file, never deletes a model and never
picks a side when prices differ. A change above the threshold (default 20%) is marked as needing
approval. LiteLLM's pricing data is MIT-licensed (Copyright (c) 2023 Berri AI), so any value
imported from it keeps that attribution.
