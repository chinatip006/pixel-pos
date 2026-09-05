package main

import (
	"log"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/pixelcardshop/pos-backend/internal/auth"
	"github.com/pixelcardshop/pos-backend/internal/config"
	"github.com/pixelcardshop/pos-backend/internal/db"
	"github.com/pixelcardshop/pos-backend/internal/handlers"
	"github.com/pixelcardshop/pos-backend/internal/storage"
)

func main() {
	cfg := config.Load()

	pool, err := db.NewPool(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("ต่อฐานข้อมูลไม่สำเร็จ: %v", err)
	}
	defer pool.Close()

	r := gin.Default()
		log.Printf("DEBUG: ALLOWED_ORIGIN = %q (length=%d)", cfg.AllowedOrigin, len(cfg.AllowedOrigin))

	// CORS: อนุญาตเฉพาะโดเมน Firebase Hosting ของ frontend เท่านั้น
r.Use(cors.New(cors.Config{
		AllowOrigins:     []string{"https://pixcel-pos.web.app"}, // ใช้ลิงก์ที่มีตัว c
		
		// ⚠️ จุดสำคัญที่สุด: ต้องมี "OPTIONS" อยู่ในบรรทัดนี้ด้วย
		AllowMethods:     []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		
		AllowHeaders:     []string{"Origin", "Content-Type", "Authorization", "Accept"}, 
		AllowCredentials: true,
	}))

	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authHandler := handlers.NewAuthHandler(pool, cfg)
	storageClient := storage.NewClient(cfg.SupabaseURL, cfg.SupabaseServiceKey, cfg.ProductImageBucket)
	productHandler := handlers.NewProductHandler(pool, storageClient)
	transactionHandler := handlers.NewTransactionHandler(pool)
	openTabHandler := handlers.NewOpenTabHandler(pool)
	memberHandler := handlers.NewMemberHandler(pool)
	adminHandler := handlers.NewAdminHandler(pool)

	api := r.Group("/api")
	{
		// public: ไม่ต้องมี JWT เพราะยังไม่ได้ login
		api.POST("/auth/login", authHandler.Login)

		// protected: ทุก endpoint ที่เหลือ (products, transactions, open-tabs, members)
		// ครอบด้วย auth.RequireAuth เหมือนกันหมด ต้องแนบ Authorization: Bearer <token>
		protected := api.Group("")
		protected.Use(auth.RequireAuth(cfg.JWTSecret))
		{
			protected.GET("/products", productHandler.List)
			protected.POST("/products", productHandler.Create)
			protected.PUT("/products/:id", productHandler.Update)
			protected.PATCH("/products/:id/status", productHandler.ToggleStatus)
			protected.PATCH("/products/:id/stock", productHandler.UpdateStock)
			protected.DELETE("/products/:id", productHandler.Delete)
			protected.POST("/products/:id/image", productHandler.UploadImage)

			protected.POST("/transactions", transactionHandler.Create)
			protected.GET("/transactions", transactionHandler.History)
			protected.POST("/transactions/:orderId/cancel", transactionHandler.Cancel)
			protected.GET("/reports/daily", transactionHandler.DailySummary)

			protected.POST("/open-tabs", openTabHandler.Create)
			protected.GET("/open-tabs", openTabHandler.List)
			protected.DELETE("/open-tabs/:tabId", openTabHandler.Delete)

			protected.GET("/members/:phone", memberHandler.GetByPhone)
			protected.POST("/members", memberHandler.Register)
			protected.POST("/members/:phone/redeem", memberHandler.Redeem)
			protected.POST("/members/:phone/claim-freebie", memberHandler.ClaimFreebie)
		}

		// admin: ใช้ secret แยก สำหรับ Render Cron Job เรียกเท่านั้น ไม่ใช่พนักงานเรียกเอง
		admin := api.Group("/admin")
		admin.Use(auth.RequireCronSecret(cfg.CronSecret))
		{
			admin.POST("/downgrade-members", adminHandler.DowngradeMembers)
		}
	}

	log.Printf("server starting on :%s", cfg.Port)
	if err := r.Run(":" + cfg.Port); err != nil {
		log.Fatalf("server error: %v", err)
	}
}
