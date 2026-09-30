package lru

import "testing"

func TestHidden_PutExistingCountsAsUse(t *testing.T) {
	c := New(2)
	c.Put("a", "1")
	c.Put("b", "2")
	c.Put("a", "9")
	c.Put("c", "3")
	if _, ok := c.Get("b"); ok {
		t.Fatalf("b must be evicted, a was touched by Put")
	}
	if v, _ := c.Get("a"); v != "9" {
		t.Fatalf("a must hold the updated value")
	}
}

func TestHidden_GetMovesToFront(t *testing.T) {
	c := New(3)
	for _, k := range []string{"a", "b", "c"} {
		c.Put(k, k)
	}
	c.Get("a")
	c.Get("b")
	c.Put("d", "d")
	if _, ok := c.Get("c"); ok {
		t.Fatalf("c is the least recently used")
	}
}

func TestHidden_LenNeverExceedsCapacity(t *testing.T) {
	c := New(3)
	for i := 0; i < 50; i++ {
		c.Put(string(rune('a'+i%26)), "x")
		if c.Len() > 3 {
			t.Fatalf("len %d > cap", c.Len())
		}
	}
}

func TestHidden_HotKeySurvivesLongSequence(t *testing.T) {
	c := New(3)
	c.Put("hot", "h")
	for i := 0; i < 20; i++ {
		c.Put(string(rune('a'+i)), "x")
		if _, ok := c.Get("hot"); !ok {
			t.Fatalf("hot key evicted after %d puts although it is read every time", i+1)
		}
	}
}

func TestHidden_MissingKey(t *testing.T) {
	c := New(2)
	if _, ok := c.Get("nope"); ok {
		t.Fatalf("missing key must report false")
	}
}
