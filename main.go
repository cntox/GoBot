package main

import (
	"log"
	"os"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/workspace/bot/bot"
	"github.com/workspace/bot/config"
	"github.com/workspace/bot/db"
)

func main() {
	// Read session string from env or file
	sessionString := strings.TrimSpace(os.Getenv("SESSION_STRING"))
	if sessionString == "" {
		data, err := os.ReadFile("session.txt")
		if err == nil {
			sessionString = strings.TrimSpace(string(data))
		}
	}
	if sessionString == "" {
		log.Fatal("SESSION_STRING not set. Run: go run generate_session/main.go")
	}

	// Initialize database
	db.Init()
	db.UpdateBotStats()

	// Create Telegram client with string session
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:         config.APIId,
		AppHash:       config.APIHash,
		StringSession: sessionString,
		LogLevel:      telegram.LogInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Connect + authorize via Start()
	if err := client.Start(); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}

	log.Println("Bot started successfully!")

	// Register all handlers
	bot.RegisterHandlers(client)

	// Block until terminated
	client.Idle()
}
