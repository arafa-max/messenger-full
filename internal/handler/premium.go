package handler

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"time"

	"messenger/internal/config"
	db "messenger/internal/db/sqlc"

	"github.com/gin-gonic/gin"
	"github.com/stripe/stripe-go/v76"
	portalsession "github.com/stripe/stripe-go/v76/billingportal/session"
	"github.com/stripe/stripe-go/v76/customer"
	"github.com/stripe/stripe-go/v76/subscription"
	"github.com/stripe/stripe-go/v76/webhook"
)

type PremiumHandler struct {
	queries       *db.Queries
	webhookSecret string
	priceID       string
}

func NewPremiumHandler(queries *db.Queries, cfg config.StripeConfig) *PremiumHandler {
	stripe.Key = cfg.SecretKey
	return &PremiumHandler{
		queries:       queries,
		webhookSecret: cfg.WebhookSecret,
		priceID:       cfg.PremiumPriceID,
	}
}

// POST /premium/subscribe
func (h *PremiumHandler) Subscribe(c *gin.Context) {
	// FIX: используем getUserID
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	user, err := h.queries.GetUserByID(c, userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "user not found"})
		return
	}

	sub, _ := h.queries.GetSubscriptionByUserID(c, userID)

	var customerID string
	if sub.StripeCustomerID != "" {
		customerID = sub.StripeCustomerID
	} else {
		cust, err := customer.New(&stripe.CustomerParams{
			Email: stripe.String(user.Email.String),
			Metadata: map[string]string{
				"user_id": userID.String(),
			},
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create customer"})
			return
		}
		customerID = cust.ID
	}

	subParams := &stripe.SubscriptionParams{
		Customer: stripe.String(customerID),
		Items: []*stripe.SubscriptionItemsParams{
			{Price: stripe.String(h.priceID)},
		},
		PaymentBehavior: stripe.String("default_incomplete"),
		Expand:          []*string{stripe.String("latest_invoice.payment_intent")},
	}

	stripeSub, err := subscription.New(subParams)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create subscription"})
		return
	}

	err = h.queries.UpsertSubscription(c, db.UpsertSubscriptionParams{
		UserID:           userID,
		StripeCustomerID: customerID,
		StripeSubID:      stripeSub.ID,
		Plan:             "premium",
		Status:           "incomplete",
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save subscription"})
		return
	}

	clientSecret := ""
	if stripeSub.LatestInvoice != nil && stripeSub.LatestInvoice.PaymentIntent != nil {
		clientSecret = stripeSub.LatestInvoice.PaymentIntent.ClientSecret
	}

	c.JSON(http.StatusOK, gin.H{
		"subscription_id": stripeSub.ID,
		"client_secret":   clientSecret,
		"status":          stripeSub.Status,
	})
}

// DELETE /premium/subscribe
func (h *PremiumHandler) Cancel(c *gin.Context) {
	// FIX: используем getUserID
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	sub, err := h.queries.GetSubscriptionByUserID(c, userID)
	if err != nil || sub.StripeSubID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no active subscription"})
		return
	}

	params := &stripe.SubscriptionParams{
		CancelAtPeriodEnd: stripe.Bool(true),
	}
	_, err = subscription.Update(sub.StripeSubID, params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to cancel subscription"})
		return
	}

	_ = h.queries.UpdateSubscriptionCancelAtPeriodEnd(c, db.UpdateSubscriptionCancelAtPeriodEndParams{
		UserID:            userID,
		CancelAtPeriodEnd: true,
	})

	c.JSON(http.StatusOK, gin.H{"message": "subscription will be canceled at period end"})
}

// GET /premium/status
func (h *PremiumHandler) Status(c *gin.Context) {
	// FIX: используем getUserID
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	sub, err := h.queries.GetSubscriptionByUserID(c, userID)
	if err != nil {
		c.JSON(http.StatusOK, gin.H{
			"plan":   "free",
			"status": "inactive",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"plan":                 sub.Plan,
		"status":               sub.Status,
		"current_period_end":   sub.CurrentPeriodEnd,
		"cancel_at_period_end": sub.CancelAtPeriodEnd,
	})
}

// POST /premium/portal
func (h *PremiumHandler) BillingPortal(c *gin.Context) {
	// FIX: используем getUserID
	userID, ok := getUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
		return
	}

	sub, err := h.queries.GetSubscriptionByUserID(c, userID)
	if err != nil || sub.StripeCustomerID == "" {
		c.JSON(http.StatusNotFound, gin.H{"error": "no subscription found"})
		return
	}

	var req struct {
		ReturnURL string `json:"return_url" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	ps, err := portalsession.New(&stripe.BillingPortalSessionParams{
		Customer:  stripe.String(sub.StripeCustomerID),
		ReturnURL: stripe.String(req.ReturnURL),
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create portal session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"url": ps.URL})
}

// POST /premium/webhook
func (h *PremiumHandler) Webhook(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil {
		c.Status(http.StatusBadRequest)
		return
	}

	event, err := webhook.ConstructEvent(body, c.GetHeader("Stripe-Signature"), h.webhookSecret)
	if err != nil {
		c.Status(http.StatusUnauthorized)
		return
	}

	switch event.Type {
	case "customer.subscription.updated", "customer.subscription.created":
		var s stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		periodEnd := time.Unix(s.CurrentPeriodEnd, 0)
		_ = h.queries.UpdateSubscriptionByStripeID(c, db.UpdateSubscriptionByStripeIDParams{
			StripeSubID:       s.ID,
			Status:            string(s.Status),
			CurrentPeriodEnd:  sql.NullTime{Time: periodEnd, Valid: true},
			CancelAtPeriodEnd: s.CancelAtPeriodEnd,
		})

	case "customer.subscription.deleted":
		var s stripe.Subscription
		if err := json.Unmarshal(event.Data.Raw, &s); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		_ = h.queries.UpdateSubscriptionByStripeID(c, db.UpdateSubscriptionByStripeIDParams{
			StripeSubID: s.ID,
			Status:      "canceled",
		})

	case "invoice.payment_failed":
		var inv stripe.Invoice
		if err := json.Unmarshal(event.Data.Raw, &inv); err != nil {
			c.Status(http.StatusBadRequest)
			return
		}
		if inv.Subscription != nil {
			_ = h.queries.UpdateSubscriptionByStripeID(c, db.UpdateSubscriptionByStripeIDParams{
				StripeSubID: inv.Subscription.ID,
				Status:      "past_due",
			})
		}
	}

	c.Status(http.StatusOK)
}