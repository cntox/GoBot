package main

import (
    "log"
    "github.com/amarnathcjd/gogram/telegram"
)


func main() {
    client, err := telegram.NewClient(telegram.ClientConfig{
        AppID: 26593951, AppHash: "98d8a63d74b4082b129defe4f941ab4d",
    })

    if err != nil {
        log.Fatal(err)
    }

    client.Conn() // important, establishes connection to Telegram servers

    // Login as bot or user
    client.LoginBot("8793661673:AAEn5NK7sJ-dV328XBEVgeWhoDWbAICLzUI") 
    // or client.Login("<phone-number>") for user account
    // or client.AuthPrompt() for interactive login

    // Handle incoming messages
    client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
    _, err := m.Reply("Hello from Gogram!")
    return err
}, telegram.IsCommand, telegram.IsGroup)
 // waits for private messages only

    client.Idle() // block main goroutine until client is closed
}
