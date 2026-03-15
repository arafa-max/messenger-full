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

	"messenger/internal/ai"
	"messenger/internal/bot"
	"messenger/internal/config"
	"messenger/internal/crypto"
	"messenger/internal/database"
	db "messenger/internal/db/sqlc"
	"messenger/internal/gif"
	"messenger/internal/handler"
	"messenger/internal/middleware"
	rdb "messenger/internal/redis"
	"messenger/internal/storage"
	"messenger/internal/store"
	"messenger/internal/stories"
	"messenger/internal/worker"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {

	// ──────────────────────────────────────────
	// КОНФИГ
	// Загружает .env — DB, Redis, MinIO, JWT и т.д.
	// ──────────────────────────────────────────
	cfg := config.Load()

	// ──────────────────────────────────────────
	// БАЗА ДАННЫХ
	// pgDB  — pgxpool (WebSocket, быстрые запросы)
	// sqlDB — database/sql (sqlc, транзакции)
	// ──────────────────────────────────────────
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

	// ──────────────────────────────────────────
	// REDIS
	// Rate limiting, кэш сессий, онлайн-статус,
	// pub/sub для WebSocket fanout
	// ──────────────────────────────────────────
	redisClient, err := rdb.Connect(cfg.Redis.URL)
	if err != nil {
		log.Fatalf("❌ redis: %v", err)
	}
	defer redisClient.Close()

	// ──────────────────────────────────────────
	// MINIO
	// Хранилище файлов: медиа, стикеры, аватары
	// presigned PUT/GET, автоочистка через worker
	// ──────────────────────────────────────────
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

	// ──────────────────────────────────────────
	// SQLC QUERIES
	// Типобезопасные SQL запросы, генерируются
	// командой: sqlc generate
	// ──────────────────────────────────────────
	queries := db.New(sqlDB)

	// ──────────────────────────────────────────
	// AI
	// Если OLLAMA_URL пустой → заглушка (stub)
	// Если задан → реальный Ollama (Llama/Gemma)
	// Данные не покидают сервер, sanitizer чистит
	// личные данные перед отправкой в модель
	// ──────────────────────────────────────────
	aiClient := ai.New(cfg.AI)

	moderation := middleware.NewModeration(aiClient)
	// ──────────────────────────────────────────
	// CANARY TOKEN (E2EE безопасность)
	// Генерируется при старте сервера.
	// Если утёк — значит сервер скомпрометирован.
	// Пользователь может проверить его подлинность.
	// ──────────────────────────────────────────
	canarySecret := []byte(cfg.Server.Host + "canary-secret")
	canary, err := crypto.GenerateCanary(canarySecret)
	if err != nil {
		log.Fatalf("❌ canary: %v", err)
	}

	// ──────────────────────────────────────────
	// ХЕНДЛЕРЫ
	// Каждый хендлер отвечает за свою область.
	// Зависимости передаются явно (DI вручную).
	// ──────────────────────────────────────────

	// Аутентификация: register, login, refresh, logout, me
	authH := handler.NewAuthHandler(sqlDB, cfg, redisClient)

	// WebSocket: реалтайм соединения клиентов
	wsH := handler.NewWSHandlerWithRedis(pgDB, redisClient)
	go wsH.StartChatSubscriber(context.Background())

	// Звонки: WebRTC сигналинг через WebSocket
	callH := handler.NewCallHandler(wsH)

	// Сообщения: отправка, редактирование, реакции,
	// пересылка, закреп, напоминания, поиск
	msgStore := store.NewMessageStore(sqlDB, redisClient)

	// Чаты: DM, группы, каналы, роли, баны,
	// папки, архив, инвайты, темы, сообщества
	chatH := handler.NewChatHandler(sqlDB)

	// GIF: поиск и trending через Tenor API
	gifService := gif.NewService(cfg.Tenor.APIKey)
	gifH := handler.NewGifHandler(gifService)

	// Истории: создание, просмотры, реакции,
	// архив, close friends
	storiesH := handler.NewStoriesHandler(queries)

	// Animated emoji: список и batch-загрузка из MinIO
	emojiH := handler.NewAnimatedEmojiHandler(queries, minioClient)

	// Медиа: presigned upload/confirm для файлов
	mediaH := handler.NewMediaHandler(minioClient, queries)

	// Стикеры: паки, установка, поиск, подсказки
	stickerH := handler.NewStickerHandler(queries, minioClient)

	// Геолокация: отправка, live location
	geoH := handler.NewGeoHandler(sqlDB, wsH)

	// TURN: credentials для WebRTC P2P соединений
	turnH := handler.NewTURNHandler(handler.TURNConfig{
		Host:       cfg.TURN.Host,
		Port:       cfg.TURN.Port,
		TLSPort:    cfg.TURN.TLSPort,
		AuthSecret: cfg.TURN.AuthSecret,
		TTL:        cfg.TURN.TTL,
	})

	// AI хендлер: Smart Reply, авто-резюме чата
	// Использует aiClient (stub или Ollama)
	aiH := handler.NewAIHandler(aiClient, queries)
	sdClient := ai.NewSDClient(cfg.SD.URL, cfg.SD.Timeout)
	imgGenH := handler.NewImageGenHandler(sdClient, minioClient)
	whisperClient := ai.NewWhisperClient(cfg.Whisper.URL, cfg.Whisper.Timeout)
	whisperH := handler.NewWhisperHandler(whisperClient)
	piperClient := ai.NewPiperClient(cfg.Piper.URL, cfg.Piper.Timeout)
	ttsH := handler.NewTTSHandler(piperClient, minioClient)
	translateClient := ai.NewLibreTranslateClient(cfg.LibreTranslate.URL, cfg.LibreTranslate.APIKey, cfg.LibreTranslate.Timeout)
	translateH := handler.NewTranslateHandler(translateClient)
	msgH := handler.NewMessageHandler(msgStore, sqlDB, minioClient, translateClient) // ← сюда

	// bots
	// Bot dispatcher
	botSender := bot.NewWSSender(slog.Default())
	botDisp := bot.NewDispatcher(queries, botSender, slog.Default())
	botH := handler.NewBotHandler(queries, botDisp)

	paymentH := handler.NewPaymentHandler(queries, cfg.Stripe)

	searchH := handler.NewSearchHandler(pgDB)
	voiceRoomH := handler.NewVoiceRoomHandler(sqlDB, wsH)
	recoveryH := handler.NewSocialRecoveryHandler(sqlDB)

	// Premium
	premiumH := handler.NewPremiumHandler(queries, cfg.Stripe)
	premiumSettingsH := handler.NewPremiumSettingsHandler(queries)

	// ──────────────────────────────────────────
	// ФОНОВЫЕ ЗАДАЧИ (worker)
	// Каждую минуту:
	//   - удаляет истёкшие сообщения
	//   - отправляет запланированные сообщения
	//   - срабатывают напоминания
	// ──────────────────────────────────────────
	w := worker.NewWithDB(queries, minioClient, sqlDB)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go w.Start(workerCtx)

	// Планировщик историй: удаляет истёкшие (24ч)
	go stories.NewScheduler(queries, slog.Default()).Start(workerCtx)

	// ──────────────────────────────────────────
	// GIN ROUTER
	// ──────────────────────────────────────────
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	r := gin.New()
	r.Use(gin.Recovery())         // паника → 500, не крашит сервер
	r.Use(middleware.Logger())    // логирует каждый запрос
	r.Use(middleware.CORS())      // разрешает запросы с фронтенда
	r.Use(middleware.RequestID()) // уникальный ID каждого запроса

	// ──────────────────────────────────────────
	// МАРШРУТЫ
	// ──────────────────────────────────────────

	// Health check — без авторизации
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     pgDB.Ping(ctx) == nil,
			"ts":     time.Now(),
		})
	})

	// Canary — публичный эндпоинт для проверки E2EE
	r.GET("/api/v1/canary", handler.GetCanary(canary))

	// Swagger UI
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Bot webhook (без авторизации — вызывается клиентом)
	r.POST("/bot/webhook/:token", botH.HandleWebhook)

	// Stripe webhook — без авторизации
	r.POST("/payments/webhook", paymentH.StripeWebhook)
	r.POST("/api/v1/premium/webhook", premiumH.Webhook)
	// WebSocket — требует авторизации
	r.GET("/ws", middleware.Auth(cfg, redisClient), wsH.Handle)

	// Публичные маршруты (без токена)
	public := r.Group("/api/v1/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", authH.Login)
		public.POST("/refresh", authH.RefreshToken)
		public.GET("/pow/challenge", authH.GetPowChallenge)
	}

	// Приватные маршруты (требуют JWT)
	private := r.Group("/api/v1")
	private.Use(middleware.Auth(cfg, redisClient))
	private.Use(middleware.MsgPack())
	{
		// ── Профиль ──────────────────────────
		private.POST("/auth/logout", authH.Logout)
		private.GET("/auth/me", authH.Me)
		private.GET("/users/:username", authH.GetUser)

		// ── Действия с конкретным сообщением ─
		msgs := private.Group("/messages")
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

		// ── Медиа ────────────────────────────
		private.POST("/media/upload", mediaH.RequestUpload)
		private.POST("/media/confirm", mediaH.ConfirmUpload)

		// ── Чаты ─────────────────────────────
		chats := private.Group("/chats")
		{
			// Сообщения в чате
			chats.GET("/:id/messages", msgH.Getmessages)
			chats.GET("/:id/messages/search", msgH.Search)
			chats.GET("/:id/pinned", msgH.GetPinned)
			chats.POST("/:id/read", msgH.MarkChatRead)
			chats.POST("/:id/typing", msgH.Typing)

			// Управление чатом
			chats.POST("/dm", chatH.CreateDM)
			chats.POST("/group", chatH.CreateGroup)
			chats.GET("", chatH.GetMyChats)
			chats.GET("/:id", chatH.GetChat)
			chats.GET("/:id/members", chatH.GetMembers)
			chats.POST("/:id/leave", chatH.Leave)

			// Модерация участников
			chats.POST("/:id/ban", chatH.BanMember)
			chats.POST("/:id/unban", chatH.UnbanMember)
			chats.POST("/:id/kick", chatH.KickMember)
			chats.POST("/:id/mute", chatH.MuteMember)
			chats.POST("/:id/unmute", chatH.UnmuteMember)
			chats.PUT("/:id/role", chatH.SetRole)

			// Инвайт-ссылки
			chats.POST("/:id/invite", chatH.CreateInvite)
			chats.DELETE("/:id/invite/:code", chatH.RevokeInvite)
			private.POST("/invite/:code", chatH.JoinByInvite)

			// Архив
			chats.POST("/:id/archive", chatH.ArchiveChat)
			chats.DELETE("/:id/archive", chatH.UnarchiveChat)
			chats.GET("/archived", chatH.GetArchived)

			// Каналы
			chats.POST("/channel", chatH.CreateChannel)
			chats.GET("/public", chatH.GetPublicChats)
			chats.PUT("/:id/visibility", chatH.UpdateVisibility)

			// Темы (Topics)
			chats.POST("/:id/topics", chatH.CreateTopic)
			chats.GET("/:id/topics", chatH.GetTopics)
			chats.POST("/:id/topics/:topic_id/close", chatH.CloseTopic)
			chats.DELETE("/:id/topics/:topic_id", chatH.DeleteTopic)

			// Верификация чата
			chats.POST("/:id/verify", chatH.VerifyChat)
			chats.DELETE("/:id/verify", chatH.UnverifyChat)

			// Сообщества
			chats.POST("/community", chatH.CreateCommunity)
			chats.POST("/:id/community/chats", chatH.AddToCommunity)
			chats.GET("/:id/community/chats", chatH.GetCommunityChats)
			chats.DELETE("/:id/community/chats/:chat_id", chatH.RemoveFromCommunity)

			// Геолокация
			chats.POST("/:id/location", geoH.SendLocation)
			chats.PUT("/:id/location/live", geoH.UpdateLiveLocation)
			chats.DELETE("/:id/location/live", geoH.StopLiveLocation)

			// AI ассистент
			chats.GET("/:id/smart-reply", aiH.SmartReply) // 3 варианта ответа
			chats.GET("/:id/summary", aiH.ChatSummary)    // резюме чата

			chats.POST("/:id/messages", moderation.Handle(), msgH.Send)

			// В блок chats добавь:
			chats.POST("/:id/voice-rooms", voiceRoomH.CreateRoom)
			chats.GET("/:id/voice-rooms", voiceRoomH.GetRooms)
		}

		// ── Папки ────────────────────────────
		folders := private.Group("/folders")
		{
			folders.POST("", chatH.CreateFolder)
			folders.GET("", chatH.GetFolders)
			folders.DELETE("/:id", chatH.DeleteFolder)
			folders.POST("/:id/chats", chatH.AddToFolder)
			folders.DELETE("/:id/chats/:chat_id", chatH.RemoveFromFolder)
			folders.GET("/:id/chats", chatH.GetFolderChats)
		}

		// ── Стикеры ──────────────────────────
		stickers := private.Group("/stickers")
		{
			stickers.GET("/packs", stickerH.GetPublicPacks)
			stickers.GET("/packs/my", stickerH.GetMyPacks)
			stickers.POST("/packs/:id/install", stickerH.InstallPack)
			stickers.DELETE("/packs/:id/install", stickerH.UninstallPack)
			stickers.GET("/packs/:id/stickers", stickerH.GetPackStickers)
			stickers.GET("/search", stickerH.SearchStickers)
			stickers.GET("/suggest", stickerH.SuggestStickers)
		}
		search := private.Group("/search")
		{
			search.GET("/users", searchH.SearchUsers)
			search.GET("/chats", searchH.SearchChats)
			search.GET("/global", searchH.SearchGlobal)
			search.GET("/gif", searchH.SearchGIF)
			search.GET("/media", searchH.SearchMedia)
			search.GET("/stickers", searchH.SearchStickers)
		}
		chats.GET("/:id/search", searchH.SearchMessages)
		// ── GIF ──────────────────────────────
		gifs := private.Group("/gifs")
		{
			gifs.GET("/search", gifH.Search)
			gifs.GET("/trending", gifH.Trending)
		}

		// ── Истории ──────────────────────────
		storiesGroup := private.Group("/stories")
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

		// ── Close Friends (для историй) ──────
		cf := private.Group("/close-friends")
		{
			cf.GET("", storiesH.GetCloseFriends)
			cf.POST("", storiesH.AddCloseFriend)
			cf.DELETE("/:friendID", storiesH.RemoveCloseFriend)
		}

		// ── Animated Emoji ───────────────────
		emoji := private.Group("/emoji")
		{
			emoji.GET("/animated", emojiH.GetAnimatedEmoji)
			emoji.GET("/animated/batch", emojiH.GetAnimatedEmojiBatch)
		}

		// ── E2EE Ключи ───────────────────────
		// Загрузка и получение ключей для Signal Protocol
		keys := private.Group("/keys")
		{
			keys.POST("", handler.UploadKeys(queries))
			keys.GET("/count", handler.GetPreKeyCount(queries))
			keys.GET("/:user_id", handler.GetKeyBundle(queries))
		}

		// ── Звонки (WebRTC) ──────────────────
		// TURN credentials + сигналинг offer/answer/ICE
		calls := private.Group("/calls")
		{
			calls.GET("/turn", turnH.GetTURNCredentials)
			calls.POST("", callH.InitiateCall)
			calls.POST("/:id/answer", callH.AnswerCall)
			calls.POST("/:id/reject", callH.RejectCall)
			calls.POST("/:id/hangup", callH.HangupCall)
			calls.POST("/:id/ice", callH.SendICECandidate)
		}

		// Bots
		bots := private.Group("/bots")
		{
			bots.POST("", botH.CreateBot)
			bots.GET("", botH.GetMyBots)
			bots.DELETE("/:id", botH.DeactivateBot)
			bots.PUT("/:id/webhook", botH.SetWebhook)
			bots.GET("/:id/commands", botH.GetCommands)
			bots.POST("/:id/commands", botH.AddCommand)
			// Payments
			bots.POST("/:id/payments", paymentH.CreatePayment)
			bots.GET("/:id/payments", paymentH.GetBotPayments)

		}
		// AI image generation
		private.POST("/ai/generate-image", imgGenH.Generate)

		// Whisper STT
		private.POST("/ai/transcribe", whisperH.Transcribe)

		// TTS
		private.POST("/ai/tts", ttsH.Synthesize)

		// Translation
		private.POST("/ai/translate", translateH.Translate)
		private.POST("/ai/detect-language", translateH.DetectLanguage)

		recovery := private.Group("/recovery")
		{
			recovery.POST("/setup", recoveryH.SetupRecovery)
			recovery.GET("/sessions", recoveryH.GetSessions)
			recovery.GET("/shares", recoveryH.GetMyShares)
			recovery.POST("/recover", recoveryH.Recover)
			recovery.DELETE("/sessions/:id", recoveryH.CancelSession)
		}
		// Voice Rooms
		voiceRooms := private.Group("/voice-rooms")
		{
			voiceRooms.POST("/:id/join", voiceRoomH.JoinRoom)
			voiceRooms.POST("/:id/leave", voiceRoomH.LeaveRoom)
			voiceRooms.PUT("/:id/state", voiceRoomH.UpdateState)
			voiceRooms.GET("/:id/participants", voiceRoomH.GetParticipants)
			voiceRooms.POST("/:id/raise-hand", voiceRoomH.RaiseHand)
			voiceRooms.DELETE("/:id", voiceRoomH.CloseRoom)
		}
		// ── Premium ──────────────────────────
		premium := private.Group("/premium")
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
	}

	// ──────────────────────────────────────────
	// HTTP СЕРВЕР
	// ──────────────────────────────────────────
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

	// ──────────────────────────────────────────
	// GRACEFUL SHUTDOWN
	// Ждём SIGINT/SIGTERM, даём 10 сек на завершение
	// активных запросов перед остановкой
	// ──────────────────────────────────────────
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
