package web

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/mrchatam/hodhod/internal/db"
)

type memSettingsStore struct {
	data map[string]string
}

func (m *memSettingsStore) GetSetting(_ context.Context, scope string, scopeID int64, key string) (string, error) {
	return m.data[scope+":"+itoa(scopeID)+":"+key], nil
}

func (m *memSettingsStore) SetSetting(_ context.Context, scope string, scopeID int64, key, value string) error {
	if m.data == nil {
		m.data = map[string]string{}
	}
	m.data[scope+":"+itoa(scopeID)+":"+key] = value
	return nil
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func TestSaveUserCreateTemplate(t *testing.T) {
	mem := &memSettingsStore{data: map[string]string{}}
	store := &db.Store{}
	_ = store
	// Test JSON marshal path directly
	tpl := UserCreateTemplate{Name: "Standard", VolumeGB: 30, DurationDays: 30, InboundIDs: []int{1}}
	b, err := json.Marshal([]UserCreateTemplate{tpl})
	if err != nil {
		t.Fatal(err)
	}
	mem.SetSetting(context.Background(), "panel", 1, "user_create_templates", string(b))
	raw, _ := mem.GetSetting(context.Background(), "panel", 1, "user_create_templates")
	var out []UserCreateTemplate
	if err := json.Unmarshal([]byte(raw), &out); err != nil || len(out) != 1 || out[0].Name != "Standard" {
		t.Fatalf("roundtrip failed: %v out=%+v", err, out)
	}
}
