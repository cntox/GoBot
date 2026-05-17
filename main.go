package main

import (
	"log/slog"
	"os"

	"quiz/client"
	"quiz/config"
	"quiz/database"

	_ "quiz/modules"
)

func main() {

	// Logger
	logger := slog.New(
		slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		}),
	)

	slog.SetDefault(logger)

	slog.Info("🚀 Quiz Bot Starting...", "version", "v1.0.0")

	// Load Config
	config.Load()

	// Database Init
	database.Init()

	// Start Bot
	if err := client.InitBot(); err != nil {
		slog.Error("Failed to start bot", "error", err)
		os.Exit(1)
	}

	// Register Handlers
	client.RegisterHandlers()

	slog.Info("✅ Quiz Bot Running!")

	// Run Forever
	client.Run()
}
