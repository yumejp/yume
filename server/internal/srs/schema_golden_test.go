package srs

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// The schema a fresh database ends up with, described structurally so the
// golden survives cosmetic differences in CREATE text. testdata/schema.json
// is regenerated with UPDATE_GOLDEN=1.
type schemaColumn struct {
	Name, Type, Default string
	NotNull, PK         bool
}

type schemaDump struct {
	Tables   map[string][]schemaColumn
	Indexes  map[string]string
	Triggers map[string]string
	Version  int
}

var wsRe = regexp.MustCompile(`\s+`)

func dumpSchema(t *testing.T, s *Store) schemaDump {
	t.Helper()
	d := schemaDump{Tables: map[string][]schemaColumn{}, Indexes: map[string]string{}, Triggers: map[string]string{}}
	rows, err := s.q().Query(`SELECT type, name, tbl_name, COALESCE(sql, '') FROM sqlite_master WHERE name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	type obj struct{ typ, name, tbl, sql string }
	var objs []obj
	for rows.Next() {
		var o obj
		if err := rows.Scan(&o.typ, &o.name, &o.tbl, &o.sql); err != nil {
			t.Fatal(err)
		}
		objs = append(objs, o)
	}
	rows.Close()
	norm := func(sql string) string { return strings.TrimSpace(wsRe.ReplaceAllString(sql, " ")) }
	for _, o := range objs {
		switch o.typ {
		case "table":
			cols, err := s.q().Query(`PRAGMA table_info(` + o.name + `)`)
			if err != nil {
				t.Fatal(err)
			}
			for cols.Next() {
				var cid, notnull, pk int
				var c schemaColumn
				var dflt *string
				if err := cols.Scan(&cid, &c.Name, &c.Type, &notnull, &dflt, &pk); err != nil {
					t.Fatal(err)
				}
				c.NotNull, c.PK = notnull == 1, pk > 0
				if dflt != nil {
					c.Default = *dflt
				}
				d.Tables[o.name] = append(d.Tables[o.name], c)
			}
			cols.Close()
		case "index":
			if o.sql != "" { // autoindexes have no sql
				d.Indexes[o.name] = norm(o.sql)
			}
		case "trigger":
			d.Triggers[o.name] = norm(o.sql)
		}
	}
	if err := s.q().QueryRow(`PRAGMA user_version`).Scan(&d.Version); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestFreshSchemaMatchesGolden(t *testing.T) {
	s := openTestStore(t)
	got := dumpSchema(t, s)
	b, _ := json.MarshalIndent(got, "", "  ")
	path := filepath.Join("testdata", "schema.json")
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		// The raw CREATE statements, for writing a baseline by hand.
		rows, _ := s.q().Query(`SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY CASE type WHEN 'table' THEN 0 WHEN 'index' THEN 1 ELSE 2 END, rowid`)
		var raw []string
		for rows.Next() {
			var q string
			rows.Scan(&q)
			raw = append(raw, q+";")
		}
		rows.Close()
		os.WriteFile(filepath.Join(os.TempDir(), "yume-schema-raw.sql"), []byte(strings.Join(raw, "\n\n")+"\n"), 0o644)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(strings.ReplaceAll(string(want), "\r\n", "\n")) != strings.TrimSpace(string(b)) {
		var w schemaDump
		_ = json.Unmarshal(want, &w)
		var names []string
		for n := range got.Tables {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Errorf("fresh schema differs from testdata/schema.json (tables: %v); regenerate with UPDATE_GOLDEN=1 only if the change is intended", names)
	}
}
