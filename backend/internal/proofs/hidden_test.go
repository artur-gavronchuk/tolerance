package proofs

import "testing"

func TestHiddenTestNames(t *testing.T) {
	goTar, err := TarFiles(map[string][]byte{
		"hidden_test.go": []byte("package x\n\nfunc TestMain(m *testing.M) {}\nfunc TestHidden_A(t *testing.T) {}\nfunc helper() {}\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := HiddenTestNames("go", goTar)
	if err != nil || len(got) != 1 || got[0] != "TestHidden_A" {
		t.Fatalf("go: %v %v", got, err)
	}
	pyTar, err := TarFiles(map[string][]byte{
		"test_hidden_limiter.py":    []byte("import limiter\n\ndef test_hidden_a():\n    pass\n\ndef helper():\n    pass\n\nclass TestX:\n    def test_method(self):\n        pass\n"),
		"tests/test_hidden_more.py": []byte("def test_hidden_b():\n    pass\n"),
		"limiter_fixture.py":        []byte("def test_not_collected():\n    pass\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err = HiddenTestNames("python", pyTar)
	if err != nil || len(got) != 2 || got[0] != "test_hidden_limiter.py::test_hidden_a" || got[1] != "tests/test_hidden_more.py::test_hidden_b" {
		t.Fatalf("python: %v %v", got, err)
	}
	if _, err := HiddenTestNames("rust", pyTar); err == nil {
		t.Fatalf("unknown language must be an error")
	}
}

func TestTestFileTouched(t *testing.T) {
	cases := []struct {
		lang, diff string
		want       bool
	}{
		{"go", "diff --git a/retry_test.go b/retry_test.go\n--- a/retry_test.go\n+++ b/retry_test.go\n", true},
		{"go", "diff --git a/retry.go b/retry.go\n--- a/retry.go\n+++ b/retry.go\n", false},
		{"python", "diff --git a/test_mine.py b/test_mine.py\nnew file mode 100644\n--- /dev/null\n+++ b/test_mine.py\n", true},
		{"python", "diff --git a/pkg/limiter_test.py b/pkg/limiter_test.py\n", true},
		{"python", "diff --git a/conftest.py b/conftest.py\n--- /dev/null\n+++ b/conftest.py\n", true},
		{"python", "diff --git a/pyproject.toml b/pyproject.toml\n", true},
		{"python", "diff --git a/sitecustomize.py b/sitecustomize.py\n", true},
		{"python", "diff --git a/evil.pth b/evil.pth\n", true},
		{"python", "rename from limiter.py\nrename to test_limiter.py\n", true},
		{"python", "diff --git \"a/my dir/test_x.py\" \"b/my dir/test_x.py\"\n", true},
		{"python", "diff --git a/limiter.py b/limiter.py\n--- a/limiter.py\n+++ b/limiter.py\n@@ -1 +1 @@\n-# see test_old.py\n+# conftest.py is mentioned in a comment\n", false},
		{"python", "diff --git a/latest.py b/latest.py\n", false},
	}
	for _, c := range cases {
		if got := TestFileTouched(c.lang, c.diff); got != c.want {
			t.Errorf("%s %q: got %v", c.lang, c.diff, got)
		}
	}
}
