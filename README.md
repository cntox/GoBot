# Quiz Poll Bot (Go + Gogram)

A Telegram userbot that intercepts quiz polls from QuizBot, strips emojis/numbering, and forwards them as closed quiz polls — requiring users to join specific channels first.

## Setup

### 1. Generate a Gogram String Session

```bash
cd bot
go run generate_session/main.go
```

Follow the prompts: enter your phone number and the OTP Telegram sends. Copy the printed session string.

### 2. Set the Session String

**Option A — Environment variable (recommended):**
```bash
export SESSION_STRING='your_session_string_here'
```

**Option B — File:**
```bash
echo 'your_session_string_here' > bot/session.txt
```

### 3. Run the Bot

```bash
cd bot
go run main.go
```

Or build and run:
```bash
cd bot
go build -o bin/bot .
./bin/bot
```

## Configuration

Edit `config/config.go` to change:
- `APIId` / `APIHash` — your Telegram app credentials
- `AdminIDs` — your Telegram user IDs (for admin commands)
- `RequiredChannels` — channels users must join
- `ChannelDisplay` — markdown link display for each channel

## Commands

| Command | Description | Who |
|---------|-------------|-----|
| `/start` | Show help message | Everyone |
| `/pn` | Start quiz (reply to a quiz share message) | Members |
| `/again` | Retry the current quiz | Members |
| `/stop` | Stop the current quiz session | Members |
| `/check` | Check channel membership | Everyone |
| `/status` | Detailed channel status | Everyone |
| `/refresh` | Force-refresh channel status | Everyone |
| `/stats` | Bot statistics | Admins |
| `/broadcast` | Broadcast message to all users | Admins |
| `/resetdb` | Reset all user data | Admins |

## Dependencies

- [gogram](https://github.com/amarnathcjd/gogram) — Telegram MTProto client for Go
- [go-sqlite3](https://github.com/mattn/go-sqlite3) — SQLite driver for Go
