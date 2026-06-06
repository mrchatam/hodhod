package botconfig

import (
	"context"
	"strconv"
	"strings"

	"github.com/mrchatam/hodhod/internal/db"
)

// Reader loads bot configuration with normalized tables first, legacy settings fallback.
type Reader struct {
	Store *db.Store
}

func (r *Reader) Setting(ctx context.Context, botID int64, key string) string {
	v, _ := r.Store.GetSetting(ctx, "bot", botID, key)
	return v
}

func (r *Reader) WelcomeText(ctx context.Context, botID int64, lang string) string {
	key := "welcome_text_" + lang
	if v := r.Setting(ctx, botID, key); v != "" {
		return v
	}
	if v := r.Setting(ctx, botID, "welcome_text"); v != "" {
		return v
	}
	return ""
}

func (r *Reader) CardNumbersText(ctx context.Context, botID int64) string {
	cards, err := r.Store.ListActivePaymentCards(ctx, botID)
	if err == nil && len(cards) > 0 {
		var lines []string
		for _, c := range cards {
			line := c.CardNumber
			if c.Label != "" {
				line = c.Label + ": " + c.CardNumber
			}
			lines = append(lines, line)
		}
		return strings.Join(lines, "\n")
	}
	return r.Setting(ctx, botID, "card_numbers")
}

func (r *Reader) ForceJoinChannel(ctx context.Context, botID int64) string {
	chs, err := r.Store.ListActiveBotChannels(ctx, botID)
	if err == nil {
		for _, ch := range chs {
			if ch.Mandatory {
				return ch.Username
			}
		}
	}
	return r.Setting(ctx, botID, "force_join_channel")
}

func (r *Reader) ActiveChannels(ctx context.Context, botID int64) ([]db.BotChannel, error) {
	chs, err := r.Store.ListActiveBotChannels(ctx, botID)
	if err != nil {
		return nil, err
	}
	if len(chs) > 0 {
		return chs, nil
	}
	legacy := r.Setting(ctx, botID, "force_join_channel")
	if legacy != "" {
		return []db.BotChannel{{
			BotID: botID, Username: legacy, Label: legacy, Mandatory: true, Active: true,
		}}, nil
	}
	return nil, nil
}

func (r *Reader) ApproverIDs(ctx context.Context, botID int64) []int64 {
	targets, _ := r.Store.ListNotificationTargets(ctx, botID)
	if len(targets) > 0 {
		out := make([]int64, 0, len(targets))
		for _, t := range targets {
			out = append(out, t.TelegramID)
		}
		return out
	}
	return parseApproverIDs(r.Setting(ctx, botID, "approver_tg_id"))
}

func (r *Reader) IsApprover(ctx context.Context, botID, tgID int64) bool {
	for _, id := range r.ApproverIDs(ctx, botID) {
		if id == tgID {
			return true
		}
	}
	bot, err := r.Store.GetBot(ctx, botID)
	if err != nil {
		return false
	}
	agent, err := r.Store.GetAgent(ctx, bot.AgentID)
	if err != nil || agent.TgAdminID == nil {
		return false
	}
	return *agent.TgAdminID == tgID
}

func (r *Reader) MenuButtons(ctx context.Context, botID int64) ([]db.BotMenuButton, error) {
	btns, err := r.Store.ListMenuButtons(ctx, botID)
	if err != nil {
		return nil, err
	}
	if len(btns) > 0 {
		return btns, nil
	}
	defaults := []struct{ key, fa, en string }{
		{"buy", "", ""}, {"services", "", ""}, {"wallet", "", ""}, {"support", "", ""}, {"lang", "", ""},
	}
	out := make([]db.BotMenuButton, len(defaults))
	for i, d := range defaults {
		out[i] = db.BotMenuButton{BotID: botID, ButtonKey: d.key, Enabled: true, SortOrder: i}
	}
	return out, nil
}

func (r *Reader) SettingsMap(ctx context.Context, botID int64) map[string]string {
	keys := []string{
		"approver_tg_id", "support_contact", "welcome_text", "welcome_text_fa", "welcome_text_en",
		"help_text_fa", "help_text_en", "card_numbers", "warn_percent", "force_join_channel", "currency",
		"topup_min_toman", "topup_max_toman", "trial_enabled", "trial_duration_hours", "trial_volume_gb", "trial_max_per_user",
	}
	out := make(map[string]string, len(keys))
	for _, k := range keys {
		out[k] = r.Setting(ctx, botID, k)
	}
	return out
}

func SaveSettingsKeys(store *db.Store, ctx context.Context, botID int64, keys []string, form map[string]string) {
	for _, k := range keys {
		if v, ok := form[k]; ok {
			_ = store.SetSetting(ctx, "bot", botID, k, v)
		}
	}
}

func ParseApproverField(raw string) []int64 {
	return parseApproverIDs(raw)
}

func ParseInt64(s string, def int64) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return def
	}
	return n
}
