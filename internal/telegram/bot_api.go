package telegram

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-telegram/bot"
	"github.com/go-telegram/bot/models"
	"github.com/mrchatam/hodhod/internal/crypto"
	"github.com/mrchatam/hodhod/internal/db"
	"github.com/mrchatam/hodhod/internal/i18n"
)

// HTTPClient returns the shared outbound HTTP client.
func (m *Manager) HTTPClient() *http.Client { return m.http }

var botTokenRE = regexp.MustCompile(`^\d+:[A-Za-z0-9_-]+$`)

var telegramAPIBase = "https://api.telegram.org"

const (
	telegramPollTimeout = time.Minute
	telegramInitTimeout = 30 * time.Second
)

func telegramBotOptions(httpClient *http.Client, skipInitGetMe bool) []bot.Option {
	client := httpClient
	if client == nil {
		client = http.DefaultClient
	}
	opts := []bot.Option{
		bot.WithHTTPClient(telegramPollTimeout, client),
		bot.WithCheckInitTimeout(telegramInitTimeout),
	}
	if skipInitGetMe {
		opts = append(opts, bot.WithSkipGetMe())
	}
	return opts
}

// ValidateToken checks a bot token via getMe and returns the bot username.
func ValidateToken(ctx context.Context, box *crypto.Box, token string, httpClient *http.Client) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", fmt.Errorf("empty token")
	}
	if !botTokenRE.MatchString(token) {
		return "", fmt.Errorf("invalid token format — use the token from @BotFather (e.g. 123456789:AAH…)")
	}
	valCtx, cancel := context.WithTimeout(context.Background(), telegramInitTimeout)
	defer cancel()
	username, err := getMeViaHTTP(valCtx, httpClient, token)
	if err != nil {
		return "", FriendlyTokenError(err)
	}
	if username == "" {
		return "", fmt.Errorf("bot has no @username — set one in @BotFather")
	}
	return username, nil
}

type getMeResponse struct {
	OK     bool `json:"ok"`
	Result struct {
		Username string `json:"username"`
	} `json:"result"`
	Description string `json:"description"`
}

func getMeViaHTTP(ctx context.Context, httpClient *http.Client, token string) (string, error) {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	u := telegramAPIBase + "/bot" + token + "/getMe"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, http.NoBody)
	if err != nil {
		return "", err
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 64*1024))
	if err != nil {
		return "", err
	}
	var payload getMeResponse
	if err := json.Unmarshal(body, &payload); err != nil {
		return "", fmt.Errorf("unexpected Telegram response (status %d)", resp.StatusCode)
	}
	if !payload.OK {
		if resp.StatusCode == http.StatusUnauthorized || strings.Contains(strings.ToLower(payload.Description), "not found") {
			return "", fmt.Errorf("not found: %s", payload.Description)
		}
		return "", fmt.Errorf("telegram API error: %s", payload.Description)
	}
	return payload.Result.Username, nil
}

// FriendlyTokenError maps Telegram API errors to user-facing messages.
func FriendlyTokenError(err error) error {
	if err == nil {
		return nil
	}
	msg := strings.ToLower(err.Error())
	switch {
	case strings.Contains(msg, "not found"), strings.Contains(msg, "404"):
		return fmt.Errorf("invalid or revoked bot token — copy a fresh token from @BotFather")
	case strings.Contains(msg, "connection refused"):
		return fmt.Errorf("cannot reach Telegram API — check server network and firewall")
	case strings.Contains(msg, "i/o timeout"):
		return fmt.Errorf("Docker container cannot reach api.telegram.org — run: sudo bash scripts/fix-docker-egress.sh --apply")
	case strings.Contains(msg, "deadline exceeded"), strings.Contains(msg, "context deadline"):
		return fmt.Errorf("cannot reach Telegram API (timed out) — run: sudo bash scripts/fix-docker-egress.sh --apply")
	case strings.Contains(msg, "timeout"), strings.Contains(msg, "no such host"), strings.Contains(msg, "network"):
		return fmt.Errorf("cannot reach Telegram API — check server network and firewall")
	case strings.Contains(msg, "401"), strings.Contains(msg, "unauthorized"):
		return fmt.Errorf("invalid bot token — verify the token from @BotFather")
	default:
		return fmt.Errorf("could not verify bot token — %s", err.Error())
	}
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
	api, err := bot.New(token, telegramBotOptions(m.http, false)...)
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
	api, err := bot.New(token, telegramBotOptions(m.http, false)...)
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

// SendDocument uploads a file to Telegram via sendDocument (standalone bot token).
func SendDocument(ctx context.Context, httpClient *http.Client, token string, chatID int64, filename string, data []byte, caption string) error {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	var body strings.Builder
	boundary := "hodhodBoundary"
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(`Content-Disposition: form-data; name="chat_id"` + "\r\n\r\n")
	fmt.Fprintf(&body, "%d\r\n", chatID)
	if caption != "" {
		body.WriteString("--" + boundary + "\r\n")
		body.WriteString(`Content-Disposition: form-data; name="caption"` + "\r\n\r\n")
		body.WriteString(caption + "\r\n")
	}
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString(fmt.Sprintf(`Content-Disposition: form-data; name="document"; filename="%s"`, filename) + "\r\n")
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.Write(data)
	body.WriteString("\r\n--" + boundary + "--\r\n")

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendDocument", token)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(body.String()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("telegram sendDocument: %s", string(b))
	}
	return nil
}
