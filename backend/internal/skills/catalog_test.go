package skills

import (
	"path/filepath"
	"testing"
)

func TestLoadCatalog(t *testing.T) {
	sk, tasks, err := LoadCatalog(filepath.Join("..", "..", "fixtures", "skills"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sk) != 2 || sk[0].Slug != "go" || sk[1].Slug != "python" {
		t.Fatalf("skills: %+v", sk)
	}
	if len(tasks) != 6 {
		t.Fatalf("tasks: %d", len(tasks))
	}
	for _, task := range tasks {
		if task.SkillSlug == "" || task.Difficulty < 1 || task.HiddenTests == 0 || len(task.RepoTar) == 0 || len(task.HiddenTar) == 0 {
			t.Fatalf("bad task %+v", task.Slug)
		}
	}
}
