// database/database.go

package database

import (
	"database/sql"
	"log/slog"
	"time"

	"quiz/config"

	_ "github.com/mattn/go-sqlite3"
)

var DB *sql.DB

// Init Database
func Init() {

	var err error

	DB, err = sql.Open("sqlite3", config.DBName)

	if err != nil {
		slog.Error("Failed to open database", "error", err)
		panic(err)
	}

	DB.SetMaxOpenConns(1)

	createTables()

	slog.Info("Database initialized successfully")
}

// Create Tables
func createTables() {

	stmts := []string{

		`CREATE TABLE IF NOT EXISTS users (
			user_id INTEGER PRIMARY KEY,
			username TEXT,
			first_name TEXT,
			last_name TEXT,
			joined_all_channels BOOLEAN DEFAULT 0,
			last_check TIMESTAMP,
			last_warning TIMESTAMP,
			welcome_sent BOOLEAN DEFAULT 0,
			created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS channel_joins (
			user_id INTEGER,
			channel_id TEXT,
			joined BOOLEAN DEFAULT 0,
			last_checked TIMESTAMP,
			PRIMARY KEY (user_id, channel_id)
		)`,

		`CREATE TABLE IF NOT EXISTS user_actions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			user_id INTEGER,
			action TEXT,
			timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,

		`CREATE TABLE IF NOT EXISTS bot_stats (
			id INTEGER PRIMARY KEY,
			total_users INTEGER DEFAULT 0,
			active_users INTEGER DEFAULT 0,
			total_polls INTEGER DEFAULT 0,
			last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		)`,
	}

	for _, stmt := range stmts {

		if _, err := DB.Exec(stmt); err != nil {

			slog.Error(
				"Failed to create table",
				"error",
				err,
			)

			panic(err)
		}
	}

	// Migrations
	migrations := []string{
		`ALTER TABLE users ADD COLUMN username TEXT`,
		`ALTER TABLE users ADD COLUMN first_name TEXT`,
		`ALTER TABLE users ADD COLUMN last_name TEXT`,
		`ALTER TABLE users ADD COLUMN last_warning TIMESTAMP`,
		`ALTER TABLE users ADD COLUMN welcome_sent BOOLEAN DEFAULT 0`,
	}

	for _, m := range migrations {
		DB.Exec(m)
	}
}

// UserInfo
type UserInfo struct {
	Username  string
	FirstName string
	LastName  string
}

// User Status
type UserStatus struct {
	JoinedAll     bool
	WelcomeSent   bool
	ChannelStatus map[string]bool
}

// Bot Stats
type BotStats struct {
	TotalUsers      int
	ActiveUsers     int
	TotalPolls      int
	UsersWithAccess int
	UsersWelcomed   int
	NewUsers24h     int
	ChannelStats    map[string]int
	LastUpdated     time.Time
}

// Update User Channels
func UpdateUserChannels(
	userID int64,
	channelsStatus map[string]bool,
	userInfo *UserInfo,
) (bool, bool) {

	var previousJoinedAll bool

	row := DB.QueryRow(
		`SELECT joined_all_channels FROM users WHERE user_id=?`,
		userID,
	)

	row.Scan(&previousJoinedAll)

	now := time.Now()

	if userInfo != nil {

		var count int

		DB.QueryRow(
			`SELECT COUNT(*) FROM users WHERE user_id=?`,
			userID,
		).Scan(&count)

		if count > 0 {

			DB.Exec(
				`UPDATE users
				SET username=?,
				first_name=?,
				last_name=?,
				last_check=?
				WHERE user_id=?`,
				nullStr(userInfo.Username),
				nullStr(userInfo.FirstName),
				nullStr(userInfo.LastName),
				now,
				userID,
			)

		} else {

			DB.Exec(
				`INSERT INTO users
				(user_id, username, first_name, last_name, last_check)
				VALUES (?,?,?,?,?)`,
				userID,
				nullStr(userInfo.Username),
				nullStr(userInfo.FirstName),
				nullStr(userInfo.LastName),
				now,
			)
		}
	}

	for channelID, joined := range channelsStatus {

		joinedInt := 0

		if joined {
			joinedInt = 1
		}

		DB.Exec(
			`INSERT OR REPLACE INTO channel_joins
			(user_id, channel_id, joined, last_checked)
			VALUES (?,?,?,?)`,
			userID,
			channelID,
			joinedInt,
			now,
		)
	}

	var joinedCount int

	DB.QueryRow(
		`SELECT COUNT(*) FROM channel_joins
		WHERE user_id=? AND joined=1`,
		userID,
	).Scan(&joinedCount)

	hasJoinedAll := joinedCount >= len(config.RequiredChannels)

	joinedAllInt := 0

	if hasJoinedAll {
		joinedAllInt = 1
	}

	DB.Exec(
		`UPDATE users
		SET joined_all_channels=?, last_check=?
		WHERE user_id=?`,
		joinedAllInt,
		now,
		userID,
	)

	action := "channel_check_missing"

	if hasJoinedAll {
		action = "channel_check_success"
	}

	DB.Exec(
		`INSERT INTO user_actions (user_id, action)
		VALUES (?,?)`,
		userID,
		action,
	)

	UpdateBotStats()

	statusChanged := previousJoinedAll != hasJoinedAll

	return hasJoinedAll, statusChanged
}

// Get User Status
func GetUserStatus(userID int64) UserStatus {

	status := UserStatus{
		ChannelStatus: make(map[string]bool),
	}

	rows, err := DB.Query(`
		SELECT u.joined_all_channels,
		u.welcome_sent,
		c.channel_id,
		c.joined
		FROM users u
		LEFT JOIN channel_joins c
		ON u.user_id = c.user_id
		WHERE u.user_id = ?`,
		userID,
	)

	if err != nil {
		return status
	}

	defer rows.Close()

	first := true

	for rows.Next() {

		var joinedAll bool
		var welcomeSent bool
		var channelID sql.NullString
		var joined sql.NullBool

		rows.Scan(
			&joinedAll,
			&welcomeSent,
			&channelID,
			&joined,
		)

		if first {

			status.JoinedAll = joinedAll
			status.WelcomeSent = welcomeSent

			first = false
		}

		if channelID.Valid {
			status.ChannelStatus[channelID.String] = joined.Bool
		}
	}

	return status
}

// Update Welcome Sent
func UpdateWelcomeSent(userID int64, sent bool) {

	v := 0

	if sent {
		v = 1
	}

	DB.Exec(
		`UPDATE users SET welcome_sent=? WHERE user_id=?`,
		v,
		userID,
	)
}

// Remove User
func RemoveUser(userID int64) {

	DB.Exec(`DELETE FROM channel_joins WHERE user_id=?`, userID)

	DB.Exec(`DELETE FROM user_actions WHERE user_id=?`, userID)

	DB.Exec(`DELETE FROM users WHERE user_id=?`, userID)
}

// Reset Database
func ResetDatabase() error {

	tables := []string{
		"channel_joins",
		"user_actions",
		"users",
		"bot_stats",
	}

	for _, t := range tables {

		if _, err := DB.Exec(
			`DELETE FROM ` + t,
		); err != nil {

			return err
		}
	}

	return nil
}

// Update Bot Stats
func UpdateBotStats() {

	var total, active, polls int

	DB.QueryRow(
		`SELECT COUNT(*) FROM users`,
	).Scan(&total)

	sevenDaysAgo := time.Now().Add(-7 * 24 * time.Hour)

	DB.QueryRow(
		`SELECT COUNT(*) FROM users
		WHERE last_check >= ?`,
		sevenDaysAgo,
	).Scan(&active)

	DB.QueryRow(
		`SELECT COUNT(*) FROM user_actions
		WHERE action LIKE '%success%'`,
	).Scan(&polls)

	DB.Exec(
		`INSERT OR REPLACE INTO bot_stats
		(id, total_users, active_users, total_polls, last_updated)
		VALUES (1,?,?,?,?)`,
		total,
		active,
		polls,
		time.Now(),
	)
}

// Get Bot Stats
func GetBotStats() *BotStats {

	s := &BotStats{
		ChannelStats: make(map[string]int),
	}

	DB.QueryRow(
		`SELECT total_users,
		active_users,
		total_polls,
		last_updated
		FROM bot_stats WHERE id=1`,
	).Scan(
		&s.TotalUsers,
		&s.ActiveUsers,
		&s.TotalPolls,
		&s.LastUpdated,
	)

	DB.QueryRow(
		`SELECT COUNT(*) FROM users
		WHERE joined_all_channels=1`,
	).Scan(&s.UsersWithAccess)

	DB.QueryRow(
		`SELECT COUNT(*) FROM users
		WHERE welcome_sent=1`,
	).Scan(&s.UsersWelcomed)

	oneDayAgo := time.Now().Add(-24 * time.Hour)

	DB.QueryRow(
		`SELECT COUNT(*) FROM users
		WHERE created_at >= ?`,
		oneDayAgo,
	).Scan(&s.NewUsers24h)

	for _, ch := range config.RequiredChannels {

		var count int

		DB.QueryRow(
			`SELECT COUNT(*) FROM channel_joins
			WHERE channel_id=? AND joined=1`,
			ch,
		).Scan(&count)

		s.ChannelStats[ch] = count
	}

	return s
}

// Should Send Warning
func ShouldSendWarning(userID int64) bool {

	var lastWarning sql.NullString

	DB.QueryRow(
		`SELECT last_warning FROM users WHERE user_id=?`,
		userID,
	).Scan(&lastWarning)

	if !lastWarning.Valid || lastWarning.String == "" {
		return true
	}

	t, err := time.Parse(
		"2006-01-02 15:04:05.999999999-07:00",
		lastWarning.String,
	)

	if err != nil {

		t, err = time.Parse(
			time.RFC3339Nano,
			lastWarning.String,
		)
	}

	if err != nil {
		return true
	}

	return time.Since(t) > 30*time.Second
}

// Update Last Warning
func UpdateLastWarning(userID int64) {

	DB.Exec(
		`UPDATE users
		SET last_warning=?
		WHERE user_id=?`,
		time.Now(),
		userID,
	)
}

// Helper
func nullStr(s string) interface{} {

	if s == "" {
		return nil
	}

	return s
}
