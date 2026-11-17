# Test fixtures

Snapshots of files that live in [wso2/api-platform](https://github.com/wso2/api-platform), taken at
commit `a9992d2fe`. The tests compare the Hub's definitions with them, so a definition that drifts
from what the gateway and Platform API ship today fails a test.

| Fixture | Copied from |
| --- | --- |
| `templates/gateway/*.yaml` | `gateway/gateway-controller/default-llm-provider-templates/` |
| `templates/platform/*.yaml` | `platform-api/resources/default-llm-provider-templates/` |
| `model_prices.json` | `gateway/configs/llm-pricing/model_prices.json` |
