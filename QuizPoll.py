from telethon import TelegramClient, events
from telethon.sessions import StringSession
from telethon.tl.types import Poll, PollAnswer, InputMediaPoll, TextWithEntities
from telethon.tl import types, functions
import re
import time
import random
import asyncio
import logging

# Configure logging
logging.basicConfig(level=logging.INFO, format='%(asctime)s - %(levelname)s - %(message)s')

# Replace with your values
api_id = '12380656'
api_hash = 'd927c13beaaf5110f25c505b7c071273'
session_string = '1BZWaqwUAUCh6H9L_y-ZZABkOOGCyTm5ytu3pUTT3qQbkogiGlJs-XxvmRr241FhkpKi5k-INCsFfZ7yAtNwcweGlipYLRo5dWGZd_RUpNIPYjJjfRvkZ4W84mJos03u-qMqOvCp1S94-Uub7Ts__1iMC7sbQxwzKicwQ0AdbvOEBUccBaVqIW5b-Pku6U3FJo5pJ3r1ZkmUlXK69ugXfgUMT4dinnamUMqRtIVe9EczMnuwCQqUzXBLdlzY5NsadxPXDEWsH7nmpVLfIuFi7HuUACettKV6cYzgzYtshNjLuYHZgZNb81n0Y78ozS4yYyvkKCt0afruNQ8FSr_PY9TYpCv8Zq_w='

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

@client.on(events.NewMessage(pattern='/pn'))
async def pn_handler(event):
    global target_chat
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
    await client.send_message('QuizBot', f'/start {quiz_id}')

@client.on(events.NewMessage(from_users='QuizBot'))
async def quiz_handler(event):
    global target_chat
    if not target_chat:
        return

    msg = event.message

    if msg.buttons:
        for i, row in enumerate(msg.buttons):
            for j, btn in enumerate(row):
                button_text = btn.text.lower()
                if 'i am ready' in button_text:
                    await msg.click(i, j)
                    logging.info("Clicked 'I am ready' button")
                    return

    if msg.poll and msg.poll.poll.quiz:
        options_bytes = [ans.option for ans in msg.poll.poll.answers]
        vote_option = random.choice(options_bytes)
        await client(functions.messages.SendVoteRequest(
            peer='QuizBot',
            msg_id=msg.id,
            options=[vote_option]
        ))

        await asyncio.sleep(2)  # Wait for the message to update

        updated_msg = await client.get_messages('QuizBot', ids=msg.id)
        attempts = 0
        while not updated_msg.poll.results.results and attempts < 10:
            await asyncio.sleep(1)
            updated_msg = await client.get_messages('QuizBot', ids=msg.id)
            attempts += 1

        if not updated_msg.poll.results.results:
            logging.warning("No poll results received after retries")
            await client.send_message(target_chat, "Error: No poll results received")
            return

        correct_option = None
        for res in updated_msg.poll.results.results:
            if res.correct:
                correct_option = res.option
                break

        if correct_option is None:
            logging.error("No correct option found in poll results")
            await client.send_message(target_chat, "No correct option found in poll results")
            return

        original_poll = updated_msg.poll.poll
        question_text = original_poll.question.text
        logging.info(f"Original question: {question_text}")
        # Remove numbering like [1/65], 1/65., Question 1 of 65:, etc.
        match = re.match(r'^\[?\d+/\d+\]?\s*[.:]?\s*|^Question\s+\d+\s+of\s+\d+[:.]?\s*', question_text, re.IGNORECASE)
        if match:
            prefix_len = match.end()
            new_question_text = question_text[prefix_len:]
        else:
            new_question_text = question_text

        # Remove emojis from question
        new_question_text = emoji_pattern.sub('', new_question_text)
        logging.info(f"Cleaned question: {new_question_text}")

        # Set entities to empty for simple text
        new_entities = []

        question = types.TextWithEntities(text=new_question_text, entities=new_entities)

        # Clean answers: remove emojis and no entities
        answers = []
        for ans in original_poll.answers:
            if ans.text is not None:
                clean_text_str = emoji_pattern.sub('', ans.text.text)
                clean_text = types.TextWithEntities(text=clean_text_str, entities=[])
                answers.append(types.PollAnswer(text=clean_text, option=ans.option))
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
            logging.info(f"Sending quiz poll to chat {target_chat}")
            sent_msg = await client.send_message(target_chat, file=media)
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
                peer=target_chat,
                id=sent_msg.id,
                media=closed_media
            ))
            logging.info("Edited the poll to close it")

        except Exception as e:
            logging.error(f"Failed to send or close poll: {str(e)}")
            await client.send_message(target_chat, f"Error sending poll: {str(e)}")
            return

with client:
    client.run_until_disconnected()
