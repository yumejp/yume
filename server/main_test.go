package main

import "testing"

// Open mode stays on loopback unless the operator states the address is
// private; required mode binds anywhere.
func TestCheckOpenBind(t *testing.T) {
	cases := []struct {
		mode, bind, allow string
		ok                bool
	}{
		{"open", "127.0.0.1", "", true},
		{"open", "localhost", "", true},
		{"open", "::1", "", true},
		{"open", "127.0.0.2", "", true},
		{"open", "100.101.102.103", "", false},
		{"open", "0.0.0.0", "", false},
		{"open", "", "", false},
		{"open", "192.168.1.20", "0", false},
		{"open", "100.101.102.103", "1", true},
		{"open", "0.0.0.0", "true", true},
		{"required", "0.0.0.0", "", true},
		{"required", "100.101.102.103", "", true},
	}
	for _, c := range cases {
		err := checkOpenBind(c.mode, c.bind, c.allow)
		if (err == nil) != c.ok {
			t.Errorf("checkOpenBind(%q, %q, %q) = %v, want ok=%v", c.mode, c.bind, c.allow, err, c.ok)
		}
	}
}
