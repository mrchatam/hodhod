package botconfig

import (
	"context"
	"fmt"
	"math/rand"
	"strconv"
	"strings"

	"github.com/mrchatam/hodhod/internal/db"
)

// CardPicker selects a payment card for display.
type CardPicker struct {
	Store *db.Store
}

// Pick returns one active card using the bot display mode.
func (p *CardPicker) Pick(ctx context.Context, bot *db.Bot) (*db.BotPaymentCard, error) {
	cards, err := p.Store.ListActivePaymentCards(ctx, bot.ID)
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, nil
	}
	mode := bot.CardDisplayMode
	if mode == "" {
		mode = "random"
	}
	switch mode {
	case "fixed_order":
		return &cards[0], nil
	case "round_robin":
		idx := bot.CardRRIndex % len(cards)
		_ = p.Store.IncrementCardRRIndex(ctx, bot.ID)
		return &cards[idx], nil
	case "weighted":
		total := 0
		for _, c := range cards {
			w := c.Weight
			if w < 1 {
				w = 1
			}
			total += w
		}
		if total <= 0 {
			return &cards[0], nil
		}
		r := rand.Intn(total)
		for _, c := range cards {
			w := c.Weight
			if w < 1 {
				w = 1
			}
			r -= w
			if r < 0 {
				return &c, nil
			}
		}
		return &cards[0], nil
	default:
		return &cards[rand.Intn(len(cards))], nil
	}
}

// FormatCard returns user-facing card text.
func FormatCard(c *db.BotPaymentCard) string {
	if c == nil {
		return ""
	}
	if c.Label != "" {
		return fmt.Sprintf("%s\n%s", c.Label, c.CardNumber)
	}
	return c.CardNumber
}

func parseApproverIDs(raw string) []int64 {
	var out []int64
	for _, p := range strings.Split(raw, ",") {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		id, err := strconv.ParseInt(p, 10, 64)
		if err == nil && id != 0 {
			out = append(out, id)
		}
	}
	return out
}
