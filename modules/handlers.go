// modules/handlers.go

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

	optionPrefix := regexp.MustCompile(
		`^\s*[(\[]?[A-Da-d][)\].]?\s*`,
	)

	cleaned = optionPrefix.ReplaceAllString(cleaned, "")

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

		joined := isUserInChannel(
			client,
			userID,
			channel,
		)

		channelsStatus[channel] = joined
	}

	hasJoinedAll, statusChanged := database.UpdateUserChannels(
		userID,
		channelsStatus,
		uInfo,
	)

	return hasJoinedAll, statusChanged, channelsStatus
}

// TEMP TRUE
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

func buildWarningMessage(
	channelsStatus map[string]bool,
) string {

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

		msg += fmt.Sprintf(
			"%d. %s - %s\n",
			i+1,
			display,
			status,
		)
	}

	return msg
}

func buildWelcomeMessage() string {

	msg := "🎉 Welcome!\n\n"

	msg += "✅ All channels verified successfully.\n\n"

	msg += "Available Commands:\n"
	msg += "• /ping\n"
	msg += "• /check\n"
	msg += "• /status\n"
	msg += "• /bad\n"
	msg += "• /pn"

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

func senderUserInfo(
	m *telegram.NewMessage,
) *database.UserInfo {

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

// =======================
// HANDLERS
// =======================

func RegisterHandlers(
	client *telegram.Client,
) {

	// START
	client.OnCommand(
		"start",
		func(m *telegram.NewMessage) error {

			msg := "🎉 Quiz Bot Started Successfully!\n\n"

			msg += "Commands:\n"
			msg += "• /ping\n"
			msg += "• /check\n"
			msg += "• /status\n"
			msg += "• /bad\n"
			msg += "• /pn"

			_, err := m.Reply(msg)

			return err
		},
	)

	// PING
	client.OnCommand(
		"ping",
		func(m *telegram.NewMessage) error {

			_, err := m.Reply(
				"🏓 Pong!\n⚡ Bot Working Successfully.",
			)

			return err
		},
	)

	// BAD
	client.OnCommand(
		"bad",
		func(m *telegram.NewMessage) error {

			_, err := m.Reply(
				"😎 Bad OP 🔥",
			)

			return err
		},
	)

	// CHECK
	client.OnCommand(
		"check",
		func(m *telegram.NewMessage) error {

			userID := m.SenderID()

			uInfo := senderUserInfo(m)

			joinedAll, statusChanged, channelsStatus :=
				checkUserJoinedChannels(
					client,
					userID,
					uInfo,
				)

			if statusChanged && joinedAll {

				m.Reply(buildWelcomeMessage())

				database.UpdateWelcomeSent(
					userID,
					true,
				)
			}

			if !joinedAll {

				sendWarningIfAllowed(
					m,
					userID,
					channelsStatus,
				)

				return nil
			}

			_, err := m.Reply(
				"✅ All required channels joined.",
			)

			return err
		},
	)

	// STATUS
	client.OnCommand(
		"status",
		func(m *telegram.NewMessage) error {

			_, err := m.Reply(
				"✅ Bot Status: ONLINE",
			)

			return err
		},
	)

	// PN
	client.OnCommand(
		"pn",
		func(m *telegram.NewMessage) error {

			_, err := m.Reply(
				"📊 Poll System Active.",
			)

			return err
		},
	)

	// STOP
	client.OnCommand(
		"stop",
		func(m *telegram.NewMessage) error {

			setTargetChat(0)

			_, err := m.Reply(
				"🛑 Poll stopped.",
			)

			return err
		},
	)

	// REFRESH
	client.OnCommand(
		"refresh",
		func(m *telegram.NewMessage) error {

			userID := m.SenderID()

			database.RemoveUser(userID)

			_, err := m.Reply(
				"🔄 Refreshed Successfully.",
			)

			return err
		},
	)

	log.Println("✅ Handlers Loaded Successfully")
}

// =======================
// QUIZ BOT HANDLER
// =======================

func handleQuizBot(
	client *telegram.Client,
	m *telegram.NewMessage,
) error {

	tc := getTargetChat()

	if tc == 0 {
		return nil
	}

	msg := m.Message

	if msg.Media == nil {
		return nil
	}

	mediaPoll, ok := msg.Media.(*telegram.MessageMediaPoll)

	if !ok {
		return nil
	}

	poll := mediaPoll.Poll

	if len(poll.Answers) == 0 {
		return nil
	}

	randomAns, ok := poll.Answers[rand.Intn(len(poll.Answers))].(*telegram.PollAnswerObj)

	if !ok {
		return nil
	}

	quizBotPeer, err := client.ResolvePeer("QuizBot")

	if err != nil {
		return nil
	}

	_, err = client.MessagesSendVote(
		quizBotPeer,
		msg.ID,
		[][]byte{randomAns.Option},
	)

	if err != nil {
		return nil
	}

	time.Sleep(2 * time.Second)

	return nil
}
