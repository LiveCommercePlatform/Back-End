package main

import (
	"livecommerce/internal/auth"
	"livecommerce/internal/config"
	"livecommerce/internal/database"
	"livecommerce/internal/models"
	"livecommerce/internal/cache"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"livecommerce/docs"
)

// @title LiveCommerce API
// @version 1.0
// @description Backend API for Live Commerce project.
// @host localhost:8080
// @BasePath /
func main() {
	docs.SwaggerInfo.BasePath = "/"
	cache.InitRedis()
	// Load environment variables
	config.LoadEnv()

	// Connect to database
	database.Connect()
	database.DB.AutoMigrate(
	&models.User{}, 
	&models.RefreshToken{})

	// Setup Gin router
	r := gin.Default()

	// Swagger route
	r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// ========================
	// AUTH ROUTES
	// ========================
	// --- Auth Routes ---
	authRoutes := r.Group("/auth")
	{
		authRoutes.POST("/signup", auth.Signup)
		authRoutes.POST("/verify", auth.VerifyEmail)
		authRoutes.POST("/login", auth.Login)
		authRoutes.POST("/refresh", auth.RefreshToken)
		authRoutes.POST("/forgot-password", auth.ForgotPassword)
		authRoutes.POST("/reset-password", auth.ResetPassword)
		authRoutes.POST("/logout", auth.AuthMiddleware(""), auth.Logout)
		authRoutes.POST("/change-password", auth.AuthMiddleware(""), auth.ChangePassword)}
	
	authRoutes = r.Group("/profile")
	{
		authRoutes.GET("/get", auth.AuthMiddleware(""), auth.GetProfile)
		authRoutes.PUT("/update", auth.AuthMiddleware(""), auth.UpdateProfile)
		authRoutes.GET("/get/:id", auth.GetUserByID)
		authRoutes.GET("/completed", auth.AuthMiddleware(""), auth.ProfileCompleted)
	}


	// ========================
	// START SERVER
	// ========================
	r.Run(":8080")
}
