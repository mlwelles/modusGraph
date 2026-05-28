package migrate

import (
	"context"
	"sort"
	"sync"

	dg "github.com/dolan-in/dgman/v2"
	"github.com/dgraph-io/dgo/v250"
	mg "github.com/matthewmcneely/modusgraph"
	"github.com/matthewmcneely/modusgraph/load"
)

// fakeStore is an in-memory store for unit testing the Runner.
type fakeStore struct {
	mu      sync.Mutex
	records []appliedRecord
	locked  bool
	bootErr error
	saveErr error
	lockErr error
}

func newFakeStore() *fakeStore { return &fakeStore{} }

func (f *fakeStore) bootstrap(_ context.Context) error { return f.bootErr }

func (f *fakeStore) loadApplied(_ context.Context) ([]appliedRecord, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]appliedRecord, len(f.records))
	copy(out, f.records)
	return out, nil
}

func (f *fakeStore) save(_ context.Context, id int64, name, checksum string, durationMs int64) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.records = append(f.records, appliedRecord{ID: id, Name: name, Checksum: checksum, DurationMs: durationMs})
	sort.Slice(f.records, func(i, j int) bool { return f.records[i].ID < f.records[j].ID })
	return nil
}

func (f *fakeStore) remove(_ context.Context, id int64) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.records {
		if r.ID == id {
			f.records = append(f.records[:i], f.records[i+1:]...)
			return nil
		}
	}
	return nil
}

func (f *fakeStore) acquireLock(_ context.Context) error {
	if f.lockErr != nil {
		return f.lockErr
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.locked {
		return ErrLocked
	}
	f.locked = true
	return nil
}

func (f *fakeStore) releaseLock(_ context.Context) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.locked = false
	return nil
}

// stubClient implements mg.Client for unit tests. Schema calls are recorded;
// everything else is a no-op.
type stubClient struct {
	schemaErr      error
	alterSchemaErr error
	appliedTypes   [][]any
	alteredDQL     []string
}

func (s *stubClient) Insert(_ context.Context, _ any) error              { return nil }
func (s *stubClient) InsertRaw(_ context.Context, _ any) error           { return nil }
func (s *stubClient) Upsert(_ context.Context, _ any, _ ...string) error { return nil }
func (s *stubClient) Update(_ context.Context, _ any) error              { return nil }
func (s *stubClient) Get(_ context.Context, _ any, _ string) error       { return nil }
func (s *stubClient) Query(_ context.Context, _ any) *dg.Query           { return nil }
func (s *stubClient) Delete(_ context.Context, _ []string) error         { return nil }
func (s *stubClient) Close()                                             {}
func (s *stubClient) UpdateSchema(_ context.Context, obj ...any) error {
	s.appliedTypes = append(s.appliedTypes, obj)
	return s.schemaErr
}
func (s *stubClient) AlterSchema(_ context.Context, schema string) error {
	s.alteredDQL = append(s.alteredDQL, schema)
	return s.alterSchemaErr
}
func (s *stubClient) GetSchema(_ context.Context) (string, error) { return "", nil }
func (s *stubClient) DropAll(_ context.Context) error             { return nil }
func (s *stubClient) DropData(_ context.Context) error            { return nil }
func (s *stubClient) QueryRaw(_ context.Context, _ string, _ map[string]string) ([]byte, error) {
	return []byte(`{}`), nil
}
func (s *stubClient) DgraphClient() (*dgo.Dgraph, func(), error) { return nil, func() {}, nil }
func (s *stubClient) WithRetry(_ context.Context, _ mg.RetryPolicy, fn func() error) error {
	return fn()
}
func (s *stubClient) LoadData(_ context.Context, _ string, _ ...load.Option) error { return nil }
