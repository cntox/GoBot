package bot

import (
        "fmt"
        "log"
        "math/rand"
        "regexp"
        "strings"
        "sync"
        "time"

        "github.com/amarnathcjd/gogram/telegram"
        "github.com/workspace/bot/config"
        "github.com/workspace/bot/db"
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
                        `\x{3030}]+`)

        numberPrefixRegex = regexp.MustCompile(
                `(?i)^(?:\[?\s*\d+\s*(?:of|\/)\s*\d+\s*\]?\s*[.:]?\s*` +
                        `|Question\s+\d+\s*(?:of|\/)\s*\d+\s*[.:]?\s*` +
                        `|\(\s*\d+\s*/\s*\d+\s*\)\s*` +
                        `|Q\s*\d+\s*[.:]?\s*)`)
)

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
        optionPrefix := regexp.MustCompile(`^\s*[(\[]?[A-Da-d][)\].]?\s*`)
        cleaned = optionPrefix.ReplaceAllString(cleaned, "")
        return strings.TrimSpace(cleaned)
}

func checkUserJoinedChannels(client *telegram.Client, userID int64, uInfo *db.UserInfo) (bool, bool, map[string]bool) {
        channelsStatus := make(map[string]bool)
        for _, channel := range config.RequiredChannels {
                joined := isUserInChannel(client, userID, channel)
                channelsStatus[channel] = joined
        }
        hasJoinedAll, statusChanged := db.UpdateUserChannels(userID, channelsStatus, uInfo)
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
        if db.ShouldSendWarning(userID) {
                m.Reply(buildWarningMessage(channelsStatus))
                db.UpdateLastWarning(userID)
        }
}

func senderUserInfo(m *telegram.NewMessage) *db.UserInfo {
        sender, err := m.GetSender()
        if err != nil || sender == nil {
                return nil
        }
        return &db.UserInfo{
                Username:  sender.Username,
                FirstName: sender.FirstName,
                LastName:  sender.LastName,
        }
}

// RegisterHandlers registers all command and message handlers on the client.
func RegisterHandlers(client *telegram.Client) {
        client.OnCommand("start", func(m *telegram.NewMessage) error {
                return handleStart(m)
        })
        client.OnCommand("pn", func(m *telegram.NewMessage) error {
                return handlePn(client, m)
        })
        client.OnCommand("again", func(m *telegram.NewMessage) error {
                return handleAgain(client, m)
        })
        client.OnCommand("stop", func(m *telegram.NewMessage) error {
                return handleStop(client, m)
        })
        client.OnCommand("refresh", func(m *telegram.NewMessage) error {
                return handleRefresh(client, m)
        })
        client.OnCommand("check", func(m *telegram.NewMessage) error {
                return handleCheck(client, m)
        })
        client.OnCommand("status", func(m *telegram.NewMessage) error {
                return handleStatus(m)
        })
        client.OnCommand("stats", func(m *telegram.NewMessage) error {
                return handleStats(m)
        })
        client.OnCommand("resetdb", func(m *telegram.NewMessage) error {
                return handleResetDB(m)
        })
        client.OnCommand("confirm_reset", func(m *telegram.NewMessage) error {
                return handleConfirmReset(m)
        })
        client.OnCommand("broadcast", func(m *telegram.NewMessage) error {
                return handleBroadcast(client, m)
        })

        // Catch all messages from QuizBot (user ID 776597425)
        client.AddMessageHandler("", func(m *telegram.NewMessage) error {
                return handleQuizBot(client, m)
        }, telegram.FromUsers(int64(776597425)))

        log.Println("All handlers registered")
}

func handleStart(m *telegram.NewMessage) error {
        userID := m.SenderID()
        msg := "👋 **Welcome to the Quiz Poll Bot!**\n\n"
        msg += "**This bot helps you forward quiz polls from QuizBot.**\n\n"
        msg += "**Commands:**\n"
        msg += "• `/pn` - Start a new quiz (reply to a quiz message)\n"
        msg += "• `/again` - Try the quiz again\n"
        msg += "• `/stop` - Stop the current quiz session\n"
        msg += "• `/check` - Check your channel status\n"
        msg += "• `/status` - Detailed channel status\n"
        msg += "• `/refresh` - Force refresh your status\n"
        if config.IsAdmin(userID) {
                msg += "• `/stats` - View bot statistics (Admin)\n"
                msg += "• `/broadcast` - Broadcast message to all users (Admin)\n"
                msg += "• `/resetdb` - Reset database (Admin)\n"
        }
        msg += "\n**Note:** You need to join all required channels before using /pn command."
        _, err := m.Reply(msg)
        return err
}

func handlePn(client *telegram.Client, m *telegram.NewMessage) error {
        userID := m.SenderID()
        uInfo := senderUserInfo(m)

        joinedAll, statusChanged, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)

        if statusChanged && joinedAll {
                status := db.GetUserStatus(userID)
                if !status.WelcomeSent {
                        m.Reply(buildWelcomeMessage())
                        db.UpdateWelcomeSent(userID, true)
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
                m.Reply("Reply to a quiz share message.")
                return nil
        }

        text := reply.MessageText()
        quizID := extractQuizID(text, reply)
        if quizID == "" {
                m.Reply("Invalid quiz share format.")
                return nil
        }

        setTargetChat(m.ChatID())
        log.Printf("Starting quiz with ID: %s", quizID)

        client.SendMessage("QuizBot", "/stop")
        time.Sleep(1 * time.Second)
        client.SendMessage("QuizBot", fmt.Sprintf("/start %s", quizID))
        return nil
}

func extractQuizID(text string, reply *telegram.NewMessage) string {
        if strings.Contains(text, "t.me/QuizBot?start=") {
                re := regexp.MustCompile(`start=([\w-]+)`)
                if m := re.FindStringSubmatch(text); len(m) > 1 {
                        return m[1]
                }
        }
        if strings.Contains(text, "@QuizBot quiz:") {
                re := regexp.MustCompile(`quiz:([\w-]+)`)
                if m := re.FindStringSubmatch(text); len(m) > 1 {
                        return "quiz:" + m[1]
                }
        }
        // Check inline keyboard buttons
        markupPtr := reply.ReplyMarkup()
        if markupPtr != nil {
                if kb, ok := (*markupPtr).(*telegram.ReplyInlineMarkup); ok {
                        for _, row := range kb.Rows {
                                for _, btn := range row.Buttons {
                                        if urlBtn, ok := btn.(*telegram.KeyboardButtonURL); ok {
                                                if strings.Contains(urlBtn.URL, "t.me/QuizBot?start=") {
                                                        re := regexp.MustCompile(`start=([\w-]+)`)
                                                        if m := re.FindStringSubmatch(urlBtn.URL); len(m) > 1 {
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

func handleAgain(client *telegram.Client, m *telegram.NewMessage) error {
        userID := m.SenderID()
        uInfo := senderUserInfo(m)

        joinedAll, _, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)
        if !joinedAll {
                sendWarningIfAllowed(m, userID, channelsStatus)
                return nil
        }

        tc := getTargetChat()
        if tc == 0 || m.ChatID() != tc {
                m.Reply("No active quiz session. Use /pn to start a quiz.")
                return nil
        }

        msgs, err := client.GetHistory("QuizBot", &telegram.HistoryOption{Limit: 10})
        if err != nil {
                m.Reply("Could not fetch QuizBot messages.")
                return nil
        }

        for i := range msgs {
                msg := &msgs[i]
                markupPtr := msg.ReplyMarkup()
                if markupPtr == nil {
                        continue
                }
                if kb, ok := (*markupPtr).(*telegram.ReplyInlineMarkup); ok {
                        for _, row := range kb.Rows {
                                for _, btn := range row.Buttons {
                                        if cb, ok := btn.(*telegram.KeyboardButtonCallback); ok {
                                                if strings.Contains(strings.ToLower(cb.Text), "try again") {
                                                        peer, _ := client.ResolvePeer("QuizBot")
                                                        client.MessagesGetBotCallbackAnswer(&telegram.MessagesGetBotCallbackAnswerParams{
                                                                Peer:  peer,
                                                                MsgID: msg.ID,
                                                                Data:  cb.Data,
                                                        })
                                                        log.Println("Clicked 'Try again' button")
                                                        return nil
                                                }
                                        }
                                }
                        }
                }
        }

        m.Reply("No 'Try again' button found in recent QuizBot messages.")
        return nil
}

func handleStop(client *telegram.Client, m *telegram.NewMessage) error {
        tc := getTargetChat()
        if tc == 0 || m.ChatID() != tc {
                return nil
        }
        client.SendMessage("QuizBot", "/stop")
        setTargetChat(0)
        m.Reply("Stopped sending polls.")
        return nil
}

func handleRefresh(client *telegram.Client, m *telegram.NewMessage) error {
        userID := m.SenderID()
        db.RemoveUser(userID)
        uInfo := senderUserInfo(m)
        joinedAll, _, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)
        if joinedAll {
                status := db.GetUserStatus(userID)
                if !status.WelcomeSent {
                        m.Reply(buildWelcomeMessage())
                        db.UpdateWelcomeSent(userID, true)
                } else {
                        m.Reply("✅ **Channel membership has been refreshed and verified!** You can now use /pn command.")
                }
        } else {
                sendWarningIfAllowed(m, userID, channelsStatus)
        }
        return nil
}

func handleCheck(client *telegram.Client, m *telegram.NewMessage) error {
        userID := m.SenderID()
        uInfo := senderUserInfo(m)
        joinedAll, _, channelsStatus := checkUserJoinedChannels(client, userID, uInfo)
        if joinedAll {
                m.Reply("✅ **You have joined all required channels!**\n\nYou can use /pn command to create polls.")
        } else {
                sendWarningIfAllowed(m, userID, channelsStatus)
        }
        return nil
}

func handleStatus(m *telegram.NewMessage) error {
        userID := m.SenderID()
        status := db.GetUserStatus(userID)

        msg := "📊 **Your Channel Status:**\n\n"
        for i, channel := range config.RequiredChannels {
                joined := status.ChannelStatus[channel]
                emoji, text := "✅", "Joined"
                if !joined {
                        emoji, text = "❌", "Not Joined"
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
        m.Reply(msg)
        return nil
}

func handleStats(m *telegram.NewMessage) error {
        if !config.IsAdmin(m.SenderID()) {
                m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
                return nil
        }

        stats := db.GetBotStats()
        diff := time.Since(stats.LastUpdated)
        var timeAgo string
        switch {
        case diff.Hours() >= 24:
                timeAgo = fmt.Sprintf("%.0f days ago", diff.Hours()/24)
        case diff.Hours() >= 1:
                timeAgo = fmt.Sprintf("%.0f hours ago", diff.Hours())
        case diff.Minutes() >= 1:
                timeAgo = fmt.Sprintf("%.0f minutes ago", diff.Minutes())
        default:
                timeAgo = "just now"
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
        for _, channel := range config.RequiredChannels {
                display := config.ChannelDisplay[channel]
                if display == "" {
                        display = channel
                }
                count := stats.ChannelStats[channel]
                pct := float64(count) / float64(maxInt(stats.TotalUsers, 1)) * 100
                msg += fmt.Sprintf("• %s: `%d` (%.1f%%)\n", display, count, pct)
        }
        msg += fmt.Sprintf("\n⏰ **Last Updated:** %s\n", timeAgo)
        msg += fmt.Sprintf("📅 **Database:** `%s`", config.DBName)
        m.Reply(msg)
        return nil
}

func handleResetDB(m *telegram.NewMessage) error {
        if !config.IsAdmin(m.SenderID()) {
                m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
                return nil
        }
        m.Reply("⚠️ **Warning:** This will delete ALL user data!\n\nType `/confirm_reset` to proceed.")
        return nil
}

func handleConfirmReset(m *telegram.NewMessage) error {
        if !config.IsAdmin(m.SenderID()) {
                m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
                return nil
        }
        if err := db.ResetDatabase(); err != nil {
                m.Reply(fmt.Sprintf("❌ **Error resetting database:** %v", err))
                return nil
        }
        db.UpdateBotStats()
        m.Reply("✅ **Database has been reset successfully!**\nAll user data has been cleared.")
        return nil
}

func handleBroadcast(client *telegram.Client, m *telegram.NewMessage) error {
        if !config.IsAdmin(m.SenderID()) {
                m.Reply("❌ **Access Denied!**\nThis command is only available for administrators.")
                return nil
        }

        text := strings.TrimSpace(strings.TrimPrefix(m.MessageText(), "/broadcast"))
        if text == "" {
                m.Reply("❌ **Usage:** `/broadcast your message here`")
                return nil
        }

        m.Reply("📢 **Starting broadcast...**\nThis may take a while.")
        userIDs := db.GetAllUserIDs()
        total := len(userIDs)
        successful, failed := 0, 0

        for _, uid := range userIDs {
                _, err := client.SendMessage(uid, text)
                if err != nil {
                        log.Printf("Broadcast failed for user %d: %v", uid, err)
                        failed++
                } else {
                        successful++
                }
                time.Sleep(100 * time.Millisecond)
        }

        pct := float64(successful) / float64(maxInt(total, 1)) * 100
        summary := "📢 **Broadcast Completed!**\n\n"
        summary += fmt.Sprintf("• Total Users: `%d`\n", total)
        summary += fmt.Sprintf("• Successful: `%d`\n", successful)
        summary += fmt.Sprintf("• Failed: `%d`\n", failed)
        summary += fmt.Sprintf("• Success Rate: `%.1f%%`", pct)
        m.Reply(summary)
        return nil
}

func handleQuizBot(client *telegram.Client, m *telegram.NewMessage) error {
        tc := getTargetChat()
        if tc == 0 {
                return nil
        }
        localTarget := tc
        msg := m.Message

        // Handle "I am ready" button
        if msg.ReplyMarkup != nil {
                if kb, ok := msg.ReplyMarkup.(*telegram.ReplyInlineMarkup); ok {
                        for _, row := range kb.Rows {
                                for _, btn := range row.Buttons {
                                        if cb, ok := btn.(*telegram.KeyboardButtonCallback); ok {
                                                if strings.Contains(strings.ToLower(cb.Text), "i am ready") {
                                                        peer, err := client.ResolvePeer("QuizBot")
                                                        if err == nil {
                                                                client.MessagesGetBotCallbackAnswer(&telegram.MessagesGetBotCallbackAnswerParams{
                                                                        Peer:  peer,
                                                                        MsgID: msg.ID,
                                                                        Data:  cb.Data,
                                                                })
                                                                log.Println("Clicked 'I am ready' button")
                                                        }
                                                        return nil
                                                }
                                        }
                                }
                        }
                }
        }

        if msg.Media == nil {
                return nil
        }
        mediaPoll, ok := msg.Media.(*telegram.MessageMediaPoll)
        if !ok || !mediaPoll.Poll.Quiz {
                return nil
        }

        poll := mediaPoll.Poll
        if len(poll.Answers) == 0 {
                return nil
        }

        // Vote with a random option to unlock the correct answer
        randomAns, ok := poll.Answers[rand.Intn(len(poll.Answers))].(*telegram.PollAnswerObj)
        if !ok {
                return nil
        }

        quizBotPeer, err := client.ResolvePeer("QuizBot")
        if err != nil {
                log.Printf("Failed to resolve QuizBot: %v", err)
                return nil
        }

        _, err = client.MessagesSendVote(quizBotPeer, msg.ID, [][]byte{randomAns.Option})
        if err != nil {
                log.Printf("Error sending vote: %v", err)
                return nil
        }

        time.Sleep(2 * time.Second)

        // Poll for updated results with correct answer revealed
        var updatedMediaPoll *telegram.MessageMediaPoll
        for attempts := 0; attempts < 10; attempts++ {
                msgs, err := client.GetMessages("QuizBot", &telegram.SearchOption{
                        IDs: []int32{msg.ID},
                })
                if err == nil && len(msgs) > 0 {
                        if mp, ok := msgs[0].Message.Media.(*telegram.MessageMediaPoll); ok {
                                if mp.Results != nil && len(mp.Results.Results) > 0 {
                                        updatedMediaPoll = mp
                                        break
                                }
                        }
                }
                time.Sleep(1 * time.Second)
        }

        if updatedMediaPoll == nil {
                log.Println("No poll results after retries")
                client.SendMessage(localTarget, "Error: No poll results received")
                return nil
        }

        // Find correct answer index
        correctIndex := -1
        for _, res := range updatedMediaPoll.Results.Results {
                if res.Correct {
                        for idx, ans := range poll.Answers {
                                if ansObj, ok := ans.(*telegram.PollAnswerObj); ok {
                                        if string(ansObj.Option) == string(res.Option) {
                                                correctIndex = idx
                                                break
                                        }
                                }
                        }
                        break
                }
        }

        if correctIndex == -1 {
                log.Println("No correct option found")
                client.SendMessage(localTarget, "No correct option found in poll results")
                return nil
        }

        // Clean question text
        cleanedQuestion := cleanQuestion(poll.Question.Text)
        log.Printf("Original question: %s", poll.Question.Text)
        log.Printf("Cleaned question: %s", cleanedQuestion)

        // Build clean answer list
        var cleanedAnswers []string
        for _, ans := range poll.Answers {
                if ansObj, ok := ans.(*telegram.PollAnswerObj); ok {
                        originalText := ""
                        if ansObj.Text != nil {
                                originalText = ansObj.Text.Text
                        }
                        cleaned := cleanAnswerText(originalText)
                        if cleaned == "" {
                                cleaned = originalText
                        }
                        cleanedAnswers = append(cleanedAnswers, cleaned)
                }
        }

        if len(cleanedAnswers) == 0 {
                return nil
        }

        // Send the quiz poll (open first so Telegram accepts it)
        sentMsg, err := client.SendPoll(localTarget, cleanedQuestion, cleanedAnswers, &telegram.PollOptions{
                IsQuiz:         true,
                PublicVoters:   false,
                CorrectAnswers: []int{correctIndex},
        })
        if err != nil {
                log.Printf("Failed to send poll: %v", err)
                client.SendMessage(localTarget, fmt.Sprintf("Error sending poll: %v", err))
                return nil
        }
        log.Println("Quiz poll sent successfully")

        time.Sleep(1 * time.Second)

        // Close the poll immediately via MessagesEditMessage with a closed InputMediaPoll
        closedPollAnswers := make([]telegram.PollAnswer, 0, len(cleanedAnswers))
        for i, text := range cleanedAnswers {
                closedPollAnswers = append(closedPollAnswers, &telegram.PollAnswerObj{
                        Text:   &telegram.TextWithEntities{Text: text},
                        Option: []byte{byte(i)},
                })
        }

        // Retrieve the poll ID from the sent message
        var pollID int64
        if sentMsg != nil && sentMsg.Message != nil {
                if mp, ok := sentMsg.Message.Media.(*telegram.MessageMediaPoll); ok {
                        pollID = mp.Poll.ID
                }
        }

        closedMedia := &telegram.InputMediaPoll{
                Poll: &telegram.Poll{
                        ID:             pollID,
                        Question:       &telegram.TextWithEntities{Text: cleanedQuestion},
                        Answers:        closedPollAnswers,
                        PublicVoters:   false,
                        MultipleChoice: false,
                        Quiz:           true,
                        Closed:         true,
                },
                CorrectAnswers: []int32{int32(correctIndex)},
        }

        targetPeer, err := client.ResolvePeer(localTarget)
        if err == nil {
                _, editErr := client.MessagesEditMessage(&telegram.MessagesEditMessageParams{
                        Peer:  targetPeer,
                        ID:    sentMsg.ID,
                        Media: closedMedia,
                })
                if editErr != nil {
                        log.Printf("Failed to close poll: %v", editErr)
                } else {
                        log.Println("Poll closed successfully")
                }
        }

        return nil
}

func maxInt(a, b int) int {
        if a > b {
                return a
        }
        return b
}
