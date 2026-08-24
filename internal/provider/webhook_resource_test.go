package provider

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
	"github.com/hashicorp/terraform-plugin-testing/terraform"

	"github.com/ECCOShoes/terraform-provider-contentful/internal/client"
)

func TestAccWebhook_basic(t *testing.T) {
	name := "tf_acc_basic"
	resourceName := "contentful_webhook_definition.test"
	spaceID := os.Getenv("CONTENTFUL_SPACE_ID")

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookConfigBasic(spaceID, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "url", "https://example.com/webhook"),
					resource.TestCheckResourceAttr(resourceName, "topics.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "active", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				Config: testAccWebhookConfigFull(spaceID, name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "header.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "filter.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "filter.0.property", "sys.environment.sys.id"),
					resource.TestCheckResourceAttr(resourceName, "filter.0.equals", "master"),
					resource.TestCheckResourceAttr(resourceName, "filter.1.property", "sys.contentType.sys.id"),
					resource.TestCheckResourceAttr(resourceName, "filter.1.in.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "filter.1.negate", "true"),
					resource.TestCheckResourceAttr(resourceName, "transformation.method", "POST"),
				),
			},
			{
				ResourceName:      resourceName,
				ImportState:       true,
				ImportStateVerify: true,
				ImportStateIdFunc: func(s *terraform.State) (string, error) {
					rs := s.RootModule().Resources[resourceName]
					return rs.Primary.Attributes["space_id"] + "/" + rs.Primary.ID, nil
				},
				ImportStateVerifyIgnore: []string{"http_basic_password", "header"},
			},
		},
	})
}

// testAccCheckWebhookDestroy verifies every contentful_webhook_definition in
// state was actually deleted from Contentful.
func testAccCheckWebhookDestroy(s *terraform.State) error {
	c, err := client.New(client.Config{
		ManagementToken: os.Getenv("CONTENTFUL_MANAGEMENT_TOKEN"),
		APIURL:          defaultAPIURL,
	})
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "contentful_webhook_definition" {
			continue
		}
		spaceID := rs.Primary.Attributes["space_id"]
		if _, err := c.GetWebhook(context.Background(), spaceID, rs.Primary.ID); !client.IsNotFound(err) {
			return fmt.Errorf("webhook %s still exists (or unexpected error: %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccWebhookConfigBasic(spaceID, name string) string {
	return fmt.Sprintf(`
resource "contentful_webhook_definition" "test" {
  space_id = %[1]q
  name     = %[2]q
  url      = "https://example.com/webhook"
  topics   = ["Entry.publish"]
}
`, spaceID, name)
}

func testAccWebhookConfigFull(spaceID, name string) string {
	return fmt.Sprintf(`
resource "contentful_webhook_definition" "test" {
  space_id = %[1]q
  name     = %[2]q
  url      = "https://example.com/webhook"
  topics   = ["Entry.publish", "Entry.unpublish"]
  active   = true

  http_basic_username = "user"
  http_basic_password  = "pass"

  header = [
    {
      key   = "X-Custom"
      value = "custom-value"
    },
    {
      key    = "X-Secret"
      value  = "secret-value"
      secret = true
    },
  ]

  filter = [
    {
      property = "sys.environment.sys.id"
      equals   = "master"
    },
    {
      property = "sys.contentType.sys.id"
      in       = ["blogPost", "author"]
      negate   = true
    },
  ]

  transformation = {
    method       = "POST"
    content_type = "application/json"
  }
}
`, spaceID, name)
}
