package store

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "app")
	d := Open(path)
	type rec struct {
		A int    `json:"a"`
		B string `json:"b"`
	}
	var r rec
	if d.Load("x.json", &r) {
		t.Fatal("loaded a file never saved")
	}
	d.Save("x.json", rec{1, "b"}, false)
	d.Save("y.json", rec{2, "c"}, true)
	for name, want := range map[string]string{"x.json": `{"a":1,"b":"b"}`, "y.json": "{\n  \"a\": 2,\n  \"b\": \"c\"\n}"} {
		b, err := os.ReadFile(filepath.Join(path, name))
		if err != nil || string(b) != want {
			t.Fatalf("%s: %q %v", name, b, err)
		}
	}
	// Saved again over it, the file is replaced, with nothing left beside.
	d.Save("y.json", rec{2, "c"}, true)
	if entries, _ := os.ReadDir(path); len(entries) != 2 {
		t.Fatalf("files: %v", entries)
	}
	if !d.Load("y.json", &r) || r != (rec{2, "c"}) {
		t.Fatalf("loaded %+v", r)
	}
	os.WriteFile(filepath.Join(path, "bad.json"), []byte(`{"a":3,`), 0o600)
	if d.Load("bad.json", &r) {
		t.Fatal("a file cut short loaded")
	}
	d.Remove("x.json")
	if _, err := os.Stat(filepath.Join(path, "x.json")); !os.IsNotExist(err) {
		t.Fatalf("removed file: %v", err)
	}

	none := Open("")
	none.Save("x.json", rec{}, false)
	none.Remove("x.json")
	if none.Load("y.json", &r) {
		t.Fatal("a store of no folder loaded")
	}
}
