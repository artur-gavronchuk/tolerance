// Package city contains public synthetic data for the first evaluator.
package city

import _ "embed"

//go:embed snapshot.json
var Snapshot []byte

//go:embed valid-route.json
var Example []byte
