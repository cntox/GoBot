package src


import (
	"fmt"
	"log"
	"math/rand"
	"regexp"
	"strings"
	"sync"
	"time"

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

	hasJoinedAll, statusChanged := database.UpdateUserChannels(
		userID,
		channelsStatus,
		uInfo,
	)

	return hasJoinedAll, statusChanged, channelsStatus
}

func isUserInChannel(
	client *telegram.Client,
	userID int64,
	channel string,
) bool {

	_, err := client.GetChatMember(channel, userID)

	if err != nil {

		errStr := err.Error()

		if strings.Contains(errStr, "USER_NOT_PARTICIPANT") ||
			strings.Contains(errStr, "CHANNEL_PRIVATE") ||
			strings.Contains(errStr, "not a channel") ||
			strings.Contains(errStr, "peer is not") {

			return false
		}

		log.Printf(
			"Error checking channel %s for user %d: %v",
			channel,
			userID,
			err,
		)

		return false
	}

	return true
}

func buildWarningMessage(channelsStatus map[string]bool) string {

	msg := "**⚠️ Please join all required channels first!**\n\n"

	msg += "**Required Channels:**\n"

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

	msg += "\nAfter joining all channels, send /pn again."

	return msg
}

func buildWelcomeMessage() string {

	msg := "🎉 **Welcome! All channels verified successfully!**\n\n"

	msg += "✅ You can now use `/pn` command.\n\n"

	msg += "**Joined Channels:**\n"

	for i, channel := range config.RequiredChannels {

		display := config.ChannelDisplay[channel]

		if display == "" {
			display = channel
		}

		msg += fmt.Sprintf("%d. %s\n", i+1, display)
	}

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

func maxInt(a, b int) int {

	if a > b {
		return a
	}

	return b
}

func randomID() int64 {
	return rand.Int63()
}
