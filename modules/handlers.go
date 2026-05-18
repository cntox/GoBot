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
			`\xFE0F` +
			`\x{3030}]+`,
	)

	numberPrefixRegex = regexp.MustCompile(
		`(?i)^(?:\[?\s*\d+\s*(?:of|\/)\s*\d+\s*\]?\s*[.:]?\s*` +
			`|Question\s+\d+\s*(?:of|\/)\s*\d+\s*[.:]?\s*` +
			`|\(\s*\d+\s*/\s*\d+\s*\)\s*` +
			`|Q\s*\d+\s*[.:]?\s*)`,
	)

	quizStartIDRegex = regexp.MustCompile(`start=([\w-]+)`)
	quizIDRegex      = regexp.MustCompile(`quiz:([\w-]+)`)

	pendingResetMu sync.Mutex
	pendingReset   = make(map[int64]bool)
)

// =======================
// HELPERS
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

func clearTargetChat() {
	targetChatMu.Lock()
	defer targetChatMu.Unlock()
	targetChat = 0
}

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
	optionPrefix := regexp.MustCompile(`^\s*[(\[]?[A-Da-d][\)\].]?\s*`)
	cleaned = optionPrefix.ReplaceAllString(cleaned, "")
	return strings.TrimSpace(cleaned)
}

func checkUserJoinedChannels(
	client *telegram.Client,
	userID int64,
	uInfo *database.UserInfo,
) (bool, bool, map[string]bool) {
	channelsStatus := make(map[string]bool)
	for _, channel := range config.RequiredChannels {
		channelsStatus[channel] = isUserInChannel(client, userID, channel)
	}
	hasJoinedAll, statusChanged := database.UpdateUserChannels(userID, channelsStatus, uInfo)
	return hasJoinedAll, statusChanged, channelsStatus
}

func isUserInChannel(client *telegram.Client, userID int64, channel string) bool {
	_, err := client.GetChatMember(channel, userID)
	if err != nil {
		errStr := err.Error()
		if strings.Contains(errStr, "USER_NOT_PARTICIPANT") ||
			strings.Contains(errStr, "CHANNEL_PRIVATE") ||
			strings.Contains(errStr, "not a channel") ||
			strings.Contains(errStr, "peer is not") {
			return false
		}
		log.Printf("Error checking channel %s for user %d: %v", channel, userID, err)
		return false
	}
	return true
}

func buildWarningMessage(channelsStatus map[string]bool) string {
	msg := "**⚠️ 𝐏𝐡𝐥𝐞 𝐬𝐚𝐫𝐞 𝐠𝐫𝐨𝐮𝐩 𝐨𝐫 𝐜𝐡𝐚𝐧𝐧𝐞𝐥 𝐣𝐨𝐢𝐧 𝐤𝐫 𝐧𝐡𝐢 𝐦 𝐧𝐡𝐢 𝐛𝐧𝐚 𝐫𝐡𝐚 𝐭𝐮𝐦𝐡𝐚𝐫𝐞 𝐤𝐨𝐢 𝐩𝐨𝐥𝐥𝐬 🙂😏!**\n\n"
	msg += "**Please join these channels:**\n"
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
	msg := "🎉 **Welcome! Thank you for joining all required channels!**\n\n"
	msg += "✅ **You can now use /pn command to create polls.**\n\n"
	msg += "**Channels you joined:**\n"
	for i, channel := range config.RequiredChannels {
		display := config.ChannelDisplay[channel]
		if display == "" {
			display = channel
		}
		msg += fmt.Sprintf("%d. %s\n", i+1, display)
	}
	msg += "\n**How to use:**\n"
	msg += "1. Reply to a quiz share message with `/pn`\n"
	msg += "2. The bot will forward the quiz as a closed poll\n\n"
	msg += "**Other commands:**\n"
	msg += "• `/again` - Try the quiz again\n"
	msg += "• `/stop` - Stop the current quiz session\n"
	msg += "• `/check` - Check your channel status\n"
	msg += "• `/refresh` - Force refresh your status"
	return msg
}

func sendWarningIfAllowed(m *telegram.NewMessage, userID int64, channelsStatus map[string]bool) {
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

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func randomID() int64 {
	return rand.Int63()
}

// clickButtonByText clicks the first inline button whose text contains the given substring (case-insensitive).
// Returns true if clicked successfully.
func clickButtonByText(m *telegram.NewMessage, textSearch string) bool {
	rm := m.ReplyMarkup()
	if rm == nil {
		return false
	}
	markup, ok := (*rm).(*telegram.ReplyInlineMarkup)
	if !ok {
		return false
	}
	for _, row := range markup.Rows {
		for _, btn := range row.Buttons {
			if cb, ok := btn.(*telegram.KeyboardButtonCallback); ok {
				if strings.Contains(strings.ToLower(string(cb.Text)), textSearch) {
					m.Click(cb.Text)
					return true
				}
			}
		}
	}
	return false
}

// getInlineURLs returns all URL buttons from a message's inline keyboard.
func getInlineURLs(m *telegram.NewMessage) []string {
	rm := m.ReplyMarkup()
	if rm == nil {
		return nil
	}
	markup, ok := (*rm).(*telegram.ReplyInlineMarkup)
	if !ok {
		return nil
	}
	var urls []string
	for _, row := range markup.Rows {
		for _, btn := range row.Buttons {
			if urlBtn, ok := btn.(*telegram.KeyboardButtonURL); ok {
				urls = append(urls, urlBtn.URL)
			}
		}
	}
	return urls
}

// =======================
// QUIZBOT HANDLER LOGIC
// =======================

func handleQuizBotMessage(client *telegram.Client, m *telegram.NewMessage) error {
	localTarget := getTargetChat()
	if localTarget == 0 {
		return nil
	}

	// Click "I am ready" button if present
	if clickButtonByText(m, "i am ready") {
		log.Println("Clicked 'I am ready' button")
		return nil
	}

	// Only process quiz polls
	pollMedia := m.Poll()
	if pollMedia == nil {
		return nil
	}
	poll := pollMedia.Poll
	if poll == nil || !poll.Quiz {
		return nil
	}
	if len(poll.Answers) == 0 {
		return nil
	}

	// Pick a random answer option and vote
	randAns := poll.Answers[rand.Intn(len(poll.Answers))]
	randAnsObj, ok := randAns.(*telegram.PollAnswerObj)
	if !ok {
		log.Println("Could not cast PollAnswer to PollAnswerObj")
		return nil
	}
	voteOption := randAnsObj.Option

	// Resolve QuizBot peer for voting
	quizBotPeer, err := client.ResolvePeer("QuizBot")
	if err != nil {
		log.Printf("Could not resolve QuizBot: %v", err)
		return err
	}

	if _, err := client.MessagesSendVote(quizBotPeer, int32(m.ID), [][]byte{voteOption}); err != nil {
		log.Printf("Error sending vote: %v", err)
		return err
	}

	time.Sleep(2 * time.Second)

	// Poll for updated results (up to 10 retries)
	var updatedMsg *telegram.NewMessage
	for i := 0; i < 10; i++ {
		msgs, err := client.GetMessages("QuizBot", &telegram.SearchOption{
			IDs: int32(m.ID),
		})
		if err == nil && len(msgs) > 0 {
			copy := msgs[0]
			updatedMsg = &copy
			pm := updatedMsg.Poll()
			if pm != nil && pm.Results != nil && len(pm.Results.Results) > 0 {
				break
			}
		}
		time.Sleep(1 * time.Second)
	}

	if updatedMsg == nil {
		log.Println("No updated message received after retries")
		client.SendMessage(localTarget, "Error: Could not fetch updated poll")
		return nil
	}

	updatedPollMedia := updatedMsg.Poll()
	if updatedPollMedia == nil || updatedPollMedia.Results == nil || len(updatedPollMedia.Results.Results) == 0 {
		log.Println("No poll results received after retries")
		client.SendMessage(localTarget, "Error: No poll results received")
		return nil
	}

	// Find the correct answer
	var correctOption []byte
	for _, res := range updatedPollMedia.Results.Results {
		if res.Correct {
			correctOption = res.Option
			break
		}
	}
	if correctOption == nil {
		log.Println("No correct option found in poll results")
		client.SendMessage(localTarget, "No correct option found in poll results")
		return nil
	}

	originalPoll := updatedPollMedia.Poll

	cleanedQuestion := cleanQuestion(originalPoll.Question.Text)

	// Clean answer texts, preserve original Option bytes (needed for correct answer matching)
	var cleanedAnswers []telegram.PollAnswer
	for _, ans := range originalPoll.Answers {
		ansObj, ok := ans.(*telegram.PollAnswerObj)
		if !ok {
			continue
		}
		cleanedAnswers = append(cleanedAnswers, &telegram.PollAnswerObj{
			Text:   &telegram.TextWithEntities{Text: cleanAnswerText(ansObj.Text.Text)},
			Option: ansObj.Option,
		})
	}

	// correctOption[0] is ASCII: '0'=48,'1'=49,'2'=50,'3'=51
	// Subtract '0' to get zero-based index directly
	correctIdx := int(correctOption[0] - '0')
	if correctIdx < 0 {
		correctIdx = 0
	}
	if correctIdx >= len(cleanedAnswers) {
		correctIdx = len(cleanedAnswers) - 1
	}
	fmt.Println("FINAL correctIdx =", correctIdx)

	// PollOptions.CorrectAnswers is [][]byte in patched gogram
	sentMsg, err := client.SendPoll(
		localTarget,
		cleanedQuestion,
		cleanedOptionsFromAnswers(cleanedAnswers),
		&telegram.PollOptions{
			IsQuiz:         true,
			CorrectAnswers: [][]byte{{byte(correctIdx)}},
			PublicVoters:   false,
		},
	)
	if err != nil {
		log.Printf("Failed to send poll: %v", err)
		client.SendMessage(localTarget, fmt.Sprintf("Error sending poll: %v", err))
		return err
	}
	log.Printf("Quiz poll sent, ID=%d", sentMsg.ID)

	time.Sleep(1 * time.Second)

	closedAnswers := make([]telegram.PollAnswer, len(cleanedAnswers))
	for i, ans := range cleanedAnswers {
		ansObj := ans.(*telegram.PollAnswerObj)
		closedAnswers[i] = &telegram.PollAnswerObj{
			Text:   ansObj.Text,
			Option: []byte{byte(i)},
		}
	}
	closedPoll := &telegram.InputMediaPoll{
		Poll: &telegram.Poll{
			ID:       originalPoll.ID,
			Question: &telegram.TextWithEntities{Text: cleanedQuestion},
			Answers:  closedAnswers,
			Quiz:     true,
			Closed:   true,
		},
		// InputMediaPoll.CorrectAnswers is [][]byte in patched gogram
		CorrectAnswers: [][]byte{{byte(correctIdx)}},
	}
	if _, err := client.EditMessage(localTarget, int32(sentMsg.ID), closedPoll); err != nil {
		log.Printf("Failed to close poll: %v", err)
	} else {
		log.Println("Poll closed successfully")
	}

	return nil
}

// cleanedOptionsFromAnswers extracts cleaned text strings from PollAnswer slice
func cleanedOptionsFromAnswers(answers []telegram.PollAnswer) []string {
	var opts []string
	for _, ans := range answers {
		if a, ok := ans.(*telegram.PollAnswerObj); ok {
			opts = append(opts, a.Text.Text)
		}
	}
	return opts
}

// =======================
// REGISTER HANDLERS
// =======================

func RegisterHandlers(client *telegram.Client) {

	// ── /start ──────────────────────────────────────────────────────────────────
	client.OnCommand("start", func(m *telegram.NewMessage) error {
		msg := "👋 **Welcome to the Quiz Poll Bot!**\n\n"
		msg += "**This bot helps you forward quiz polls from QuizBot.**\n\n"
		msg += "**Commands:**\n"
		msg += "• `/pn` - Start a new quiz (reply to a quiz message)\n"
		msg += "• `/again` - Try the quiz again\n"
		msg += "• `/stop` - Stop the current quiz session\n"
		msg += "• `/check` - Check your channel status\n"
		msg += "• `/status` - Detailed channel status\n"
		msg += "• `/refresh` - Force refresh your status\n"
		if config.IsAdmin(m.SenderID()) {
			msg += "• `/stats` - View bot statistics (Admin)\n"
			msg += "• `/broadcast` - Broadcast message (Admin)\n"
			msg += "• `/resetdb` - Reset database (Admin)\n"
		}
		msg += "\n**Note:** You need to join all required channels before using /pn command."
		_, err := m.Reply(msg)
		return err
	})

	// ── /ping ───────────────────────────────────────────────────────────────────
	client.OnCommand("ping", func(m *telegram.NewMessage) error {
		_, err := m.Reply("🏓 Pong!\n⚡ Bot Working Successfully.")
		return err
	})

	// ── /bad ────────────────────────────────────────────────────────────────────
	client.OnCommand("bad", func(m *telegram.NewMessage) error {
		_, err := m.Reply("😎 BAD OP 🔥 VIVAN LUND KA TOPI")
		return err
	})

	// ── /pn ─────────────────────────────────────────────────────────────────────
	client.OnCommand("pn", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		uInfo := senderUserInfo(m)

		joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)

		if statusChanged && joinedAll {
			status := database.GetUserStatus(userID)
			if !status.WelcomeSent {
				m.Reply(buildWelcomeMessage())
				database.UpdateWelcomeSent(userID, true)
			} else {
				m.Reply("✅ **Channel membership verified!** You can now use /pn command.")
			}
		}

		if !joinedAll {
			sendWarningIfAllowed(m, userID, channelsStatus)
			return nil
		}

		reply, err := m.GetReplyMessage()
		if err != nil || reply == nil {
			_, err = m.Reply("Reply to a quiz share message.")
			return err
		}

		text := reply.Text()
		quizID := ""

		if strings.Contains(text, "t.me/QuizBot?start=") {
			if match := quizStartIDRegex.FindStringSubmatch(text); len(match) > 1 {
				quizID = match[1]
			}
		} else if strings.Contains(text, "@QuizBot quiz:") {
			if match := quizIDRegex.FindStringSubmatch(text); len(match) > 1 {
				quizID = "quiz:" + match[1]
			}
		}

		// Fallback: check inline URL buttons
		if quizID == "" {
			for _, url := range getInlineURLs(reply) {
				if strings.Contains(url, "t.me/QuizBot?start=") {
					if match := quizStartIDRegex.FindStringSubmatch(url); len(match) > 1 {
						quizID = match[1]
						break
					}
				}
			}
		}

		if quizID == "" {
			_, err = m.Reply("Invalid quiz share format.")
			return err
		}

		setTargetChat(m.ChatID())
		log.Printf("Starting quiz with ID: %s", quizID)
		client.SendMessage("QuizBot", "/stop")
		time.Sleep(1 * time.Second)
		client.SendMessage("QuizBot", "/start "+quizID)
		return nil
	})

	// ── /again ──────────────────────────────────────────────────────────────────
	client.OnCommand("again", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		joinedAll, _, channelsStatus := checkUserJoinedChannels(client, userID, senderUserInfo(m))
		if !joinedAll {
			sendWarningIfAllowed(m, userID, channelsStatus)
			return nil
		}

		chat := getTargetChat()
		if chat == 0 || m.ChatID() != chat {
			_, err := m.Reply("No active quiz session. Use /pn to start a quiz.")
			return err
		}

		msgs, err := client.GetMessages("QuizBot", &telegram.SearchOption{Limit: 10})
		if err != nil {
			_, err = m.Reply("Could not fetch QuizBot messages.")
			return err
		}

		for idx := range msgs {
			msg := &msgs[idx]
			rm := msg.ReplyMarkup()
			if rm == nil {
				continue
			}
			if markup, ok := (*rm).(*telegram.ReplyInlineMarkup); ok {
				for _, row := range markup.Rows {
					for _, btn := range row.Buttons {
						if cb, ok := btn.(*telegram.KeyboardButtonCallback); ok {
							if strings.Contains(strings.ToLower(string(cb.Text)), "try again") {
								msg.Click(cb.Text)
								log.Println("Clicked 'Try again' button via /again command")
								return nil
							}
						}
					}
				}
			}
		}

		_, err = m.Reply("No 'Try again' button found in recent QuizBot messages.")
		return err
	})

	// ── /stop ───────────────────────────────────────────────────────────────────
	client.OnCommand("stop", func(m *telegram.NewMessage) error {
		chat := getTargetChat()
		if chat == 0 || m.ChatID() != chat {
			return nil
		}
		client.SendMessage("QuizBot", "/stop")
		clearTargetChat()
		_, err := m.Reply("Stopped sending polls.")
		return err
	})

	// ── /refresh ────────────────────────────────────────────────────────────────
	client.OnCommand("refresh", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		database.RemoveUser(userID)

		joinedAll, _, channelsStatus := checkUserJoinedChannels(client, userID, senderUserInfo(m))
		if joinedAll {
			status := database.GetUserStatus(userID)
			if !status.WelcomeSent {
				m.Reply(buildWelcomeMessage())
				database.UpdateWelcomeSent(userID, true)
			} else {
				m.Reply("✅ **Channel membership has been refreshed and verified!** You can now use /pn command.")
			}
		} else {
			sendWarningIfAllowed(m, userID, channelsStatus)
		}
		return nil
	})

	// ── /check ──────────────────────────────────────────────────────────────────
	client.OnCommand("check", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, senderUserInfo(m))

		if statusChanged && joinedAll {
			m.Reply(buildWelcomeMessage())
			database.UpdateWelcomeSent(userID, true)
		}

		if !joinedAll {
			sendWarningIfAllowed(m, userID, channelsStatus)
			return nil
		}

		_, err := m.Reply("✅ **You have joined all required channels!**\n\nYou can use /pn command to create polls.")
		return err
	})

	// ── /status ─────────────────────────────────────────────────────────────────
	client.OnCommand("status", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		_, _, channelsStatus := checkUserJoinedChannels(client, userID, senderUserInfo(m))
		status := database.GetUserStatus(userID)

		msg := "📊 **Your Channel Status:**\n\n"
		for i, channel := range config.RequiredChannels {
			joined := channelsStatus[channel]
			emoji, text := "❌", "Not Joined"
			if joined {
				emoji, text = "✅", "Joined"
			}
			display := config.ChannelDisplay[channel]
			if display == "" {
				display = channel
			}
			msg += fmt.Sprintf("%d. %s - %s %s\n", i+1, display, emoji, text)
		}

		overallStatus := "❌ Missing channels"
		if status.JoinedAll {
			overallStatus = "✅ All channels joined"
		}
		welcomeStatus := "❌ No"
		if status.WelcomeSent {
			welcomeStatus = "✅ Yes"
		}
		msg += fmt.Sprintf("\n**Overall Status:** %s\n", overallStatus)
		msg += fmt.Sprintf("**Welcome Sent:** %s\n\n", welcomeStatus)
		if status.JoinedAll {
			msg += "You can use `/pn` command to create polls."
		} else {
			msg += "Please join all channels to use `/pn` command."
		}

		_, err := m.Reply(msg)
		return err
	})

	// ── /stats (admin) ──────────────────────────────────────────────────────────
	client.OnCommand("stats", func(m *telegram.NewMessage) error {
		if !config.IsAdmin(m.SenderID()) {
			_, err := m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
			return err
		}

		stats := database.GetBotStats()
		if stats == nil {
			_, err := m.Reply("❌ **Error:** Could not retrieve statistics.")
			return err
		}

		ago := time.Since(stats.LastUpdated)
		var timeStr string
		switch {
		case ago.Hours() >= 24:
			timeStr = fmt.Sprintf("%.0f days ago", ago.Hours()/24)
		case ago.Hours() >= 1:
			timeStr = fmt.Sprintf("%.0f hours ago", ago.Hours())
		case ago.Minutes() >= 1:
			timeStr = fmt.Sprintf("%.0f minutes ago", ago.Minutes())
		default:
			timeStr = "just now"
		}

		msg := "📈 **Bot Statistics** 📈\n\n"
		msg += "👥 **User Statistics:**\n"
		msg += fmt.Sprintf("• Total Users: `%d`\n", stats.TotalUsers)
		msg += fmt.Sprintf("• Active Users (7 days): `%d`\n", stats.ActiveUsers)
		msg += fmt.Sprintf("• New Users (24 hours): `%d`\n", stats.NewUsers24h)
		msg += fmt.Sprintf("• Users with Access: `%d`\n", stats.UsersWithAccess)
		msg += fmt.Sprintf("• Users Welcomed: `%d`\n\n", stats.UsersWelcomed)
		msg += "📊 **Poll Statistics:**\n"
		msg += fmt.Sprintf("• Total Polls Created: `%d`\n\n", stats.TotalPolls)
		msg += "📢 **Channel Statistics:**\n"
		for _, ch := range config.RequiredChannels {
			display := config.ChannelDisplay[ch]
			if display == "" {
				display = ch
			}
			cnt := stats.ChannelStats[ch]
			pct := float64(0)
			if stats.TotalUsers > 0 {
				pct = float64(cnt) / float64(stats.TotalUsers) * 100
			}
			msg += fmt.Sprintf("• %s: `%d` (%.1f%%)\n", display, cnt, pct)
		}
		msg += fmt.Sprintf("\n⏰ **Last Updated:** %s\n", timeStr)
		msg += fmt.Sprintf("📅 **Database:** `%s`", config.DBName)

		_, err := m.Reply(msg)
		return err
	})

	// ── /resetdb (admin) ────────────────────────────────────────────────────────
	client.OnCommand("resetdb", func(m *telegram.NewMessage) error {
		if !config.IsAdmin(m.SenderID()) {
			_, err := m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
			return err
		}
		pendingResetMu.Lock()
		pendingReset[m.SenderID()] = true
		pendingResetMu.Unlock()
		_, err := m.Reply("⚠️ **Warning:** This will delete ALL user data!\n\nType `/confirm_reset` to proceed or anything else to cancel.")
		return err
	})

	// ── /confirm_reset (admin) ──────────────────────────────────────────────────
	client.OnCommand("confirm_reset", func(m *telegram.NewMessage) error {
		userID := m.SenderID()
		if !config.IsAdmin(userID) {
			_, err := m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
			return err
		}

		pendingResetMu.Lock()
		waiting := pendingReset[userID]
		delete(pendingReset, userID)
		pendingResetMu.Unlock()

		if !waiting {
			_, err := m.Reply("⚠️ No pending reset. Use /resetdb first.")
			return err
		}

		if err := database.ResetDatabase(); err != nil {
			_, err2 := m.Reply(fmt.Sprintf("❌ **Error resetting database:** %v", err))
			return err2
		}
		_, err := m.Reply("✅ **Database has been reset successfully!**\nAll user data has been cleared.")
		return err
	})

	// ── /broadcast (admin) ──────────────────────────────────────────────────────
	client.OnCommand("broadcast", func(m *telegram.NewMessage) error {
		if !config.IsAdmin(m.SenderID()) {
			_, err := m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
			return err
		}

		broadcastText := strings.TrimSpace(strings.TrimPrefix(m.Text(), "/broadcast"))
		if broadcastText == "" {
			_, err := m.Reply("❌ **Usage:** `/broadcast your message here`")
			return err
		}

		m.Reply("📢 **Starting broadcast...**\nThis may take a while.")

		users := database.GetAllUserIDs()
		total := len(users)
		successful, failed := 0, 0

		for _, uid := range users {
			if _, err := client.SendMessage(uid, broadcastText); err != nil {
				log.Printf("Broadcast failed for user %d: %v", uid, err)
				failed++
			} else {
				successful++
			}
			time.Sleep(100 * time.Millisecond)
		}

		pct := float64(0)
		if total > 0 {
			pct = float64(successful) / float64(total) * 100
		}
		summary := "📢 **Broadcast Completed!**\n\n"
		summary += fmt.Sprintf("• Total Users: `%d`\n", total)
		summary += fmt.Sprintf("• Successful: `%d`\n", successful)
		summary += fmt.Sprintf("• Failed: `%d`\n", failed)
		summary += fmt.Sprintf("• Success Rate: `%.1f%%`", pct)

		_, err := m.Reply(summary)
		return err
	})

	// ── QuizBot message handler ─────────────────────────────────────────────────
	// Uses OnMessage with pattern OnNewMessage + custom filter for QuizBot username
	client.OnMessage(string(telegram.OnNewMessage), func(m *telegram.NewMessage) error {
		// Only handle messages from QuizBot
		sender, err := m.GetSender()
		if err != nil || sender == nil {
			return nil
		}
		if !strings.EqualFold(sender.Username, "QuizBot") {
			return nil
		}
		return handleQuizBotMessage(client, m)
	})

	log.Println("Handlers Loaded Successfully")
}
