package dict

import (
	"errors"
	"strings"
	"testing"
)

func TestNotInstalledNamesTheSetupCommand(t *testing.T) {
	old := setupCommand
	t.Cleanup(func() { setupCommand = old })

	SetDataRoot("/srv/yume")
	err := notInstalled()
	if !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("%v is not ErrNotInstalled", err)
	}
	want := "dictionary not installed: run `yume-server setup -data /srv/yume` (or set YUME_JMDICT), then restart the server"
	if err.Error() != want {
		t.Fatalf("got  %q\nwant %q", err, want)
	}

	// A folder with a space is quoted so the command can be pasted.
	SetDataRoot("/Users/a/Library/Application Support/Yume")
	if got := notInstalled().Error(); !strings.Contains(got, `-data "/Users/a/Library/Application Support/Yume"`) {
		t.Fatalf("unquoted folder: %q", got)
	}
}
