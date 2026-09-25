package identity

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
)

func TestHashPassword_VerifiesAndRejects(t *testing.T) {
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !strings.HasPrefix(h, "$argon2id$v=19$") {
		t.Fatalf("unexpected encoding: %s", h)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Fatalf("correct password rejected")
	}
	if VerifyPassword(h, "correct horse batter") {
		t.Fatalf("wrong password accepted")
	}
	if VerifyPassword("garbage", "x") {
		t.Fatalf("malformed hash accepted")
	}
	h2, _ := HashPassword("correct horse battery")
	if h2 == h {
		t.Fatalf("salt must differ between hashes")
	}
}

func TestSetHashConcurrency_BoundsConcurrentHashing(t *testing.T) {
	old := hashSem
	t.Cleanup(func() { hashSem = old })
	SetHashConcurrency(1)

	var inflight, maxInflight int32
	var wg sync.WaitGroup
	const n = 8
	wg.Add(n)
	for i := 0; i < n; i++ {
		go func() {
			defer wg.Done()
			acquireHashSlot()
			cur := atomic.AddInt32(&inflight, 1)
			for {
				m := atomic.LoadInt32(&maxInflight)
				if cur <= m || atomic.CompareAndSwapInt32(&maxInflight, m, cur) {
					break
				}
			}
			atomic.AddInt32(&inflight, -1)
			releaseHashSlot()
		}()
	}
	wg.Wait()
	if maxInflight != 1 {
		t.Fatalf("with concurrency 1, at most one goroutine should ever hold the slot at once, observed %d", maxInflight)
	}

	// The real assertion: HashPassword/VerifyPassword still work correctly
	// under a tight concurrency bound, and the encoded format is unchanged.
	SetHashConcurrency(2)
	h, err := HashPassword("correct horse battery")
	if err != nil {
		t.Fatalf("hash: %v", err)
	}
	if !VerifyPassword(h, "correct horse battery") {
		t.Fatalf("hash produced under a bounded semaphore must still verify")
	}

	var wg2 sync.WaitGroup
	for i := 0; i < n; i++ {
		wg2.Add(1)
		go func() {
			defer wg2.Done()
			if !VerifyPassword(h, "correct horse battery") {
				t.Error("concurrent verify must still succeed")
			}
		}()
	}
	wg2.Wait()
}
