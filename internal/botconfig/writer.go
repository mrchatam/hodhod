package botconfig

import (
	"context"

	"github.com/mrchatam/hodhod/internal/db"
)

// Writer is the single write path for bot KV settings.
type Writer struct {
	Store *db.Store
}

// SaveKeys writes bot-scoped settings from a form map.
func (w *Writer) SaveKeys(ctx context.Context, botID int64, keys []string, form map[string]string) {
	SaveSettingsKeys(w.Store, ctx, botID, keys, form)
}

// SetBool writes a boolean bot setting as "true"/"false".
func (w *Writer) SetBool(ctx context.Context, botID int64, key string, on bool) error {
	v := "false"
	if on {
		v = "true"
	}
	return w.Store.SetSetting(ctx, "bot", botID, key, v)
}
