package management

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

// GetUsers handles GET /v0/management/users
// Query params: page (default 1), page_size (default 20), search, role
func (h *Handler) GetUsers(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	search := strings.TrimSpace(c.Query("search"))
	role := strings.TrimSpace(c.Query("role"))

	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}

	users, total, err := usage.GormListUsers(search, role, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if users == nil {
		users = []usage.User{}
	}
	c.JSON(200, gin.H{
		"users": users,
		"total": total,
	})
}

// PostUser handles POST /v0/management/users
func (h *Handler) PostUser(c *gin.Context) {
	var body struct {
		Name       string  `json:"name" binding:"required"`
		Username   *string `json:"username"`
		Email      *string `json:"email"`
		Role       string  `json:"role"`
		LastSeenAt *string `json:"last_seen_at"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body: name is required"})
		return
	}

	user := &usage.User{
		Name:     body.Name,
		Username: body.Username,
		Email:    body.Email,
		Role:     body.Role,
	}

	if err := usage.GormCreateUser(user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// PutUser handles PUT /v0/management/users/:id
func (h *Handler) PutUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	var body struct {
		Name     string  `json:"name"`
		Username *string `json:"username"`
		Email    *string `json:"email"`
		Role     string  `json:"role"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid body"})
		return
	}

	updates := make(map[string]interface{})
	updates["name"] = strings.TrimSpace(body.Name)

	// Username: nil pointer → set DB to NULL; non-nil → trimmed value (empty → NULL)
	if body.Username != nil {
		trimmed := strings.TrimSpace(*body.Username)
		if trimmed == "" {
			updates["username"] = nil
		} else {
			updates["username"] = trimmed
		}
	} else {
		updates["username"] = nil
	}

	// Email: same logic
	if body.Email != nil {
		trimmed := strings.TrimSpace(*body.Email)
		if trimmed == "" {
			updates["email"] = nil
		} else {
			updates["email"] = trimmed
		}
	} else {
		updates["email"] = nil
	}

	updates["role"] = strings.TrimSpace(body.Role)

	if err := usage.GormUpdateUser(id, updates); err != nil {
		status := http.StatusInternalServerError
		if err.Error() == "user not found" {
			status = http.StatusNotFound
		}
		if strings.Contains(err.Error(), "duplicate") || strings.Contains(err.Error(), "UNIQUE") {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	// Return updated user
	user, err := usage.GormGetUserByID(id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(200, gin.H{"user": user})
}

// DeleteUser handles DELETE /v0/management/users/:id
func (h *Handler) DeleteUser(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil || id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}

	if err := usage.GormDeleteUser(id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(200, gin.H{"status": "ok"})
}
