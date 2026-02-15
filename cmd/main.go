package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"livecommerce/docs"
	"livecommerce/internal/auth"
	"livecommerce/internal/cache"
	"livecommerce/internal/config"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"livecommerce/internal/product"
	"livecommerce/internal/seeds"
	"livecommerce/internal/syncer"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// @title LiveCommerce API
// @version 1.0
// @description Backend API for Live Commerce project.
// @host localhost:8080
// @BasePath /
func main() {
	docs.SwaggerInfo.BasePath = "/"

	// env
	config.LoadEnv()

	// init redis + db
	cache.InitRedis()
	database.Connect()

	// migrate
	if err := database.DB.AutoMigrate(
		&models.User{},
		&models.RefreshToken{},
		&models.Category{},
		&models.Tag{},
		&models.Product{},
		&models.ProductMedia{},
		&models.Comment{},
		&models.ProductReport{},
	); err != nil {
		log.Fatal(err)
	}

	// seed
	if err := seed.SeedCategories(); err != nil {
		log.Fatal(err)
	}

	// router
	r := gin.Default()

	// static uploads
	r.Static("/uploads", "./uploads")

	// swagger
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// --------------------
	// Auth
	// --------------------
	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/signup", auth.Signup)
		authRoutes.POST("/verify", auth.VerifyEmail)
		authRoutes.POST("/login", auth.Login)
		authRoutes.POST("/refresh", auth.RefreshToken)
		authRoutes.POST("/forgot-password", auth.ForgotPassword)
		authRoutes.POST("/reset-password", auth.ResetPassword)

		authRoutes.POST("/logout", auth.AuthMiddleware(""), auth.Logout)
		authRoutes.POST("/change-password", auth.AuthMiddleware(""), auth.ChangePassword)
	}

	// --------------------
	// Profile
	// --------------------
	profileRoutes := r.Group("/profile")
	{
		profileRoutes.GET("/get", auth.AuthMiddleware(""), auth.GetProfile)
		profileRoutes.PUT("/update", auth.AuthMiddleware(""), auth.UpdateProfile)
		profileRoutes.GET("/get/:id", auth.GetUserByID)
		profileRoutes.GET("/completed", auth.AuthMiddleware(""), auth.ProfileCompleted)
	}

	// --------------------
	// Categories
	// --------------------
	catRoutes := r.Group("/categories")
	{
		catRoutes.GET("/tree", product.GetCategoryTree)
	}

	// --------------------
	// Products
	// --------------------
	productRoutes := r.Group("/products")
	{
		// Create product: نیاز به login + پروفایل کامل
		productRoutes.POST("",
			auth.AuthMiddleware(""),
			auth.RequireProfileCompleted(),
			product.CreateProduct,
		)

		// Search products
		productRoutes.GET("", product.SearchProducts)

		// Get product by id:
		// ✅ Optional auth برای اینکه مهمان هم ببیند
		// و اگر admin/owner بود view افزایش پیدا نکند
		productRoutes.GET("/:id",
			auth.OptionalAuthMiddleware(),
			product.GetProductByID,
		)

		// Update/Delete product (نیاز به login)
		productRoutes.PUT("/:id", auth.AuthMiddleware(""), product.UpdateProductByID)
		productRoutes.DELETE("/:id", auth.AuthMiddleware(""), product.DeleteProductByID)

		// Upload media: login + پروفایل کامل
		productRoutes.POST("/:id/media",
			auth.AuthMiddleware(""),
			auth.RequireProfileCompleted(),
			product.UploadMediaByProductID,
		)

		// Engagement: نیاز به login
		productRoutes.POST("/:id/like", auth.AuthMiddleware(""), product.LikeProductByID)
		productRoutes.DELETE("/:id/like", auth.AuthMiddleware(""), product.UnlikeProductByID)
		productRoutes.POST("/:id/dislike", auth.AuthMiddleware(""), product.DisLikeProductByID)
		productRoutes.DELETE("/:id/dislike", auth.AuthMiddleware(""), product.UndislikeProductByID)

		// Stats public
		productRoutes.GET("/:id/stats", product.GetProductStatisticsByID)

		// Engagement/me protected
		productRoutes.GET("/:id/engagement/me", auth.AuthMiddleware(""), product.GetMyProductEngagement)

		// Rating:
		productRoutes.POST("/:id/rating", auth.AuthMiddleware(""), product.UpsertProductRating)
		productRoutes.DELETE("/:id/rating", auth.AuthMiddleware(""), product.DeleteProductRating)
		productRoutes.GET("/:id/rating/me", auth.AuthMiddleware(""), product.GetMyProductRating)
		productRoutes.GET("/:id/rating/summary", product.GetProductRatingSummary)

		// Comments:
		productRoutes.GET("/:id/comments", product.GetProductCommentsByID)

		// comment create: login (اگه بخوای پروفایل کامل هم می‌تونیم اضافه کنیم)
		productRoutes.POST("/:id/comments", auth.AuthMiddleware(""), product.CreateComment)
	}

	// --------------------
	// Comments: update/delete
	// --------------------
	commentRoutes := r.Group("/comments")
	{
		commentRoutes.PUT("/:id", auth.AuthMiddleware(""), product.UpdateComment)
		commentRoutes.DELETE("/:id", auth.AuthMiddleware(""), product.DeleteComment)
	}

	// --------------------
	// Media delete (اگر در file_handler.go یا file routes داری، اینجا تنظیمش کن)
	// --------------------
	mediaRoutes := r.Group("/media")
	{
		mediaRoutes.DELETE("/:id", auth.AuthMiddleware(""), product.DeleteMedia)
	}

	// --------------------
	// Users: owner products
	// --------------------
	userRoutes := r.Group("/users")
	{
		userRoutes.GET("/:owner_id/products", product.GetOwnerProducts)
	}

	// --------------------
	// Start sync + graceful shutdown
	// --------------------
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	syncer.StartRedisSync(ctx, syncer.DefaultConfig())

	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	go func() {
		log.Println("Server running on :8080")
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()

	<-ctx.Done()
	fmt.Println("Shutting down gracefully...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_ = srv.Shutdown(shutdownCtx)
}