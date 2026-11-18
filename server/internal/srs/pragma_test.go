package srs

import (
	"context"
	"database/sql"
	"testing"
)

// SQLite pragmas are per-connection and database/sql pools connections. The
// store registers a driver-level connection hook so EVERY pooled connection
// gets foreign_keys etc. This checks out several dedicated connections at
// once (forcing the pool to open distinct raw connections) and asserts each
// one sees foreign_keys=1. Guards against regressing to db.Exec-only pragma
// setup — and documents why SetMaxOpenConns(1) is NOT the fix (nested
// queries during rows iteration would deadlock; see Open()).
func TestForeignKeysOnAllConnections(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	conns := make([]*sql.Conn, 3)
	for i := range conns {
		c, err := s.q().Conn(ctx)
		if err != nil {
			t.Fatalf("conn %d: %v", i, err)
		}
		defer c.Close()
		conns[i] = c
	}
	for i, c := range conns {
		var v int
		if err := c.QueryRowContext(ctx, `SELECT foreign_keys FROM pragma_foreign_keys`).Scan(&v); err != nil {
			t.Fatalf("conn %d pragma read: %v", i, err)
		}
		if v != 1 {
			t.Fatalf("conn %d has foreign_keys=%d, want 1", i, v)
		}
	}
}
