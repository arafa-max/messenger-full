package handler

import (
	"encoding/json"
	"io"
	"net/http"

	"messenger/internal/config"
	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/stripe/stripe-go/v76"
	"github.com/stripe/stripe-go/v76/checkout/session"
	"github.com/stripe/stripe-go/v76/webhook"
)

type PaymentHandler struct {
	queries       *db.Queries
	stripeKey     string
	webhookSecret string
}

func NewPaymentHandler(queries *db.Queries, cfg config.StripeConfig) *PaymentHandler {
	stripe.Key = cfg.SecretKey
	return &PaymentHandler{
		queries:       queries,
		stripeKey:     cfg.SecretKey,
		webhookSecret: cfg.WebhookSecret,
	}
}

// POST /bots/:id/payments — создать платёж
func (h *PaymentHandler) CreatePayment(c *gin.Context) {
	if h.stripeKey == "" {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "payments not configured"})
		return
	}

	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	userID, err := uuid.Parse(c.GetString("user_id"))
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	var req struct {
		Amount      int64  `json:"amount"      binding:"required"` // в центах
		Currency    string `json:"currency"    binding:"required"`
		Description string `json:"description" binding:"required"`
		SuccessURL  string `json:"success_url" binding:"required"`
		CancelURL   string `json:"cancel_url"  binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Создаём Stripe Checkout Session
	params := &stripe.CheckoutSessionParams{
		PaymentMethodTypes: stripe.StringSlice([]string{"card"}),
		LineItems: []*stripe.CheckoutSessionLineItemParams{
			{
				PriceData: &stripe.CheckoutSessionLineItemPriceDataParams{
					Currency: stripe.String(req.Currency),
					ProductData: &stripe.CheckoutSessionLineItemPriceDataProductDataParams{
						Name: stripe.String(req.Description),
					},
					UnitAmount: stripe.Int64(req.Amount),
				},
				Quantity: stripe.Int64(1),
			},
		},
		Mode:       stripe.String(string(stripe.CheckoutSessionModePayment)),
		SuccessURL: stripe.String(req.SuccessURL),
		CancelURL:  stripe.String(req.CancelURL),
	}

	s, err := session.New(params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create payment"})
		return
	}

	// Сохраняем в БД
	payment, err := h.queries.CreatePayment(c, db.CreatePaymentParams{
		BotID:       botID,
		UserID:      userID,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Description: req.Description,
		StripeID:    s.ID,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save payment"})
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"payment_id":  payment.ID,
		"checkout_url": s.URL,  // редиректим пользователя сюда
	})
}

// POST /payments/webhook — Stripe webhook
func (h *PaymentHandler) StripeWebhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	// Верифицируем подпись Stripe
	sig := c.GetHeader("Stripe-Signature")
	event, err := webhook.ConstructEvent(body, sig, h.webhookSecret)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	switch event.Type {
	case "checkout.session.completed":
		var s stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		// Обновляем статус платежа
		payment, err := h.queries.GetPaymentByStripeID(c, s.ID)
		if err == nil {
			_ = h.queries.UpdatePaymentStatus(c, db.UpdatePaymentStatusParams{
				ID:     payment.ID,
				Status: "completed",
			})
		}

	case "checkout.session.expired":
		var s stripe.CheckoutSession
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}

		payment, err := h.queries.GetPaymentByStripeID(c, s.ID)
		if err == nil {
			_ = h.queries.UpdatePaymentStatus(c, db.UpdatePaymentStatusParams{
				ID:     payment.ID,
				Status: "expired",
			})
		}
	}

	c.Status(http.StatusOK)
}

// GET /bots/:id/payments — история платежей бота
func (h *PaymentHandler) GetBotPayments(c *gin.Context) {
	botID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bot_id"})
		return
	}

	payments, err := h.queries.GetBotPayments(c, db.GetBotPaymentsParams{
		BotID:  botID,
		Limit:  20,
		Offset: 0,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get payments"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"payments": payments})
}