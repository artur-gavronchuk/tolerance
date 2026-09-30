package lru

import "testing"

func TestPutGet(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	if v, ok := c.Get("a"); !ok || v != "1" {
		t.Fatalf("get a: %q %v", v, ok)
	}
}

func TestEvictsLeastRecentlyUsed(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Get("a") // a is now most recent
	c.Put("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatalf("b should have been evicted")
	}
	if _, ok := c.Get("a"); !ok {
		t.Fatalf("a should survive")
	}
}
