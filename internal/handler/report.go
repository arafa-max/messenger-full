package handler

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"

	db "messenger/internal/db/sqlc"
	"messenger/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ReportHandler struct {
	queries *db.Queries
}

func NewReportHandler(q *db.Queries) *ReportHandler {
	return &ReportHandler{queries: q}
}

type CreateReportRequest struct {
	ReportedUserID    string `json:"reported_user_id" binding:"required"`
	ReportedMessageID *int64 `json:"reported_message_id"`
	Reason            string `json:"reason" binding:"required,oneof=spam harassment nsfw violence other"`
	Description       string `json:"description" binding:"max=500"`
}

func (h *ReportHandler) CreateReport(c *gin.Context) {
	// FIX: используем getUserID вместо GetString
	reporterUUID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_user_id"})
		return
	}

	var req CreateReportRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if req.ReportedUserID == reporterUUID.String() {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot_report_yourself"})
		return
	}

	reportedUUID, err := uuid.Parse(req.ReportedUserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_reported_user_id"})
		return
	}

	var msgID sql.NullInt64
	if req.ReportedMessageID != nil {
		msgID = sql.NullInt64{Int64: *req.ReportedMessageID, Valid: true}
	}

	var desc sql.NullString
	if req.Description != "" {
		desc = sql.NullString{String: req.Description, Valid: true}
	}

	report, err := h.queries.CreateReport(c.Request.Context(), db.CreateReportParams{
		ReporterID:        reporterUUID,
		ReportedUserID:    reportedUUID,
		ReportedMessageID: msgID,
		Reason:            req.Reason,
		Description:       desc,
	})
	if err != nil {
		logger.Error("Failed to create report", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	logger.Info("Report created", "reportID", report.ID, "reporter", reporterUUID, "reported", req.ReportedUserID)
	c.JSON(http.StatusOK, gin.H{"message": "report_submitted", "report_id": report.ID})
}

func (h *ReportHandler) GetReports(c *gin.Context) {
	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	page, err := strconv.Atoi(c.DefaultQuery("page", "1"))
	if err != nil || page < 1 {
		page = 1
	}

	limit := int32(50)
	offset := int32((page - 1) * int(limit))

	reports, err := h.queries.GetPendingReports(c.Request.Context(), db.GetPendingReportsParams{
		Limit:  limit,
		Offset: offset,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	c.JSON(http.StatusOK, reports)
}

type BanUserRequest struct {
	UserID    string `json:"user_id" binding:"required"`
	Reason    string `json:"reason" binding:"required"`
	Permanent bool   `json:"permanent"`
	Days      int    `json:"days" binding:"min=0,max=3650"`
}

func (h *ReportHandler) BanUser(c *gin.Context) {
	// FIX: используем getUserID
	adminUUID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid_admin_id"})
		return
	}

	if !isAdmin(c) {
		c.JSON(http.StatusForbidden, gin.H{"error": "forbidden"})
		return
	}

	var req BanUserRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	targetUUID, err := uuid.Parse(req.UserID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid_user_id"})
		return
	}

	var expiresAt sql.NullTime
	if !req.Permanent && req.Days > 0 {
		expiresAt = sql.NullTime{
			Time:  time.Now().UTC().AddDate(0, 0, req.Days),
			Valid: true,
		}
	}

	ban, err := h.queries.BanUser(c.Request.Context(), db.BanUserParams{
		UserID:    targetUUID,
		BannedBy:  uuid.NullUUID{UUID: adminUUID, Valid: true},
		Reason:    req.Reason,
		Permanent: sql.NullBool{Bool: req.Permanent, Valid: true},
		ExpiresAt: expiresAt,
	})
	if err != nil {
		logger.Error("Failed to ban user", "error", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal_error"})
		return
	}

	logger.Warn("User banned", "userID", req.UserID, "bannedBy", adminUUID, "permanent", req.Permanent)
	c.JSON(http.StatusOK, gin.H{"message": "user_banned", "ban_id": ban.ID})
}

func isAdmin(c *gin.Context) bool {
	role, _ := c.Get("role")
	return role == "admin"
}