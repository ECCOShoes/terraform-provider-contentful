package client

import (
	"net/http"
	"strings"
	"testing"
)

func TestAPIError_Error_validation(t *testing.T) {
	body := `{"sys":{"id":"ValidationFailed","type":"Error"},"message":"Validation error","details":{"errors":[{"name":"filters","path":[1,"in",0,"doc"],"details":"Filter 1: doc value \"sys.contentTYpe.sys.id\" is not allowed."},{"name":"filters","path":[1,"in",1],"details":"Filter 1: 'in' second item must be an array of id-strings"}]}}`
	err := &APIError{StatusCode: http.StatusUnprocessableEntity, Body: body}

	got := err.Error()
	wantLines := []string{
		"Contentful API error (status 422 ValidationFailed): Validation error",
		`- filters[1].in[0].doc: Filter 1: doc value "sys.contentTYpe.sys.id" is not allowed.`,
		"- filters[1].in[1]: Filter 1: 'in' second item must be an array of id-strings",
	}
	for _, want := range wantLines {
		if !strings.Contains(got, want) {
			t.Errorf("Error() = %q, want it to contain %q", got, want)
		}
	}
}

func TestAPIError_Error_noDetails(t *testing.T) {
	err := &APIError{StatusCode: http.StatusNotFound, Body: `{"sys":{"id":"NotFound","type":"Error"},"message":"The resource could not be found."}`}

	want := "Contentful API error (status 404 NotFound): The resource could not be found."
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}

func TestAPIError_Error_fallsBackOnUnrecognizedBody(t *testing.T) {
	err := &APIError{StatusCode: http.StatusBadGateway, Body: "<html>502 Bad Gateway</html>"}

	want := `Contentful API error (status 502): <html>502 Bad Gateway</html>`
	if got := err.Error(); got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}
}
