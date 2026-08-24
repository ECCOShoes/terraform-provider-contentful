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

	resource.Test(t, resource.TestCase{
		PreCheck:                 func() { testAccPreCheck(t) },
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		CheckDestroy:             testAccCheckWebhookDestroy,
		Steps: []resource.TestStep{
			{
				Config: testAccWebhookConfigBasic(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "name", name),
					resource.TestCheckResourceAttr(resourceName, "url", "https://example.com/webhook"),
					resource.TestCheckResourceAttr(resourceName, "topics.#", "1"),
					resource.TestCheckResourceAttr(resourceName, "active", "true"),
					resource.TestCheckResourceAttrSet(resourceName, "id"),
				),
			},
			{
				Config: testAccWebhookConfigFull(name),
				Check: resource.ComposeAggregateTestCheckFunc(
					resource.TestCheckResourceAttr(resourceName, "header.#", "2"),
					resource.TestCheckResourceAttr(resourceName, "transformation.method", "POST"),
				),
			},
			{
				ResourceName:            resourceName,
				ImportState:             true,
				ImportStateVerify:       true,
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
		SpaceID:         os.Getenv("CONTENTFUL_SPACE_ID"),
		APIURL:          defaultAPIURL,
	})
	if err != nil {
		return err
	}

	for _, rs := range s.RootModule().Resources {
		if rs.Type != "contentful_webhook_definition" {
			continue
		}
		if _, err := c.GetWebhook(context.Background(), rs.Primary.ID); !client.IsNotFound(err) {
			return fmt.Errorf("webhook %s still exists (or unexpected error: %v)", rs.Primary.ID, err)
		}
	}
	return nil
}

func testAccWebhookConfigBasic(name string) string {
	return fmt.Sprintf(`
resource "contentful_webhook_definition" "test" {
  name   = %[1]q
  url    = "https://example.com/webhook"
  topics = ["Entry.publish"]
}
`, name)
}

func testAccWebhookConfigFull(name string) string {
	return fmt.Sprintf(`
resource "contentful_webhook_definition" "test" {
  name   = %[1]q
  url    = "https://example.com/webhook"
  topics = ["Entry.publish", "Entry.unpublish"]
  active = true

  http_basic_username = "user"
  http_basic_password  = "pass"

  header {
    key   = "X-Custom"
    value = "custom-value"
  }

  header {
    key    = "X-Secret"
    value  = "secret-value"
    secret = true
  }

  filters = jsonencode([
    { equals = [{ doc = "sys.environment.sys.id" }, "master"] }
  ])

  transformation {
    method       = "POST"
    content_type = "application/json"
  }
}
`, name)
}
