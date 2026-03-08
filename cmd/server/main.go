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
	"messenger/internal/handler"
	"messenger/internal/middleware"

	_ "messenger/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

func main() {

	cfg := config.Load()

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

	authH := handler.NewAuthHandler(sqlDB, cfg)
	wsH := handler.NewWSHandler(pgDB)

	if cfg.Env == "production" {
		gin.SetMode((gin.ReleaseMode))
	}

	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(middleware.Logger())
	r.Use(middleware.CORS())
	r.Use(middleware.RequestID())

	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()
		c.JSON(http.StatusOK, gin.H{
			"status": "ok",
			"db":     pgDB.Ping(ctx) == nil,
			"ts":     time.Now(),
		})

	})
	public := r.Group("/api/v1/auth")
	{
		public.POST("/register", authH.Register)
		public.POST("/login", authH.Login)
		public.POST("/refresh", authH.RefreshToken)
	}

	private := r.Group("/api/v1/auth")
	private.Use(middleware.Auth(cfg))
	{
		private.POST("/logout", authH.Logout)
		private.GET("/me", authH.Me)
	}
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
	r.GET("/ws", middleware.Auth(cfg), wsH.Handle)

	srv := &http.Server{
		Addr:         cfg.Server.Host + ":" + cfg.Server.Port,
		Handler:      r,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		log.Printf("🚀 Server running on %s:%s [%s]", cfg.Server.Host, cfg.Server.Port, cfg.Env)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("❌ server:%v", err)
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
