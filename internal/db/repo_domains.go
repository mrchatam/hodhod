package db

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/mrchatam/hodhod/internal/domains"
	"gorm.io/gorm"
)

func (s *Store) GetAgentByDomain(ctx context.Context, host string) (*Agent, error) {
	host = domains.NormalizeDomain(host)
	if host == "" {
		return nil, gorm.ErrRecordNotFound
	}
	var a Agent
	err := s.DB.WithContext(ctx).
		Where("custom_domain = ? AND domain_enabled = ? AND domain_verified_at IS NOT NULL", host, true).
		First(&a).Error
	return &a, err
}

func (s *Store) SetAgentDomain(ctx context.Context, agentID int64, rawDomain string) error {
	domain := domains.NormalizeDomain(rawDomain)
	if rawDomain != "" && domain == "" {
		return fmt.Errorf("invalid domain")
	}
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if domain != "" {
		var existing Agent
		err := s.DB.WithContext(ctx).Where("custom_domain = ? AND id != ?", domain, agentID).First(&existing).Error
		if err == nil {
			return fmt.Errorf("domain already assigned")
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
	}
	token, err := newVerifyToken()
	if err != nil {
		return err
	}
	agent.CustomDomain = domain
	agent.DomainVerifyToken = token
	agent.DomainVerifiedAt = nil
	agent.DomainEnabled = false
	return s.UpdateAgent(ctx, agent)
}

func (s *Store) MarkAgentDomainVerified(ctx context.Context, agentID int64) error {
	now := time.Now()
	return s.DB.WithContext(ctx).Model(&Agent{}).Where("id = ?", agentID).
		Updates(map[string]any{"domain_verified_at": now}).Error
}

func (s *Store) SetAgentDomainEnabled(ctx context.Context, agentID int64, enabled bool) error {
	agent, err := s.GetAgent(ctx, agentID)
	if err != nil {
		return err
	}
	if enabled && (agent.DomainVerifiedAt == nil || agent.CustomDomain == "") {
		return fmt.Errorf("domain must be verified before enabling")
	}
	agent.DomainEnabled = enabled
	return s.UpdateAgent(ctx, agent)
}

func newVerifyToken() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
