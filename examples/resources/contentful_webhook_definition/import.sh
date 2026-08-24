# Webhooks are imported using "<space_id>/<webhook_id>", since a single
# provider config can manage webhooks across multiple Contentful spaces.
# http_basic_password and any secret header values are never returned by the
# API, so they are left unset after import until you reconfigure them.
terraform import contentful_webhook_definition.example abc123spaceid/1a2b3c4d5e6f7g8h9i0j
