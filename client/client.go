// client/client.go

package client

import (
	"log/slog"

	"quiz/config"

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

// RegisterHandlers registers all handlers
func RegisterHandlers() {

	// Example Command
	Bot.On("message:/start", func(m *telegram.NewMessage) error {

		_, err := m.Reply(
			"🎉 Quiz Bot Started Successfully!",
		)

		return err
	})
}

// Run keeps bot alive
func Run() {
	Bot.Idle()
}
