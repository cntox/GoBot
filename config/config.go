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

// ADMIN IDS
var AdminIDs = []int64{
	6644859358,
	8451305181,
	7183060880,
}

// REQUIRED CHANNELS
var RequiredChannels = []string{
	"jdnekmsms",
}

// CHANNEL DISPLAY
var ChannelDisplay = map[string]string{
	"jdnekmsms",
}

// Load Config
func Load() {

	err := godotenv.Load()

	if err != nil {
		log.Println(".env file not found, using system env")
	}

	APIHash = os.Getenv("API_HASH")
	SessionString = os.Getenv("SESSION_STRING")

	DBName = "quiz.db"

	// API ID
	APIId = 12380656
}

// Check Admin
func IsAdmin(userID int64) bool {

	for _, id := range AdminIDs {

		if id == userID {
			return true
		}
	}

	return false
}
