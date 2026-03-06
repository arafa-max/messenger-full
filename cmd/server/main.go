package main

import (
	"log"
	"messenger/internal/config"
	"messenger/internal/database"
	"messenger/internal/handler"

	"github.com/gin-gonic/gin"
)

func main() {
	cfg := config.Load()

	if err :=database.RunMigrations(cfg.Database.DSN(),"./migrations");err!=nil{
	log.Fatalf("❌ error migration: %v",err)
	}
	db,err:=database.New(cfg.Database.DSN())
	if err != nil{
		log.Fatalf("❌ error connected with BD: %v",err)
	}
	defer db.Close()

	r:=gin.Default()

	r.GET("/health",func(ctx *gin.Context) {
		ctx.JSON(200,gin.H{
			"status":"ok",
			"database":"connected",
		})
	})
wsHandler :=handler.NewWSHandler(db)
r.GET("/ws", wsHandler.Handle)

log.Printf("🚀 server launched on port %s",cfg.Server.Port)
if err :=r.Run(":"+cfg.Server.Port);err !=nil{
	log.Fatalf("❌ error launch server: %v",err)
}
}