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

# All settings may instead be supplied via environment variables:
#   CONTENTFUL_MANAGEMENT_TOKEN, CONTENTFUL_API_URL
