package main

import (
	"go-mqtt/internal/config"
	"go-mqtt/internal/handler"
	"go-mqtt/internal/middleware"
	"go-mqtt/internal/mqtt"
	"log"

	"github.com/gin-gonic/gin"
	"github.com/joho/godotenv"
)

func main() {
	if err := godotenv.Load(); err != nil {
		log.Println("未检测到 .env 文件，继续使用系统环境变量")
	} else {
		log.Println("已加载 .env 环境变量")
	}

	config.InitDB()

	subscriber := mqtt.NewSubscriber()
	mqtt.SetDefaultSubscriber(subscriber)
	if err := subscriber.Start(); err != nil {
		log.Printf("MQTT订阅器启动失败: %v", err)
	} else {
		log.Println("MQTT订阅器启动成功")
	}

	r := gin.Default()

	r.GET("/ping", func(c *gin.Context) {
		c.JSON(200, gin.H{
			"code":    0,
			"message": "pong",
			"data":    gin.H{},
		})
	})
	// 设备handler
	deviceHandler := handler.NewDeviceHandler()
	emqxHandler := handler.NewEMQXHandler()
	authHandler := handler.NewAuthHandler()

	api := r.Group("/api/v1")
	{
		authGroup := api.Group("/auth")
		{
			authGroup.POST("/login", authHandler.Login)
		}

		secured := api.Group("")
		secured.Use(middleware.JWTAuth())
		{
			secured.GET("/auth/me", authHandler.Me)
		}

		device := api.Group("/device")
		device.Use(middleware.JWTAuth())
		{
			device.GET("", middleware.RequireRoles("admin", "operator", "viewer"), deviceHandler.GetDeviceList)
			device.GET("/:id", middleware.RequireRoles("admin", "operator", "viewer"), deviceHandler.GetDevice)
			device.GET("/:id/command", middleware.RequireRoles("admin", "operator", "viewer"), deviceHandler.GetCommandHistory)

			device.POST("", middleware.RequireRoles("admin"), deviceHandler.CreateDevice)
			device.PUT("/:id", middleware.RequireRoles("admin"), deviceHandler.UpdateDevice)
			device.DELETE("/:id", middleware.RequireRoles("admin"), deviceHandler.DeleteDevice)
			device.POST("/:id/control", middleware.ControlProtection(), middleware.RequireRoles("admin", "operator"), deviceHandler.ControlDevice)
		}
		// EMQX业务接口
		emqx := api.Group("/emqx")
		{
			emqx.POST("/auth", emqxHandler.Auth)
			emqx.POST("/webhook", emqxHandler.Webhook)
		}
	}

	log.Println("服务启动，监听端口 8080")
	r.Run(":8080")
}
