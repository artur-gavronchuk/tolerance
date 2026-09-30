package skills

import (
	"path/filepath"
	"testing"
)

func TestHiddenNamesByTask(t *testing.T) {
	sk, tasks, err := LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	m, err := HiddenNamesByTask(sk, tasks)
	if err != nil {
		t.Fatal(err)
	}
	if len(m) != len(tasks) {
		t.Fatalf("got %d entries for %d tasks", len(m), len(tasks))
	}
	for _, task := range tasks {
		if len(m[task.Slug]) != task.HiddenTests {
			t.Errorf("%s: %d names, manifest says %d", task.Slug, len(m[task.Slug]), task.HiddenTests)
		}
	}
	if got := m["py-interval-merge"]; !contains(got, "test_hidden_intervals.py::test_hidden_points_touching") {
		t.Errorf("python names: %v", got)
	}
	if got := m["go-cursor-pagination"]; !contains(got, "TestHidden_CursorAtLastItemReturnsEmpty") {
		t.Errorf("go names: %v", got)
	}
	if _, err := HiddenNamesByTask(nil, tasks); err == nil {
		t.Fatalf("unknown skill must be an error")
	}
}
