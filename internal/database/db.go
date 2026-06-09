package database

// import (
//     "fmt"
//     "log"

//     "gorm.io/driver/sqlite"
//     "gorm.io/gorm"
// )

// var DB *gorm.DB

// func Connect() {
//     var err error

//     DB, err = gorm.Open(sqlite.Open("live_commerce.db"), &gorm.Config{})
//     if err != nil {
//         log.Fatal("❌ Failed to connect to database:", err)
//     }

// 	if err := DB.Exec("PRAGMA foreign_keys = ON;").Error; err != nil {
// 		log.Fatal("❌ Failed to enable foreign keys:", err)
// 	}

//     fmt.Println("✅ SQLite database connected successfully")
// }



import (
	"fmt"
	"log"
	"os"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

var DB *gorm.DB

func Connect() {
	var err error
	dsn := fmt.Sprintf(
		"host=%s user=%s password=%s dbname=%s port=%s sslmode=disable",
		os.Getenv("DB_HOST"),
		os.Getenv("DB_USER"),
		os.Getenv("DB_PASSWORD"),
		os.Getenv("DB_NAME"),
		os.Getenv("DB_PORT"),
	)

	DB, err = gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatal("❌ Failed to connect to database:", err)
	}
	fmt.Println("✅ Database connected successfully")
}
