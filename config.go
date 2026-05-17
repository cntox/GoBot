package config

// Replace with your actual values
const (
	APIId   = 12380656
	APIHash = "d927c13beaaf5110f25c505b7c071273"
	DBName  = "poll_bot.db"
)

// ADMIN_IDS — add your Telegram user IDs here
var AdminIDs = []int64{6644859358, 8451305181, 7183060880}

// REQUIRED_CHANNELS — channels users must join
var RequiredChannels = []string{
	"@exampurrs",
	"@exampurss_official",
	"@sarakari_result",
	"@FONT_CHANNEL_01",
}

// ChannelDisplay — markdown display names for channels
var ChannelDisplay = map[string]string{
	"@exampurrs":          "[ᴇxᴀᴍᴘᴜʀ](https://t.me/exampurrs)",
	"@exampurss_official": "[ᴇxᴀᴍᴘᴜʀ Qᴜɪᴢ](https://t.me/exampurss_official)",
	"@sarakari_result":    "[ꜱᴀʀᴋᴀʀɪ ʀᴇꜱᴜʟᴛ](https://t.me/sarakari_result)",
	"@FONT_CHANNEL_01":    "[ꜱᴛʏʟɪꜱʜ ꜰᴏɴᴛ](https://t.me/FONT_CHANNEL_01)",
}

func IsAdmin(userID int64) bool {
	for _, id := range AdminIDs {
		if id == userID {
			return true
		}
	}
	return false
}
