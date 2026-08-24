package provider

import (
	"os"
	"testing"

	"github.com/hashicorp/terraform-plugin-framework/providerserver"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// testAccProtoV6ProviderFactories provides the provider to the acceptance test
// framework under the "contentful" name.
var testAccProtoV6ProviderFactories = map[string]func() (tfprotov6.ProviderServer, error){
	"contentful": providerserver.NewProtocol6WithError(New("test")()),
}

// testAccPreCheck verifies the environment is configured before running
// acceptance tests. Acceptance tests talk to a real Contentful space and
// only run when TF_ACC is set.
func testAccPreCheck(t *testing.T) {
	required := []string{
		"CONTENTFUL_MANAGEMENT_TOKEN",
		"CONTENTFUL_SPACE_ID",
	}
	for _, env := range required {
		if os.Getenv(env) == "" {
			t.Fatalf("%s must be set for acceptance tests", env)
		}
	}
}
