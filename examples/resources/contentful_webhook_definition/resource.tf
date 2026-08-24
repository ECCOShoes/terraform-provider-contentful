resource "contentful_webhook_definition" "example" {
  name   = "my-webhook"
  url    = "https://www.example.com/contentful-webhook"
  topics = ["Entry.publish", "Entry.unpublish", "Asset.*"]

  # Optional: HTTP basic authentication for the webhook call.
  http_basic_username = "webhook-user"
  http_basic_password = var.webhook_basic_password

  # Optional: custom headers. Set secret = true for values that shouldn't be
  # readable back (e.g. shared secrets); Terraform keeps the last configured
  # value for those instead of reading it back from Contentful.
  header {
    key   = "X-Custom-Header"
    value = "custom-value"
  }

  header {
    key    = "X-Webhook-Secret"
    value  = var.webhook_shared_secret
    secret = true
  }

  # Optional: restrict delivery, e.g. to a single environment.
  filters = jsonencode([
    { equals = [{ doc = "sys.environment.sys.id" }, "master"] }
  ])

  # Optional: customize the outgoing request.
  transformation {
    method       = "POST"
    content_type = "application/vnd.contentful.management.v1+json"
  }
}
