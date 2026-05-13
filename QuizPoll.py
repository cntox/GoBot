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
import contextlib
from datetime import datetime, timedelta
import os

logging.basicConfig(
    level=logging.INFO,
    format='%(asctime)s - %(levelname)s - %(name)s - %(message)s'
)

api_id = '12380656'
api_hash = 'd927c13beaaf5110f25c505b7c071273'
session_string = '1BZWaqwUAUCh6H9L_y-ZZABkOOGCyTm5ytu3pUTT3qQbkogiGlJs-XxvmRr241FhkpKi5k-INCsFfZ7yAtNwcweGlipYLRo5dWGZd_RUpNIPYjJjfRvkZ4W84mJos03u-qMqOvCp1S94-Uub7Ts__1iMC7sbQxwzKicwQ0AdbvOEBUccBaVqIW5b-Pku6U3FJo5pJ3r1ZkmUlXK69ugXfgUMT4dinnamUMqRtIVe9EczMnuwCQqUzXBLdlzY5NsadxPXDEWsH7nmpVLfIuFi7HuUACettKV6cYzgzYtshNjLuYHZgZNb81n0Y78ozS4yYyvkKCt0afruNQ8FSr_PY9TYpCv8Zq_w='

REQUIRED_CHANNELS = [
    '@exampurrs',
    '@exampurss_official',
    '@sarakari_result',
    '@FONT_CHANNEL_01'
]

CHANNEL_DISPLAY = {
    '@exampurrs': '[ᴇxᴀᴍᴘᴜʀ](https://t.me/exampurrs)',
    '@exampurss_official': '[ᴇxᴀᴍᴘᴜʀ Qᴜɪᴢ](https://t.me/exampurss_official)',
    '@sarakari_result': '[ꜱᴀʀᴋᴀʀɪ ʀᴇꜱᴜʟᴛ](https://t.me/sarakari_result)',
    '@FONT_CHANNEL_01': '[ꜱᴛʏʟɪꜱʜ ꜰᴏɴᴛ](https://t.me/FONT_CHANNEL_01)'
}

DB_NAME = 'poll_bot.db'
ADMIN_IDS = [6644859358, 8451305181, 7183060880]

active_sessions = {}

# ---------- Database helpers ----------
@contextlib.contextmanager
def get_db():
    conn = sqlite3.connect(DB_NAME)
    conn.execute("PRAGMA busy_timeout=10000;")
    try:
        yield conn
    finally:
        conn.close()

def init_database():
    with get_db() as conn:
        conn.execute("PRAGMA journal_mode=WAL;")
        conn.execute("PRAGMA busy_timeout=10000;")
        cursor = conn.cursor()
        cursor.execute("SELECT name FROM sqlite_master WHERE type='table' AND name='users'")
        users_table_exists = cursor.fetchone() is not None
        if not users_table_exists:
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
        cursor.execute('''
        CREATE TABLE IF NOT EXISTS channel_joins (
            user_id INTEGER,
            channel_id TEXT,
            joined BOOLEAN DEFAULT 0,
            last_checked TIMESTAMP,
            PRIMARY KEY (user_id, channel_id)
        )''')
        cursor.execute('''
        CREATE TABLE IF NOT EXISTS user_actions (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            user_id INTEGER,
            action TEXT,
            timestamp TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )''')
        cursor.execute('''
        CREATE TABLE IF NOT EXISTS bot_stats (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            total_users INTEGER DEFAULT 0,
            active_users INTEGER DEFAULT 0,
            total_polls INTEGER DEFAULT 0,
            last_updated TIMESTAMP DEFAULT CURRENT_TIMESTAMP
        )''')
        conn.commit()
        logging.info("Database initialized")

def is_admin(user_id):
    return user_id in ADMIN_IDS

def update_user_channels(user_id, channels_status, user_info=None):
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('SELECT joined_all_channels FROM users WHERE user_id = ?', (user_id,))
            previous_result = cursor.fetchone()
            previous_joined_all = previous_result[0] if previous_result else False
            if user_info:
                cursor.execute('SELECT COUNT(*) FROM users WHERE user_id = ?', (user_id,))
                user_exists = cursor.fetchone()[0] > 0
                if user_exists:
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
                    cursor.execute('''
                    INSERT INTO users 
                    (user_id, username, first_name, last_name, last_check) 
                    VALUES (?, ?, ?, ?, ?)
                    ''', (user_id, user_info.get('username'), user_info.get('first_name'),
                          user_info.get('last_name'), datetime.now()))
            else:
                cursor.execute('INSERT OR IGNORE INTO users (user_id, last_check) VALUES (?, ?)', (user_id, datetime.now()))
                cursor.execute('UPDATE users SET last_check = ? WHERE user_id = ?', (datetime.now(), user_id))
            for channel_id, joined in channels_status.items():
                cursor.execute('''
                INSERT OR REPLACE INTO channel_joins (user_id, channel_id, joined, last_checked)
                VALUES (?, ?, ?, ?)
                ''', (user_id, channel_id, 1 if joined else 0, datetime.now()))
            cursor.execute('SELECT COUNT(*) FROM channel_joins WHERE user_id = ? AND joined = 1', (user_id,))
            joined_count = cursor.fetchone()[0]
            has_joined_all = joined_count >= len(REQUIRED_CHANNELS)
            cursor.execute('''
            UPDATE users SET joined_all_channels = ?, last_check = ? WHERE user_id = ?
            ''', (1 if has_joined_all else 0, datetime.now(), user_id))
            cursor.execute('INSERT INTO user_actions (user_id, action) VALUES (?, ?)',
                           (user_id, f"channel_check_{'all_joined' if has_joined_all else 'missing_channels'}"))
            update_bot_stats(conn)
            conn.commit()
            status_changed = previous_joined_all != has_joined_all
            return has_joined_all, status_changed
        except Exception as e:
            logging.exception(f"Error updating user channels for {user_id}")
            conn.rollback()
            return False, False

def update_bot_stats(conn=None):
    own_conn = False
    if conn is None:
        conn = sqlite3.connect(DB_NAME)
        conn.execute("PRAGMA busy_timeout=10000;")
        own_conn = True
    try:
        cursor = conn.cursor()
        cursor.execute('SELECT COUNT(*) FROM users')
        total_users = cursor.fetchone()[0]
        seven_days_ago = datetime.now() - timedelta(days=7)
        cursor.execute('SELECT COUNT(*) FROM users WHERE last_check >= ?', (seven_days_ago,))
        active_users = cursor.fetchone()[0]
        cursor.execute("SELECT COUNT(*) FROM user_actions WHERE action LIKE '%channel_check_all_joined%'")
        total_polls = cursor.fetchone()[0]
        cursor.execute('''
        INSERT OR REPLACE INTO bot_stats (id, total_users, active_users, total_polls, last_updated)
        VALUES (1, ?, ?, ?, ?)
        ''', (total_users, active_users, total_polls, datetime.now()))
        conn.commit()
    except Exception as e:
        logging.exception("Error updating bot stats")
        conn.rollback()
    finally:
        if own_conn:
            conn.close()

def get_bot_stats():
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('SELECT total_users, active_users, total_polls, last_updated FROM bot_stats WHERE id = 1')
            stats_result = cursor.fetchone()
            if stats_result:
                total_users, active_users, total_polls, last_updated = stats_result
            else:
                total_users = active_users = total_polls = 0
                last_updated = datetime.now()
            cursor.execute('SELECT COUNT(*) FROM users WHERE joined_all_channels = 1')
            users_with_access = cursor.fetchone()[0]
            cursor.execute('SELECT COUNT(*) FROM users WHERE welcome_sent = 1')
            users_welcomed = cursor.fetchone()[0]
            one_day_ago = datetime.now() - timedelta(days=1)
            cursor.execute('SELECT COUNT(*) FROM users WHERE created_at >= ?', (one_day_ago,))
            new_users_24h = cursor.fetchone()[0]
            channel_stats = {}
            for channel in REQUIRED_CHANNELS:
                cursor.execute('SELECT COUNT(*) FROM channel_joins WHERE channel_id = ? AND joined = 1', (channel,))
                joined_count = cursor.fetchone()[0]
                channel_stats[channel] = joined_count
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
            logging.exception("Error getting bot stats")
            return None

def get_user_status(user_id):
    with get_db() as conn:
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
                if result[2]:
                    channels_status[result[2]] = bool(result[3])
            return joined_all, welcome_sent, channels_status
        except Exception as e:
            logging.exception(f"Error getting user status for {user_id}")
            return False, False, {}

def update_welcome_sent(user_id, sent=True):
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('UPDATE users SET welcome_sent = ? WHERE user_id = ?', (1 if sent else 0, user_id))
            conn.commit()
        except Exception as e:
            logging.exception(f"Error updating welcome sent for {user_id}")
            conn.rollback()

def update_last_warning(user_id):
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('UPDATE users SET last_warning = ? WHERE user_id = ?', (datetime.now(), user_id))
            conn.commit()
        except Exception as e:
            logging.exception(f"Error updating last warning for {user_id}")
            conn.rollback()

def should_send_warning(user_id):
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('SELECT last_warning FROM users WHERE user_id = ?', (user_id,))
            result = cursor.fetchone()
            if not result or not result[0]:
                return True
            last_warning = datetime.fromisoformat(result[0]) if isinstance(result[0], str) else result[0]
            return datetime.now() - last_warning > timedelta(seconds=30)
        except Exception as e:
            logging.exception(f"Error checking warning time for {user_id}")
            return True

def remove_user(user_id):
    with get_db() as conn:
        cursor = conn.cursor()
        try:
            cursor.execute('DELETE FROM channel_joins WHERE user_id = ?', (user_id,))
            cursor.execute('DELETE FROM user_actions WHERE user_id = ?', (user_id,))
            cursor.execute('DELETE FROM users WHERE user_id = ?', (user_id,))
            conn.commit()
            logging.info(f"Removed user {user_id}")
        except Exception as e:
            logging.exception(f"Error removing user {user_id}")
            conn.rollback()

init_database()
client = TelegramClient(StringSession(session_string), api_id, api_hash)

emoji_pattern = re.compile(
    "["
    "\U0001F600-\U0001F64F"
    "\U0001F300-\U0001F5FF"
    "\U0001F680-\U0001F6FF"
    "\U0001F1E0-\U0001F1FF"
    "\U00002500-\U00002BEF"
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
    "\ufe0f"
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
    channels_status = {}
    user_info = {}
    if sender:
        user_info = {
            'username': sender.username,
            'first_name': sender.first_name,
            'last_name': sender.last_name
        }
    for channel in REQUIRED_CHANNELS:
        try:
            await client(GetParticipantRequest(channel=channel, participant=user_id))
            channels_status[channel] = True
        except UserNotParticipantError:
            channels_status[channel] = False
        except Exception as e:
            logging.exception(f"Error checking channel {channel} for user {user_id}")
            channels_status[channel] = False
    has_joined_all, status_changed = update_user_channels(user_id, channels_status, user_info)
    return has_joined_all, status_changed, channels_status

async def send_channel_warning(event, missing_channels):
    if not should_send_warning(event.sender_id):
        return
    warning_message = "**⚠️ Please join all required channels first!**\n\n"
    warning_message += "**Required channels:**\n"
    for i, channel in enumerate(REQUIRED_CHANNELS, 1):
        status = "✅ Joined" if channel not in missing_channels else "❌ Not Joined"
        channel_display = CHANNEL_DISPLAY.get(channel, channel)
        warning_message += f"{i}. {channel_display} - {status}\n"
    warning_message += "\nAfter joining all channels, send /pn again."
    try:
        await event.reply(warning_message, parse_mode='md')
        update_last_warning(event.sender_id)
    except Exception as e:
        logging.exception("Error sending channel warning")

async def send_welcome_message(event):
    welcome_message = "🎉 **Welcome! You have joined all channels!**\n\n"
    welcome_message += "✅ You can now use `/pn` to create polls.\n\n"
    welcome_message += "**Joined channels:**\n"
    for i, channel in enumerate(REQUIRED_CHANNELS, 1):
        channel_display = CHANNEL_DISPLAY.get(channel, channel)
        welcome_message += f"{i}. {channel_display}\n"
    welcome_message += "\n**Commands:**\n"
    welcome_message += "• `/pn` - start a new quiz (reply to a quiz message)\n"
    welcome_message += "• `/again` - retry the last quiz\n"
    welcome_message += "• `/stop` - stop current quiz\n"
    welcome_message += "• `/check` - check your channel status"
    try:
        await event.reply(welcome_message, parse_mode='md')
        update_welcome_sent(event.sender_id)
    except Exception as e:
        logging.exception("Error sending welcome message")

# ---------- Command Handlers ----------
@client.on(events.NewMessage(pattern='/pn'))
async def pn_handler(event):
    global current_target_chat
    current_target_chat = event.chat_id
    try:
        user_id = event.sender_id
        joined_all, status_changed, channels_status = await check_user_joined_channels(user_id, event.sender)
        if status_changed and joined_all:
            _, welcome_sent, _ = get_user_status(user_id)
            if not welcome_sent:
                await send_welcome_message(event)
            else:
                await event.reply("✅ Channel membership verified!", parse_mode='md')
        if not joined_all:
            missing = [ch for ch, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing)
            return

        reply = await event.get_reply_message()
        if not reply:
            await event.reply('Reply to a quiz share message.')
            return

        text = reply.text or ''
        quiz_id = None
        if 't.me/QuizBot?start=' in text:
            match = re.search(r'start=([\w-]+)', text)
            if match:
                quiz_id = match.group(1)
        elif '@QuizBot quiz:' in text:
            match = re.search(r'quiz:([\w-]+)', text)
            if match:
                quiz_id = 'quiz:' + match.group(1)

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

        active_sessions[event.chat_id] = quiz_id
        logging.info(f"Starting quiz {quiz_id} for chat {event.chat_id}")
        await client.send_message('QuizBot', '/stop')
        await asyncio.sleep(1)
        await client.send_message('QuizBot', f'/start {quiz_id}')

    except Exception as e:
        logging.exception("Error in pn_handler")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/again'))
async def again_handler(event):
    global current_target_chat
    if event.chat_id in active_sessions:
        current_target_chat = event.chat_id
    try:
        user_id = event.sender_id
        joined_all, _, _ = await check_user_joined_channels(user_id, event.sender)
        if not joined_all:
            await send_channel_warning(event, REQUIRED_CHANNELS)
            return

        quiz_id = active_sessions.get(event.chat_id)
        if not quiz_id:
            await event.reply('No previous quiz. Use /pn to start one.')
            return

        await client.send_message('QuizBot', '/stop')
        await asyncio.sleep(1)
        await client.send_message('QuizBot', f'/start {quiz_id}')
        await event.reply('🔄 Restarting quiz...')

    except Exception as e:
        logging.exception("Error in again_handler")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/stop'))
async def stop_handler(event):
    global current_target_chat
    if event.chat_id == current_target_chat:
        current_target_chat = None
    try:
        if event.chat_id in active_sessions:
            del active_sessions[event.chat_id]
        await client.send_message('QuizBot', '/stop')
        await event.reply('Stopped current quiz session.')
    except Exception as e:
        logging.exception("Error in stop_handler")

@client.on(events.NewMessage(pattern='/refresh'))
async def refresh_handler(event):
    try:
        user_id = event.sender_id
        remove_user(user_id)
        joined_all, status_changed, channels_status = await check_user_joined_channels(user_id, event.sender)
        if joined_all:
            _, welcome_sent, _ = get_user_status(user_id)
            if not welcome_sent:
                await send_welcome_message(event)
            else:
                await event.reply("✅ Channel membership refreshed!", parse_mode='md')
        else:
            missing = [ch for ch, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing)
    except Exception as e:
        logging.exception("Error in refresh_handler")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/check'))
async def check_handler(event):
    try:
        user_id = event.sender_id
        joined_all, _, channels_status = await check_user_joined_channels(user_id, event.sender)
        if joined_all:
            await event.reply("✅ You have joined all required channels!", parse_mode='md')
        else:
            missing = [ch for ch, joined in channels_status.items() if not joined]
            await send_channel_warning(event, missing)
    except Exception as e:
        logging.exception("Error in check_handler")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/status'))
async def status_handler(event):
    try:
        user_id = event.sender_id
        joined_all, welcome_sent, channels_status = get_user_status(user_id)
        msg = "📊 **Your Channel Status:**\n\n"
        for i, ch in enumerate(REQUIRED_CHANNELS, 1):
            joined = channels_status.get(ch, False)
            display = CHANNEL_DISPLAY.get(ch, ch)
            msg += f"{i}. {display} - {'✅ Joined' if joined else '❌ Not Joined'}\n"
        msg += f"\n**Overall:** {'✅ All joined' if joined_all else '❌ Missing channels'}\n"
        msg += f"**Welcome sent:** {'✅ Yes' if welcome_sent else '❌ No'}"
        await event.reply(msg, parse_mode='md')
    except Exception as e:
        logging.exception("Error in status_handler")
        await event.reply(f"An error occurred: {str(e)}")

@client.on(events.NewMessage(pattern='/stats'))
async def stats_handler(event):
    if not is_admin(event.sender_id):
        await event.reply("❌ Admin only.")
        return
    stats = get_bot_stats()
    if not stats:
        await event.reply("Error retrieving stats.")
        return
    await event.reply(
        f"📈 **Bot Stats**\n"
        f"• Users: {stats['total_users']}\n"
        f"• Active (7d): {stats['active_users']}\n"
        f"• With access: {stats['users_with_access']}\n"
        f"• Polls created: {stats['total_polls']}",
        parse_mode='md'
    )

@client.on(events.NewMessage(pattern='/resetdb'))
async def reset_db_handler(event):
    if not is_admin(event.sender_id):
        return
    await event.reply("⚠️ Type `/confirm_reset` to delete all user data.")

@client.on(events.NewMessage(pattern='/confirm_reset'))
async def confirm_reset_handler(event):
    if not is_admin(event.sender_id):
        return
    if os.path.exists(DB_NAME):
        os.remove(DB_NAME)
    init_database()
    await event.reply("✅ Database reset.")

@client.on(events.NewMessage(pattern='/broadcast'))
async def broadcast_handler(event):
    if not is_admin(event.sender_id):
        return
    text = event.text.replace('/broadcast', '').strip()
    if not text:
        await event.reply("Usage: `/broadcast message`")
        return
    await event.reply("Broadcasting...")
    with get_db() as conn:
        cursor = conn.cursor()
        cursor.execute('SELECT user_id FROM users')
        users = cursor.fetchall()
    success = 0
    for (uid,) in users:
        try:
            await client.send_message(uid, text)
            success += 1
            await asyncio.sleep(0.1)
        except Exception:
            pass
    await event.reply(f"Broadcast sent to {success}/{len(users)} users.")

@client.on(events.NewMessage(pattern='/start'))
async def start_handler(event):
    welcome = (
        "👋 **Quiz Poll Bot**\n\n"
        "Reply to a quiz share with `/pn` to forward the poll.\n"
        "Use `/again` to retry the last quiz.\n"
        "You must join all required channels first – use `/check` to verify."
    )
    await event.reply(welcome, parse_mode='md')

# ---------- Core Quiz Handler ----------
current_target_chat = None

# Use the hardcoded numeric ID of @QuizBot (resolved at startup)
@client.on(events.NewMessage(from_users=983000232))
async def quiz_handler(event):
    global current_target_chat
    logging.info("quiz_handler triggered")
    if not current_target_chat:
        logging.warning("quiz_handler: no target chat")
        return

    msg = event.message
    if msg.buttons:
        for i, row in enumerate(msg.buttons):
            for j, btn in enumerate(row):
                if 'i am ready' in btn.text.lower():
                    try:
                        await msg.click(i, j)
                        logging.info("Clicked 'I am ready'")
                    except Exception as e:
                        logging.exception("Error clicking 'I am ready'")
                    return

    if msg.poll and msg.poll.poll.quiz:
        answers = msg.poll.poll.answers
        if not answers:
            logging.error("No answers in poll")
            await client.send_message(current_target_chat, "Error: No answer options.")
            return
        options = [ans.option for ans in answers]
        vote = random.choice(options)
        try:
            await client(functions.messages.SendVoteRequest(
                peer='QuizBot',
                msg_id=msg.id,
                options=[vote]
            ))
            logging.info(f"Voted {vote}")
        except Exception as e:
            logging.exception("Vote failed")
            await client.send_message(current_target_chat, f"Vote error: {str(e)}")
            return

        await asyncio.sleep(2)
        updated = await client.get_messages('QuizBot', ids=msg.id)
        attempts = 0
        while not updated.poll.results.results and attempts < 10:
            await asyncio.sleep(1)
            updated = await client.get_messages('QuizBot', ids=msg.id)
            attempts += 1

        if not updated.poll.results.results:
            logging.warning("No poll results after retries")
            await client.send_message(current_target_chat, "Error: No results received.")
            return

        correct = None
        for res in updated.poll.results.results:
            if res.correct:
                correct = res.option
                break
        if correct is None:
            logging.error("No correct answer found")
            await client.send_message(current_target_chat, "Error: No correct answer.")
            return

        original = updated.poll.poll
        question_text = original.question.text
        entities = original.question.entities or []
        twe_orig = types.TextWithEntities(text=question_text, entities=entities)
        twe_no_emoji = remove_emojis_preserve_entities(twe_orig)

        num_pattern = re.compile(
            r'^(?:\[?\s*\d+\s*(?:of|\/)\s*\d+\s*\]?\s*[.:]?\s*|Question\s+\d+\s*(?:of|\/)\s*\d+\s*[.:]?\s*|\(\s*\d+\s*/\s*\d+\s*\)\s*|Q\s*\d+\s*[.:]?\s*)',
            re.IGNORECASE
        )
        prefix_len = 0
        clean_text = twe_no_emoji.text
        while True:
            m = num_pattern.match(clean_text)
            if not m:
                break
            prefix_len += m.end()
            clean_text = clean_text[m.end():]

        new_entities = []
        for ent in twe_no_emoji.entities:
            old_start = ent.offset
            old_end = old_start + ent.length
            if old_start >= prefix_len:
                new_start = old_start - prefix_len
                new_len = ent.length
                new_entities.append(type(ent)(offset=new_start, length=new_len, **{k:v for k,v in ent.__dict__.items() if k not in ['offset','length']}))
            elif old_end > prefix_len:
                overlap = prefix_len - old_start
                new_len = ent.length - overlap
                if new_len > 0:
                    new_entities.append(type(ent)(offset=0, length=new_len, **{k:v for k,v in ent.__dict__.items() if k not in ['offset','length']}))
        question = types.TextWithEntities(text=clean_text, entities=new_entities)
        logging.info(f"Cleaned question: {question.text}")

        answer_list = []
        answer_options = []
        for ans in original.answers:
            if ans.text is None:
                continue
            clean_ans = remove_emojis_preserve_entities(ans.text)
            answer_list.append(types.PollAnswer(text=clean_ans, option=ans.option))
            answer_options.append(ans.option)

        if not answer_list:
            logging.error("No valid answers after cleaning")
            await client.send_message(current_target_chat, "Error: No valid answers.")
            return

        if correct not in answer_options:
            logging.error(f"Correct option {correct!r} not in {answer_options!r}")
            await client.send_message(current_target_chat, "Error: Correct answer mismatch.")
            return

        poll_hash = random.getrandbits(64)

        poll = types.Poll(
            id=int(time.time()),
            question=question,
            answers=answer_list,
            public_voters=False,
            multiple_choice=False,
            quiz=True,
            hash=poll_hash
        )

        media = types.InputMediaPoll(poll=poll, correct_answers=[correct])

        try:
            sent = await client.send_message(current_target_chat, file=media)
            logging.info("Poll sent successfully")
            await asyncio.sleep(1)

            closed_poll = types.Poll(
                id=poll.id,
                question=question,
                answers=answer_list,
                public_voters=False,
                multiple_choice=False,
                quiz=True,
                closed=True,
                hash=poll_hash
            )
            closed_media = types.InputMediaPoll(poll=closed_poll, correct_answers=[correct])
            await client(functions.messages.EditMessageRequest(
                peer=current_target_chat,
                id=sent.id,
                media=closed_media
            ))
            logging.info("Poll closed after sending")
        except Exception as e:
            logging.exception("Failed to send/close poll")
            await client.send_message(current_target_chat, f"Poll error: {str(e)}")

# ---------- Main ----------
async def main():
    await client.start()
    logging.info("Bot started successfully!")
    # Resolve QuizBot ID just for logging; the hardcoded value is used above
    try:
        qid = (await client.get_input_entity('@QuizBot')).user_id
        logging.info(f"Resolved QuizBot ID (for reference): {qid}")
    except Exception:
        logging.warning("Could not resolve @QuizBot, but hardcoded ID 983000232 is used.")
    update_bot_stats()
    await client.run_until_disconnected()

if __name__ == '__main__':
    try:
        client.loop.run_until_complete(main())
    except KeyboardInterrupt:
        logging.info("Bot stopped")
    except Exception as e:
        logging.exception(f"Fatal error: {e}")
