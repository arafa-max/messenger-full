package handler

import (
	"database/sql"
	"net/http"
	"time"

	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type AnalyticsHandler struct {
	queries *db.Queries
}

func NewAnalyticsHandler(queries *db.Queries) *AnalyticsHandler {
	return &AnalyticsHandler{queries: queries}
}

// GET /channels/:id/analytics?from=2024-01-01&to=2024-01-31
func (h *AnalyticsHandler) GetChannelStats(c *gin.Context) {
	channelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_id"})
		return
	}

	from, to := parseDateRange(c)

	stats, err := h.queries.GetChannelStats(c, db.GetChannelStatsParams{
		ChannelID: channelID,
		Date:      from,
		Date_2:    to,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get stats"})
		return
	}

	summary, err := h.queries.GetChannelStatsSummary(c, db.GetChannelStatsSummaryParams{
		ChannelID: channelID,
		Date:      from,
		Date_2:    to,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get summary"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"channel_id": channelID,
		"from":       from,
		"to":         to,
		"summary":    summary,
		"daily":      stats,
	})
}

// POST /channels/:id/analytics/view
func (h *AnalyticsHandler) RecordView(c *gin.Context) {
	channelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_id"})
		return
	}

	err = h.queries.RecordChannelStat(c, db.RecordChannelStatParams{
		ChannelID:   channelID,
		Date:        time.Now().UTC().Truncate(24 * time.Hour),
		Views:       sql.NullInt32{Int32: 1, Valid: true},
		Shares:      sql.NullInt32{Int32: 0, Valid: true},
		NewMembers:  sql.NullInt32{Int32: 0, Valid: true},
		LeftMembers: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record view"})
		return
	}
	c.Status(http.StatusNoContent)
}

// POST /channels/:id/analytics/share
func (h *AnalyticsHandler) RecordShare(c *gin.Context) {
	channelID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid channel_id"})
		return
	}

	err = h.queries.RecordChannelStat(c, db.RecordChannelStatParams{
		ChannelID:   channelID,
		Date:        time.Now().UTC().Truncate(24 * time.Hour),
		Views:       sql.NullInt32{Int32: 0, Valid: true},
		Shares:      sql.NullInt32{Int32: 1, Valid: true},
		NewMembers:  sql.NullInt32{Int32: 0, Valid: true},
		LeftMembers: sql.NullInt32{Int32: 0, Valid: true},
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to record share"})
		return
	}
	c.Status(http.StatusNoContent)
}

// вспомогательная — парсим from/to из query params
func parseDateRange(c *gin.Context) (time.Time, time.Time) {
	to := time.Now().UTC().Truncate(24 * time.Hour)
	from := to.AddDate(0, -1, 0) // по умолчанию последние 30 дней

	if f := c.Query("from"); f != "" {
		if t, err := time.Parse("2006-01-02", f); err == nil {
			from = t
		}
	}
	if t := c.Query("to"); t != "" {
		if parsed, err := time.Parse("2006-01-02", t); err == nil {
			to = parsed
		}
	}
	return from, to
}
