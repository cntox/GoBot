package modules

import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

	"quiz/config"
	"quiz/database"

	"github.com/amarnathcjd/gogram/telegram"
)

var (
	targetChat   int64
	targetChatMu sync.Mutex

	quizBotUserID   int64
	quizBotUserIDMu sync.RWMutex

	emojiRegex = regexp.MustCompile(
		`[\x{1F600}-\x{1F64F}` +
			`\x{1F300}-\x{1F5FF}` +
			`\x{1F680}-\x{1F6FF}` +
			`\x{1F1E0}-\x{1F1FF}` +
			`\x{2702}-\x{27B0}` +
			`\x{24C2}-\x{1F251}` +
			`\x{1F926}-\x{1F937}` +
			`\x{2640}-\x{2642}` +
			`\x{2600}-\x{2B55}` +
			`\uFE0F` +
			`\u3030]+`,
	)

	numberPrefixRegex = regexp.MustCompile(
		`(?i)^(?:\[?\s*\d+\s*(?:of|\/)\s*\d+\s*\]?\s*[.:]?\s*` +
			`|Question\s+\d+\s*(?:of|\/)\s*\d+\s*[.:]?\s*` +
			`|\(\s*\d+\s*/\s*\d+\s*\)\s*` +
			`|Q\s*\d+\s*[.:]?\s*)`,
	)

	optionPrefixRegex = regexp.MustCompile(
		`^\s*[(\[]?[A-Da-d][)\].]?\s*`,
	)

	quizStartRegex = regexp.MustCompile(`start=([\w-]+)`)
	quizIDRegex    = regexp.MustCompile(`quiz:([\w-]+)`)
)

// =======================
// TARGET CHAT HELPERS
// =======================

func getTargetChat() int64 {
	targetChatMu.Lock()
	defer targetChatMu.Unlock()
	return targetChat
}

func setTargetChat(id int64) {
	targetChatMu.Lock()
	defer targetChatMu.Unlock()
	targetChat = id
}

// =======================
// QUIZBOT USER ID CACHE
// =======================

func getQuizBotUserID() int64 {
	quizBotUserIDMu.RLock()
	defer quizBotUserIDMu.RUnlock()
	return quizBotUserID
}

func setQuizBotUserID(id int64) {
	quizBotUserIDMu.Lock()
	defer quizBotUserIDMu.Unlock()
	quizBotUserID = id
}

// =======================
// TEXT CLEANERS
// =======================

func removeEmojis(text string) string {
	return emojiRegex.ReplaceAllString(text, "")
}

func cleanQuestion(text string) string {
	text = removeEmojis(text)
	for {
		loc := numberPrefixRegex.FindStringIndex(text)
		if loc == nil {
			break
		}
		text = text[loc[1]:]
	}
	return strings.TrimSpace(text)
}

func cleanAnswerText(text string) string {
	cleaned := removeEmojis(text)
	cleaned = optionPrefixRegex.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

// =======================
// CHANNEL CHECK
// =======================

func checkUserJoinedChannels(
	client *telegram.Client,
	userID int64,
	uInfo *database.UserInfo,
) (bool, bool, map[string]bool) {

	channelsStatus := make(map[string]bool)

	for _, channel := range config.RequiredChannels {
		joined := isUserInChannel(client, userID, channel)
		channelsStatus[channel] = joined
	}

	hasJoinedAll, statusChanged := database.UpdateUserChannels(userID, channelsStatus, uInfo)

	return hasJoinedAll, statusChanged, channelsStatus
}

func isUserInChannel(
	client *telegram.Client,
	userID int64,
	channel string,
) bool {
	return true
}

// =======================
// MESSAGE BUILDERS
// =======================

func buildWarningMessage(channelsStatus map[string]bool) string {
	msg := "⚠️ Please join all required channels first!\n\n"

	for i, channel := range config.RequiredChannels {
		status := "✅ Joined"
		if !channelsStatus[channel] {
			status = "❌ Not Joined"
		}
		display := config.ChannelDisplay[channel]
		if display == "" {
			display = channel
		}
		msg += fmt.Sprintf("%d. %s - %s\n", i+1, display, status)
	}

	msg += "\nAfter joining all channels, send /pn command again."
	return msg
}

func buildWelcomeMessage() string {
	msg := "🎉 Welcome! Thank you for joining all required channels!\n\n"
	msg += "✅ You can now use /pn command to create polls.\n\n"
	msg += "Available Commands:\n"
	msg += "• /pn — Start a new quiz (reply to a quiz share message)\n"
	msg += "• /again — Try the quiz again\n"
	msg += "• /stop — Stop the current quiz session\n"
	msg += "• /check — Check your channel status\n"
	msg += "• /status — Detailed channel status\n"
	msg += "• /refresh — Force refresh your status\n"
	msg += "• /ping"
	return msg
}

func sendWarningIfAllowed(
	m *telegram.NewMessage,
	userID int64,
	channelsStatus map[string]bool,
) {
	if database.ShouldSendWarning(userID) {
		m.Reply(buildWarningMessage(channelsStatus))
		database.UpdateLastWarning(userID)
	}
}

func senderUserInfo(m *telegram.NewMessage) *database.UserInfo {
	sender, err := m.GetSender()
	if err != nil || sender == nil {
		return nil
	}
	return &database.UserInfo{
		Username:  sender.Username,
		FirstName: sender.FirstName,
		LastName:  sender.LastName,
	}
}

func isAdmin(userID int64) bool {
	for _, id := range config.AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}

// =======================
// QUIZ ID EXTRACTOR
// =======================

// extractQuizID tries to extract a QuizBot quiz ID from a message's text and inline buttons.
func extractQuizID(msg *telegram.MessageObj) string {
	// Try text
	text := ""
	if msg.Message != "" {
		text = msg.Message
	}

	if strings.Contains(text, "t.me/QuizBot?start=") {
		if m := quizStartRegex.FindStringSubmatch(text); len(m) > 1 {
			return m[1]
		}
	}
	if strings.Contains(text, "@QuizBot quiz:") {
		if m := quizIDRegex.FindStringSubmatch(text); len(m) > 1 {
			return "quiz:" + m[1]
		}
	}

	// Try inline keyboard buttons
	if msg.ReplyMarkup != nil {
		switch rm := msg.ReplyMarkup.(type) {
		case *telegram.ReplyInlineMarkup:
			for _, row := range rm.Rows {
				for _, btn := range row.Buttons {
					if urlBtn, ok := btn.(*telegram.KeyboardButtonUrl); ok {
						if strings.Contains(urlBtn.Url, "t.me/QuizBot?start=") {
							if m := quizStartRegex.FindStringSubmatch(urlBtn.Url); len(m) > 1 {
								return m[1]
							}
						}
					}
				}
			}
		}
	}

	return ""
}

// =======================
// HANDLERS
// =======================

func RegisterHandlers(client *telegram.Client) {

	// Resolve and cache QuizBot user ID once
	go func() {
		peer, err := client.ResolvePeer("QuizBot")
		if err != nil {
			log.Println("⚠️ Could not resolve QuizBot peer:", err)
			return
		}
		if userPeer, ok := peer.(*telegram.InputPeerUser); ok {
			setQuizBotUserID(userPeer.UserId)
			log.Printf("✅ QuizBot user ID cached: %d", userPeer.UserId)
		}
	}()

	// START
	client.OnCommand("start", func(m *telegram.NewMessage) error {
		userID := m.SenderID()

		msg := "👋 Welcome to the Quiz Poll Bot!\n\n"
		msg += "This bot helps you forward quiz polls from QuizBot.\n\n"
		msg += "Commands:\n"
		msg += "• /pn — Start a new quiz (reply to a quiz share message)\n"
		msg += "• /again — Try the quiz again\n"
		msg += "• /stop — Stop the current quiz session\n"
		msg += "• /check — Check your channel status\n"
		msg += "• /status — Detailed channel status\n"
		msg += "• /refresh — Force refresh your status\n"
		msg += "• /ping — Check bot\n"

		if isAdmin(userID) {
			msg += "• /stats — View bot statistics (Admin)\n"
			msg += "• /broadcast <text> — Broadcast message (Admin)\n"
			msg += "• /resetdb — Reset database (Admin)\n"
		}

		msg += "\nNote: You need to join all required channels before using /pn command."

		_, err := m.Reply(msg)
		return err
	})

	// PING
	client.OnCommand("ping", func(m *telegram.NewMessage) error {
		_, err := m.Reply("🏓 Pong!\n⚡ Bot Working Successfully.")
		return err
	})

	// BAD
	client.OnCommand("bad", func(m *telegram.NewMessage) error {
		_, err := m.Reply("😎 Bad OP 🔥")
		return err
	})

	// CHECK
	client.OnCommand("check", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		uInfo := senderUserInfo(m)

		joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)

		if statusChanged && joinedAll {
			m.Reply(buildWelcomeMessage())
			database.UpdateWelcomeSent(userID, true)
			return nil
		}

		if !joinedAll {
			sendWarningIfAllowed(m, userID, channelsStatus)
			return nil
		}

		_, err := m.Reply("✅ You have joined all required channels!\n\nYou can use /pn command to create polls.")
		return err
	})

	// STATUS
	client.OnCommand("status", func(m *telegram.NewMessage) error {
		userID := m.SenderID()

		joinedAll, welcomeSent, channelsStatus := database.GetUserStatus(userID)

		statusMsg := "📊 Your Channel Status:\n\n"

		for i, channel := range config.RequiredChannels {
			joined := channelsStatus[channel]
			emoji := "✅"
			text := "Joined"
			if !joined {
				emoji = "❌"
				text = "Not Joined"
			}
			display := config.ChannelDisplay[channel]
			if display == "" {
				display = channel
			}
			statusMsg += fmt.Sprintf("%d. %s - %s %s\n", i+1, display, emoji, text)
		}

		overallStatus := "❌ Missing channels"
		if joinedAll {
			overallStatus = "✅ All channels joined"
		}
		welcomeStatus := "❌ No"
		if welcomeSent {
			welcomeStatus = "✅ Yes"
		}

		statusMsg += fmt.Sprintf("\nOverall Status: %s\n", overallStatus)
		statusMsg += fmt.Sprintf("Welcome Sent: %s\n\n", welcomeStatus)

		tc := getTargetChat()
		if tc != 0 {
			statusMsg += fmt.Sprintf("📡 Poll forwarding active → chat: %d\n", tc)
		} else {
			statusMsg += "📴 Poll forwarding not active. Use /pn to start.\n"
		}

		if joinedAll {
			statusMsg += "\nYou can use /pn command to create polls."
		} else {
			statusMsg += "\nPlease join all channels to use /pn command."
		}

		_, err := m.Reply(statusMsg)
		return err
	})

	// PN — parse quiz share, start QuizBot, set target chat
	client.OnCommand("pn", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		uInfo := senderUserInfo(m)

		// Check channel membership
		joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)

		if statusChanged && joinedAll {
			_, welcomeSent, _ := database.GetUserStatus(userID)
			if !welcomeSent {
				m.Reply(buildWelcomeMessage())
				database.UpdateWelcomeSent(userID, true)
			} else {
				m.Reply("✅ Channel membership verified! You can now use /pn command.")
			}
		}

		if !joinedAll {
			sendWarningIfAllowed(m, userID, channelsStatus)
			return nil
		}

		// Handle optional chat_id argument
		args := strings.TrimSpace(m.Args())
		var overrideChatID int64

		if args != "" {
			// Try to parse as explicit chat ID override (used without reply)
			var parsed int64
			if _, err := fmt.Sscanf(args, "%d", &parsed); err == nil && parsed != 0 {
				overrideChatID = parsed
			}
		}

		// Try to get replied-to message for quiz link
		replyTo := m.Message.ReplyTo
		if replyTo == nil && overrideChatID == 0 {
			_, err := m.Reply("📌 Reply to a quiz share message to start.\nOr use: /pn <chat_id>")
			return err
		}

		var quizID string

		if replyTo != nil {
			// Fetch the replied-to message
			replyMsgID := replyTo.(*telegram.MessageReplyHeader).ReplyToMsgId
			chatPeer, err := client.ResolvePeer(m.ChatID())
			if err == nil {
				msgs, err := client.GetMessages(chatPeer, []telegram.InputMessage{
					&telegram.InputMessageID{Id: replyMsgID},
				})
				if err == nil && msgs != nil {
					switch container := msgs.(type) {
					case *telegram.MessagesMessages:
						if len(container.Messages) > 0 {
							if msgObj, ok := container.Messages[0].(*telegram.MessageObj); ok {
								quizID = extractQuizID(msgObj)
							}
						}
					case *telegram.MessagesMessagesSlice:
						if len(container.Messages) > 0 {
							if msgObj, ok := container.Messages[0].(*telegram.MessageObj); ok {
								quizID = extractQuizID(msgObj)
							}
						}
					case *telegram.MessagesChannelMessages:
						if len(container.Messages) > 0 {
							if msgObj, ok := container.Messages[0].(*telegram.MessageObj); ok {
								quizID = extractQuizID(msgObj)
							}
						}
					}
				}
			}
		}

		if quizID == "" && overrideChatID == 0 {
			_, err := m.Reply("❌ Invalid quiz share format.\n\nReply to a message containing a QuizBot link.")
			return err
		}

		// Set target chat
		chatID := m.ChatID()
		if overrideChatID != 0 {
			chatID = overrideChatID
		}
		setTargetChat(chatID)

		// Start QuizBot
		if quizID != "" {
			quizBotPeer, err := client.ResolvePeer("QuizBot")
			if err != nil {
				_, replyErr := m.Reply("❌ Could not reach QuizBot. Please try again.")
				return replyErr
			}

			// Stop any running quiz first
			client.SendMessage(quizBotPeer, "/stop", &telegram.SendMessageParams{})
			time.Sleep(1 * time.Second)

			// Start the quiz
			client.SendMessage(quizBotPeer, "/start "+quizID, &telegram.SendMessageParams{})

			log.Printf("✅ Started quiz ID: %s → forwarding to chat: %d", quizID, chatID)

			_, err = m.Reply(fmt.Sprintf(
				"📊 Quiz started!\n✅ Forwarding polls to chat: %d\n\nQuiz ID: %s\n\nUse /stop to disable.",
				chatID, quizID,
			))
			return err
		}

		// No quiz ID but chat ID was set
		_, err := m.Reply(fmt.Sprintf(
			"📊 Poll forwarding activated.\n✅ Target chat set to: %d\n\nUse /stop to disable.",
			chatID,
		))
		return err
	})

	// AGAIN — click "Try again" in QuizBot DM
	client.OnCommand("again", func(m *telegram.NewMessage) error {
		tc := getTargetChat()
		if tc == 0 || m.ChatID() != tc {
			_, err := m.Reply("No active quiz session. Use /pn to start a quiz.")
			return err
		}

		quizBotPeer, err := client.ResolvePeer("QuizBot")
		if err != nil {
			_, err = m.Reply("❌ Could not reach QuizBot.")
			return err
		}

		// Fetch recent messages from QuizBot DM
		msgs, err := client.GetMessages(quizBotPeer, []telegram.InputMessage{
			&telegram.InputMessageID{Id: 0},
		})
		_ = msgs

		// Use MessagesGetHistory to get recent messages
		history, histErr := client.MessagesGetHistory(&telegram.MessagesGetHistoryParams{
			Peer:  quizBotPeer,
			Limit: 10,
		})

		if histErr != nil {
			_, err = m.Reply("❌ Could not fetch QuizBot messages.")
			return err
		}

		var messages []telegram.Message
		switch h := history.(type) {
		case *telegram.MessagesMessages:
			messages = h.Messages
		case *telegram.MessagesMessagesSlice:
			messages = h.Messages
		case *telegram.MessagesChannelMessages:
			messages = h.Messages
		}

		for _, rawMsg := range messages {
			msgObj, ok := rawMsg.(*telegram.MessageObj)
			if !ok || msgObj.ReplyMarkup == nil {
				continue
			}
			inlineKB, ok := msgObj.ReplyMarkup.(*telegram.ReplyInlineMarkup)
			if !ok {
				continue
			}
			for i, row := range inlineKB.Rows {
				for j, btn := range row.Buttons {
					cbBtn, ok := btn.(*telegram.KeyboardButtonCallback)
					if !ok {
						continue
					}
					if strings.Contains(strings.ToLower(cbBtn.Text), "try again") ||
						strings.Contains(strings.ToLower(cbBtn.Text), "again") {
						_, clickErr := client.MessagesGetBotCallbackAnswer(&telegram.MessagesGetBotCallbackAnswerParams{
							Peer:  quizBotPeer,
							MsgId: msgObj.Id,
							Data:  cbBtn.Data,
						})
						if clickErr == nil {
							log.Printf("✅ Clicked 'Try again' button (row %d, btn %d)", i, j)
							_, err = m.Reply("🔄 Clicked 'Try again' — next question incoming!")
							return err
						}
					}
				}
			}
		}

		_, err = m.Reply("⚠️ No 'Try again' button found in recent QuizBot messages.")
		return err
	})

	// STOP
	client.OnCommand("stop", func(m *telegram.NewMessage) error {
		tc := getTargetChat()
		if tc != 0 {
			quizBotPeer, err := client.ResolvePeer("QuizBot")
			if err == nil {
				client.SendMessage(quizBotPeer, "/stop", &telegram.SendMessageParams{})
			}
		}
		setTargetChat(0)
		_, err := m.Reply("🛑 Poll forwarding stopped.")
		return err
	})

	// REFRESH
	client.OnCommand("refresh", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		uInfo := senderUserInfo(m)

		database.RemoveUser(userID)

		joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)

		if joinedAll {
			_, welcomeSent, _ := database.GetUserStatus(userID)
			if !welcomeSent || statusChanged {
				m.Reply(buildWelcomeMessage())
				database.UpdateWelcomeSent(userID, true)
			} else {
				m.Reply("✅ Channel membership has been refreshed and verified! You can now use /pn command.")
			}
			return nil
		}

		sendWarningIfAllowed(m, userID, channelsStatus)
		return nil
	})

	// STATS (admin only)
	client.OnCommand("stats", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		if !isAdmin(userID) {
			_, err := m.Reply("❌ Access Denied!\nThis command is only available for administrators.")
			return err
		}

		stats := database.GetBotStats()
		if stats == nil {
			_, err := m.Reply("❌ Error: Could not retrieve statistics.")
			return err
		}

		msg := "📈 Bot Statistics 📈\n\n"
		msg += "👥 User Statistics:\n"
		msg += fmt.Sprintf("• Total Users: %d\n", stats.TotalUsers)
		msg += fmt.Sprintf("• Active Users (7 days): %d\n", stats.ActiveUsers)
		msg += fmt.Sprintf("• New Users (24h): %d\n", stats.NewUsers24h)
		msg += fmt.Sprintf("• Users with Access: %d\n", stats.UsersWithAccess)
		msg += fmt.Sprintf("• Users Welcomed: %d\n\n", stats.UsersWelcomed)

		msg += "📊 Poll Statistics:\n"
		msg += fmt.Sprintf("• Total Polls Created: %d\n\n", stats.TotalPolls)

		msg += "📢 Channel Statistics:\n"
		for _, channel := range config.RequiredChannels {
			display := config.ChannelDisplay[channel]
			if display == "" {
				display = channel
			}
			count := stats.ChannelStats[channel]
			pct := 0.0
			if stats.TotalUsers > 0 {
				pct = float64(count) / float64(stats.TotalUsers) * 100
			}
			msg += fmt.Sprintf("• %s: %d (%.1f%%)\n", display, count, pct)
		}

		_, err := m.Reply(msg)
		return err
	})

	// BROADCAST (admin only)
	client.OnCommand("broadcast", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		if !isAdmin(userID) {
			_, err := m.Reply("❌ Access Denied!\nThis command is only available for administrators.")
			return err
		}

		broadcastText := strings.TrimSpace(m.Args())
		if broadcastText == "" {
			_, err := m.Reply("❌ Usage: /broadcast your message here")
			return err
		}

		m.Reply("📢 Starting broadcast... This may take a while.")

		users := database.GetAllUserIDs()
		total := len(users)
		successful := 0
		failed := 0

		for _, uid := range users {
			peer, err := client.ResolvePeer(uid)
			if err != nil {
				failed++
				continue
			}
			_, err = client.SendMessage(peer, broadcastText, &telegram.SendMessageParams{})
			if err != nil {
				failed++
			} else {
				successful++
			}
			time.Sleep(100 * time.Millisecond)
		}

		summary := "📢 Broadcast Completed!\n\n"
		summary += fmt.Sprintf("• Total Users: %d\n", total)
		summary += fmt.Sprintf("• Successful: %d\n", successful)
		summary += fmt.Sprintf("• Failed: %d\n", failed)
		if total > 0 {
			summary += fmt.Sprintf("• Success Rate: %.1f%%", float64(successful)/float64(total)*100)
		}

		_, err := m.Reply(summary)
		return err
	})

	// RESETDB (admin only)
	client.OnCommand("resetdb", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		if !isAdmin(userID) {
			_, err := m.Reply("❌ Access Denied!\nThis command is only available for administrators.")
			return err
		}

		err := database.ResetDatabase()
		if err != nil {
			_, replyErr := m.Reply(fmt.Sprintf("❌ Error resetting database: %s", err.Error()))
			return replyErr
		}

		_, replyErr := m.Reply("✅ Database has been reset successfully!\nAll user data has been cleared.")
		return replyErr
	})

	// QuizBot message handler (messages in DM from QuizBot)
	client.On(telegram.OnMessage, func(m *telegram.NewMessage) error {
		return handleQuizBot(client, m)
	})

	log.Println("✅ Handlers Loaded Successfully")
}

// =======================
// QUIZ BOT HANDLER
// =======================

func handleQuizBot(client *telegram.Client, m *telegram.NewMessage) error {

	// Only process messages in the QuizBot DM
	qbID := getQuizBotUserID()
	if qbID == 0 {
		return nil
	}

	// Message must be from QuizBot in a private DM
	senderID := m.SenderID()
	if senderID != qbID {
		return nil
	}

	tc := getTargetChat()
	if tc == 0 {
		return nil
	}

	msg := m.Message

	// ── Handle inline keyboard buttons (e.g., "I am ready") ──
	if msg.ReplyMarkup != nil {
		if inlineKB, ok := msg.ReplyMarkup.(*telegram.ReplyInlineMarkup); ok {
			for _, row := range inlineKB.Rows {
				for _, btn := range row.Buttons {
					cbBtn, ok := btn.(*telegram.KeyboardButtonCallback)
					if !ok {
						continue
					}
					if strings.Contains(strings.ToLower(cbBtn.Text), "i am ready") ||
						strings.Contains(strings.ToLower(cbBtn.Text), "ready") {

						quizBotPeer, err := client.ResolvePeer("QuizBot")
						if err != nil {
							return nil
						}
						_, err = client.MessagesGetBotCallbackAnswer(&telegram.MessagesGetBotCallbackAnswerParams{
							Peer:  quizBotPeer,
							MsgId: msg.Id,
							Data:  cbBtn.Data,
						})
						if err == nil {
							log.Println("✅ Clicked 'I am ready' button")
						}
						return nil
					}
				}
			}
		}
	}

	// ── Handle quiz poll ──
	if msg.Media == nil {
		return nil
	}

	mediaPoll, ok := msg.Media.(*telegram.MessageMediaPoll)
	if !ok {
		return nil
	}

	poll := mediaPoll.Poll
	pollObj, ok := poll.(*telegram.PollObj)
	if !ok || !pollObj.Quiz {
		return nil
	}

	if len(pollObj.Answers) == 0 {
		return nil
	}

	// Vote randomly on the poll
	randomAns, ok := pollObj.Answers[rand.Intn(len(pollObj.Answers))].(*telegram.PollAnswerObj)
	if !ok {
		return nil
	}

	quizBotPeer, err := client.ResolvePeer("QuizBot")
	if err != nil {
		log.Println("⚠️ Could not resolve QuizBot:", err)
		return nil
	}

	_, err = client.MessagesSendVote(quizBotPeer, msg.Id, [][]byte{randomAns.Option})
	if err != nil {
		log.Println("⚠️ Could not send vote:", err)
		return nil
	}

	// Wait for the poll results to come in
	time.Sleep(2 * time.Second)

	// Poll for updated results (up to 10 attempts)
	var updatedPollMedia *telegram.MessageMediaPoll
	for attempt := 0; attempt < 10; attempt++ {
		msgs, err := client.GetMessages(quizBotPeer, []telegram.InputMessage{
			&telegram.InputMessageID{Id: msg.Id},
		})
		if err != nil {
			time.Sleep(1 * time.Second)
			continue
		}

		var msgList []telegram.Message
		switch c := msgs.(type) {
		case *telegram.MessagesMessages:
			msgList = c.Messages
		case *telegram.MessagesMessagesSlice:
			msgList = c.Messages
		case *telegram.MessagesChannelMessages:
			msgList = c.Messages
		}

		for _, rawMsg := range msgList {
			if updatedMsg, ok := rawMsg.(*telegram.MessageObj); ok {
				if updatedMsg.Media != nil {
					if pm, ok := updatedMsg.Media.(*telegram.MessageMediaPoll); ok {
						if pm.Results != nil {
							if res, ok := pm.Results.(*telegram.PollResultsObj); ok {
								if len(res.Results) > 0 {
									updatedPollMedia = pm
									goto gotResults
								}
							}
						}
					}
				}
			}
		}
		time.Sleep(1 * time.Second)
	}

gotResults:
	if updatedPollMedia == nil {
		log.Println("⚠️ No poll results received after retries")
		return nil
	}

	results, ok := updatedPollMedia.Results.(*telegram.PollResultsObj)
	if !ok {
		return nil
	}

	// Find correct answer
	var correctOption []byte
	for _, res := range results.Results {
		if voter, ok := res.(*telegram.PollAnswerVotersObj); ok && voter.Correct {
			correctOption = voter.Option
			break
		}
	}

	if correctOption == nil {
		log.Println("⚠️ No correct option found in poll results")
		return nil
	}

	// Clean question text
	originalPoll, ok := updatedPollMedia.Poll.(*telegram.PollObj)
	if !ok {
		return nil
	}

	questionText := ""
	if originalPoll.Question != nil {
		if twEntities, ok := originalPoll.Question.(*telegram.TextWithEntities); ok {
			questionText = twEntities.Text
		}
	}
	cleanedQuestion := cleanQuestion(questionText)

	if cleanedQuestion == "" {
		log.Println("⚠️ Question is empty after cleaning, skipping")
		return nil
	}

	// Clean answers
	var cleanedAnswers []telegram.PollAnswer
	for _, ans := range originalPoll.Answers {
		ansObj, ok := ans.(*telegram.PollAnswerObj)
		if !ok {
			continue
		}

		ansText := ""
		if ansObj.Text != nil {
			if twEntities, ok := ansObj.Text.(*telegram.TextWithEntities); ok {
				ansText = twEntities.Text
			}
		}

		cleaned := cleanAnswerText(ansText)
		if cleaned == "" {
			cleaned = ansText
		}

		cleanedAnswers = append(cleanedAnswers, &telegram.PollAnswerObj{
			Text:   &telegram.TextWithEntities{Text: cleaned},
			Option: ansObj.Option,
		})
	}

	if len(cleanedAnswers) == 0 {
		log.Println("⚠️ No valid answers after cleaning")
		return nil
	}

	// Build the quiz poll to send
	pollID := time.Now().UnixNano()

	newPoll := &telegram.PollObj{
		Id: pollID,
		Question: &telegram.TextWithEntities{
			Text: cleanedQuestion,
		},
		Answers:        cleanedAnswers,
		Quiz:           true,
		PublicVoters:   false,
		MultipleChoice: false,
		Closed:         false,
	}

	inputMedia := &telegram.InputMediaPoll{
		Poll:           newPoll,
		CorrectAnswers: [][]byte{correctOption},
	}

	// Resolve target chat peer — handles -100XXXXXXXXXX supergroup IDs
	targetPeer, err := client.ResolvePeer(tc)
	if err != nil {
		log.Printf("⚠️ Could not resolve target chat %d: %v", tc, err)
		return nil
	}

	// Send the open quiz poll first
	sentMsg, err := client.SendMedia(targetPeer, inputMedia, &telegram.SendMediaParams{})
	if err != nil {
		log.Printf("⚠️ Failed to send quiz poll: %v", err)
		return nil
	}

	log.Printf("✅ Quiz poll sent to chat %d (msg ID: %d)", tc, sentMsg.ID)

	time.Sleep(1 * time.Second)

	// Close the poll immediately so the correct answer is revealed
	closedPoll := &telegram.PollObj{
		Id: pollID,
		Question: &telegram.TextWithEntities{
			Text: cleanedQuestion,
		},
		Answers:        cleanedAnswers,
		Quiz:           true,
		PublicVoters:   false,
		MultipleChoice: false,
		Closed:         true,
	}

	closedMedia := &telegram.InputMediaPoll{
		Poll:           closedPoll,
		CorrectAnswers: [][]byte{correctOption},
	}

	_, err = client.MessagesEditMessage(&telegram.MessagesEditMessageParams{
		Peer:  targetPeer,
		Id:    sentMsg.ID,
		Media: closedMedia,
	})
	if err != nil {
		log.Printf("⚠️ Failed to close poll: %v", err)
	} else {
		log.Println("✅ Poll closed — correct answer revealed")
	}

	return nil
}
