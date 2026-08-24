# terraform-provider-contentful

A Terraform provider for [Contentful](https://www.contentful.com/), focused on
managing **webhooks**.

## Features

- `contentful_webhook_definition` resource, with full support for Contentful's
  Webhook Definition API: `topics`, `active`, HTTP basic auth, custom
  `header`s (including secret headers), `filters`, and `transformation`.
- Import support.

## Requirements

- [Terraform](https://developer.hashicorp.com/terraform/downloads) >= 1.0
- [Go](https://go.dev/doc/install) >= 1.23 (to build the provider)

## Using the provider

```hcl
terraform {
  required_providers {
    contentful = {
      source = "ECCOShoes/contentful"
    }
  }
}

provider "contentful" {
  management_token = var.contentful_management_token
}

resource "contentful_webhook_definition" "example" {
  space_id = var.contentful_space_id
  name     = "my-webhook"
  url      = "https://www.example.com/contentful-webhook"
  topics   = ["Entry.publish"]
}
```

`space_id` is set per-resource (not on the provider), so a single provider
config can manage webhooks across multiple Contentful spaces. Every provider
setting can also be supplied via environment variables:
`CONTENTFUL_MANAGEMENT_TOKEN` and `CONTENTFUL_API_URL`.

See [`examples/`](./examples) and the generated [`docs/`](./docs) for full usage.

## Developing the provider

```sh
# Build
make build

# Format & vet
make fmt
make vet

# Unit tests
make test

# Regenerate documentation (requires Terraform on PATH)
go generate ./...

# Install the provider into the local plugin directory
make install
```

### Acceptance tests

Acceptance tests create and destroy real resources in a Contentful space and
only run when `TF_ACC` is set. Configure the `CONTENTFUL_*` environment
variables first:

```sh
export CONTENTFUL_MANAGEMENT_TOKEN=...
export CONTENTFUL_SPACE_ID=...
make testacc
```

## License

This provider is distributed under the [MIT License](./LICENSE).

