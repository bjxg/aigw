package usage

import (
	"fmt"
	"strings"

	log "github.com/sirupsen/logrus"
	"gorm.io/gorm"
)

// --- GORM User helpers ---

// GormListUsers retrieves a paginated list of users with optional search and role filter.
// search performs fuzzy matching on name, username, and email.
// role performs exact match (empty string = no filter).
// page and pageSize are 1-based; returns users and total count.
func GormListUsers(search, role string, page, pageSize int) ([]User, int64, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, 0, fmt.Errorf("database not initialised")
	}

	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}

	query := gormDB.Model(&User{})

	// Apply role filter
	if role != "" {
		query = query.Where("role = ?", role)
	}

	// Apply search filter (fuzzy match on name, username, email)
	search = strings.TrimSpace(search)
	if search != "" {
		like := "%" + search + "%"
		query = query.Where("name LIKE ? OR username LIKE ? OR email LIKE ?", like, like, like)
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		return nil, 0, fmt.Errorf("usage: GORM count users: %w", err)
	}

	var users []User
	offset := (page - 1) * pageSize
	if err := query.Order("id ASC").Offset(offset).Limit(pageSize).Find(&users).Error; err != nil {
		return nil, 0, fmt.Errorf("usage: GORM list users: %w", err)
	}

	return users, total, nil
}

// GormGetUserByID retrieves a single user by ID.
func GormGetUserByID(id int64) (*User, error) {
	gormDB := getGormDB()
	if gormDB == nil {
		return nil, fmt.Errorf("database not initialised")
	}

	var user User
	if err := gormDB.Where("id = ?", id).First(&user).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, nil
		}
		return nil, fmt.Errorf("usage: GORM get user by id %d: %w", id, err)
	}
	return &user, nil
}

// GormCreateUser inserts a new user record.
func GormCreateUser(user *User) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	user.Name = strings.TrimSpace(user.Name)
	if user.Name == "" {
		return fmt.Errorf("name is required")
	}
	user.Role = strings.TrimSpace(user.Role)
	if user.Role == "" {
		user.Role = UserRolePending
	}
	if !IsValidUserRole(user.Role) {
		return fmt.Errorf("invalid role: %s", user.Role)
	}
	if user.Username != nil {
		trimmed := strings.TrimSpace(*user.Username)
		if trimmed == "" {
			user.Username = nil
		} else {
			user.Username = &trimmed
		}
	}
	if user.Email != nil {
		trimmed := strings.TrimSpace(*user.Email)
		if trimmed == "" {
			user.Email = nil
		} else {
			user.Email = &trimmed
		}
	}

	// Check username uniqueness
	if user.Username != nil && *user.Username != "" {
		var count int64
		gormDB.Model(&User{}).Where("username = ?", *user.Username).Count(&count)
		if count > 0 {
			return fmt.Errorf("duplicate username: %s", *user.Username)
		}
	}

	// Check email uniqueness
	if user.Email != nil && *user.Email != "" {
		var count int64
		gormDB.Model(&User{}).Where("email = ?", *user.Email).Count(&count)
		if count > 0 {
			return fmt.Errorf("duplicate email: %s", *user.Email)
		}
	}

	if err := gormDB.Create(user).Error; err != nil {
		return fmt.Errorf("usage: GORM create user: %w", err)
	}
	return nil
}

// GormUpdateUser updates an existing user by ID.
// Only non-nil fields in the patch map are updated.
func GormUpdateUser(id int64, updates map[string]interface{}) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	// Validate role if present
	if roleVal, ok := updates["role"]; ok {
		roleStr, _ := roleVal.(string)
		if !IsValidUserRole(roleStr) {
			return fmt.Errorf("invalid role: %s", roleStr)
		}
	}

	// Validate name if present
	if nameVal, ok := updates["name"]; ok {
		nameStr, _ := nameVal.(string)
		if strings.TrimSpace(nameStr) == "" {
			return fmt.Errorf("name is required")
		}
	}

	// Check username uniqueness before update
	if usernameVal, ok := updates["username"]; ok {
		if usernameVal != nil {
			usernameStr, _ := usernameVal.(string)
			if strings.TrimSpace(usernameStr) != "" {
				var count int64
				gormDB.Model(&User{}).Where("username = ? AND id != ?", usernameStr, id).Count(&count)
				if count > 0 {
					return fmt.Errorf("duplicate username: %s", usernameStr)
				}
			}
		}
	}

	// Check email uniqueness before update
	if emailVal, ok := updates["email"]; ok {
		if emailVal != nil {
			emailStr, _ := emailVal.(string)
			if strings.TrimSpace(emailStr) != "" {
				var count int64
				gormDB.Model(&User{}).Where("email = ? AND id != ?", emailStr, id).Count(&count)
				if count > 0 {
					return fmt.Errorf("duplicate email: %s", emailStr)
				}
			}
		}
	}

	// Handle nullable fields: empty string => nil
	if usernameVal, ok := updates["username"]; ok {
		usernameStr, ok := usernameVal.(string)
		if ok && strings.TrimSpace(usernameStr) == "" {
			updates["username"] = nil
		}
	}
	if emailVal, ok := updates["email"]; ok {
		emailStr, ok := emailVal.(string)
		if ok && strings.TrimSpace(emailStr) == "" {
			updates["email"] = nil
		}
	}

	result := gormDB.Model(&User{}).Where("id = ?", id).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("usage: GORM update user %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// GormDeleteUser removes a user by ID.
func GormDeleteUser(id int64) error {
	gormDB := getGormDB()
	if gormDB == nil {
		return fmt.Errorf("database not initialised")
	}

	result := gormDB.Where("id = ?", id).Delete(&User{})
	if result.Error != nil {
		return fmt.Errorf("usage: GORM delete user %d: %w", id, result.Error)
	}
	if result.RowsAffected == 0 {
		log.Warnf("usage: GORM delete user %d: no rows affected", id)
	}
	return nil
}
