# ApexClaw

ApexClaw is a highly capable and extensible AI agent running as a Telegram Bot, powered by the Z.AI model engine. It features an array of built-in tools allowing it to seamlessly navigate the web, manage local files, interact with APIs, and execute complex workflows autonomously right from your Telegram chat.

## Highlights
ApexClaw is designed to be your personal command center, featuring extensive tool integrations that allow it to act on your behalf across a multitude of platforms and services.

*   **Autonomous Operation:** Handles multi-step reasoning natively. Ask it to research a topic, compile the data, format it, and email it to you.
*   **Persistent Memory Tracking:** Can recall facts globally across chats, build knowledge bases, and retrieve previous conversational context intelligently. 
*   **Multimodal Capabilities:** Upload images directly to the bot inside Telegram. Powered by vision models, ApexClaw will instantly parse and respond based on the visual input. Plus, built-in Text-To-Speech (TTS) and seamless Speech-to-Text for voice commands (using Google STT).

## Full Feature List & Tools

ApexClaw boasts a massive internal registry of tools that give it unparalleled flexibility:

### 🌐 Web & Browser
*   **Web Protocol:** Fetch raw HTTP data or perform DuckDuckGo Instant Answers searches.
*   **Headless Browser Automation:** Chrome engine integration via chromedp. ApexClaw can open pages, read and extract text, take screenshots, evaluate JS, and click or type precisely to bypass basic anti-bot screens.
*   **Social & Media:** Fetch YouTube search queries, Reddit feeds, Google News headlines, and Pinterest data natively.

### 💻 System & Local File Control
*   **Execution:** Run literal terminal shell scripts or generate and execute ad-hoc Python code natively.
*   **File Management:** Explore local directories, search files, read, write, append, move, create, and delete resources instantly.
*   **System Diagnostics:** Inspect active running processes, system stats, kill frozen processes, and access the system's clipboard if hosted locally.

### ✉️ Communication & Telegram Native
*   **Email Management (IMAP/SMTP):** Automatically read your inbox or send outbound emails dynamically via prompts.
*   **Telegram Superpowers:** Have the AI forward messages, delete old ones, read a chat's info, pin items, react to items, download chat media locally, or even update the bot's own profile picture on the fly!

### 🌍 Real-world Utility
*   **Flight Radar:** Look up airports and track active real-world flight routes.
*   **Maps & Navigation:** Turn addresses into geocodes, calculate complex routes, and even predict the sunny side vs the shaded side of your bus/car trip natively!
*   **Finance:** Check live real-time stock prices.
*   **Conversions & Metrics:** Fetch live global weather, perform complex mathematical calculations, convert currencies natively, check localized timezones, perform real-time text translations, and parse HEX/RGB color info.
*   **IT & Ops:** DNS lookups, IP tracking, Github issue/repository readers, hash and Base64 encode/decode, or evaluate complex RegEx strings.

### 📅 Productivity
*   **Timers & Scheduling:** Set one-off Pomodoro intervals, schedule custom future tasks periodically, fetch cron job statuses, or set up a Daily Digest generation.
*   **Notes & Task Tracking:** Operate a highly competent persistent To-Do system (Add, List, Done, Delete items) seamlessly through natural language commands, as well as updating complex "Notes" via file records.
*   **Media Generation:** Synthetically spoken MP3s from generated strings via TTS.

## Quick Start
Getting ApexClaw running takes only a few minutes. You can either compile it from source or download the pre-compiled binary.

### 1. Requirements
*   Go 1.22 or higher (if compiling from source)
*   A Telegram Bot Token (from `@BotFather`)
*   Telegram API ID and Hash (from `my.telegram.org`)
*   `ffmpeg` installed on your system (for Voice-to-Text conversion)

### 2. Environment Configuration
Create a `.env` file in the root of the project with your credentials:

```ini
# Core Configuration
TELEGRAM_BOT_TOKEN="your_bot_token"
TELEGRAM_API_ID=12345678
TELEGRAM_API_HASH="your_api_hash"
OWNER_ID=your_telegram_user_id

# AI Model Authentication (optional, by default uses anonymous access)
ZAI_TOKEN="your_zai_token"
```

### 3. Build & Run
If you want to run directly from source:
```bash
go run .
```

To compile a binary for production:
```bash
go build -o apexclaw .
./apexclaw
```

## Gmail Integration Setup
ApexClaw includes tools to read and send emails directly through your Telegram chat. To set up Gmail specifically, you must use a Google "App Password" rather than your standard account password, as standard login is blocked for basic IMAP/SMTP connections.

### 1. Generate an App Password
1. Go to your Google Account settings -> Security.
2. Ensure you have 2-Step Verification turned ON.
3. Search for "App Passwords" in the Security settings search bar.
4. Create a new app password (name it "ApexClaw" or similar). Google will provide a 16-character string.

### 2. Update Environment Variables
Add the following to your `.env` file, replacing the placeholder values with your Gmail address and the 16-character App Password you just generated (no spaces).

```ini
# Email configuration for Gmail
EMAIL_ADDRESS="your.email@gmail.com"
EMAIL_PASSWORD="your-app-password-no-spaces"

# IMAP for reading emails
EMAIL_IMAP_HOST="imap.gmail.com"
EMAIL_IMAP_PORT="993"

# SMTP for sending emails
EMAIL_SMTP_HOST="smtp.gmail.com"
EMAIL_SMTP_PORT="587"
```

Restart your bot after adding these variables. You can now prompt ApexClaw with requests like:
*   *"Read my 5 most recent emails"*
*   *"Send an email to john@example.com letting him know I will be late to the meeting"*

## PRs
PRs with more functionality are welcome!

## License
This project is open source and available for unrestricted use and modification. No attribution is required.
