package runstate

import (
	"fmt"
	"sync"
	"testing"
)

func TestLockDir_ConcurrentStartsFromTenStoresAllLand(t *testing.T) {
	dir := t.TempDir()
	id := testIdentity()
	const n = 10

	release := make(chan struct{})
	errs := make([]error, n)
	var wg sync.WaitGroup
	for i := range n {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			s, err := Open(dir)
			if err != nil {
				errs[i] = err
				return
			}
			<-release
			_, errs[i] = s.Start(id, KindMutationRow, fmt.Sprintf("row-%d", i), map[string]int{"index": i}, "", "")
		}(i)
	}
	close(release)
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d: %v", i, err)
		}
	}

	l, err := openStore(t, dir).Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(l.Records) != n {
		t.Fatalf("records = %d, want %d; a concurrent Start lost another's record", len(l.Records), n)
	}

	seqs := make(map[int]bool, n)
	for _, rec := range l.Records {
		if rec.Status != StatusStarted {
			t.Errorf("record %s = %q, want %q", rec.ID, rec.Status, StatusStarted)
		}
		if seqs[rec.Seq] {
			t.Errorf("seq %d appears twice", rec.Seq)
		}
		seqs[rec.Seq] = true
	}
	for i := range n {
		key := fmt.Sprintf("row-%d", i)
		if Current(l, KindMutationRow, key) == nil {
			t.Errorf("no current record for %s", key)
		}
		if !seqs[i+1] {
			t.Errorf("seq %d missing; seqs are not 1..%d", i+1, n)
		}
	}
}
