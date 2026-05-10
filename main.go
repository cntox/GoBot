package main

import (
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/amarnathcjd/gogram/telegram"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/showwin/speedtest-go/speedtest"
)

// Session stores poll creation data per (chat, user)
type PollSession struct {
	InstructionMsgID int
	Question         string
	Options          []string
	Ready            bool
}

var sessions sync.Map // key: "chatID:userID" -> *PollSession

func main() {
	client, err := telegram.NewClient(telegram.ClientConfig{
		AppID:   26593951,
		AppHash: "98d8a63d74b4082b129defe4f941ab4d",
	})
	if err != nil {
		log.Fatal(err)
	}

	client.Conn()
	client.LoginBot("8793661673:AAEn5NK7sJ-dV328XBEVgeWhoDWbAICLzUI")
	client.ParseMode("html")

	// Message handlers
	client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		text := m.Text()

		switch {
		case strings.HasPrefix(text, "/start"):
			return handleStart(m, client)
		case strings.HasPrefix(text, "/ping"):
			return handlePing(m)
		case strings.HasPrefix(text, "/speedtest"):
			return handleSpeedtest(m)
		case strings.HasPrefix(text, "/stats"):
			return handleStats(m)
		default:
			// Handle replies that contain poll data
			return handlePollDataReply(m, client)
		}
	}, telegram.IsGroup) // Commands work in groups; reply handling also in groups

	client.On(telegram.OnCallbackQuery, func(cb *telegram.CallbackQuery) error {
		return handleCallback(cb, client)
	})

	client.Idle()
}

// handleStart sends instruction message with inline button
func handleStart(m *telegram.NewMessage, client *telegram.Client) error {
	chatID := m.Chat.ID
	userID := m.Sender.ID
	key := fmt.Sprintf("%d:%d", chatID, userID)

	instruction := "📝 <b>Create a poll</b>\n\n" +
		"Reply to this message with your poll data in this format:\n" +
		"<b>First line:</b> Question\n" +
		"<b>Next lines:</b> Each option on a new line (max 10 options)\n\n" +
		"<i>Example:</i>\n" +
		"<code>What is your favorite color?\nRed\nBlue\nGreen</code>\n\n" +
		"After sending your poll data, click the button below to start the countdown."

	kb := &telegram.InlineKeyboardMarkup{
		InlineKeyboard: [][]telegram.InlineKeyboardButton{
			{
				{Text: "✅ I am ready start", CallbackData: "ready_start"},
			},
		},
	}

	sentMsg, err := m.Reply(instruction, kb)
	if err != nil {
		return err
	}

	// Store session with instruction message ID, poll data not ready yet
	sessions.Store(key, &PollSession{
		InstructionMsgID: sentMsg.ID,
		Ready:            false,
	})
	return nil
}

// handlePollDataReply processes a reply to the instruction message to extract poll question/options
func handlePollDataReply(m *telegram.NewMessage, client *telegram.Client) error {
	if !m.IsReply() {
		return nil
	}
	chatID := m.Chat.ID
	userID := m.Sender.ID
	key := fmt.Sprintf("%d:%d", chatID, userID)

	val, ok := sessions.Load(key)
	if !ok {
		return nil
	}
	session := val.(*PollSession)

	// Check if this reply is to the instruction message
	if m.ReplyToMsgID != session.InstructionMsgID {
		return nil
	}

	// Parse poll data: first line = question, following lines = options
	lines := strings.Split(strings.TrimSpace(m.Text()), "\n")
	if len(lines) < 2 {
		_, _ = m.Reply("❌ Invalid format. Please include a question and at least 2 options (one per line).")
		return nil
	}
	question := strings.TrimSpace(lines[0])
	options := make([]string, 0)
	for i := 1; i < len(lines); i++ {
		opt := strings.TrimSpace(lines[i])
		if opt != "" {
			options = append(options, opt)
		}
	}
	if len(options) < 2 {
		_, _ = m.Reply("❌ You need at least 2 options.")
		return nil
	}
	if len(options) > 10 {
		_, _ = m.Reply("❌ Maximum 10 options allowed.")
		return nil
	}

	// Store poll data and mark as ready
	session.Question = question
	session.Options = options
	session.Ready = true
	sessions.Store(key, session)

	_, _ = m.Reply("✅ Poll data received! Click the <b>\"I am ready start\"</b> button to begin the countdown.")
	return nil
}

// handleCallback processes inline button clicks (ready_start)
func handleCallback(cb *telegram.CallbackQuery, client *telegram.Client) error {
	if cb.Data != "ready_start" {
		return nil
	}

	chatID := cb.Message.Chat.ID
	userID := cb.From.ID
	key := fmt.Sprintf("%d:%d", chatID, userID)

	val, ok := sessions.Load(key)
	if !ok || !val.(*PollSession).Ready {
		_ = cb.Answer("Please send your poll data first (reply to the instruction message).", true)
		return nil
	}
	session := val.(*PollSession)

	// Answer callback immediately to avoid timeout, then start countdown in goroutine
	_ = cb.Answer("Starting countdown...", false)

	go func() {
		msgID := cb.Message.ID
		chatID := cb.Message.Chat.ID

		// Countdown steps
		for i := 3; i > 0; i-- {
			text := fmt.Sprintf("⏳ <b>Poll starting in %d...</b>", i)
			_, _ = client.EditMessage(chatID, msgID, text, &telegram.EditMessageOpts{ParseMode: "html"})
			time.Sleep(1 * time.Second)
		}

		// Final countdown edit
		_, _ = client.EditMessage(chatID, msgID, "<b>🎉 Creating poll...</b>", &telegram.EditMessageOpts{ParseMode: "html"})

		// Send the poll
		_, err := client.SendPoll(chatID, session.Question, session.Options, nil)
		if err != nil {
			_, _ = client.SendMessage(chatID, fmt.Sprintf("❌ Failed to create poll: %v", err), nil)
		} else {
			// Update the instruction message to show completion
			_, _ = client.EditMessage(chatID, msgID, "✅ <b>Poll has been created!</b>", &telegram.EditMessageOpts{ParseMode: "html"})
		}

		// Cleanup session
		sessions.Delete(key)
	}()

	return nil
}

// handlePing remains unchanged
func handlePing(m *telegram.NewMessage) error {
	start := time.Now()
	_, err := m.Reply("🏓 <i>Pinging...</i>")
	if err != nil {
		return err
	}
	elapsed := time.Since(start)
	res := fmt.Sprintf("🏓 <b>Pong!</b>\n\n<b>Latency:</b> <code>%v</code>\n<b>Server Time:</b> <code>%s</code>",
		elapsed, time.Now().Format("15:04:05 MST"))
	_, err = m.Reply(res)
	return err
}

// handleSpeedtest remains unchanged
func handleSpeedtest(m *telegram.NewMessage) error {
	_, _ = m.Reply("🚀 <i>Starting speedtest... Please wait.</i>")

	st := speedtest.New()
	serverList, err := st.FetchServers()
	if err != nil {
		_, err = m.Reply("❌ <b>Error fetching servers.</b>")
		return err
	}

	targets, err := serverList.FindServer([]int{})
	if err != nil || len(targets) == 0 {
		_, err = m.Reply("❌ <b>No suitable server found.</b>")
		return err
	}

	s := targets[0]
	s.PingTest(nil)
	s.DownloadTest()
	s.UploadTest()

	res := fmt.Sprintf("🚀 <b>Speedtest Results</b>\n\n<b>Server:</b> <code>%s</code>\n<b>Ping:</b> <code>%v</code>\n<b>Download:</b> <code>%.2f Mbps</code>\n<b>Upload:</b> <code>%.2f Mbps</code>",
		s.Host, s.Latency, s.DLSpeed, s.ULSpeed)

	_, err = m.Reply(res)
	return err
}

// handleStats remains unchanged
func handleStats(m *telegram.NewMessage) error {
	v, _ := mem.VirtualMemory()
	c, _ := cpu.Percent(0, false)
	d, _ := disk.Usage("/")

	var cpuUsed float64
	if len(c) > 0 {
		cpuUsed = c[0]
	}

	const gb = 1024 * 1024 * 1024

	res := fmt.Sprintf(
		"📊 <b>System Statistics</b>\n\n"+
			"<b>CPU Usage:</b> <code>%.2f%%</code>\n\n"+
			"<b>RAM Memory:</b>\n"+
			"• Total: <code>%.2f GB</code>\n"+
			"• Used: <code>%.2f GB</code> (<code>%.1f%%</code>)\n"+
			"• Free: <code>%.2f GB</code>\n\n"+
			"<b>Disk Storage:</b>\n"+
			"• Total: <code>%.2f GB</code>\n"+
			"• Used: <code>%.2f GB</code> (<code>%.1f%%</code>)\n"+
			"• Free: <code>%.2f GB</code>",
		cpuUsed,
		float64(v.Total)/gb, float64(v.Used)/gb, v.UsedPercent, float64(v.Free)/gb,
		float64(d.Total)/gb, float64(d.Used)/gb, d.UsedPercent, float64(d.Free)/gb,
	)

	_, err := m.Reply(res)
	return err
}
