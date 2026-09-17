package transform

import (
	"context"
	"log/slog"
	"sync"
	"testing"
)

type testDB struct {
	lock       sync.Mutex
	data       map[string]string
	relational map[string]int
	byBase     map[string][]string
}

func (db *testDB) PreLoad() error {
	db.lock.Lock()
	defer db.lock.Unlock()
	db.data = make(map[string]string)
	return nil
}

func (db *testDB) PostLoad() error {
	return nil
}

func (db *testDB) MetaSave(key, value string) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	db.data[key] = value
	return nil
}

func (db *testDB) CNPJsByBase(_ context.Context, _ string, bases []string) (map[string][]string, error) {
	db.lock.Lock()
	defer db.lock.Unlock()
	out := make(map[string][]string)
	for _, b := range bases {
		out[b] = db.byBase[b]
	}
	return out, nil
}

func (db *testDB) CopyRelational(_ context.Context, table string, _ []string, rows [][]any) error {
	db.lock.Lock()
	defer db.lock.Unlock()
	if db.relational == nil {
		db.relational = make(map[string]int)
	}
	db.relational[table] += len(rows)
	return nil
}

func TestLoadEstabelecimentos(t *testing.T) {
	ctx := context.Background()
	srcs := sources()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	ext := loadAllTestSources(t, kv, srcs)
	db := &testDB{}
	if err := db.PreLoad(); err != nil {
		t.Fatalf("expected no error calling PreLoad, got %s", err)
	}
	if err := loadEstabelecimentos(ctx, slog.New(slog.DiscardHandler), srcs, db, 8192, ext, ""); err != nil {
		t.Fatalf("expected no error processing test data, got %s", err)
	}
	db.lock.Lock()
	defer db.lock.Unlock()
	if got := db.relational["estabelecimento"]; got != 1 {
		t.Errorf("expected 1 estabelecimento to be persisted, got %d", got)
	}
	if got := db.relational["estabelecimento_cnae"]; got == 0 {
		t.Error("expected the secondary CNAEs to be persisted")
	}
}

func TestLoadRelational(t *testing.T) {
	ctx := context.Background()
	kv, err := newBadger(t.TempDir(), false)
	if err != nil {
		t.Fatalf("expected no error creating badger, got %s", err)
	}
	defer func() {
		if err := kv.db.Close(); err != nil {
			t.Errorf("expected no error closing badger, got %s", err)
		}
	}()
	srcs := sources()
	loadAllTestSources(t, kv, srcs)
	db := &testDB{}
	if _, err := loadRelational(ctx, kv, db, srcs, ""); err != nil {
		t.Fatalf("expected no error loading relational data, got %s", err)
	}
	db.lock.Lock()
	defer db.lock.Unlock()
	// The sampled Empresas source carries 2 bases, independent of how many have
	// an establishment in the sampled Estabelecimentos file.
	if got := db.relational["empresa"]; got != 2 {
		t.Errorf("expected 2 empresa, got %d", got)
	}
	if got := db.relational["cnae"]; got == 0 {
		t.Error("expected the cnae reference table to be populated")
	}
	if got := db.relational["estabelecimento"]; got != 0 {
		t.Errorf("expected estabelecimento to come from the stage-3 stream, not loadRelational, got %d", got)
	}
	total := 0
	for _, n := range db.relational {
		total += n
	}
	if total == 0 {
		t.Error("expected loadRelational to write some rows, got none")
	}
}
