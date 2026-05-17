// Session generator — run this ONCE to get your Gogram string session.
// Usage: cd bot && go run generate_session/main.go
// Prompts for your phone number and OTP (and 2FA password if set).
// Copy the session string and use it as SESSION_STRING env var
// or save it to bot/session.txt before running the main bot.
package main

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/workspace/bot/config"
)

func main() {
	reader := bufio.NewReader(os.Stdin)

	fmt.Print("Enter your phone number (with country code, e.g. +919876543210): ")
	phone, _ := reader.ReadString('\n')
	phone = strings.TrimSpace(phone)

	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:    config.APIId,
		AppHash:  config.APIHash,
		LogLevel: telegram.LogInfo,
	})
	if err != nil {
		log.Fatalf("Failed to create client: %v", err)
	}

	// Login prompts for code/password via callbacks
	_, err = client.Login(phone, &telegram.LoginOptions{
		CodeCallback: func() (string, error) {
			fmt.Print("Enter the OTP code Telegram sent you: ")
			code, _ := reader.ReadString('\n')
			return strings.TrimSpace(code), nil
		},
		PasswordCallback: func() (string, error) {
			fmt.Print("Enter your 2FA password: ")
			pw, _ := reader.ReadString('\n')
			return strings.TrimSpace(pw), nil
		},
	})
	if err != nil {
		log.Fatalf("Login failed: %v", err)
	}

	session := client.ExportStringSession()
	fmt.Println("\n✅ Session generated successfully!")
	fmt.Println("Your Gogram string session:")
	fmt.Println("─────────────────────────────────────────")
	fmt.Println(session)
	fmt.Println("─────────────────────────────────────────")
	fmt.Println("\nTo use it, choose one of:")
	fmt.Println("  Option 1: export SESSION_STRING='<your_session_string>'")
	fmt.Println("  Option 2: echo '<your_session_string>' > bot/session.txt")
	fmt.Println("\n⚠️  Keep this session string secret — it grants full account access!")

	fmt.Print("\nSave to session.txt automatically? (y/n): ")
	choice, _ := reader.ReadString('\n')
	if strings.TrimSpace(strings.ToLower(choice)) == "y" {
		if err := os.WriteFile("session.txt", []byte(session), 0600); err != nil {
			log.Printf("Failed to write session.txt: %v", err)
		} else {
			fmt.Println("✅ Saved to session.txt (mode 600)")
		}
	}

	client.Disconnect()
}
