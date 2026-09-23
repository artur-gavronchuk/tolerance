package sandbox

import "context"

// Fake returns a canned result; tests and ARENA_SANDBOX=fake use it.
type Fake struct {
	Result Result
	Err    error
	Calls  []Request
}

func (f *Fake) Run(_ context.Context, req Request) (Result, error) {
	f.Calls = append(f.Calls, req)
	return f.Result, f.Err
}
