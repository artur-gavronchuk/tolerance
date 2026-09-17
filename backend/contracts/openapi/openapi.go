// Package openapi embeds the API contract and exposes helpers so handler
// tests can validate real responses against it.
package openapi

import (
	_ "embed"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/routers"
	"github.com/getkin/kin-openapi/routers/gorillamux"
)

//go:embed openapi.yaml
var raw []byte

func Doc() (*openapi3.T, error) {
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromData(raw)
	if err != nil {
		return nil, err
	}
	if err := doc.Validate(loader.Context); err != nil {
		return nil, err
	}
	return doc, nil
}

func Router() (routers.Router, error) {
	doc, err := Doc()
	if err != nil {
		return nil, err
	}
	return gorillamux.NewRouter(doc)
}
