package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
)

// HTTPClient returns the shared outbound HTTP client.
func (m *Manager) HTTPClient() *http.Client { return m.http }

// ValidateToken checks a bot token via getMe and returns the bot username.
func ValidateToken(ctx context.Context, box *crypto.Box, token string, httpClient *http.Client) (string, error) {
	if token == "" {
		return "", fmt.Errorf("empty token")
	}
	api, err := bot.New(token, bot.WithHTTPClient(15, httpClient))
	if err != nil {
		return "", err
	}
	me, err := api.GetMe(ctx)
	if err != nil {
		return "", fmt.Errorf("invalid token: %w", err)
	}
	if me.Username == "" {
		return "", fmt.Errorf("bot has no username")
	}
	return me.Username, nil
}

// WebhookInfo returns webhook URL and status for a bot.
func (m *Manager) WebhookInfo(ctx context.Context, botID int64) (url, status string) {
	rec, err := m.store.GetBot(ctx, botID)
	if err != nil {
		return "", "unknown"
	}
	url = fmt.Sprintf("%s/wh/tg/%s", m.cfg.PublicBaseURL, rec.PublicID)
	token, err := m.box.Decrypt(rec.TokenEnc)
	if err != nil {
		return url, "error"
	}
	api, err := bot.New(token, bot.WithHTTPClient(15, m.http))
	if err != nil {
		return url, "error"
	}
	info, err := api.GetWebhookInfo(ctx)
	if err != nil {
		return url, "unreachable"
	}
	if info.URL == "" {
		return url, "not set"
	}
	if info.LastErrorMessage != "" {
		return url, info.LastErrorMessage
	}
	return url, "ok"
}

// FetchFile downloads a Telegram file by file_id for receipt display.
func (m *Manager) FetchFile(ctx context.Context, botID int64, fileID string) ([]byte, string, error) {
	rec, err := m.store.GetBot(ctx, botID)
	if err != nil {
		return nil, "", err
	}
	token, err := m.box.Decrypt(rec.TokenEnc)
	if err != nil {
		return nil, "", err
	}
	api, err := bot.New(token, bot.WithHTTPClient(15, m.http))
	if err != nil {
		return nil, "", err
	}
	f, err := api.GetFile(ctx, &bot.GetFileParams{FileID: fileID})
	if err != nil {
		return nil, "", err
	}
	fileURL := fmt.Sprintf("https://api.telegram.org/file/bot%s/%s", token, f.FilePath)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fileURL, nil)
	if err != nil {
		return nil, "", err
	}
	resp, err := m.http.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, "", err
	}
	ctype := resp.Header.Get("Content-Type")
	if ctype == "" {
		ctype = "image/jpeg"
	}
	return data, ctype, nil
}

// BotSettings loads per-bot settings into a map.
func (m *Manager) BotSettings(ctx context.Context, botID int64) map[string]string {
	keys := []string{"approver_tg_id", "support_contact", "welcome_text", "card_numbers", "warn_percent", "force_join_channel"}
	out := make(map[string]string)
	for _, k := range keys {
		v, _ := m.store.GetSetting(ctx, "bot", botID, k)
		out[k] = v
	}
	return out
}

func approverID(ctx context.Context, store *db.Store, botID int64) (int64, bool) {
	v, _ := store.GetSetting(ctx, "bot", botID, "approver_tg_id")
	if v == "" {
		return 0, false
	}
	var id int64
	if _, err := fmt.Sscanf(v, "%d", &id); err != nil || id == 0 {
		return 0, false
	}
	return id, true
}

// checkForceJoin returns true if the user must join a channel before continuing.
func (h *Handlers) checkForceJoin(ctx context.Context, mb *managedBot, user *db.EndUser, chatID int64) (bool, error) {
	channel, _ := h.mgr.store.GetSetting(ctx, "bot", mb.record.ID, "force_join_channel")
	channel = strings.TrimSpace(channel)
	if channel == "" {
		return false, nil
	}
	member, err := mb.api.GetChatMember(ctx, &bot.GetChatMemberParams{
		ChatID: channel,
		UserID: user.TelegramID,
	})
	if err != nil {
		text := i18n.T(user.Lang, "force_join_required", channel)
		joinURL := channelJoinURL(channel)
		var kb *models.InlineKeyboardMarkup
		if joinURL != "" {
			kb = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: channel, URL: joinURL}},
			}}
		}
		_, _ = mb.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: kb})
		return true, nil
	}
	if member.Type == models.ChatMemberTypeLeft || member.Type == models.ChatMemberTypeBanned {
		text := i18n.T(user.Lang, "force_join_required", channel)
		joinURL := channelJoinURL(channel)
		var kb *models.InlineKeyboardMarkup
		if joinURL != "" {
			kb = &models.InlineKeyboardMarkup{InlineKeyboard: [][]models.InlineKeyboardButton{
				{{Text: channel, URL: joinURL}},
			}}
		}
		_, _ = mb.api.SendMessage(ctx, &bot.SendMessageParams{ChatID: chatID, Text: text, ReplyMarkup: kb})
		return true, nil
	}
	return false, nil
}

func channelJoinURL(channel string) string {
	if strings.HasPrefix(channel, "@") {
		return "https://t.me/" + strings.TrimPrefix(channel, "@")
	}
	return ""
}
