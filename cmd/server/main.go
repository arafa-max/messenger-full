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
	"messenger/internal/config"
	"messenger/internal/crypto"
	"messenger/internal/database"
	db "messenger/internal/db/sqlc"
	"messenger/internal/handler"
	"messenger/internal/middleware"
	rdb "messenger/internal/redis"
	"messenger/internal/storage"
	"messenger/internal/store"
	"messenger/internal/worker"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "messenger/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {

	cfg := config.Load()

	//--PostgreSQL
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

	//--Redis
	redisClient, err := rdb.Connect(cfg.Redis.URL)
	if err != nil {
		log.Fatalf("❌ redis: %v", err)
	}
	defer redisClient.Close()

	//--MinIO
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

	//--Handlers
	authH := handler.NewAuthHandler(sqlDB, cfg, redisClient)
	wsH := handler.NewWSHandler(pgDB)
	callH := handler.NewCallHandler(wsH)
	msgStore := store.NewMessageStore(sqlDB, redisClient)
	msgH := handler.NewMessageHandler(msgStore, sqlDB, minioClient)
	chatH := handler.NewChatHandler(sqlDB)

	queries := db.New(sqlDB)

	// Canary — генерируем при старте сервера
	canarySecret := []byte(cfg.Server.Host + "canary-secret")
	canary, err := crypto.GenerateCanary(canarySecret)
	if err != nil {
		log.Fatalf("❌ canary: %v", err)
	}

	mediaH := handler.NewMediaHandler(minioClient, queries)

	//--Worker
	w := worker.New(queries, minioClient)
	workerCtx, workerCancel := context.WithCancel(context.Background())
	defer workerCancel()
	go w.Start(workerCtx)

	//--Gin
	if cfg.Env == "production" {
		gin.SetMode((gin.ReleaseMode))
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())
	//--Health
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     pgDB.Ping(ctx) == nil,
			"ts":     time.Now(),
		})

	})

	//--Public routes
	public := r.Group("/api/v1/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", authH.Login)
		public.POST("/refresh", authH.RefreshToken)
	}
	//--Private routes
	private := r.Group("/api/v1")
	private.Use(middleware.Auth(cfg, redisClient))
	{
		// Auth
		private.POST("/auth/logout", authH.Logout)
		private.GET("/auth/me", authH.Me)
		private.GET("/users/:username", authH.GetUser)
		// Messages-action with specific message
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
		}
		// Chats - actions in context chats
		chats := private.Group("/chats")
		{
			chats.POST("/:id/messages", msgH.Send)
			chats.GET("/:id/messages", msgH.Getmessages)
			chats.GET("/:id/messages/search", msgH.Search)
			chats.GET("/:id/pinned", msgH.GetPinned)
			chats.POST("/:id/read", msgH.MarkChatRead)
			chats.POST("/:id/typing", msgH.Typing)

			// Media
			private.POST("/media/upload", mediaH.RequestUpload)
			private.POST("/media/confirm", mediaH.ConfirmUpload)

			// Chat management
			chats.POST("/dm", chatH.CreateDM)
			chats.POST("/group", chatH.CreateGroup)
			chats.GET("", chatH.GetMyChats)
			chats.GET("/:id", chatH.GetChat)
			chats.GET("/:id/members", chatH.GetMembers)
			chats.POST("/:id/leave", chatH.Leave)

			// Moderation
			chats.POST("/:id/ban", chatH.BanMember)
			chats.POST("/:id/unban", chatH.UnbanMember)
			chats.POST("/:id/kick", chatH.KickMember)
			chats.POST("/:id/mute", chatH.MuteMember)
			chats.POST("/:id/unmute", chatH.UnmuteMember)
			chats.PUT("/:id/role", chatH.SetRole)

			// Invite links
			chats.POST("/:id/invite", chatH.CreateInvite)
			chats.DELETE("/:id/invite/:code", chatH.RevokeInvite)

			// Archive
			chats.POST("/:id/archive", chatH.ArchiveChat)
			chats.DELETE("/:id/archive", chatH.UnarchiveChat)
			chats.GET("/archived", chatH.GetArchived)

			// Channels
			chats.POST("/channel", chatH.CreateChannel)
			chats.GET("/public", chatH.GetPublicChats)

			// Visibility
			chats.PUT("/:id/visibility", chatH.UpdateVisibility)

			// Topics
			chats.POST("/:id/topics", chatH.CreateTopic)
			chats.GET("/:id/topics", chatH.GetTopics)
			chats.POST("/:id/topics/:topic_id/close", chatH.CloseTopic)
			chats.DELETE("/:id/topics/:topic_id", chatH.DeleteTopic)

			// Verification
			chats.POST("/:id/verify", chatH.VerifyChat)
			chats.DELETE("/:id/verify", chatH.UnverifyChat)

			// Community
			chats.POST("/community", chatH.CreateCommunity)
			chats.POST("/:id/community/chats", chatH.AddToCommunity)
			chats.GET("/:id/community/chats", chatH.GetCommunityChats)
			chats.DELETE("/:id/community/chats/:chat_id", chatH.RemoveFromCommunity)
		}

		// Folders
		folders := private.Group("/folders")
		{
			folders.POST("", chatH.CreateFolder)
			folders.GET("", chatH.GetFolders)
			folders.DELETE("/:id", chatH.DeleteFolder)
			folders.POST("/:id/chats", chatH.AddToFolder)
			folders.DELETE("/:id/chats/:chat_id", chatH.RemoveFromFolder)
			folders.GET("/:id/chats", chatH.GetFolderChats)
		}

		// Join by invite
		private.POST("/invite/:code", chatH.JoinByInvite)

		// E2EE Keys
		keys := private.Group("/keys")
		{
			keys.POST("", handler.UploadKeys(queries))
			keys.GET("/count", handler.GetPreKeyCount(queries))
			keys.GET("/:user_id", handler.GetKeyBundle(queries))
		}

		// Calls — WebRTC сигналинг (offer/answer/ICE relay)
		turnH := handler.NewTURNHandler(handler.TURNConfig{
			Host:       cfg.TURN.Host,
			Port:       cfg.TURN.Port,
			TLSPort:    cfg.TURN.TLSPort,
			AuthSecret: cfg.TURN.AuthSecret,
			TTL:        cfg.TURN.TTL,
		})
		calls := private.Group("/calls")
		{
			calls.GET("/turn", turnH.GetTURNCredentials)
			calls.POST("", callH.InitiateCall)
			calls.POST("/:id/answer", callH.AnswerCall)
			calls.POST("/:id/reject", callH.RejectCall)
			calls.POST("/:id/hangup", callH.HangupCall)
			calls.POST("/:id/ice", callH.SendICECandidate)
		}

	}

	// Canary — публичный
	r.GET("/api/v1/canary", handler.GetCanary(canary))

	//--Swagger + WebSocket
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/ws", middleware.Auth(cfg, redisClient), wsH.Handle)
	//--Server
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
			log.Fatalf("❌ server:%v", err)
		}

	}()
	//--Graceful shutdown
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
