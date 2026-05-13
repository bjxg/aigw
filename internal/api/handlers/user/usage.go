package user

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/api/bodyutil"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/config"
	"github.com/router-for-me/CLIProxyAPI/v6/internal/usage"
)

const userUsageBodyLimit int64 = 8 << 10

type userUsageRequest struct {
	APIKey string `json:"api_key"`
	Days   int    `json:"days"`
	Page   int    `json:"page"`
	Size   int    `json:"size"`
	Model  string `json:"model"`
	Status string `json:"status"`
	Part   string `json:"part"`
	Format string `json:"format"`
}

func readUserUsageRequest(c *gin.Context) userUsageRequest {
	req := userUsageRequest{}
	if c.Request.Method == http.MethodPost {
		body, err := bodyutil.ReadRequestBody(c, userUsageBodyLimit)
		if err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
	}

	req.APIKey = strings.TrimSpace(req.APIKey)
	if req.Page < 1 {
		req.Page = intQueryDefault(c, "page", 1)
	}
	if req.Size < 1 {
		req.Size = intQueryDefault(c, "size", 50)
	}
	if req.Days < 1 {
		req.Days = intQueryDefault(c, "days", 7)
	}
	if strings.TrimSpace(req.Model) == "" {
		req.Model = strings.TrimSpace(c.Query("model"))
	}
	if strings.TrimSpace(req.Status) == "" {
		req.Status = strings.TrimSpace(c.Query("status"))
	}
	if strings.TrimSpace(req.Part) == "" {
		req.Part = strings.TrimSpace(c.Query("part"))
	}
	if strings.TrimSpace(req.Format) == "" {
		req.Format = strings.TrimSpace(c.Query("format"))
	}

	req.Model = strings.TrimSpace(req.Model)
	req.Status = strings.TrimSpace(req.Status)
	req.Part = normalizeLogContentPartValue(req.Part)
	req.Format = normalizeLogContentFormatValue(req.Format)

	return req
}

func intQueryDefault(c *gin.Context, key string, def int) int {
	v := strings.TrimSpace(c.Query(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 1 {
		return def
	}
	return n
}

func normalizeLogContentPartValue(part string) string {
	part = strings.ToLower(strings.TrimSpace(part))
	if part == "" {
		return "both"
	}
	switch part {
	case "both", "input", "output", "details":
		return part
	default:
		return "both"
	}
}

func normalizeLogContentFormatValue(format string) string {
	format = strings.ToLower(strings.TrimSpace(format))
	if format == "" {
		return "json"
	}
	switch format {
	case "json", "text":
		return format
	default:
		return "json"
	}
}



type userAPIKeyGroupItem struct {
	Name   string   `json:"name"`
	Paths  []string `json:"paths"`
	Models []string `json:"models"`
}

type userAPIKeyItem struct {
	ID               int64                 `json:"id"`
	Name             string                `json:"name"`
	Key              string                `json:"key"`
	Disabled         bool                  `json:"disabled"`
	DailyLimit       int                   `json:"daily_limit"`
	TotalQuota       int                   `json:"total_quota"`
	SpendingLimit    float64               `json:"spending_limit"`
	ConcurrencyLimit int                   `json:"concurrency_limit"`
	RPMLimit         int                   `json:"rpm_limit"`
	TPMLimit         int                   `json:"tpm_limit"`
	ChannelGroups    []userAPIKeyGroupItem `json:"channel_groups"`
}

// GetUserAPIKeys returns the API keys belonging to the currently authenticated user,
// enriched with routing config channel group paths and allowed models.
func (h *Handler) GetUserAPIKeys(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	keys := usage.ListAPIKeysByUserID(userID)
	if keys == nil {
		keys = []usage.APIKeyRow{}
	}

	routingCfg := usage.GetRoutingConfig()
	if routingCfg == nil {
		routingCfg = &config.RoutingConfig{}
	}

	// Build lookup maps for routing config
	pathRoutesByGroup := make(map[string][]string)
	for _, pr := range routingCfg.PathRoutes {
		pathRoutesByGroup[pr.Group] = append(pathRoutesByGroup[pr.Group], pr.Path+"/v1")
	}
	modelsByGroup := make(map[string][]string)
	for _, cg := range routingCfg.ChannelGroups {
		modelsByGroup[cg.Name] = cg.AllowedModels
	}

	result := make([]userAPIKeyItem, 0, len(keys))
	for _, k := range keys {
		groups := make([]userAPIKeyGroupItem, 0, len(k.AllowedChannelGroups))
		for _, gName := range k.AllowedChannelGroups {
			paths := pathRoutesByGroup[gName]
			if paths == nil {
				paths = []string{}
			}
			models := modelsByGroup[gName]
			if models == nil {
				models = []string{}
			}
			groups = append(groups, userAPIKeyGroupItem{
				Name:   gName,
				Paths:  paths,
				Models: models,
			})
		}
		result = append(result, userAPIKeyItem{
			ID:               k.ID,
			Name:             k.Name,
			Key:              k.Key,
			Disabled:         k.Disabled,
			DailyLimit:       k.DailyLimit,
			TotalQuota:       k.TotalQuota,
			SpendingLimit:    k.SpendingLimit,
			ConcurrencyLimit: k.ConcurrencyLimit,
			RPMLimit:         k.RPMLimit,
			TPMLimit:         k.TPMLimit,
			ChannelGroups:    groups,
		})
	}

	c.JSON(http.StatusOK, gin.H{"items": result})
}

// ToggleUserAPIKey toggles the disabled status of an API key belonging to the
// currently authenticated user. The key is identified by numeric ID in the URL.
func (h *Handler) ToggleUserAPIKey(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	idStr := c.Param("id")
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid key id"})
		return
	}

	type toggleRequest struct {
		Disabled bool `json:"disabled"`
	}
	var req toggleRequest
	if c.Request.Method == http.MethodPost {
		body, err := bodyutil.ReadRequestBody(c, userUsageBodyLimit)
		if err == nil && len(body) > 0 {
			_ = json.Unmarshal(body, &req)
		}
	}

	if err := usage.UpdateAPIKeyDisabledByIDAndUserID(id, userID, req.Disabled); err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "disabled": req.Disabled})
}

// GetUserUsageLogs returns paginated request log entries for the currently
// authenticated user. An optional api_key parameter can scope the results to a
// single key, but the query is always filtered by the user's ID.
func (h *Handler) GetUserUsageLogs(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := readUserUsageRequest(c)

	// Resolve optional API key and verify ownership
	var apiKeyID int64
	if req.APIKey != "" {
		row := usage.GetAPIKey(req.APIKey)
		if row == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_key not found"})
			return
		}
		if row.UserID == nil || *row.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "api_key does not belong to current user"})
			return
		}
		apiKeyID = row.ID
	}

	params := usage.LogQueryParams{
		Page:     req.Page,
		Size:     req.Size,
		Days:     req.Days,
		UserID:   userID,
		APIKeyID: apiKeyID,
		Model:    req.Model,
		Status:   req.Status,
	}

	result, err := usage.QueryLogs(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	stats, err := usage.QueryStats(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	// SECURITY: Strip sensitive fields from user-facing response
	for i := range result.Items {
		result.Items[i].Source = ""
		result.Items[i].AuthIndex = ""
		result.Items[i].ChannelName = ""
		result.Items[i].APIKeyName = ""
	}

	// Model filter options scoped to this user
	models, _ := usage.QueryModelsForUser(userID, params.Days)
	if models == nil {
		models = make([]string, 0)
	}

	if result.Items == nil {
		result.Items = make([]usage.LogRow, 0)
	}

	c.JSON(http.StatusOK, gin.H{
		"items":   result.Items,
		"total":   result.Total,
		"page":    result.Page,
		"size":    result.Size,
		"stats":   stats,
		"filters": gin.H{"models": models},
	})
}

// GetUserUsageChartData returns chart data for the currently authenticated user.
func (h *Handler) GetUserUsageChartData(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := readUserUsageRequest(c)

	// Resolve optional API key and verify ownership
	var apiKeyID int64
	if req.APIKey != "" {
		row := usage.GetAPIKey(req.APIKey)
		if row == nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "api_key not found"})
			return
		}
		if row.UserID == nil || *row.UserID != userID {
			c.JSON(http.StatusForbidden, gin.H{"error": "api_key does not belong to current user"})
			return
		}
		apiKeyID = row.ID
	}

	days := req.Days

	daily, err := usage.QueryDailySeriesForUser(userID, apiKeyID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if daily == nil {
		daily = []usage.DailySeriesPoint{}
	}

	models, err := usage.QueryModelDistributionForUser(userID, apiKeyID, days)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if models == nil {
		models = []usage.ModelDistributionPoint{}
	}

	stats, err := usage.QueryStats(usage.LogQueryParams{UserID: userID, APIKeyID: apiKeyID, Days: days})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"daily_series":       daily,
		"model_distribution": models,
		"stats":              stats,
	})
}

// GetUserLogContent returns log content for a single entry, but only if it
// belongs to the currently authenticated user.
func (h *Handler) GetUserLogContent(c *gin.Context) {
	userIDVal, exists := c.Get("userID")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}
	userID, ok := userIDVal.(int64)
	if !ok || userID == 0 {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	req := readUserUsageRequest(c)

	idStr := c.Param("id")
	id, err := strconv.ParseInt(strings.TrimSpace(idStr), 10, 64)
	if err != nil || id < 1 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid log id"})
		return
	}

	part := req.Part
	format := req.Format
	if part == "details" {
		c.JSON(http.StatusForbidden, gin.H{"error": "request details are only available in the management API"})
		return
	}

	if format == "text" && part == "both" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "format=text requires part=input or part=output"})
		return
	}

	if part == "both" {
		result, err := usage.QueryLogContentForUser(id, userID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				c.JSON(http.StatusNotFound, gin.H{"error": "log entry not found"})
				return
			}
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, result)
		return
	}

	result, err := usage.QueryLogContentPartForUser(id, userID, part)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			c.JSON(http.StatusNotFound, gin.H{"error": "log entry not found"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if format == "text" {
		c.Header("Content-Type", "text/plain; charset=utf-8")
		c.Header("X-Log-Id", strconv.FormatInt(result.ID, 10))
		c.Header("X-Log-Part", result.Part)
		if strings.TrimSpace(result.Model) != "" {
			c.Header("X-Model", result.Model)
		}
		c.String(http.StatusOK, result.Content)
		return
	}

	c.JSON(http.StatusOK, result)
}
