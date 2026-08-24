resource "contentful_webhook_definition" "example" {
  space_id = var.contentful_space_id
  name     = "my-webhook"
  url      = "https://www.example.com/contentful-webhook"
  topics   = ["Entry.publish", "Entry.unpublish", "Asset.*"]

  # Optional: whether the webhook is enabled. Defaults to true.
  active = true

  # Optional: HTTP basic authentication for the webhook call.
  http_basic_username = "webhook-user"
  http_basic_password = var.webhook_basic_password

  # Optional: custom headers. Set secret = true for values that shouldn't be
  # readable back (e.g. shared secrets); Terraform keeps the last configured
  # value for those instead of reading it back from Contentful.
  header = [
    {
      key   = "X-Custom-Header"
      value = "custom-value"
    },
    {
      key    = "X-Webhook-Secret"
      value  = var.webhook_shared_secret
      secret = true
    },
  ]

  # Optional: restrict delivery, e.g. to a single environment. Multiple
  # filter entries are combined with AND.
  filter = [
    {
      property = "sys.environment.sys.id"
      equals   = "master"
    },
    # negate = true inverts the match, i.e. "not in".
    {
      property = "sys.contentType.sys.id"
      in       = ["blogPost", "author"]
      negate   = true
    },
  ]

  # Optional: customize the outgoing request.
  transformation = {
    method       = "POST"
    content_type = "application/vnd.contentful.management.v1+json"
  }
}
