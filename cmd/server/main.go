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
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"messenger/internal/config"
	"messenger/internal/database"
	db "messenger/internal/db/sqlc"
	"messenger/internal/handler"
	"messenger/internal/middleware"
	rdb "messenger/internal/redis"
	"messenger/internal/storage"
	"messenger/internal/store"
	"messenger/internal/worker"

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
	)
	if err != nil {
		log.Fatalf("❌ minio: %v", err)
	}

	//--Handlers
	authH := handler.NewAuthHandler(sqlDB, cfg, redisClient)
	wsH := handler.NewWSHandler(pgDB)
	msgStore := store.NewMessageStore(sqlDB, redisClient)
	msgH := handler.NewMessageHandler(msgStore)
	chatH := handler.NewChatHandler(sqlDB)

	queries := db.New(sqlDB)

	mediaH := handler.NewMediaHandler(minioClient, queries)

	//--Worker
	w := worker.New(queries)
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
			private.POST("/media/confirm",mediaH.ConfirmUpload)
			// Chat management
			chats.POST("/dm", chatH.CreateDM)
			chats.POST("/group", chatH.CreateGroup)
			chats.GET("", chatH.GetMyChats)
			chats.GET("/:id", chatH.GetChat)
			chats.GET("/:id/members", chatH.GetMembers)
			chats.POST("/:id/leave", chatH.Leave)
		}
	}
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
