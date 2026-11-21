package snapshots

import (
	"encoding/json"
	"testing"
)

func TestSaveListLoadDelete(t *testing.T) {
	t.Setenv("TERRAVIZ_HOME", t.TempDir())

	snap, err := Save("before-change", []string{"a", "b"}, 2)
	if err != nil {
		t.Fatal(err)
	}
	if !validID(snap.ID) || snap.Name != "before-change" || snap.Items != 2 {
		t.Fatalf("unexpected metadata: %#v", snap)
	}

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != snap.ID {
		t.Fatalf("unexpected list: %#v", list)
	}

	payload, err := Load(snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	if err := json.Unmarshal(payload, &got); err != nil || len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("unexpected payload: %s (%v)", payload, err)
	}

	if err := Delete(snap.ID); err != nil {
		t.Fatal(err)
	}
	list, err = List()
	if err != nil || len(list) != 0 {
		t.Fatalf("delete failed: %v %#v", err, list)
	}
}

func TestRejectsTraversalID(t *testing.T) {
	for _, id := range []string{"../../etc/passwd", "", "abc", "20060102T150405-0123456789ab/x"} {
		if validID(id) {
			t.Errorf("accepted invalid id %q", id)
		}
	}
}
