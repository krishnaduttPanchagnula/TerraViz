package snapshots

import (
	"testing"

	"terraviz/internal/models"
)

func result(name string) *models.ScanResult {
	return &models.ScanResult{Diagram: models.Diagram{
		Name:        name,
		Resources:   []models.Resource{{ID: "a"}},
		Connections: []models.Connection{{ID: "c"}},
	}}
}

func TestSaveListLoadDelete(t *testing.T) {
	t.Setenv("TERRAVIZ_HOME", t.TempDir())

	snap, err := Save(result("prod"), "before-change")
	if err != nil {
		t.Fatal(err)
	}
	if !validID(snap.ID) || snap.Name != "before-change" || snap.Resources != 1 || snap.Connections != 1 {
		t.Fatalf("unexpected metadata: %#v", snap)
	}

	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != snap.ID {
		t.Fatalf("unexpected list: %#v", list)
	}

	loaded, err := Load(snap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Diagram.Name != "prod" || len(loaded.Diagram.Resources) != 1 {
		t.Fatalf("unexpected loaded result: %#v", loaded.Diagram)
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
