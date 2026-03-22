// @title Messenger API
// @version 1.0
// @description Best messenger
// @host localhost:8080
// @BasePath /api/v1
// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization

package main

import (
	"context"
	"log"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "messenger/docs"
	"messenger/pkg/logger"

	"messenger/internal/ai"
	"messenger/internal/bot"
	"messenger/internal/config"
	"messenger/internal/crypto"
	"messenger/internal/database"
	db "messenger/internal/db/sqlc"
	"messenger/internal/email"
	"messenger/internal/gif"
	"messenger/internal/handler"
	"messenger/internal/middleware"
	"messenger/internal/ocr"
	"messenger/internal/push"
	rdb "messenger/internal/redis"
	appsentry "messenger/internal/sentry"
	"messenger/internal/storage"
	"messenger/internal/store"
	"messenger/internal/stories"
	"messenger/internal/worker"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

var Version = "dev"

func main() {
	// ══════════════════════════════════════════
	// 1. КОНФИГ + ЛОГИ + SENTRY
	// ══════════════════════════════════════════
	cfg := config.Load()
	logger.Init(cfg.Env)
	if err := appsentry.Init(cfg.Sentry.DSN, cfg.Env, Version); err != nil {
		log.Printf("⚠️  Sentry init failed: %v", err)
	}
	defer appsentry.Flush()

	// ══════════════════════════════════════════
	// 2. БАЗА ДАННЫХ
	// pgDB  — pgxpool (WebSocket, быстрые запросы)
	// sqlDB — database/sql (sqlc, транзакции)
	// ══════════════════════════════════════════
	pgDB, err := database.New(cfg.Database.DSN())
	if err != nil {
		log.Fatalf("❌ pgxpool: %v", err)
	}
	defer pgDB.Close()

	sqlDB, err := database.NewSQL(cfg.Database.DSN())
	if err != nil {
		log.Fatalf("❌ sql.DB: %v", err)
	}
	defer sqlDB.Close()

	queries := db.New(sqlDB)

	// ══════════════════════════════════════════
	// 3. REDIS
	// Rate limiting, кэш сессий, онлайн-статус,
	// pub/sub для WebSocket fanout
	// ══════════════════════════════════════════
	redisClient, err := rdb.Connect(cfg.Redis.URL)
	if err != nil {
		log.Fatalf("❌ redis: %v", err)
	}
	defer redisClient.Close()

	// ══════════════════════════════════════════
	// 4. MINIO (S3-совместимое хранилище файлов)
	// Медиа, стикеры, аватары
	// presigned PUT/GET, автоочистка через worker
	// ══════════════════════════════════════════
	minioClient, err := storage.NewMinIOClient(
		cfg.MinIO.Endpoint,
		cfg.MinIO.AccessKey,
		cfg.MinIO.SecretKey,
		cfg.MinIO.Bucket,
		cfg.MinIO.UseSSL,
		cfg.MinIO.PublicHost,
	)
	if err != nil {
		log.Fatalf("❌ minio: %v", err)
	}

	// ══════════════════════════════════════════
	// 5. AI КЛИЕНТЫ
	// Если URL пустой → заглушка (stub)
	// Если задан → реальный сервис
	// ══════════════════════════════════════════
	aiClient := ai.New(cfg.AI)
	sdClient := ai.NewSDClient(cfg.SD.URL, cfg.SD.Timeout)
	whisperClient := ai.NewWhisperClient(cfg.Whisper.URL, cfg.Whisper.Timeout)
	piperClient := ai.NewPiperClient(cfg.Piper.URL, cfg.Piper.Timeout)
	translateClient := ai.NewLibreTranslateClient(cfg.LibreTranslate.URL, cfg.LibreTranslate.APIKey, cfg.LibreTranslate.Timeout)

	// ══════════════════════════════════════════
	// 6. MIDDLEWARE
	// ══════════════════════════════════════════
	moderation := middleware.NewModeration(aiClient)
	antiSpam := middleware.NewAntiSpam(redisClient)

	// ══════════════════════════════════════════
	// 7. ХЕНДЛЕРЫ
	// Зависимости передаются явно (DI вручную)
	// ══════════════════════════════════════════

	// — Auth —
	authH := handler.NewAuthHandler(sqlDB, cfg, redisClient)
	oauthH := handler.NewOAuthHandler(sqlDB, cfg, redisClient)
	magicH := handler.NewMagicHandler(queries, cfg, redisClient, email.NewResendClient(cfg.Email.ResendAPIKey, cfg.Email.From))
	passkeyH, err := handler.NewPasskeyHandler(queries, cfg.Server.WebAuthnRPID, cfg.Server.WebAuthnOrigin)
	if err != nil {
		log.Fatalf("❌ webauthn: %v", err)
	}

	// — Безопасность —
	canary, err := crypto.GenerateCanary([]byte(cfg.Server.Host + "canary-secret"))
	if err != nil {
		log.Fatalf("❌ canary: %v", err)
	}
	blockH := handler.NewBlockHandler(queries)
	reportHandler := handler.NewReportHandler(queries)

	// — WebSocket + Звонки —
	wsH := handler.NewWSHandlerWithRedis(pgDB, redisClient)
	go wsH.StartChatSubscriber(context.Background())
	wsH.SetQueries(queries)
	callH := handler.NewCallHandler(wsH)

	// — Чаты + Сообщения —
	chatH := handler.NewChatHandler(sqlDB)
	msgH := handler.NewMessageHandler(store.NewMessageStore(sqlDB, redisClient), sqlDB, minioClient, translateClient)

	// — Медиа —
	mediaH := handler.NewMediaHandler(minioClient, queries)
	stickerH := handler.NewStickerHandler(queries, minioClient)
	emojiH := handler.NewAnimatedEmojiHandler(queries, minioClient)
	gifH := handler.NewGifHandler(gif.NewService(cfg.Tenor.APIKey))
	ocrH := handler.NewOCRHandler(queries, minioClient, ocr.NewClient("rus+eng"))
	geoH := handler.NewGeoHandler(sqlDB, wsH)

	// — Голосовые комнаты —
	voiceRoomH := handler.NewVoiceRoomHandler(sqlDB, wsH, os.Getenv("SFU_URL"))

	// — AI фичи —
	aiH := handler.NewAIHandler(aiClient, queries)
	imgGenH := handler.NewImageGenHandler(sdClient, minioClient)
	whisperH := handler.NewWhisperHandler(whisperClient)
	ttsH := handler.NewTTSHandler(piperClient, minioClient)
	translateH := handler.NewTranslateHandler(translateClient)

	// — Боты —
	botDisp := bot.NewDispatcher(queries, bot.NewWSSender(slog.Default()), slog.Default())
	botH := handler.NewBotHandler(queries, botDisp)

	// — Push уведомления —
	notifH := handler.NewNotificationHandler(sqlDB, push.NewClient(push.Config{
		FCMServerKey: os.Getenv("FCM_SERVER_KEY"),
	}))

	// — Поиск —
	searchH := handler.NewSearchHandler(pgDB)

	// — Premium + Бизнес —
	paymentH := handler.NewPaymentHandler(queries, cfg.Stripe)
	premiumH := handler.NewPremiumHandler(queries, cfg.Stripe)
	premiumSettingsH := handler.NewPremiumSettingsHandler(queries)
	analyticsH := handler.NewAnalyticsHandler(queries)
	businessH := handler.NewBusinessHandler(queries)

	// — Прочее —
	storiesH := handler.NewStoriesHandler(queries)
	turnH := handler.NewTURNHandler(handler.TURNConfig{
		Host:       cfg.TURN.Host,
		Port:       cfg.TURN.Port,
		TLSPort:    cfg.TURN.TLSPort,
		AuthSecret: cfg.TURN.AuthSecret,
		TTL:        cfg.TURN.TTL,
	})
	recoveryH := handler.NewSocialRecoveryHandler(sqlDB)
	gdprH := handler.NewGDPRHandler(sqlDB)

	// ══════════════════════════════════════════
	// 8. ФОНОВЫЕ ЗАДАЧИ
	// Каждую минуту: истёкшие сообщения,
	// отложенная отправка, напоминания
	// ══════════════════════════════════════════
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go worker.NewWithDB(queries, minioClient, sqlDB).Start(workerCtx)
	go stories.NewScheduler(queries, slog.Default()).Start(workerCtx)

	// ══════════════════════════════════════════
	// 9. РОУТЕР
	// ══════════════════════════════════════════
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(
		middleware.Sentry(),
		gin.Recovery(),
		middleware.Logger(),
		middleware.CORS(),
		middleware.RequestID(),
		middleware.Metrics(),
	)

	// ── Публичные системные маршруты ──────────
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     pgDB.Ping(ctx) == nil,
			"ts":     time.Now(),
		})
	})
	r.GET("/metrics", gin.WrapH(promhttp.Handler()))
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/api/v1/canary", handler.GetCanary(canary))

	// ── Вебхуки (без авторизации) ─────────────
	r.POST("/bot/webhook/:token", botH.HandleWebhook)
	r.POST("/payments/webhook", paymentH.StripeWebhook)
	r.POST("/api/v1/premium/webhook", premiumH.Webhook)
	r.GET("/api/v1/business/:username", businessH.GetPublicProfile)

	// ── WebSocket ─────────────────────────────
	r.GET("/ws", middleware.Auth(cfg, redisClient), wsH.Handle)

	// ── Авторизация (без токена) ──────────────
	auth := r.Group("/api/v1/auth")
	{
		auth.POST("/register", authH.Register)
		auth.POST("/login", authH.Login)
		auth.POST("/refresh", authH.RefreshToken)
		auth.GET("/pow/challenge", authH.GetPowChallenge)
		auth.GET("/oauth/:provider", oauthH.Redirect)
		auth.GET("/oauth/:provider/callback", oauthH.Callback)
		auth.POST("/magic/request", magicH.Request)
		auth.GET("/magic/verify", magicH.Verify)
		auth.POST("/passkey/login/begin", passkeyH.BeginLogin)
		auth.POST("/passkey/login/finish", passkeyH.FinishLogin)
		// Регистрация passkey требует JWT
		auth.POST("/passkey/register/begin", middleware.Auth(cfg, redisClient), passkeyH.BeginRegistration)
		auth.POST("/passkey/register/finish", middleware.Auth(cfg, redisClient), passkeyH.FinishRegistration)
	}

	// ── Приватные маршруты (JWT required) ─────
	api := r.Group("/api/v1")
	api.Use(
		middleware.Auth(cfg, redisClient),
		middleware.BanCheck(queries),
		middleware.SentryUser(),
		middleware.MsgPack(),
	)
	{
		// — Профиль —
		api.POST("/auth/logout", authH.Logout)
		api.GET("/auth/me", authH.Me)
		api.GET("/users/blocked", blockH.GetBlockedUsers)
		api.GET("/users/:username", authH.GetUser)
		api.GET("/auth/passkey", passkeyH.ListPasskeys)
		api.DELETE("/auth/passkey/:id", passkeyH.DeletePasskey)
		api.GET("/auth/oauth/accounts", oauthH.GetLinkedAccounts)
		api.POST("/auth/oauth/:provider/link", oauthH.Redirect)
		api.DELETE("/auth/oauth/:provider", oauthH.UnlinkAccount)

		// — Блокировки —
		api.POST("/users/:id/block", blockH.BlockUser)
		api.DELETE("/users/:id/block", blockH.UnblockUser)
		api.POST("/reports", reportHandler.CreateReport)

		// — Сообщения —
		msgs := api.Group("/messages")
		{
			msgs.PUT("/:id", msgH.Edit)
			msgs.DELETE("/:id", msgH.DeleteForAll)
			msgs.DELETE("/:id/me", msgH.DeleteForme)
			msgs.POST("/:id/forward", msgH.Forward)
			msgs.POST("/:id/pin", msgH.Pin)
			msgs.DELETE("/:id/pin", msgH.Unpin)
			msgs.POST("/:id/react", msgH.AddReaction)
			msgs.DELETE("/:id/react", msgH.RemoveReactions)
			msgs.POST("/:id/read", msgH.MarkRead)
			msgs.POST("/:id/save", msgH.Save)
			msgs.POST("/:id/reminder", msgH.SetReminder)
			msgs.GET("/saved", msgH.GetSaved)
			msgs.GET("/:id/translate", msgH.Translate)
		}

		// — Медиа —
		api.POST("/media/upload", mediaH.RequestUpload)
		api.POST("/media/confirm", mediaH.ConfirmUpload)
		api.POST("/media/:id/ocr", ocrH.RunOCR)

		// — Чаты —
		chats := api.Group("/chats")
		{
			chats.POST("/dm", chatH.CreateDM)
			chats.POST("/group", chatH.CreateGroup)
			chats.POST("/channel", chatH.CreateChannel)
			chats.POST("/community", chatH.CreateCommunity)
			chats.GET("", chatH.GetMyChats)
			chats.GET("/archived", chatH.GetArchived)
			chats.GET("/public", chatH.GetPublicChats)
			chats.GET("/:id", chatH.GetChat)
			chats.GET("/:id/members", chatH.GetMembers)
			chats.POST("/:id/leave", chatH.Leave)
			chats.PUT("/:id/visibility", chatH.UpdateVisibility)
			chats.PUT("/:id/slow-mode", chatH.SetSlowMode)

			// Сообщения в чате
			chats.GET("/:id/messages", msgH.Getmessages)
			chats.GET("/:id/messages/search", msgH.Search)
			chats.GET("/:id/pinned", msgH.GetPinned)
			chats.GET("/:id/search", searchH.SearchMessages)
			chats.POST("/:id/read", msgH.MarkChatRead)
			chats.POST("/:id/typing", msgH.Typing)
			chats.POST("/:id/messages", antiSpam.Handle(), moderation.Handle(), msgH.Send)

			// Модерация
			chats.POST("/:id/ban", chatH.BanMember)
			chats.POST("/:id/unban", chatH.UnbanMember)
			chats.POST("/:id/kick", chatH.KickMember)
			chats.POST("/:id/mute", chatH.MuteMember)
			chats.POST("/:id/unmute", chatH.UnmuteMember)
			chats.PUT("/:id/role", chatH.SetRole)

			// Инвайты
			chats.POST("/:id/invite", chatH.CreateInvite)
			chats.DELETE("/:id/invite/:code", chatH.RevokeInvite)

			// Архив
			chats.POST("/:id/archive", chatH.ArchiveChat)
			chats.DELETE("/:id/archive", chatH.UnarchiveChat)

			// Темы
			chats.POST("/:id/topics", chatH.CreateTopic)
			chats.GET("/:id/topics", chatH.GetTopics)
			chats.POST("/:id/topics/:topic_id/close", chatH.CloseTopic)
			chats.DELETE("/:id/topics/:topic_id", chatH.DeleteTopic)

			// Верификация
			chats.POST("/:id/verify", chatH.VerifyChat)
			chats.DELETE("/:id/verify", chatH.UnverifyChat)

			// Сообщества
			chats.POST("/:id/community/chats", chatH.AddToCommunity)
			chats.GET("/:id/community/chats", chatH.GetCommunityChats)
			chats.DELETE("/:id/community/chats/:chat_id", chatH.RemoveFromCommunity)

			// Геолокация
			chats.POST("/:id/location", geoH.SendLocation)
			chats.PUT("/:id/location/live", geoH.UpdateLiveLocation)
			chats.DELETE("/:id/location/live", geoH.StopLiveLocation)

			// AI ассистент
			chats.GET("/:id/smart-reply", aiH.SmartReply)
			chats.GET("/:id/summary", aiH.ChatSummary)

			// Голосовые комнаты
			chats.POST("/:id/voice-rooms", voiceRoomH.CreateRoom)
			chats.GET("/:id/voice-rooms", voiceRoomH.GetRooms)
		}
		api.POST("/invite/:code", chatH.JoinByInvite)

		// — Папки —
		folders := api.Group("/folders")
		{
			folders.POST("", chatH.CreateFolder)
			folders.GET("", chatH.GetFolders)
			folders.DELETE("/:id", chatH.DeleteFolder)
			folders.POST("/:id/chats", chatH.AddToFolder)
			folders.DELETE("/:id/chats/:chat_id", chatH.RemoveFromFolder)
			folders.GET("/:id/chats", chatH.GetFolderChats)
		}

		// — Голосовые комнаты —
		voiceRooms := api.Group("/voice-rooms")
		{
			voiceRooms.POST("/:id/join", voiceRoomH.JoinRoom)
			voiceRooms.POST("/:id/leave", voiceRoomH.LeaveRoom)
			voiceRooms.PUT("/:id/state", voiceRoomH.UpdateState)
			voiceRooms.GET("/:id/participants", voiceRoomH.GetParticipants)
			voiceRooms.POST("/:id/raise-hand", voiceRoomH.RaiseHand)
			voiceRooms.DELETE("/:id", voiceRoomH.CloseRoom)
			voiceRooms.GET("/:id/sfu-capabilities", voiceRoomH.GetSFUCapabilities)
			voiceRooms.POST("/:id/sfu-transport", voiceRoomH.CreateSFUTransport)
			voiceRooms.GET("/:id/sfu-ws", voiceRoomH.SFUProxy)
		}

		// — Звонки (WebRTC) —
		calls := api.Group("/calls")
		{
			calls.GET("/turn", turnH.GetTURNCredentials)
			calls.POST("", callH.InitiateCall)
			calls.POST("/:id/answer", callH.AnswerCall)
			calls.POST("/:id/reject", callH.RejectCall)
			calls.POST("/:id/hangup", callH.HangupCall)
			calls.POST("/:id/ice", callH.SendICECandidate)
			calls.POST("/:id/screen/start", callH.StartScreenShare)
			calls.POST("/:id/screen/stop", callH.StopScreenShare)
		}

		// — E2EE ключи —
		keys := api.Group("/keys")
		{
			keys.POST("", handler.UploadKeys(queries))
			keys.GET("/count", handler.GetPreKeyCount(queries))
			keys.GET("/:user_id", handler.GetKeyBundle(queries))
		}

		// — Стикеры —
		stickers := api.Group("/stickers")
		{
			stickers.GET("/packs", stickerH.GetPublicPacks)
			stickers.GET("/packs/my", stickerH.GetMyPacks)
			stickers.POST("/packs/:id/install", stickerH.InstallPack)
			stickers.DELETE("/packs/:id/install", stickerH.UninstallPack)
			stickers.GET("/packs/:id/stickers", stickerH.GetPackStickers)
			stickers.GET("/search", stickerH.SearchStickers)
			stickers.GET("/suggest", stickerH.SuggestStickers)
		}

		// — GIF —
		gifs := api.Group("/gifs")
		{
			gifs.GET("/search", gifH.Search)
			gifs.GET("/trending", gifH.Trending)
		}

		// — Animated Emoji —
		emoji := api.Group("/emoji")
		{
			emoji.GET("/animated", emojiH.GetAnimatedEmoji)
			emoji.GET("/animated/batch", emojiH.GetAnimatedEmojiBatch)
		}

		// — Поиск —
		search := api.Group("/search")
		{
			search.GET("/users", searchH.SearchUsers)
			search.GET("/chats", searchH.SearchChats)
			search.GET("/global", searchH.SearchGlobal)
			search.GET("/gif", searchH.SearchGIF)
			search.GET("/media", searchH.SearchMedia)
			search.GET("/stickers", searchH.SearchStickers)
		}

		// — Истории —
		storiesGroup := api.Group("/stories")
		{
			storiesGroup.POST("", storiesH.CreateStory)
			storiesGroup.GET("/feed", storiesH.GetFeed)
			storiesGroup.GET("/my", storiesH.GetMyStories)
			storiesGroup.GET("/archive", storiesH.GetArchived)
			storiesGroup.DELETE("/:id", storiesH.DeleteStory)
			storiesGroup.POST("/:id/archive", storiesH.ArchiveStory)
			storiesGroup.POST("/:id/view", storiesH.ViewStory)
			storiesGroup.GET("/:id/viewers", storiesH.GetViewers)
			storiesGroup.POST("/:id/react", storiesH.ReactToStory)
			storiesGroup.GET("/:id/reactions", storiesH.GetReactions)
		}

		// — Close Friends —
		cf := api.Group("/close-friends")
		{
			cf.GET("", storiesH.GetCloseFriends)
			cf.POST("", storiesH.AddCloseFriend)
			cf.DELETE("/:friendID", storiesH.RemoveCloseFriend)
		}

		// — AI фичи —
		aiGroup := api.Group("/ai")
		{
			aiGroup.POST("/generate-image", imgGenH.Generate)
			aiGroup.POST("/transcribe", whisperH.Transcribe)
			aiGroup.POST("/tts", ttsH.Synthesize)
			aiGroup.POST("/translate", translateH.Translate)
			aiGroup.POST("/detect-language", translateH.DetectLanguage)
		}

		// — Боты —
		bots := api.Group("/bots")
		{
			bots.POST("", botH.CreateBot)
			bots.GET("", botH.GetMyBots)
			bots.DELETE("/:id", botH.DeactivateBot)
			bots.PUT("/:id/webhook", botH.SetWebhook)
			bots.GET("/:id/commands", botH.GetCommands)
			bots.POST("/:id/commands", botH.AddCommand)
			bots.POST("/:id/payments", paymentH.CreatePayment)
			bots.GET("/:id/payments", paymentH.GetBotPayments)
		}

		// — Push уведомления + Устройства —
		notifs := api.Group("/notifications")
		{
			notifs.GET("", notifH.GetNotifications)
			notifs.POST("/:id/read", notifH.MarkRead)
			notifs.POST("/read-all", notifH.MarkAllRead)
		}
		devices := api.Group("/devices")
		{
			devices.POST("", notifH.RegisterDevice)
			devices.DELETE("/:id", notifH.UnregisterDevice)
		}

		// — Social Recovery —
		recovery := api.Group("/recovery")
		{
			recovery.POST("/setup", recoveryH.SetupRecovery)
			recovery.GET("/sessions", recoveryH.GetSessions)
			recovery.GET("/shares", recoveryH.GetMyShares)
			recovery.POST("/recover", recoveryH.Recover)
			recovery.DELETE("/sessions/:id", recoveryH.CancelSession)
		}

		// — GDPR —
		gdpr := api.Group("/gdpr")
		{
			gdpr.GET("/export", gdprH.ExportData)
			gdpr.DELETE("/delete", gdprH.DeleteAccount)
		}

		// — Premium —
		premium := api.Group("/premium")
		{
			premium.GET("/status", premiumH.Status)
			premium.POST("/subscribe", premiumH.Subscribe)
			premium.DELETE("/subscribe", premiumH.Cancel)
			premium.POST("/portal", premiumH.BillingPortal)

			premiumOnly := premium.Group("/", middleware.RequirePremium(queries))
			{
				premiumOnly.GET("/settings", premiumSettingsH.GetSettings)
				premiumOnly.PUT("/settings", premiumSettingsH.UpdateSettings)
				premiumOnly.GET("/labels", premiumSettingsH.GetChatLabels)
				premiumOnly.POST("/labels", premiumSettingsH.AddChatLabel)
				premiumOnly.DELETE("/labels/:id", premiumSettingsH.DeleteChatLabel)
			}
		}

		// — Бизнес (Premium only) —
		business := api.Group("/business", middleware.RequirePremium(queries))
		{
			business.GET("/profile", businessH.GetProfile)
			business.PUT("/profile", businessH.UpsertProfile)
		}

		// — Аналитика каналов (Premium only) —
		channels := api.Group("/channels", middleware.RequirePremium(queries))
		{
			channels.GET("/:id/analytics", analyticsH.GetChannelStats)
			channels.POST("/:id/analytics/view", analyticsH.RecordView)
			channels.POST("/:id/analytics/share", analyticsH.RecordShare)
		}

		// — Админ —
		admin := api.Group("/admin")
		{
			admin.GET("/reports", reportHandler.GetReports)
			admin.POST("/ban", reportHandler.BanUser)
		}
	}

	// ══════════════════════════════════════════
	// 10. HTTP СЕРВЕР + GRACEFUL SHUTDOWN
	// ══════════════════════════════════════════
	srv := &http.Server{
		Addr:         cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 Server running on %s:%s [%s]", cfg.Server.Host, cfg.Server.Port, cfg.Env)
		log.Printf("📚 Swagger UI: http://localhost:8080/swagger/index.html")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ server: %v", err)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	log.Println("🛑 Shutting down...")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("⚠️ Shutdown error: %v", err)
	}
	log.Println("✅ Done")
}