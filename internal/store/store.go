// Package store 提供面板的本地持久化（bbolt）。
package store

import (
	"encoding/binary"
	"encoding/json"
	"errors"

	"time"

	bolt "go.etcd.io/bbolt"

	"gost-webui/internal/model"
)

var (
	bucketMeta     = []byte("meta")
	bucketNodes    = []byte("nodes")
	bucketTraffic  = []byte("traffic")
	bucketSessions = []byte("sessions")
)

// ErrNotFound 表示记录不存在。
var ErrNotFound = errors.New("not found")

// Store 封装 bbolt 数据库。
type Store struct {
	db *bolt.DB
}

// Open 打开（不存在则创建）数据库。
func Open(path string) (*Store, error) {
	db, err := bolt.Open(path, 0o600, &bolt.Options{Timeout: 5 * time.Second})
	if err != nil {
		return nil, err
	}
	err = db.Update(func(tx *bolt.Tx) error {
		for _, b := range [][]byte{bucketMeta, bucketNodes, bucketTraffic, bucketSessions} {
			if _, err := tx.CreateBucketIfNotExists(b); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close 关闭数据库。
func (s *Store) Close() error { return s.db.Close() }

// ---------- 设置 ----------

// GetSetting 读取字符串设置。
func (s *Store) GetSetting(key string) (string, bool) {
	var v string
	var ok bool
	_ = s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketMeta).Get([]byte(key))
		if b != nil {
			v, ok = string(b), true
		}
		return nil
	})
	return v, ok
}

// SetSetting 写入字符串设置。
func (s *Store) SetSetting(key, value string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketMeta).Put([]byte(key), []byte(value))
	})
}

// ---------- 节点 ----------

// ListNodes 返回全部节点。
func (s *Store) ListNodes() ([]*model.Node, error) {
	var out []*model.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).ForEach(func(_, v []byte) error {
			var n model.Node
			if err := json.Unmarshal(v, &n); err != nil {
				return err
			}
			out = append(out, &n)
			return nil
		})
	})
	if err != nil {
		return nil, err
	}
	sortNodes(out)
	return out, nil
}

// GetNode 按 ID 读取节点。
func (s *Store) GetNode(id string) (*model.Node, error) {
	var n *model.Node
	err := s.db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketNodes).Get([]byte(id))
		if b == nil {
			return ErrNotFound
		}
		var v model.Node
		if err := json.Unmarshal(b, &v); err != nil {
			return err
		}
		n = &v
		return nil
	})
	return n, err
}

// SaveNode 保存节点。
func (s *Store) SaveNode(n *model.Node) error {
	b, err := json.Marshal(n)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketNodes).Put([]byte(n.ID), b)
	})
}

// DeleteNode 删除节点及其流量明细。
func (s *Store) DeleteNode(id string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		if err := tx.Bucket(bucketNodes).Delete([]byte(id)); err != nil {
			return err
		}
		tb := tx.Bucket(bucketTraffic)
		if sub := tb.Bucket([]byte(id)); sub != nil {
			return tb.DeleteBucket([]byte(id))
		}
		return nil
	})
}

// ---------- 流量明细（小时粒度） ----------

func hourKey(ts int64) []byte {
	var k [8]byte
	binary.BigEndian.PutUint64(k[:], uint64(ts))
	return k[:]
}

// AddTraffic 累加某节点某小时的流量。
func (s *Store) AddTraffic(id string, hourTS int64, in, out uint64) error {
	if in == 0 && out == 0 {
		return nil
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketTraffic)
		sub, err := root.CreateBucketIfNotExists([]byte(id))
		if err != nil {
			return err
		}
		key := hourKey(hourTS)
		var inSum, outSum uint64
		if v := sub.Get(key); len(v) == 16 {
			inSum = binary.BigEndian.Uint64(v[:8])
			outSum = binary.BigEndian.Uint64(v[8:])
		}
		var buf [16]byte
		binary.BigEndian.PutUint64(buf[:8], inSum+in)
		binary.BigEndian.PutUint64(buf[8:], outSum+out)
		return sub.Put(key, buf[:])
	})
}

// RangeTraffic 返回 [from, to) 区间内的小时流量点（按时间排序）。
func (s *Store) RangeTraffic(id string, from, to int64) ([]model.Point, error) {
	out := []model.Point{}
	err := s.db.View(func(tx *bolt.Tx) error {
		sub := tx.Bucket(bucketTraffic).Bucket([]byte(id))
		if sub == nil {
			return nil
		}
		c := sub.Cursor()
		// 起始时间对齐到整点：流量按小时聚合，未对齐会导致漏掉当前小时的数据
		for k, v := c.Seek(hourKey(LocalHourStart(from))); k != nil; k, v = c.Next() {
			if len(k) != 8 || len(v) != 16 {
				continue
			}
			ts := int64(binary.BigEndian.Uint64(k))
			if ts >= to {
				break
			}
			out = append(out, model.Point{
				TS:  ts,
				In:  binary.BigEndian.Uint64(v[:8]),
				Out: binary.BigEndian.Uint64(v[8:]),
			})
		}
		return nil
	})
	return out, err
}

// PruneTraffic 删除 before 之前的流量明细。
func (s *Store) PruneTraffic(before int64) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		root := tx.Bucket(bucketTraffic)
		return root.ForEach(func(name, _ []byte) error {
			sub := root.Bucket(name)
			if sub == nil {
				return nil
			}
			c := sub.Cursor()
			var keys [][]byte
			for k, _ := c.First(); k != nil; k, _ = c.Next() {
				if len(k) != 8 {
					continue
				}
				if int64(binary.BigEndian.Uint64(k)) >= before {
					break
				}
				dup := make([]byte, len(k))
				copy(dup, k)
				keys = append(keys, dup)
			}
			for _, k := range keys {
				if err := sub.Delete(k); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

// ---------- 会话 ----------

// Session 是一个登录会话。
type Session struct {
	User      string `json:"user"`
	ExpiresAt int64  `json:"expiresAt"`
}

// SaveSession 保存会话。
func (s *Store) SaveSession(token string, ses Session) error {
	b, err := json.Marshal(ses)
	if err != nil {
		return err
	}
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).Put([]byte(token), b)
	})
}

// GetSession 读取会话。
func (s *Store) GetSession(token string) (Session, bool) {
	var ses Session
	var ok bool
	_ = s.db.View(func(tx *bolt.Tx) error {
		v := tx.Bucket(bucketSessions).Get([]byte(token))
		if v == nil {
			return nil
		}
		if err := json.Unmarshal(v, &ses); err != nil {
			return nil
		}
		ok = true
		return nil
	})
	return ses, ok
}

// DeleteSession 删除会话。
func (s *Store) DeleteSession(token string) error {
	return s.db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bucketSessions).Delete([]byte(token))
	})
}

// PruneSessions 清理过期会话。
func (s *Store) PruneSessions() error {
	now := time.Now().Unix()
	return s.db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucketSessions)
		c := b.Cursor()
		var keys [][]byte
		for k, v := c.First(); k != nil; k, v = c.Next() {
			var ses Session
			if json.Unmarshal(v, &ses) != nil || ses.ExpiresAt < now {
				dup := make([]byte, len(k))
				copy(dup, k)
				keys = append(keys, dup)
			}
		}
		for _, k := range keys {
			if err := b.Delete(k); err != nil {
				return err
			}
		}
		return nil
	})
}

func sortNodes(ns []*model.Node) {
	for i := 1; i < len(ns); i++ {
		for j := i; j > 0 && ns[j].CreatedAt < ns[j-1].CreatedAt; j-- {
			ns[j], ns[j-1] = ns[j-1], ns[j]
		}
	}
}

// LocalDayStart 返回 ts 所在本地自然日的 0 点时间戳。
func LocalDayStart(ts int64) int64 {
	t := time.Unix(ts, 0)
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Unix()
}

// LocalHourStart 返回 ts 所在整点的时间戳。
func LocalHourStart(ts int64) int64 {
	return ts - ts%3600
}
