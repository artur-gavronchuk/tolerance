package tasks

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
