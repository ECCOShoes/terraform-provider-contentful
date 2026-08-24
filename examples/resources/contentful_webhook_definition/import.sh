# Webhooks are imported using their Contentful webhook definition ID.
# http_basic_password and any secret header values are never returned by the
# API, so they are left unset after import until you reconfigure them.
terraform import contentful_webhook_definition.example 1a2b3c4d5e6f7g8h9i0j
