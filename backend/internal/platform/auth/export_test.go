package auth

// SetFetcherForTest overrides how a Verifier retrieves its JWKS. It exists
// only in test builds (this file's _test.go suffix keeps it out of the
// compiled binary) so tests can supply an in-process key set instead of a
// real HTTP fetch.
func SetFetcherForTest(v *Verifier, f Fetcher) {
	v.fetch = f
}
