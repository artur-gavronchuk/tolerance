package openapi_test

import (
	"testing"

	"tolerance/contracts/openapi"
)

func TestDoc_IsValid(t *testing.T) {
	if _, err := openapi.Doc(); err != nil {
		t.Fatalf("openapi.yaml must be a valid OpenAPI 3 document: %v", err)
	}
}
