from telethon import TelegramClient, events
from telethon.sessions import StringSession
from telethon.tl import types, functions
from telethon.tl.functions.channels import GetParticipantRequest
from telethon.errors import UserNotParticipantError
import re
import time
import random
import asyncio
import logging
import sqlite3
from datetime import datetime, timedelta
import os

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Replace with your values
api_id = '12380656'
api_hash = 'd927c13beaaf5110f25c505b7c071273'
session_string = ''

# Channel IDs that users must join with markdown formatting
REQUIRED_CHANNELS = [
    '@exampurrs',  # Replace with your channel usernames
    '@exampurss_official',
    '@sarakari_result',
    '@FONT_CHANNEL_01'
]

# Create display versions with markdown links
CHANNEL_DISPLAY = {
    '@exampurrs': '[ᴇxᴀᴍᴘᴜʀ](https://t.me/exampurrs)',
    '@exampurss_official': '[ᴇxᴀᴍᴘᴜʀ Qᴜɪᴢ](https://t.me/exampurss_official)',
    '@sarakari_result': '[ꜱᴀʀᴋᴀʀɪ ʀᴇꜱᴜʟᴛ](https://t.me/sarakari_result)',
    '@FONT_CHANNEL_01': '[ꜱᴛʏʟɪꜱʜ ꜰᴏɴᴛ](https://t.me/FONT_CHANNEL_01)'
}

# Database setup
DB_NAME = 'poll_bot.db'

# Admin user IDs (replace with your admin user IDs)
ADMIN_IDS = [6644859358, 8451305181, 7183060880]  # Add your Telegram user IDs here

def init_database():
    """Initialize SQLite database with schema migration support"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    # Check if database exists and get current schema
    cursor.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='users'")
    users_table_exists = cursor.fetchone() is not None
    
    if not users_table_exists:
        # Create fresh tables if they don't exist
        cursor.execute('''
        CREATE TABLE IF NOT EXISTS users (
            user_id INTEGER PRIMARY KEY,
            username TEXT,
            first_name TEXT,
            last_name TEXT,
            joined_all_channels BOOLEAN DEFAULT 0,
            last_check TIMESTAMP,
            last_warning TIMESTAMP,
            welcome_sent BOOLEAN DEFAULT 0,
            created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )''')
    else:
        # Check and add missing columns to users table
        cursor.execute("PRAGMA table_info(users)")
        columns = [column[1] for column in cursor.fetchall()]
        
        if 'username' not in columns:
            cursor.execute("ALTER TABLE users ADD COLUMN username TEXT")
        if 'first_name' not in columns:
            cursor.execute("ALTER TABLE users ADD COLUMN first_name TEXT")
        if 'last_name' not in columns:
            cursor.execute("ALTER TABLE users ADD COLUMN last_name TEXT")
        if 'last_warning' not in columns:
            cursor.execute("ALTER TABLE users ADD COLUMN last_warning TIMESTAMP")
        if 'welcome_sent' not in columns:
            cursor.execute("ALTER TABLE users ADD COLUMN welcome_sent BOOLEAN DEFAULT 0")
    
    # Create or update channel_joins table
    cursor.execute('''
    CREATE TABLE IF NOT EXISTS channel_joins (
        user_id INTEGER,
        channel_id TEXT,
        joined BOOLEAN DEFAULT 0,
        last_checked TIMESTAMP,
        PRIMARY KEY (user_id, channel_id)
    )''')
    
    # Create or update user_actions table
    cursor.execute('''
    CREATE TABLE IF NOT EXISTS user_actions (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        user_id INTEGER,
        action TEXT,
        timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )''')
    
    # Create table for bot statistics
    cursor.execute('''
    CREATE TABLE IF NOT EXISTS bot_stats (
        id INTEGER PRIMARY KEY AUTOINCREMENT,
        total_users INTEGER DEFAULT 0,
        active_users INTEGER DEFAULT 0,
        total_polls INTEGER DEFAULT 0,
        last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
    )''')
    
    conn.commit()
    conn.close()
    logging.info("Database initialized successfully")

def is_admin(user_id):
    """Check if user is admin"""
    return user_id in ADMIN_IDS

def update_user_channels(user_id, channels_status, user_info=None):
    """Update user's channel join status in database"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    try:
        # Get previous status
        cursor.execute('SELECT joined_all_channels FROM users WHERE user_id = ?', (user_id,))
        previous_result = cursor.fetchone()
        previous_joined_all = previous_result[0] if previous_result else False
        
        # Insert or update user with user info
        if user_info:
            # Check if user exists
            cursor.execute('SELECT COUNT(*) FROM users WHERE user_id = ?', (user_id,))
            user_exists = cursor.fetchone()[0] > 0
            
            if user_exists:
                # Update existing user
                cursor.execute('''
                UPDATE users SET 
                    username = COALESCE(?, username),
                    first_name = COALESCE(?, first_name),
                    last_name = COALESCE(?, last_name),
                    last_check = ?
                WHERE user_id = ?
                ''', (user_info.get('username'), user_info.get('first_name'), 
                      user_info.get('last_name'), datetime.now(), user_id))
            else:
                # Insert new user
                cursor.execute('''
                INSERT INTO users 
                (user_id, username, first_name, last_name, last_check) 
                VALUES (?, ?, ?, ?, ?)
                ''', (user_id, user_info.get('username'), user_info.get('first_name'), 
                      user_info.get('last_name'), datetime.now()))
        else:
            # Update only last_check for existing user
            cursor.execute('''
            INSERT OR IGNORE INTO users (user_id, last_check) 
            VALUES (?, ?)
            ''', (user_id, datetime.now()))
            cursor.execute('''
            UPDATE users SET last_check = ? WHERE user_id = ?
            ''', (datetime.now(), user_id))
        
        # Update individual channel status
        for channel_id, joined in channels_status.items():
            cursor.execute('''
            INSERT OR REPLACE INTO channel_joins (user_id, channel_id, joined, last_checked)
            VALUES (?, ?, ?, ?)
            ''', (user_id, channel_id, 1 if joined else 0, datetime.now()))
        
        # Check if user has joined all channels
        cursor.execute('''
        SELECT COUNT(*) FROM channel_joins 
        WHERE user_id = ? AND joined = 1
        ''', (user_id,))
        
        joined_count = cursor.fetchone()[0]
        has_joined_all = joined_count >= len(REQUIRED_CHANNELS)
        
        # Update user's joined_all_channels status
        cursor.execute('''
        UPDATE users SET joined_all_channels = ?, last_check = ?
        WHERE user_id = ?
        ''', (1 if has_joined_all else 0, datetime.now(), user_id))
        
        # Log the action
        action = f"channel_check_{'all_joined' if has_joined_all else 'missing_channels'}"
        cursor.execute('''
        INSERT INTO user_actions (user_id, action) VALUES (?, ?)
        ''', (user_id, action))
        
        # Update bot stats
        update_bot_stats()
        
        conn.commit()
        
        # Return if status changed
        status_changed = previous_joined_all != has_joined_all
        return has_joined_all, status_changed
        
    except Exception as e:
        logging.error(f"Error updating user channels: {e}")
        conn.rollback()
        return False, False
    finally:
        conn.close()

def update_bot_stats():
    """Update bot statistics in database"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    try:
        # Get total users
        cursor.execute('SELECT COUNT(*) FROM users')
        total_users = cursor.fetchone()[0]
        
        # Get active users (users who checked in last 7 days)
        seven_days_ago = datetime.now() - timedelta(days=7)
        cursor.execute('SELECT COUNT(*) FROM users WHERE last_check >= ?', (seven_days_ago,))
        active_users = cursor.fetchone()[0]
        
        # Get total polls created (count of pn commands)
        cursor.execute("SELECT COUNT(*) FROM user_actions WHERE action LIKE '%channel_check_all_joined%'")
        total_polls = cursor.fetchone()[0]
        
        # Update or insert stats
        cursor.execute('''
        INSERT OR REPLACE INTO bot_stats (id, total_users, active_users, total_polls, last_updated)
        VALUES (1, ?, ?, ?, ?)
        ''', (total_users, active_users, total_polls, datetime.now()))
        
        conn.commit()
        
    except Exception as e:
        logging.error(f"Error updating bot stats: {e}")
        conn.rollback()
    finally:
        conn.close()

def get_bot_stats():
    """Get bot statistics from database"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    try:
        # Get stats from bot_stats table
        cursor.execute('SELECT total_users, active_users, total_polls, last_updated FROM bot_stats WHERE id = 1')
        stats_result = cursor.fetchone()
        
        if stats_result:
            total_users, active_users, total_polls, last_updated = stats_result
        else:
            total_users = active_users = total_polls = 0
            last_updated = datetime.now()
        
        # Get additional statistics
        cursor.execute('SELECT COUNT(*) FROM users WHERE joined_all_channels = 1')
        users_with_access = cursor.fetchone()[0]
        
        cursor.execute('SELECT COUNT(*) FROM users WHERE welcome_sent = 1')
        users_welcomed = cursor.fetchone()[0]
        
        # Get recent users (last 24 hours)
        one_day_ago = datetime.now() - timedelta(days=1)
        cursor.execute('SELECT COUNT(*) FROM users WHERE created_at >= ?', (one_day_ago,))
        new_users_24h = cursor.fetchone()[0]
        
        # Get channel join statistics
        channel_stats = {}
        for channel in REQUIRED_CHANNELS:
            cursor.execute('SELECT COUNT(*) FROM channel_joins WHERE channel_id = ? AND joined = 1', (channel,))
            joined_count = cursor.fetchone()[0]
            channel_stats[channel] = joined_count
        
        conn.close()
        
        return {
            'total_users': total_users,
            'active_users': active_users,
            'total_polls': total_polls,
            'users_with_access': users_with_access,
            'users_welcomed': users_welcomed,
            'new_users_24h': new_users_24h,
            'channel_stats': channel_stats,
            'last_updated': last_updated
        }
        
    except Exception as e:
        logging.error(f"Error getting bot stats: {e}")
        conn.close()
        return None

def get_user_status(user_id):
    """Get user's channel join status from database"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    try:
        cursor.execute('''
        SELECT u.joined_all_channels, u.welcome_sent, c.channel_id, c.joined
        FROM users u
        LEFT JOIN channel_joins c ON u.user_id = c.user_id
        WHERE u.user_id = ?
        ''', (user_id,))
        
        results = cursor.fetchall()
        
        if not results:
            return False, False, {}
        
        joined_all = bool(results[0][0])
        welcome_sent = bool(results[0][1])
        channels_status = {}
        
        for result in results:
            if result[2]:  # channel_id
                channels_status[result[2]] = bool(result[3])
        
        return joined_all, welcome_sent, channels_status
        
    except Exception as e:
        logging.error(f"Error getting user status: {e}")
        return False, False, {}
    finally:
        conn.close()

def update_welcome_sent(user_id, sent=True):
    """Update welcome sent status"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    try:
        cursor.execute('UPDATE users SET welcome_sent = ? WHERE user_id = ?', (1 if sent else 0, user_id))
        conn.commit()
    except Exception as e:
        logging.error(f"Error updating welcome sent: {e}")
        conn.rollback()
    finally:
        conn.close()

def update_last_warning(user_id):
    """Update last warning timestamp"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    try:
        cursor.execute('UPDATE users SET last_warning = ? WHERE user_id = ?', (datetime.now(), user_id))
        conn.commit()
    except Exception as e:
        logging.error(f"Error updating last warning: {e}")
        conn.rollback()
    finally:
        conn.close()

def should_send_warning(user_id):
    """Check if we should send warning (rate limiting)"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    try:
        cursor.execute('SELECT last_warning FROM users WHERE user_id = ?', (user_id,))
        result = cursor.fetchone()
        
        if not result or not result[0]:
            return True
        
        last_warning = datetime.fromisoformat(result[0]) if isinstance(result[0], str) else result[0]
        # Only send warning if last warning was more than 30 seconds ago
        return datetime.now() - last_warning > timedelta(seconds=30)
        
    except Exception as e:
        logging.error(f"Error checking warning time: {e}")
        return True
    finally:
        conn.close()

def remove_user(user_id):
    """Remove user from database"""
    conn = sqlite3.connect(DB_NAME)
    cursor = conn.cursor()
    
    try:
        cursor.execute('DELETE FROM channel_joins WHERE user_id = ?', (user_id,))
        cursor.execute('DELETE FROM user_actions WHERE user_id = ?', (user_id,))
        cursor.execute('DELETE FROM users WHERE user_id = ?', (user_id,))
        
        conn.commit()
        logging.info(f"Removed user {user_id} from database")
    except Exception as e:
        logging.error(f"Error removing user: {e}")
        conn.rollback()
    finally:
        conn.close()

# Initialize database
init_database()

client = TelegramClient(StringSession(session_string), api_id, api_hash)

target_chat = None  # Global to store target chat for simplicity

# Emoji removal pattern
emoji_pattern = re.compile(
    "["
    "\U0001F600-\U0001F64F"  # emoticons
    "\U0001F300-\U0001F5FF"  # symbols & pictographs
    "\U0001F680-\U0001F6FF"  # transport & map symbols
    "\U0001F1E0-\U0001F1FF"  # flags (iOS)
    "\U00002500-\U00002BEF"  # chinese char
    "\U00002702-\U000027B0"
    "\U00002712-\U00002716"
    "\U000024C2-\U0001F251"
    "\U0001f926-\U0001f937"
    "\U00010000-\U0010ffff"
    "\u2640-\u2642"
    "\u2600-\u2B55"
    "\u200d"
    "\u23cf"
    "\u23e9"
    "\u231a"
    "\ufe0f"  # dingbats
    "\u3030"
    "]+",
    flags=re.UNICODE
)

def remove_emojis_preserve_entities(twe: types.TextWithEntities) -> types.TextWithEntities:
    original_text = twe.text
    entities = twe.entities or []
    emoji_matches = list(emoji_pattern.finditer(original_text))
    new_text = ''
    pos = 0
    for match in emoji_matches:
        new_text += original_text[pos:match.start()]
        pos = match.end()
    new_text += original_text[pos:]

    new_entities = []
    for entity in entities:
        old_start = entity.offset
        old_end = old_start + entity.length
        removed_length_before = sum(m.end() - m.start() for m in emoji_matches if m.end() <= old_start)
        removed_length_within = sum(m.end() - m.start() for m in emoji_matches if old_start < m.end() <= old_end)
        new_start = old_start - removed_length_before
        new_length = entity.length - removed_length_within
        if new_length > 0:
            new_entity = type(entity)(
                offset=new_start,
                length=new_length,
                **{k: v for k, v in entity.__dict__.items() if k not in ['offset', 'length']}
            )
            new_entities.append(new_entity)

    return types.TextWithEntities(text=new_text, entities=new_entities)

async def check_user_joined_channels(user_id, sender=None):
    """Check if user has joined all required channels"""
    channels_status = {}
    
    # Get user info from sender if available
    user_info = {}
    if sender:
        user_info = {
            'username': sender.username,
            'first_name': sender.first_name,
            'last_name': sender.last_name
        }
    
    for channel in REQUIRED_CHANNELS:
        try:
            # Try to get participant info
            await client(GetParticipantRequest(
                channel=channel,
                participant=user_id
            ))
            channels_status[channel] = True
        except UserNotParticipantError:
            channels_status[channel] = False
        except Exception as e:
            logging.error(f"Error checking channel {channel} for user {user_id}: {e}")
            channels_status[channel] = False
    
    # Update database with current status
    has_joined_all, status_changed = update_user_channels(user_id, channels_status, user_info)
    
    return has_joined_all, status_changed, channels_status

async def send_channel_warning(event, missing_channels):
    """Send warning message about missing channels"""
    if not should_send_warning(event.sender_id):
        return
    
    warning_message = "**⚠️ 𝐏𝐡𝐥𝐞 𝐬𝐚𝐫𝐞 𝐠𝐫𝐨𝐮𝐩 𝐨𝐫 𝐜𝐡𝐚𝐧𝐧𝐞𝐥 𝐣𝐨𝐢𝐧 𝐤𝐫 𝐧𝐡𝐢 𝐦 𝐧𝐡𝐢 𝐛𝐧𝐚 𝐫𝐡𝐚 𝐭𝐮𝐦𝐡𝐚𝐫𝐞 𝐤𝐨𝐢 𝐩𝐨𝐥𝐥𝐬 🙂😏!**\n\n"
    warning_message += "**Please join these channels:**\n"
    
    for i, channel in enumerate(REQUIRED_CHANNELS, 1):
        status = "✅ Joined" if channel not in missing_channels else "❌ Not Joined"
        channel_display = CHANNEL_DISPLAY.get(channel, channel)
        warning_message += f"{i}. {channel_display} - {status}\n"
    
    warning_message += "\nAfter joining all channels, send /pn command again."
    
    try:
        await event.reply(warning_message, parse_mode='md')
        update_last_warning(event.sender_id)
    except Exception as e:
        logging.error(f"Error sending warning message: {e}")

async def send_welcome_message(event):
    """Send welcome message when user joins all channels"""
    welcome_message = "🎉 **Welcome! Thank you for joining all required channels!**\n\n"
    welcome_message += "✅ **You can now use /pn command to create polls.**\n\n"
    welcome_message += "**Channels you joined:**\n"
    
    for i, channel in enumerate(REQUIRED_CHANNELS, 1):
        channel_display = CHANNEL_DISPLAY.get(channel, channel)
        welcome_message += f"{i}. {channel_display}\n"
    
    welcome_message += "\n**How to use:**\n"
    welcome_message += "1. Reply to a quiz share message with `/pn`\n"
    welcome_message += "2. The bot will forward the quiz as a closed poll\n\n"
    welcome_message += "**Other commands:**\n"
    welcome_message += "• `/again` - Try the quiz again\n"
    welcome_message += "• `/stop` - Stop the current quiz session\n"
    welcome_message += "• `/check` - Check your channel status\n"
    welcome_message += "• `/refresh` - Force refresh your status"
    
    try:
        await event.reply(welcome_message, parse_mode='md')
        update_welcome_sent(event.sender_id)
    except Exception as e:
        logging.error(f"Error sending welcome message: {e}")

@client.on(events.NewMessage(pattern='/pn'))
async def pn_handler(event):
    global target_chat
    
    try:
        # Always verify channel membership when using /pn command
        user_id = event.sender_id
        
        # Check channel membership
        joined_all, status_changed, channels_status = await check_user_joined_channels(user_id, event.sender)
        
        # If user just joined all channels (status changed from False to True)
        if status_changed and joined_all:
            _, welcome_sent, _ = get_user_status(user_id)
            if not welcome_sent:
                await send_welcome_message(event)
            else:
                await event.reply("✅ **Channel membership verified!** You can now use /pn command.", parse_mode='md')
        
        # If user hasn't joined all channels
        if not joined_all:
            # Get list of channels user hasn't joined
            missing_channels = [channel for channel, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing_channels)
            return
        
        # User has joined all channels, proceed with original functionality
        reply = await event.get_reply_message()
        if not reply:
            await event.reply('Reply to a quiz share message.')
            return

        text = reply.text if reply.text else ''
        quiz_id = None
        if 't.me/QuizBot?start=' in text:
            quiz_id = re.search(r'start=([\w-]+)', text).group(1)
        elif '@QuizBot quiz:' in text:
            quiz_id = 'quiz:' + re.search(r'quiz:([\w-]+)', text).group(1)
        
        if not quiz_id and reply.buttons:
            for row in reply.buttons:
                for btn in row:
                    if hasattr(btn, 'url') and btn.url and 't.me/QuizBot?start=' in btn.url:
                        match = re.search(r'start=([\w-]+)', btn.url)
                        if match:
                            quiz_id = match.group(1)
                            break
                if quiz_id:
                    break

        if not quiz_id:
            await event.reply('Invalid quiz share format.')
            return

        target_chat = event.chat_id
        logging.info(f"Starting quiz with ID: {quiz_id}")
        await client.send_message('QuizBot', '/stop')
        await asyncio.sleep(1)
        await client.send_message('QuizBot', f'/start {quiz_id}')
        
    except Exception as e:
        logging.error(f"Error in pn_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/again'))
async def again_handler(event):
    global target_chat
    
    try:
        # Check channel membership for /again command too
        user_id = event.sender_id
        joined_all, _, _ = await check_user_joined_channels(user_id, event.sender)
        
        if not joined_all:
            missing_channels = [channel for channel in REQUIRED_CHANNELS]
            await send_channel_warning(event, missing_channels)
            return
        
        if target_chat is None or event.chat_id != target_chat:
            await event.reply('No active quiz session. Use /pn to start a quiz.')
            return

        # Fetch recent messages from QuizBot to find the "Try again" button
        messages = await client.get_messages('QuizBot', limit=10)
        for msg in messages:
            if msg.buttons:
                for i, row in enumerate(msg.buttons):
                    for j, btn in enumerate(row):
                        if btn.text and 'try again' in btn.text.lower():
                            await msg.click(i, j)
                            logging.info("Clicked 'Try again' button via /again command")
                            return
        await event.reply("No 'Try again' button found in recent QuizBot messages.")
        
    except Exception as e:
        logging.error(f"Error in again_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/stop'))
async def stop_handler(event):
    global target_chat
    try:
        if target_chat is None or event.chat_id != target_chat:
            return
        await client.send_message('QuizBot', '/stop')
        target_chat = None
        await event.reply('Stopped sending polls.')
    except Exception as e:
        logging.error(f"Error in stop_handler: {e}")

@client.on(events.NewMessage(pattern='/refresh'))
async def refresh_handler(event):
    """Force refresh channel membership status"""
    try:
        user_id = event.sender_id
        remove_user(user_id)  # Clear old data
        joined_all, status_changed, channels_status = await check_user_joined_channels(user_id, event.sender)
        
        if joined_all:
            _, welcome_sent, _ = get_user_status(user_id)
            if not welcome_sent:
                await send_welcome_message(event)
            else:
                await event.reply("✅ **Channel membership has been refreshed and verified!** You can now use /pn command.", parse_mode='md')
        else:
            missing_channels = [channel for channel, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing_channels)
    except Exception as e:
        logging.error(f"Error in refresh_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/check'))
async def check_handler(event):
    """Check current channel membership status"""
    try:
        user_id = event.sender_id
        joined_all, _, channels_status = await check_user_joined_channels(user_id, event.sender)
        
        if joined_all:
            await event.reply("✅ **You have joined all required channels!**\n\nYou can use /pn command to create polls.", parse_mode='md')
        else:
            missing_channels = [channel for channel, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing_channels)
    except Exception as e:
        logging.error(f"Error in check_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/status'))
async def status_handler(event):
    """Show detailed status of all channels"""
    try:
        user_id = event.sender_id
        joined_all, welcome_sent, channels_status = get_user_status(user_id)
        
        status_message = "📊 **Your Channel Status:**\n\n"
        
        for i, channel in enumerate(REQUIRED_CHANNELS, 1):
            joined = channels_status.get(channel, False)
            status_emoji = "✅" if joined else "❌"
            status_text = "Joined" if joined else "Not Joined"
            channel_display = CHANNEL_DISPLAY.get(channel, channel)
            status_message += f"{i}. {channel_display} - {status_emoji} {status_text}\n"
        
        status_message += f"\n**Overall Status:** {'✅ All channels joined' if joined_all else '❌ Missing channels'}\n"
        status_message += f"**Welcome Sent:** {'✅ Yes' if welcome_sent else '❌ No'}\n\n"
        
        if joined_all:
            status_message += "You can use `/pn` command to create polls."
        else:
            status_message += "Please join all channels to use `/pn` command."
        
        await event.reply(status_message, parse_mode='md')
    except Exception as e:
        logging.error(f"Error in status_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/stats'))
async def stats_handler(event):
    """Show bot statistics (admin only)"""
    try:
        user_id = event.sender_id
        
        # Check if user is admin
        if not is_admin(user_id):
            await event.reply("❌ **Access Denied!**\nThis command is only available for administrators.", parse_mode='md')
            return
        
        # Get bot statistics
        stats = get_bot_stats()
        
        if not stats:
            await event.reply("❌ **Error:** Could not retrieve statistics.", parse_mode='md')
            return
        
        # Format last updated time
        last_updated = stats['last_updated']
        if isinstance(last_updated, str):
            last_updated = datetime.fromisoformat(last_updated)
        
        time_ago = datetime.now() - last_updated
        if time_ago.days > 0:
            last_updated_str = f"{time_ago.days} days ago"
        elif time_ago.seconds // 3600 > 0:
            last_updated_str = f"{time_ago.seconds // 3600} hours ago"
        elif time_ago.seconds // 60 > 0:
            last_updated_str = f"{time_ago.seconds // 60} minutes ago"
        else:
            last_updated_str = "just now"
        
        # Create statistics message
        stats_message = "📈 **Bot Statistics** 📈\n\n"
        
        stats_message += "👥 **User Statistics:**\n"
        stats_message += f"• Total Users: `{stats['total_users']}`\n"
        stats_message += f"• Active Users (7 days): `{stats['active_users']}`\n"
        stats_message += f"• New Users (24 hours): `{stats['new_users_24h']}`\n"
        stats_message += f"• Users with Access: `{stats['users_with_access']}`\n"
        stats_message += f"• Users Welcomed: `{stats['users_welcomed']}`\n\n"
        
        stats_message += "📊 **Poll Statistics:**\n"
        stats_message += f"• Total Polls Created: `{stats['total_polls']}`\n\n"
        
        stats_message += "📢 **Channel Statistics:**\n"
        for channel in REQUIRED_CHANNELS:
            channel_display = CHANNEL_DISPLAY.get(channel, channel)
            joined_count = stats['channel_stats'].get(channel, 0)
            percentage = (joined_count / max(stats['total_users'], 1)) * 100
            stats_message += f"• {channel_display}: `{joined_count}` ({percentage:.1f}%)\n"
        
        stats_message += f"\n⏰ **Last Updated:** {last_updated_str}\n"
        stats_message += f"📅 **Database:** `{DB_NAME}`"
        
        await event.reply(stats_message, parse_mode='md')
        
    except Exception as e:
        logging.error(f"Error in stats_handler: {e}")
        await event.reply(f"An error occurred while retrieving statistics: {str(e)}")

@client.on(events.NewMessage(pattern='/resetdb'))
async def reset_db_handler(event):
    """Reset database (admin command) - be careful with this!"""
    try:
        user_id = event.sender_id
        
        # Check if user is admin
        if not is_admin(user_id):
            await event.reply("❌ **Access Denied!**\nThis command is only available for administrators.", parse_mode='md')
            return
        
        # Ask for confirmation
        await event.reply("⚠️ **Warning:** This will delete ALL user data!\n\nType `/confirm_reset` to proceed or anything else to cancel.", parse_mode='md')
        
    except Exception as e:
        logging.error(f"Error in reset_db_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/confirm_reset'))
async def confirm_reset_handler(event):
    """Confirm database reset"""
    try:
        user_id = event.sender_id
        
        # Check if user is admin
        if not is_admin(user_id):
            await event.reply("❌ **Access Denied!**\nThis command is only available for administrators.", parse_mode='md')
            return
        
        # Close existing connections
        if os.path.exists(DB_NAME):
            os.remove(DB_NAME)
            logging.info(f"Removed database file: {DB_NAME}")
        
        # Reinitialize database
        init_database()
        
        await event.reply("✅ **Database has been reset successfully!**\nAll user data has been cleared.", parse_mode='md')
        
    except Exception as e:
        logging.error(f"Error resetting database: {e}")
        await event.reply(f"❌ **Error resetting database:** {str(e)}")

@client.on(events.NewMessage(pattern='/broadcast'))
async def broadcast_handler(event):
    """Broadcast message to all users (admin only)"""
    try:
        user_id = event.sender_id
        
        # Check if user is admin
        if not is_admin(user_id):
            await event.reply("❌ **Access Denied!**\nThis command is only available for administrators.", parse_mode='md')
            return
        
        # Get the message to broadcast
        broadcast_text = event.text.replace('/broadcast', '').strip()
        
        if not broadcast_text:
            await event.reply("❌ **Usage:** `/broadcast your message here`", parse_mode='md')
            return
        
        await event.reply("📢 **Starting broadcast...**\nThis may take a while.", parse_mode='md')
        
        # Get all users from database
        conn = sqlite3.connect(DB_NAME)
        cursor = conn.cursor()
        cursor.execute('SELECT user_id FROM users')
        users = cursor.fetchall()
        conn.close()
        
        total_users = len(users)
        successful = 0
        failed = 0
        
        # Send broadcast to each user
        for user in users:
            user_id_to_send = user[0]
            try:
                await client.send_message(user_id_to_send, broadcast_text)
                successful += 1
                await asyncio.sleep(0.1)  # Small delay to avoid rate limiting
            except Exception as e:
                logging.error(f"Failed to send broadcast to user {user_id_to_send}: {e}")
                failed += 1
        
        # Send broadcast summary
        summary = f"📢 **Broadcast Completed!**\n\n"
        summary += f"• Total Users: `{total_users}`\n"
        summary += f"• Successful: `{successful}`\n"
        summary += f"• Failed: `{failed}`\n"
        summary += f"• Success Rate: `{(successful/max(total_users, 1))*100:.1f}%`"
        
        await event.reply(summary, parse_mode='md')
        
    except Exception as e:
        logging.error(f"Error in broadcast_handler: {e}")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/start'))
async def start_handler(event):
    """Send welcome message when user starts the bot"""
    try:
        welcome_message = "👋 **Welcome to the Quiz Poll Bot!**\n\n"
        welcome_message += "**This bot helps you forward quiz polls from QuizBot.**\n\n"
        welcome_message += "**Commands:**\n"
        welcome_message += "• `/pn` - Start a new quiz (reply to a quiz message)\n"
        welcome_message += "• `/again` - Try the quiz again\n"
        welcome_message += "• `/stop` - Stop the current quiz session\n"
        welcome_message += "• `/check` - Check your channel status\n"
        welcome_message += "• `/status` - Detailed channel status\n"
        welcome_message += "• `/refresh` - Force refresh your status\n"
        
        # Only show admin commands to admins
        if is_admin(event.sender_id):
            welcome_message += "• `/stats` - View bot statistics (Admin)\n"
            welcome_message += "• `/broadcast` - Broadcast message (Admin)\n"
            welcome_message += "• `/resetdb` - Reset database (Admin)\n"
        
        welcome_message += "\n**Note:** You need to join all required channels before using /pn command."
        
        await event.reply(welcome_message, parse_mode='md')
    except Exception as e:
        logging.error(f"Error in start_handler: {e}")

@client.on(events.NewMessage(from_users='QuizBot'))
async def quiz_handler(event):
    global target_chat
    if not target_chat:
        return

    local_target = target_chat  # Capture the current target_chat to avoid race conditions

    msg = event.message

    if msg.buttons:
        for i, row in enumerate(msg.buttons):
            for j, btn in enumerate(row):
                button_text = btn.text.lower()
                if 'i am ready' in button_text:
                    try:
                        await msg.click(i, j)
                        logging.info("Clicked 'I am ready' button")
                    except Exception as e:
                        logging.error(f"Error clicking button: {e}")
                    return

    if msg.poll and msg.poll.poll.quiz:
        options_bytes = [ans.option for ans in msg.poll.poll.answers]
        vote_option = random.choice(options_bytes)
        try:
            await client(functions.messages.SendVoteRequest(
                peer='QuizBot',
                msg_id=msg.id,
                options=[vote_option]
            ))
        except Exception as e:
            logging.error(f"Error sending vote: {e}")
            return

        await asyncio.sleep(2)  # Wait for the message to update

        updated_msg = await client.get_messages('QuizBot', ids=msg.id)
        attempts = 0
        while not updated_msg.poll.results.results and attempts < 10:
            await asyncio.sleep(1)
            updated_msg = await client.get_messages('QuizBot', ids=msg.id)
            attempts += 1

        if not updated_msg.poll.results.results:
            logging.warning("No poll results received after retries")
            if local_target:
                await client.send_message(local_target, "Error: No poll results received")
            return

        correct_option = None
        for res in updated_msg.poll.results.results:
            if res.correct:
                correct_option = res.option
                break

        if correct_option is None:
            logging.error("No correct option found in poll results")
            if local_target:
                await client.send_message(local_target, "No correct option found in poll results")
            return

        original_poll = updated_msg.poll.poll
        question_text = original_poll.question.text
        logging.info(f"Original question: {question_text}")

        original_entities = original_poll.question.entities or []
        twe_original = types.TextWithEntities(text=question_text, entities=original_entities)

        # First, remove emojis preserving entities
        twe_no_emoji = remove_emojis_preserve_entities(twe_original)

        # Remove all numbering patterns like [11/70], [2/70], 11/70, Question 8 of 89, etc.
        number_pattern = re.compile(
            r'^(?:\[?\s*\d+\s*(?:of|\/)\s*\d+\s*\]?\s*[.:]?\s*|Question\s+\d+\s*(?:of|\/)\s*\d+\s*[.:]?\s*|\(\s*\d+\s*/\s*\d+\s*\)\s*|Q\s*\d+\s*[.:]?\s*)',
            re.IGNORECASE
        )
        total_prefix_len = 0
        current_text = twe_no_emoji.text
        while True:
            match = number_pattern.match(current_text)
            if not match:
                break
            prefix_len = match.end()
            total_prefix_len += prefix_len
            current_text = current_text[prefix_len:]

        # Adjust entities after removing all prefixes
        adjusted_entities = []
        for entity in twe_no_emoji.entities:
            old_start = entity.offset
            old_end = old_start + entity.length
            if old_start >= total_prefix_len:
                new_start = old_start - total_prefix_len
                new_length = entity.length
                new_entity = type(entity)(
                    offset=new_start,
                    length=new_length,
                    **{k: v for k, v in entity.__dict__.items() if k not in ['offset', 'length']}
                )
                adjusted_entities.append(new_entity)
            elif old_end > total_prefix_len:
                # Clip if overlapping prefix
                overlap = total_prefix_len - old_start
                new_start = 0
                new_length = entity.length - overlap
                if new_length > 0:
                    new_entity = type(entity)(
                        offset=new_start,
                        length=new_length,
                        **{k: v for k, v in entity.__dict__.items() if k not in ['offset', 'length']}
                    )
                    adjusted_entities.append(new_entity)

        question = types.TextWithEntities(
            text=current_text,
            entities=adjusted_entities
        )
        logging.info(f"Cleaned question: {question.text}")

        # Clean answers: remove emojis, preserve entities
        answers = []
        for ans in original_poll.answers:
            if ans.text is not None:
                clean_twe = remove_emojis_preserve_entities(ans.text)
                answers.append(types.PollAnswer(text=clean_twe, option=ans.option))
            else:
                logging.warning("Poll answer text is None")
                continue

        # Create a quiz poll
        poll = types.Poll(
            id=int(time.time()),
            question=question,
            answers=answers,
            public_voters=False,  # Ensures anonymous poll
            multiple_choice=False,
            quiz=True
        )

        # Create InputMediaPoll with quiz fields
        media_kwargs = {
            'poll': poll,
            'correct_answers': [correct_option]
        }
        media = types.InputMediaPoll(**media_kwargs)

        try:
            logging.info(f"Sending quiz poll to chat {local_target}")
            sent_msg = await client.send_message(local_target, file=media)
            logging.info("Quiz poll sent successfully")

            await asyncio.sleep(1)  # Brief wait for poll to process

            # Create closed poll
            closed_poll = types.Poll(
                id=poll.id,
                question=question,
                answers=answers,
                public_voters=False,
                multiple_choice=False,
                quiz=True,
                closed=True
            )

            closed_media_kwargs = {
                'poll': closed_poll,
                'correct_answers': [correct_option]
            }
            closed_media = types.InputMediaPoll(**closed_media_kwargs)

            # Edit the message to close the poll
            await client(functions.messages.EditMessageRequest(
                peer=local_target,
                id=sent_msg.id,
                media=closed_media
            ))
            logging.info("Edited the poll to close it")

        except Exception as e:
            logging.error(f"Failed to send or close poll: {str(e)}")
            if local_target:
                await client.send_message(local_target, f"Error sending poll: {str(e)}")
            return

# Start the client
async def main():
    await client.start()
    logging.info("Bot started successfully!")
    
    # Update bot stats on startup
    update_bot_stats()
    
    await client.run_until_disconnected()

if __name__ == '__main__':
    try:
        client.loop.run_until_complete(main())
    except KeyboardInterrupt:
        logging.info("Bot stopped by user")
    except Exception as e:
        logging.error(f"Fatal error: {e}")
