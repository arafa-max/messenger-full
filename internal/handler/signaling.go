package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// CallHandler — HTTP хендлер для управления звонками
type CallHandler struct {
	ws *WSHandler
}

func NewCallHandler(ws *WSHandler) *CallHandler {
	return &CallHandler{ws: ws}
}

type InitiateCallRequest struct {
	ToUserID string `json:"to_user_id" binding:"required"`
	CallType string `json:"call_type" binding:"required"` // "audio" | "video"
	SDP      string `json:"sdp" binding:"required"`
}

type AnswerCallRequest struct {
	CallID   string `json:"call_id" binding:"required"`
	SDP      string `json:"sdp" binding:"required"`
	ToUserID string `json:"to_user_id" binding:"required"`
}

type RejectCallRequest struct {
	CallID   string `json:"call_id" binding:"required"`
	ToUserID string `json:"to_user_id" binding:"required"`
}

type HangupRequest struct {
	CallID   string `json:"call_id" binding:"required"`
	ToUserID string `json:"to_user_id" binding:"required"`
}

type ICECandidateRequest struct {
	CallID    string `json:"call_id" binding:"required"`
	Candidate string `json:"candidate" binding:"required"`
	ToUserID  string `json:"to_user_id" binding:"required"`
}

// InitiateCall — POST /api/v1/calls
// @Summary Начать звонок
// @Tags Calls
// @Security BearerAuth
// @Accept json
// @Produce json
// @Param body body InitiateCallRequest true "Параметры звонка"
// @Success 200 {object} map[string]string
// @Failure 404 {object} map[string]string
// @Router /calls [post]
func (h *CallHandler) InitiateCall(c *gin.Context) {
	fromUserID := c.GetString("user_id")

	var req InitiateCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	if !h.ws.IsUserOnline(req.ToUserID) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user is offline"})
		return
	}

	callID := uuid.New().String()
	payload := toPayload(CallPayload{
		CallID:   callID,
		SDP:      req.SDP,
		CallType: req.CallType,
	})

	h.ws.SendSignal(req.ToUserID, &WSMessage{
		Type:    EventCallOffer,
		From:    fromUserID,
		To:      req.ToUserID,
		Payload: payload,
		TS:      time.Now().UnixMilli(),
	})

	h.ws.SendSignal(fromUserID, &WSMessage{
		Type:    EventCallRinging,
		From:    fromUserID,
		To:      req.ToUserID,
		Payload: payload,
		TS:      time.Now().UnixMilli(),
	})

	c.JSON(http.StatusOK, gin.H{"call_id": callID, "status": "ringing"})
}

// AnswerCall — POST /api/v1/calls/:id/answer
// @Summary Принять звонок
// @Tags Calls
// @Security BearerAuth
// @Router /calls/{id}/answer [post]
func (h *CallHandler) AnswerCall(c *gin.Context) {
	fromUserID := c.GetString("user_id")
	var req AnswerCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.ws.SendSignal(req.ToUserID, &WSMessage{
		Type:    EventCallAnswer,
		From:    fromUserID,
		To:      req.ToUserID,
		Payload: toPayload(CallPayload{CallID: req.CallID, SDP: req.SDP}),
		TS:      time.Now().UnixMilli(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "answered"})
}

// RejectCall — POST /api/v1/calls/:id/reject
// @Summary Отклонить звонок
// @Tags Calls
// @Security BearerAuth
// @Router /calls/{id}/reject [post]
func (h *CallHandler) RejectCall(c *gin.Context) {
	fromUserID := c.GetString("user_id")
	var req RejectCallRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.ws.SendSignal(req.ToUserID, &WSMessage{
		Type:    EventCallReject,
		From:    fromUserID,
		To:      req.ToUserID,
		Payload: toPayload(CallPayload{CallID: req.CallID}),
		TS:      time.Now().UnixMilli(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "rejected"})
}

// HangupCall — POST /api/v1/calls/:id/hangup
// @Summary Завершить звонок
// @Tags Calls
// @Security BearerAuth
// @Router /calls/{id}/hangup [post]
func (h *CallHandler) HangupCall(c *gin.Context) {
	fromUserID := c.GetString("user_id")
	var req HangupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.ws.SendSignal(req.ToUserID, &WSMessage{
		Type:    EventCallHangup,
		From:    fromUserID,
		To:      req.ToUserID,
		Payload: toPayload(CallPayload{CallID: req.CallID}),
		TS:      time.Now().UnixMilli(),
	})
	c.JSON(http.StatusOK, gin.H{"status": "hangup"})
}

// SendICECandidate — POST /api/v1/calls/:id/ice
// @Summary Передать ICE candidate (NAT traversal)
// @Tags Calls
// @Security BearerAuth
// @Router /calls/{id}/ice [post]
func (h *CallHandler) SendICECandidate(c *gin.Context) {
	fromUserID := c.GetString("user_id")
	var req ICECandidateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	h.ws.SendSignal(req.ToUserID, &WSMessage{
		Type: EventCallICE,
		From: fromUserID,
		To:   req.ToUserID,
		Payload: toPayload(CallPayload{
			CallID:    req.CallID,
			Candidate: req.Candidate,
		}),
		TS: time.Now().UnixMilli(),
	})
	c.JSON(http.StatusOK, gin.H{"ok": true})
}

func toPayload(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}