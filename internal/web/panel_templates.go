package web

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/mrchatam/hodhod/internal/db"
)

const userCreateTemplatesKey = "user_create_templates"
const maxUserCreateTemplates = 20

// UserCreateTemplate is a saved preset for panel user creation.
type UserCreateTemplate struct {
	Name         string `json:"name"`
	VolumeGB     int    `json:"volume_gb"`
	DurationDays int    `json:"duration_days"`
	IPLimit      int    `json:"ip_limit"`
	InboundIDs   []int  `json:"inbound_ids"`
	Note         string `json:"note"`
}

func loadUserCreateTemplates(ctx context.Context, store *db.Store, panelID int64) ([]UserCreateTemplate, error) {
	raw, err := store.GetSetting(ctx, "panel", panelID, userCreateTemplatesKey)
	if err != nil || raw == "" {
		return nil, err
	}
	var out []UserCreateTemplate
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func saveUserCreateTemplate(ctx context.Context, store *db.Store, panelID int64, tpl UserCreateTemplate) error {
	tpl.Name = strings.TrimSpace(tpl.Name)
	if tpl.Name == "" {
		return fmt.Errorf("template name required")
	}
	list, _ := loadUserCreateTemplates(ctx, store, panelID)
	found := false
	for i, t := range list {
		if strings.EqualFold(t.Name, tpl.Name) {
			list[i] = tpl
			found = true
			break
		}
	}
	if !found {
		if len(list) >= maxUserCreateTemplates {
			return fmt.Errorf("maximum %d templates per panel", maxUserCreateTemplates)
		}
		list = append(list, tpl)
	}
	b, err := json.Marshal(list)
	if err != nil {
		return err
	}
	return store.SetSetting(ctx, "panel", panelID, userCreateTemplatesKey, string(b))
}

func deleteUserCreateTemplate(ctx context.Context, store *db.Store, panelID int64, name string) error {
	list, err := loadUserCreateTemplates(ctx, store, panelID)
	if err != nil {
		return err
	}
	name = strings.TrimSpace(name)
	var next []UserCreateTemplate
	for _, t := range list {
		if strings.EqualFold(t.Name, name) {
			continue
		}
		next = append(next, t)
	}
	b, err := json.Marshal(next)
	if err != nil {
		return err
	}
	return store.SetSetting(ctx, "panel", panelID, userCreateTemplatesKey, string(b))
}
