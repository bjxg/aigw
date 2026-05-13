package usage

import (
	"fmt"
	"time"

	"gorm.io/gorm"
)

// GormCreateOAuthSession inserts a new OAuth session record.
func GormCreateOAuthSession(session *OAuthSession) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	now := time.Now().Unix()
	if session.CreatedAt == 0 {
		session.CreatedAt = now
	}
	if session.UpdatedAt == 0 {
		session.UpdatedAt = now
	}
	if session.ExpiresAt == 0 {
		// Default 7 days
		session.ExpiresAt = now + 7*24*60*60
	}

	if err := gormDB.Create(session).Error; err != nil {
		return fmt.Errorf("usage: GORM create oauth session: %w", err)
	}
	return nil
}

// GormGetOAuthSessionByToken retrieves an OAuth session by token.
func GormGetOAuthSessionByToken(token string) (*OAuthSession, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, fmt.Errorf("database not initialised")
	}

	var session OAuthSession
	if err := gormDB.Where("token = ?", token).First(&session).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("usage: GORM get oauth session by token: %w", err)
	}
	return &session, nil
}

// GormDeleteOAuthSessionByToken removes an OAuth session by token.
func GormDeleteOAuthSessionByToken(token string) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	result := gormDB.Where("token = ?", token).Delete(&OAuthSession{})
	if result.Error != nil {
		return fmt.Errorf("usage: GORM delete oauth session by token: %w", result.Error)
	}
	return nil
}

// GormDeleteOAuthSessionByID removes an OAuth session by ID.
func GormDeleteOAuthSessionByID(id string) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	result := gormDB.Where("id = ?", id).Delete(&OAuthSession{})
	if result.Error != nil {
		return fmt.Errorf("usage: GORM delete oauth session by id: %w", result.Error)
	}
	return nil
}

// GormDeleteExpiredOAuthSessions removes all expired OAuth sessions.
func GormDeleteExpiredOAuthSessions() error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	now := time.Now().Unix()
	result := gormDB.Where("expires_at < ?", now).Delete(&OAuthSession{})
	if result.Error != nil {
		return fmt.Errorf("usage: GORM delete expired oauth sessions: %w", result.Error)
	}
	return nil
}
