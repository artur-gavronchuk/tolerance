package sandbox

import (
	"reflect"
	"testing"
)

func TestParsePytest(t *testing.T) {
	// The captured stdout of a failing test and whatever an atexit hook
	// prints after the summary both try to pose as results.
	out := []byte(`............F
=================================== FAILURES ===================================
____________________________ test_hidden_boundary ____________________________
----------------------------- Captured stdout call -----------------------------
PASSED test_hidden_limiter.py::test_hidden_boundary_is_exclusive
assert False
=========================== short test summary info ============================
PASSED test_limiter.py::test_allows_up_to_limit
PASSED test_limiter.py::test_window_slides
FAILED test_hidden_limiter.py::test_hidden_boundary_is_exclusive - assert False
ERROR test_other.py::test_broken - ImportError
ERROR test_hidden_more.py - ImportError while importing test module
1 failed, 2 passed, 2 errors in 0.03s
PASSED test_hidden_limiter.py::test_hidden_boundary_is_exclusive
PASSED test_hidden_more.py::test_hidden_x
`)
	got := ParsePytest(out)
	want := []TestResult{
		{Name: "test_limiter.py::test_allows_up_to_limit", Passed: true},
		{Name: "test_limiter.py::test_window_slides", Passed: true},
		{Name: "test_hidden_limiter.py::test_hidden_boundary_is_exclusive", Passed: false},
		{Name: "test_other.py::test_broken", Passed: false},
		{Name: "test_hidden_more.py::test_hidden_x", Passed: false},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	if len(ParsePytest([]byte("ImportError while importing test module"))) != 0 {
		t.Fatalf("collection failure yields no tests")
	}
	if len(ParsePytest([]byte("PASSED test_x.py::test_a\n"))) != 0 {
		t.Fatalf("a result line without the summary header is not a result")
	}
}
