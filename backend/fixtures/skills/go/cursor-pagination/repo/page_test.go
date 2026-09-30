package page

import "testing"

func items(n int) []Item {
	out := make([]Item, n)
	for i := range out {
		out[i] = Item{ID: string(rune('a' + i)), Name: "n"}
	}
	return out
}

func TestFirstPage(t *testing.T) {
	r := Page(items(5), "", 2)
	if len(r.Items) != 2 || r.Items[0].ID != "a" || r.Next != "b" {
		t.Fatalf("%+v", r)
	}
}

func TestSecondPageStartsAfterCursor(t *testing.T) {
	r := Page(items(5), "b", 2)
	if len(r.Items) != 2 || r.Items[0].ID != "c" || r.Items[1].ID != "d" {
		t.Fatalf("%+v", r)
	}
}
