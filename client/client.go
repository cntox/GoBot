// client/client.go

package client

import (
	"log/slog"

	"quiz/config"
	"quiz/modules"

	"github.com/amarnathcjd/gogram/telegram"
)

var Bot *telegram.Client

// InitBot initializes telegram client
func InitBot() error {

	client, err := telegram.NewClient(
		telegram.ClientConfig{
			AppID:         config.APIId,
			AppHash:       config.APIHash,
			StringSession: config.SessionString,
			LogLevel:      telegram.LogInfo,
		},
	)

	if err != nil {
		return err
	}

	// Start Client
	if err := client.Start(); err != nil {
		return err
	}

	Bot = client

	slog.Info("Telegram client connected successfully")

	return nil
}

// RegisterHandlers
func RegisterHandlers() {

	modules.RegisterHandlers(Bot)
}

// Run
func Run() {
	Bot.Idle()
}
