package web

import (
	"context"

	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/panelsettings"
)

// UserCreateTemplate is a saved preset for panel user creation.
type UserCreateTemplate = panelsettings.UserCreateTemplate

func loadUserCreateTemplates(ctx context.Context, store *db.Store, panelID int64) ([]UserCreateTemplate, error) {
	return panelsettings.ListUserCreateTemplates(ctx, store, panelID)
}

func saveUserCreateTemplate(ctx context.Context, store *db.Store, panelID int64, tpl UserCreateTemplate) error {
	return panelsettings.SaveUserCreateTemplate(ctx, store, panelID, tpl)
}

func findUserCreateTemplate(templates []UserCreateTemplate, name string) (*UserCreateTemplate, bool) {
	return panelsettings.FindUserCreateTemplate(templates, name)
}

func deleteUserCreateTemplate(ctx context.Context, store *db.Store, panelID int64, name string) error {
	return panelsettings.DeleteUserCreateTemplate(ctx, store, panelID, name)
}
