package userbot

import (
	"context"
	"encoding/binary"
	"errors"

	"go.etcd.io/bbolt"

	"github.com/gotd/td/telegram/peers"
)

var errNotFound = errors.New("not found")

// peerStorage is a bbolt-backed peers.Storage.
//
// It persists access hashes across restarts so peers resolved from dialogs
// keep working without re-bootstrapping the whole dialog list.
type peerStorage struct {
	db *bbolt.DB
}

var _ peers.Storage = (*peerStorage)(nil)

func bucketKey(prefix, id string) []byte {
	return append([]byte(prefix), id...)
}

func (s *peerStorage) Save(ctx context.Context, key peers.Key, value peers.Value) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(dialogsBucket))
		if b == nil {
			return errors.New("dialogs bucket not found")
		}
		return b.Put(bucketKey(key.Prefix, itoa(key.ID)), itoa64(value.AccessHash))
	})
}

func (s *peerStorage) Find(ctx context.Context, key peers.Key) (peers.Value, bool, error) {
	var out peers.Value
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(dialogsBucket))
		if b == nil {
			return errors.New("dialogs bucket not found")
		}
		v := b.Get(bucketKey(key.Prefix, itoa(key.ID)))
		if v == nil {
			return nil
		}
		out.AccessHash = atoi64(v)
		return nil
	})
	if err != nil {
		return peers.Value{}, false, err
	}
	return out, out.AccessHash != 0, nil
}

func encodePhoneKey(key peers.Key) []byte {
	prefix := []byte(key.Prefix)
	out := make([]byte, 1+len(prefix)+8)
	out[0] = byte(len(prefix))
	copy(out[1:], prefix)
	copy(out[1+len(prefix):], itoa64(key.ID))
	return out
}

func (s *peerStorage) SavePhone(ctx context.Context, phone string, key peers.Key) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(phoneBucket))
		if b == nil {
			return errors.New("phone bucket not found")
		}
		return b.Put([]byte(phone), encodePhoneKey(key))
	})
}

func (s *peerStorage) FindPhone(ctx context.Context, phone string) (peers.Key, peers.Value, bool, error) {
	var (
		key   peers.Key
		value peers.Value
		found bool
	)
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(phoneBucket))
		if b == nil {
			return errors.New("phone bucket not found")
		}
		raw := b.Get([]byte(phone))
		if raw == nil {
			return nil
		}
		if len(raw) < 9 {
			return nil
		}
		plen := int(raw[0])
		if len(raw) < 1+plen+8 {
			return nil
		}
		key.Prefix = string(raw[1 : 1+plen])
		key.ID = atoi64(raw[1+plen:])
		db := tx.Bucket([]byte(dialogsBucket))
		if db == nil {
			return errors.New("dialogs bucket not found")
		}
		hash := db.Get(bucketKey(key.Prefix, itoa(key.ID)))
		if hash != nil {
			value.AccessHash = atoi64(hash)
			found = true
		}
		return nil
	})
	if err != nil {
		return peers.Key{}, peers.Value{}, false, err
	}
	return key, value, found, nil
}

func (s *peerStorage) GetContactsHash(ctx context.Context) (int64, error) {
	var out int64
	err := s.db.View(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(stateBucket))
		if b == nil {
			return errors.New("state bucket not found")
		}
		v := b.Get([]byte("contacts_hash"))
		if v == nil {
			return nil
		}
		out = atoi64(v)
		return nil
	})
	return out, err
}

func (s *peerStorage) SaveContactsHash(ctx context.Context, hash int64) error {
	return s.db.Update(func(tx *bbolt.Tx) error {
		b := tx.Bucket([]byte(stateBucket))
		if b == nil {
			return errors.New("state bucket not found")
		}
		return b.Put([]byte("contacts_hash"), itoa64(hash))
	})
}

func itoa64(v int64) []byte {
	buf := make([]byte, 8)
	binary.LittleEndian.PutUint64(buf, uint64(v))
	return buf
}

func atoi64(b []byte) int64 {
	if len(b) != 8 {
		return 0
	}
	return int64(binary.LittleEndian.Uint64(b))
}

func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	neg := v < 0
	u := uint64(v)
	if neg {
		u = uint64(-v)
	}
	var buf [20]byte
	i := len(buf)
	for u > 0 {
		i--
		buf[i] = byte('0' + u%10)
		u /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
