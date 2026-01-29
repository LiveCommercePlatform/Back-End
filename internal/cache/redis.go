package cache

import (
	"context"
	"fmt"
	"os"
	"github.com/redis/go-redis/v9"
)

var Client *redis.Client

func InitRedis() {
	Client = redis.NewClient(&redis.Options{
		Addr:     getRedisAddr(),
		Password: "", 
		DB:       0,
	})
	if err := Client.Ping(context.Background()).Err(); err != nil {
		panic("Failed to connect to Redis: " + err.Error())
	}
}

func getRedisAddr() string {
	host := os.Getenv("REDIS_HOST")
	port := os.Getenv("REDIS_PORT")
	if host == "" {
		host = "localhost"
	}
	if port == "" {
		port = "6379"
	}
	return fmt.Sprintf("%s:%s", host, port)
}
