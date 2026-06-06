package db

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

func (s *Store) SoftDeleteBot(ctx context.Context, botID int64) error {
	now := time.Now()
	return s.DB.WithContext(ctx).Model(&Bot{}).Where("id = ?", botID).Updates(map[string]any{
		"deleted_at": now,
		"status":     "disabled",
	}).Error
}

func (s *Store) CountActiveServicesByBot(ctx context.Context, botID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Service{}).
		Where("bot_id = ? AND status = ?", botID, "active").Count(&n).Error
	return n, err
}

func (s *Store) CountEndUsersByBot(ctx context.Context, botID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&EndUser{}).Where("bot_id = ?", botID).Count(&n).Error
	return n, err
}

func (s *Store) CountPendingPaymentsByBot(ctx context.Context, botID int64) (int64, error) {
	var n int64
	err := s.DB.WithContext(ctx).Model(&Payment{}).
		Where("bot_id = ? AND status = ?", botID, PaymentPending).Count(&n).Error
	return n, err
}

func (s *Store) ListPaymentCards(ctx context.Context, botID int64) ([]BotPaymentCard, error) {
	var list []BotPaymentCard
	err := s.DB.WithContext(ctx).Where("bot_id = ?", botID).Order("sort_order, id").Find(&list).Error
	return list, err
}

func (s *Store) ListActivePaymentCards(ctx context.Context, botID int64) ([]BotPaymentCard, error) {
	var list []BotPaymentCard
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND active = true", botID).Order("sort_order, id").Find(&list).Error
	return list, err
}

func (s *Store) CreatePaymentCard(ctx context.Context, c *BotPaymentCard) error {
	return s.DB.WithContext(ctx).Create(c).Error
}

func (s *Store) UpdatePaymentCard(ctx context.Context, c *BotPaymentCard) error {
	return s.DB.WithContext(ctx).Save(c).Error
}

func (s *Store) DeletePaymentCard(ctx context.Context, botID, cardID int64) error {
	return s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, cardID).Delete(&BotPaymentCard{}).Error
}

func (s *Store) GetPaymentCard(ctx context.Context, botID, cardID int64) (*BotPaymentCard, error) {
	var c BotPaymentCard
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, cardID).First(&c).Error
	return &c, err
}

func (s *Store) ListBotChannels(ctx context.Context, botID int64) ([]BotChannel, error) {
	var list []BotChannel
	err := s.DB.WithContext(ctx).Where("bot_id = ?", botID).Order("sort_order, id").Find(&list).Error
	return list, err
}

func (s *Store) ListActiveBotChannels(ctx context.Context, botID int64) ([]BotChannel, error) {
	var list []BotChannel
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND active = true", botID).Order("sort_order, id").Find(&list).Error
	return list, err
}

func (s *Store) UpsertBotChannel(ctx context.Context, ch *BotChannel) error {
	if ch.ID == 0 {
		return s.DB.WithContext(ctx).Create(ch).Error
	}
	return s.DB.WithContext(ctx).Save(ch).Error
}

func (s *Store) DeleteBotChannel(ctx context.Context, botID, channelID int64) error {
	return s.DB.WithContext(ctx).Where("bot_id = ? AND id = ?", botID, channelID).Delete(&BotChannel{}).Error
}

func (s *Store) ListMenuButtons(ctx context.Context, botID int64) ([]BotMenuButton, error) {
	var list []BotMenuButton
	err := s.DB.WithContext(ctx).Where("bot_id = ?", botID).Order("sort_order, id").Find(&list).Error
	return list, err
}

func (s *Store) UpsertMenuButton(ctx context.Context, btn *BotMenuButton) error {
	return s.DB.WithContext(ctx).Save(btn).Error
}

func (s *Store) ListNotificationTargets(ctx context.Context, botID int64) ([]BotNotificationTarget, error) {
	var list []BotNotificationTarget
	err := s.DB.WithContext(ctx).Where("bot_id = ?", botID).Find(&list).Error
	return list, err
}

func (s *Store) ReplaceNotificationTargets(ctx context.Context, botID int64, tgIDs []int64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("bot_id = ?", botID).Delete(&BotNotificationTarget{}).Error; err != nil {
			return err
		}
		for _, id := range tgIDs {
			if id == 0 {
				continue
			}
			t := &BotNotificationTarget{BotID: botID, TelegramID: id, Events: []byte(`["receipt_pending","purchase","new_user"]`)}
			if err := tx.Create(t).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) ListEndUsersByBot(ctx context.Context, botID int64, limit, offset int) ([]EndUser, error) {
	var list []EndUser
	err := s.DB.WithContext(ctx).Where("bot_id = ?", botID).
		Order("id DESC").Limit(limit).Offset(offset).Find(&list).Error
	return list, err
}

func (s *Store) CountEndUsersFiltered(ctx context.Context, botID int64, tgID int64) (int64, error) {
	q := s.DB.WithContext(ctx).Model(&EndUser{}).Where("bot_id = ?", botID)
	if tgID > 0 {
		q = q.Where("telegram_id = ?", tgID)
	}
	var n int64
	err := q.Count(&n).Error
	return n, err
}

func (s *Store) ListWalletTxByUser(ctx context.Context, botID, endUserID int64, limit int) ([]WalletTx, error) {
	var list []WalletTx
	err := s.DB.WithContext(ctx).Where("bot_id = ? AND end_user_id = ?", botID, endUserID).
		Order("id DESC").Limit(limit).Find(&list).Error
	return list, err
}

func (s *Store) SetEndUserStatus(ctx context.Context, botID, userID int64, status string) error {
	return s.DB.WithContext(ctx).Model(&EndUser{}).
		Where("bot_id = ? AND id = ?", botID, userID).Update("status", status).Error
}

func (s *Store) IncrementCardRRIndex(ctx context.Context, botID int64) error {
	return s.DB.WithContext(ctx).Model(&Bot{}).Where("id = ?", botID).
		UpdateColumn("card_rr_index", gorm.Expr("card_rr_index + 1")).Error
}

var ErrBotHasActiveServices = errors.New("bot has active services")
