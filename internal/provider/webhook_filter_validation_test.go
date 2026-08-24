package provider

import (
	"fmt"
	"regexp"
	"testing"

	"github.com/hashicorp/terraform-plugin-testing/helper/resource"
)

// TestWebhookFilter_exactlyOneOf is a plan-only regression test for the
// "filter" block's equals/in/regexp validation. It never reaches Create (no
// network access needed): PlanOnly steps stop after ValidateResourceConfig
// and PlanResourceChange, which is exactly where the ExactlyOneOf validator
// runs.
func TestWebhookFilter_exactlyOneOf(t *testing.T) {
	t.Setenv("TF_ACC", "1")

	resource.Test(t, resource.TestCase{
		ProtoV6ProviderFactories: testAccProtoV6ProviderFactories,
		Steps: []resource.TestStep{
			{
				// Exactly one of equals/in/regexp per filter block must be
				// accepted (this used to fail with "2 attributes specified"
				// because the validator was attached to the filter object
				// itself, which always counts as "specified").
				Config:             testAccWebhookConfigFilter(`equals = "master"`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config:             testAccWebhookConfigFilter(`in = ["blogPost", "author"]`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				Config:             testAccWebhookConfigFilter(`regexp = "^blog.*"`),
				PlanOnly:           true,
				ExpectNonEmptyPlan: true,
			},
			{
				// Two of them set at once must still be rejected.
				Config:      testAccWebhookConfigFilter(`equals = "master"` + "\n    " + `in = ["blogPost"]`),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)2 attributes specified when one \(and only one\) of\s*\[filter\[0\]\.equals.*\] is required`),
			},
			{
				// None of them set must still be rejected.
				Config:      testAccWebhookConfigFilter(``),
				PlanOnly:    true,
				ExpectError: regexp.MustCompile(`(?s)No attribute specified when one \(and only one\) of\s*\[filter\[0\]\.equals.*\] is required`),
			},
		},
	})
}

func testAccWebhookConfigFilter(filterBody string) string {
	return fmt.Sprintf(`
provider "contentful" {
  management_token = "dummy-token"
}

resource "contentful_webhook_definition" "test" {
  space_id = "space123"
  name     = "tf_unit_filter"
  url      = "https://example.com/webhook"
  topics   = ["Entry.publish"]

  filter = [
    {
      property = "sys.environment.sys.id"
      %s
    },
  ]
}
`, filterBody)
}
