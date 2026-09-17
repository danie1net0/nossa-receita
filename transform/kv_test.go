package transform

import (
	"fmt"

	"github.com/dgraph-io/badger/v4"
)

// Readers used by the tests to check what the ETL stored in the key-value
// store; the ETL itself only writes to it and scans it (loadRelational).

func (kv *kv) get(k []byte) ([]string, error) {
	val := kv.pool.Get().(*[]byte)
	*val = (*val)[:0]
	defer kv.pool.Put(val)
	err := kv.db.View(func(txn *badger.Txn) error {
		item, err := txn.Get(k)
		if err != nil {
			if err == badger.ErrKeyNotFound {
				return nil
			}
			return fmt.Errorf("could not get key: %w", err)
		}
		*val, err = item.ValueCopy(*val)
		if err != nil {
			return fmt.Errorf("could not read value: %w", err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("could not get key %s: %w", string(k), err)
	}
	return kv.deserialize(*val)
}

func (kv *kv) getPrefix(k []byte) ([][]string, error) {
	vs := [][]string{}
	err := kv.db.View(func(txn *badger.Txn) error {
		it := txn.NewIterator(badger.DefaultIteratorOptions)
		defer it.Close()
		for it.Seek(k); it.ValidForPrefix(k); it.Next() {
			i := it.Item()
			err := i.Value(func(b []byte) error {
				v, err := kv.deserialize(b)
				if err != nil {
					return fmt.Errorf("could not deserialize %s: %w", string(i.Key()), err)
				}
				vs = append(vs, v)
				return nil
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return vs, nil
}

func (s *source) keyPrefixFor(id string) []byte {
	if !s.isCumulative {
		return s.keyFor(id)
	}
	return fmt.Appendf([]byte{}, "%s::%s", id, s.key)
}
