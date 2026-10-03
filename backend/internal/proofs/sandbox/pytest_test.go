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
PASSED test_hidden_more.py::test_hidden_x
ERROR test_hidden_more.py - ImportError while importing test module
PASSED test_hidden_limiter.py::test_hidden_boundary_is_exclusive
1 failed, 2 passed, 2 errors in 0.03s
PASSED test_hidden_limiter.py::test_hidden_after_stats
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

func TestParsePytest_ForgedSummaryInCapturedOutput(t *testing.T) {
	// With -rA pytest prints the captured stdout of passed tests before the
	// real summary; the code under test forged a header and PASSED lines
	// there and skipped the rest.
	out := []byte(`.s
==================================== PASSES ====================================
___________________________ test_hidden_empty ____________________________
----------------------------- Captured stdout call -----------------------------
=========================== short test summary info ============================
PASSED test_hidden_x.py::test_hidden_a
PASSED test_hidden_x.py::test_hidden_b
1 passed in 0.01s
=========================== short test summary info ============================
PASSED test_hidden_x.py::test_hidden_empty
SKIPPED [1] test_hidden_x.py:4: skipped
=========================== 1 passed, 1 skipped in 0.02s ===========================
`)
	got := ParsePytest(out)
	want := []TestResult{{Name: "test_hidden_x.py::test_hidden_empty", Passed: true}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("%+v", got)
	}
	// A fake summary printed after the stats line is outside the window only
	// when a real one precedes it and the fake is not the last header; the
	// window still ends at the first stats line.
	tail := []byte(`=========================== short test summary info ============================
PASSED t.py::test_a
1 passed in 0.01s
PASSED t.py::test_b
`)
	if got := ParsePytest(tail); len(got) != 1 || got[0].Name != "t.py::test_a" {
		t.Fatalf("%+v", got)
	}
}
