// config/config.go

package config

import (
	"log"
	"os"

	"github.com/joho/godotenv"
)

var (
	APIId         int32
	APIHash       string
	SessionString string
	DBName        string
)

// =======================
// ADMIN IDS
// =======================

var AdminIDs = []int64{
	6644859358,
	8451305181,
	7183060880,
	7616808278,
}

// =======================
// REQUIRED CHANNELS
// =======================

var RequiredChannels = []string{
	"@jdnekmsms",
}

// =======================
// CHANNEL DISPLAY
// =======================

var ChannelDisplay = map[string]string{
	"@jdnekmsms": "📢 Official Channel",
}

// =======================
// LOAD CONFIG
// =======================

func Load() {

	err := godotenv.Load()

	if err != nil {
		log.Println(".env file not found, using system env")
	}

	// ENV VALUES
	APIHash = os.Getenv("API_HASH")
	SessionString = os.Getenv("SESSION_STRING")

	// DATABASE
	DBName = "quiz.db"

	// API ID
	APIId = 12380656

	log.Println("Config loaded successfully")
}

// =======================
// CHECK ADMIN
// =======================

func IsAdmin(userID int64) bool {

	for _, id := range AdminIDs {

		if id == userID {
			return true
		}
	}

	return false
}
