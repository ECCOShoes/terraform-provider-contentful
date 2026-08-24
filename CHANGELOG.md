# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Initial release of the `contentful` provider.
- `contentful_webhook_definition` resource for managing Contentful webhooks:
  `name`, `url`, `topics`, `active`, HTTP basic auth, custom `header`s
  (including secret headers), `filters`, and `transformation`.
- Resource import support. `http_basic_password` and secret header values are
  never returned by the API, so they are left unset after import until
  reconfigured.
