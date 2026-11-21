// Package snapshots stores local, timestamped copies of a scan. It is the
// account-free alternative to a hosted diagram history: files live under the
// user's config directory and never leave the machine.
package snapshots

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"terraviz/internal/models"
)

const dirPermissions = 0o700

// Snapshot is the listing metadata for one stored scan.
type Snapshot struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"created_at"`
	Resources   int       `json:"resources"`
	Connections int       `json:"connections"`
}

type stored struct {
	Snapshot Snapshot          `json:"snapshot"`
	Result   models.ScanResult `json:"result"`
}

// Dir returns the snapshot directory, creating it if needed. TERRAVIZ_HOME
// overrides the location (useful for tests and portable installs).
func Dir() (string, error) {
	base := os.Getenv("TERRAVIZ_HOME")
	if base == "" {
		var err error
		base, err = os.UserConfigDir()
		if err != nil {
			return "", fmt.Errorf("finding config dir: %w", err)
		}
		base = filepath.Join(base, "terraviz")
	}
	dir := filepath.Join(base, "snapshots")
	if err := os.MkdirAll(dir, dirPermissions); err != nil {
		return "", fmt.Errorf("creating snapshot dir: %w", err)
	}
	return dir, nil
}

// Save stores a scan and returns its metadata.
func Save(result *models.ScanResult, name string) (Snapshot, error) {
	if result == nil {
		return Snapshot{}, fmt.Errorf("saving snapshot: no scan result")
	}
	dir, err := Dir()
	if err != nil {
		return Snapshot{}, err
	}

	id := newID()
	snap := Snapshot{
		ID:          id,
		Name:        strings.TrimSpace(name),
		CreatedAt:   time.Now().UTC(),
		Resources:   len(result.Diagram.Resources),
		Connections: len(result.Diagram.Connections),
	}
	if snap.Name == "" {
		snap.Name = result.Diagram.Name
	}
	if snap.Name == "" {
		snap.Name = "Untitled"
	}

	data, err := json.MarshalIndent(stored{Snapshot: snap, Result: *result}, "", "  ")
	if err != nil {
		return Snapshot{}, fmt.Errorf("encoding snapshot: %w", err)
	}
	if err := os.WriteFile(filepath.Join(dir, id+".json"), data, 0o600); err != nil {
		return Snapshot{}, fmt.Errorf("writing snapshot: %w", err)
	}
	return snap, nil
}

// List returns all snapshots, newest first.
func List() ([]Snapshot, error) {
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading snapshots: %w", err)
	}
	out := make([]Snapshot, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		var s stored
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			continue
		}
		if err := json.Unmarshal(data, &s); err == nil && s.Snapshot.ID != "" {
			out = append(out, s.Snapshot)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].CreatedAt.After(out[j].CreatedAt) })
	return out, nil
}

// Load reads a snapshot by ID.
func Load(id string) (*models.ScanResult, error) {
	if !validID(id) {
		return nil, fmt.Errorf("invalid snapshot id")
	}
	dir, err := Dir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, id+".json"))
	if err != nil {
		return nil, fmt.Errorf("reading snapshot %s: %w", id, err)
	}
	var s stored
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decoding snapshot %s: %w", id, err)
	}
	return &s.Result, nil
}

// Delete removes a snapshot by ID.
func Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("invalid snapshot id")
	}
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Join(dir, id+".json")); err != nil {
		return fmt.Errorf("deleting snapshot %s: %w", id, err)
	}
	return nil
}

func newID() string {
	buf := make([]byte, 6)
	_, _ = rand.Read(buf)
	return fmt.Sprintf("%s-%s", time.Now().UTC().Format("20060102T150405"), hex.EncodeToString(buf))
}

// validID only allows the exact shape produced by newID, which prevents path
// traversal through a user-supplied ID.
func validID(id string) bool {
	// 20060102T150405-<12 hex chars> = 28 characters.
	if len(id) != 28 || id[8] != 'T' || id[15] != '-' {
		return false
	}
	for i, c := range id {
		if i == 8 || i == 15 {
			continue
		}
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}
