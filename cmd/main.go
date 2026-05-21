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
    liveRoom "livecommerce/internal/LiveRoom"
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

func main() {
    docs.SwaggerInfo.BasePath = "/"

    config.LoadEnv()
    cache.InitRedis()
    database.Connect()

    if err := database.DB.AutoMigrate(
        &models.User{},
        &models.RefreshToken{},
        &models.Category{},
        &models.Tag{},
        &models.Product{},
        &models.ProductMedia{},
        &models.Comment{},
        &models.ProductReport{},
        &models.LiveRoom{},
        &models.LiveRoomProduct{},
    ); err != nil {
        log.Fatal(err)
    }

    if err := seed.SeedCategories(); err != nil {
        log.Fatal(err)
    }

    // ← hub ها رو اینجا init کن، قبل از router
    liveRoom.EventsHub = liveRoom.NewRoomHub()
    liveRoom.InitChatHub()  // ← اضافه شد

    r := gin.Default()
    r.Static("/uploads", "./uploads")
    r.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

    // Auth
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
        authRoutes.POST("/ws-cookie", auth.AuthMiddleware(""), auth.SetWSChatCookie)
        authRoutes.POST("/ws-cookie/clear", auth.AuthMiddleware(""), auth.ClearWSChatCookie)
    }

    // Profile
    profileRoutes := r.Group("/profile")
    {
        profileRoutes.GET("/get", auth.AuthMiddleware(""), auth.GetProfile)
        profileRoutes.PUT("/update", auth.AuthMiddleware(""), auth.UpdateProfile)
        profileRoutes.GET("/get/:id", auth.GetUserByID)
        profileRoutes.GET("/completed", auth.AuthMiddleware(""), auth.ProfileCompleted)
    }

    // Categories
    r.Group("/categories").GET("/tree", product.GetCategoryTree)

    // Products
    productRoutes := r.Group("/products")
    {
        productRoutes.POST("", auth.AuthMiddleware(""), auth.RequireProfileCompleted(), product.CreateProduct)
        productRoutes.GET("", product.SearchProducts)
        productRoutes.GET("/:id", auth.OptionalAuthMiddleware(), product.GetProductByID)
        productRoutes.PUT("/:id", auth.AuthMiddleware(""), product.UpdateProductByID)
        productRoutes.DELETE("/:id", auth.AuthMiddleware(""), product.DeleteProductByID)
        productRoutes.POST("/:id/media", auth.AuthMiddleware(""), auth.RequireProfileCompleted(), product.UploadMediaByProductID)
        productRoutes.POST("/:id/like", auth.AuthMiddleware(""), product.LikeProductByID)
        productRoutes.DELETE("/:id/like", auth.AuthMiddleware(""), product.UnlikeProductByID)
        productRoutes.POST("/:id/dislike", auth.AuthMiddleware(""), product.DisLikeProductByID)
        productRoutes.DELETE("/:id/dislike", auth.AuthMiddleware(""), product.UndislikeProductByID)
        productRoutes.GET("/:id/stats", product.GetProductStatisticsByID)
        productRoutes.GET("/:id/engagement/me", auth.AuthMiddleware(""), product.GetMyProductEngagement)
        productRoutes.POST("/:id/rating", auth.AuthMiddleware(""), product.UpsertProductRating)
        productRoutes.DELETE("/:id/rating", auth.AuthMiddleware(""), product.DeleteProductRating)
        productRoutes.GET("/:id/rating/me", auth.AuthMiddleware(""), product.GetMyProductRating)
        productRoutes.GET("/:id/rating/summary", product.GetProductRatingSummary)
        productRoutes.GET("/:id/comments", product.GetProductCommentsByID)
        productRoutes.POST("/:id/comments", auth.AuthMiddleware(""), product.CreateComment)
    }

    // Comments
    commentRoutes := r.Group("/comments")
    {
        commentRoutes.PUT("/:id", auth.AuthMiddleware(""), product.UpdateComment)
        commentRoutes.DELETE("/:id", auth.AuthMiddleware(""), product.DeleteComment)
    }

    // Media
    r.Group("/media").DELETE("/:id", auth.AuthMiddleware(""), product.DeleteMedia)

    // Users
    r.Group("/users").GET("/:owner_id/products", product.GetOwnerProducts)

    // ====================
    // LiveRoom — WebSocket
    // ====================
    r.GET("/ws/live-rooms/:id/events",
        liveRoom.WSLiveRoomEvents(liveRoom.EventsHub),
    )

    // chat: cookie auth (WSAuthMiddleware) چون browser نمی‌تونه WS header بفرسته
    r.GET("/ws/live-rooms/:id/chat",
        auth.WSAuthMiddleware(),
        liveRoom.WSChat(),
    )

    // signaling: cookie auth هم اینجا
    r.GET("/ws/live-rooms/:id/signaling",
        auth.WSAuthMiddleware(),
        liveRoom.WSWebRTCSignaling,
    )

    // ====================
    // LiveRoom — REST
    // ====================
    lr := r.Group("/live-rooms")
    {
        lr.GET("", liveRoom.ListLiveRooms)
        lr.GET("/:id", liveRoom.GetLiveRoomByID)
        lr.GET("/:id/stats", liveRoom.LiveRoomStats)
        lr.POST("/:id/view/ping", liveRoom.ViewPing)

        lr.GET("/:id/reactions/summary", liveRoom.ReactionSummaryHandler)
        lr.POST("/:id/reactions/like", auth.AuthMiddleware(""), liveRoom.Like)
        lr.POST("/:id/reactions/dislike", auth.AuthMiddleware(""), liveRoom.Dislike)
        lr.DELETE("/:id/reactions", auth.AuthMiddleware(""), liveRoom.ClearReaction)
        lr.GET("/:id/reactions/me", auth.AuthMiddleware(""), liveRoom.MyReaction)

        lr.POST("", auth.AuthMiddleware(""), liveRoom.CreateLiveRoom)
        lr.PATCH("/:id", auth.AuthMiddleware(""), liveRoom.UpdateLiveRoom)
        lr.DELETE("/:id", auth.AuthMiddleware(""), liveRoom.DeleteLiveRoom)
        lr.POST("/:id/start", auth.AuthMiddleware(""), liveRoom.StartLive)
        lr.POST("/:id/end", auth.AuthMiddleware(""), liveRoom.EndLive)

        lr.GET("/:id/chat/history", auth.AuthMiddleware(""), liveRoom.GetChatHistory)

        lr.GET("/:id/products", liveRoom.ListAttachedProducts)
        lr.POST("/:id/products", auth.AuthMiddleware(""), liveRoom.AttachProducts)
        lr.DELETE("/:id/products/:productId", auth.AuthMiddleware(""), liveRoom.DetachProduct)
        lr.PATCH("/:id/products/:productId/pin", auth.AuthMiddleware(""), liveRoom.PinProduct)
        lr.PATCH("/:id/products/reorder", auth.AuthMiddleware(""), liveRoom.ReorderProducts)
    }

    // Syncer + graceful shutdown
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