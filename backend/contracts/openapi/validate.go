package openapi

import (
	"bytes"
	"io"
	"net/http"
	"testing"

	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
)

// ValidateResponse fails the test if resp does not match the contract for
// req's route. Requests are not validated (tests deliberately send bad
// bodies); responses always are, including error bodies.
func ValidateResponse(t *testing.T, router routers.Router, req *http.Request, resp *http.Response, body []byte) {
	t.Helper()
	route, pathParams, err := router.FindRoute(req)
	if err != nil {
		t.Fatalf("%s %s is not in openapi.yaml: %v", req.Method, req.URL.Path, err)
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{Request: req, PathParams: pathParams, Route: route,
			Options: &openapi3filter.Options{AuthenticationFunc: openapi3filter.NoopAuthenticationFunc}},
		Status: resp.StatusCode, Header: resp.Header, Body: io.NopCloser(bytes.NewReader(body)),
	}
	if err := openapi3filter.ValidateResponse(req.Context(), input); err != nil {
		t.Fatalf("%s %s → %d violates openapi.yaml: %v\n%s", req.Method, req.URL.Path, resp.StatusCode, err, body)
	}
}
