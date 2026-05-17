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

	// SQLite supports one writer
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

			slog.Error("Failed to create table",
				"error", err,
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

// UserInfo Struct
type UserInfo struct {
	Username  string
	FirstName string
	LastName  string
}

// Update User Channels
func UpdateUserChannels(
	userID int64,
	channelsStatus map[string]bool,
	userInfo *UserInfo,
) (bool, bool) {

	var previousJoinedAll bool

	row := DB.QueryRow(
		`SELECT joined_all_channels FROM users WHERE user_id = ?`,
		userID,
	)

	row.Scan(&previousJoinedAll)

	now := time.Now()

	// Insert / Update User
	if userInfo != nil {

		var count int

		DB.QueryRow(
			`SELECT COUNT(*) FROM users WHERE user_id = ?`,
			userID,
		).Scan(&count)

		if count > 0 {

			DB.Exec(
				`UPDATE users
				SET username=COALESCE(?,username),
					first_name=COALESCE(?,first_name),
					last_name=COALESCE(?,last_name),
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

	} else {

		DB.Exec(
			`INSERT OR IGNORE INTO users (user_id, last_check)
			VALUES (?,?)`,
			userID,
			now,
		)

		DB.Exec(
			`UPDATE users SET last_check=? WHERE user_id=?`,
			now,
			userID,
		)
	}

	// Update Channel Joins
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

	// Check Joined All
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

// Helper
func nullStr(s string) interface{} {

	if s == "" {
		return nil
	}

	return s
}
