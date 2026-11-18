package mcpserver

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/yumejp/yume/server/internal/srs"
)

// Without the browser extension the video library is empty; the tutor is
// told why instead of reading it as "watched nothing", and the server's
// instructions say where card writes go.
func TestVideoToolsExplainAnEmptyLibrary(t *testing.T) {
	store, err := srs.Open(filepath.Join(t.TempDir(), "cards.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	ts := httptest.NewServer(Handler("test", func() (*srs.Store, error) { return store, nil }))
	t.Cleanup(ts.Close)

	ctx := context.Background()
	client := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "1"}, nil)
	cs, err := client.Connect(ctx, &mcp.StreamableClientTransport{Endpoint: ts.URL, DisableStandaloneSSE: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cs.Close() })

	instr := cs.InitializeResult().Instructions
	for _, want := range []string{"browser extension", "/update_card", "UTF-8", "translation required"} {
		if !strings.Contains(instr, want) {
			t.Errorf("instructions lack %q", want)
		}
	}
	if strings.Contains(instr, "not shown to the learner") {
		t.Error("instructions still call learner-visible flags hidden")
	}

	res, err := cs.CallTool(ctx, &mcp.CallToolParams{Name: "list_videos", Arguments: map[string]any{}})
	if err != nil || res.IsError {
		t.Fatalf("list_videos: %v %+v", err, res)
	}
	b, _ := json.Marshal(res.StructuredContent)
	var out listVideosOut
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Videos) != 0 || out.Hint != noVideosHint {
		t.Errorf("list_videos on an empty library = %s", b)
	}
}
