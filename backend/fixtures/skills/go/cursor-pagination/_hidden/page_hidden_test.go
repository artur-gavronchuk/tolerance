package page

import "testing"

func walk(all []Item, limit int) []string {
	var seen []string
	cursor := ""
	for i := 0; i < 100; i++ {
		r := Page(all, cursor, limit)
		for _, it := range r.Items {
			seen = append(seen, it.ID)
		}
		if r.Next == "" {
			break
		}
		cursor = r.Next
	}
	return seen
}

func TestHidden_WalkVisitsEachOnce(t *testing.T) {
	for _, limit := range []int{1, 2, 3, 5, 7} {
		seen := walk(items(7), limit)
		if len(seen) != 7 {
			t.Fatalf("limit %d: visited %v", limit, seen)
		}
		for i, id := range seen {
			if id != string(rune('a'+i)) {
				t.Fatalf("limit %d: order %v", limit, seen)
			}
		}
	}
}

func TestHidden_LastPageHasNoNext(t *testing.T) {
	if r := Page(items(4), "b", 2); r.Next != "" {
		t.Fatalf("c,d is the last page, next=%q", r.Next)
	}
}

func TestHidden_UnknownCursorStartsAfterItsPosition(t *testing.T) {
	r := Page([]Item{{ID: "a"}, {ID: "c"}, {ID: "e"}}, "b", 10)
	if len(r.Items) != 2 || r.Items[0].ID != "c" {
		t.Fatalf("%+v", r)
	}
}

func TestHidden_UnsortedInput(t *testing.T) {
	r := Page([]Item{{ID: "c"}, {ID: "a"}, {ID: "b"}}, "", 2)
	if r.Items[0].ID != "a" || r.Items[1].ID != "b" || r.Next != "b" {
		t.Fatalf("%+v", r)
	}
}

func TestHidden_CursorAtLastItemReturnsEmpty(t *testing.T) {
	r := Page(items(3), "c", 2)
	if len(r.Items) != 0 || r.Next != "" {
		t.Fatalf("%+v", r)
	}
}
