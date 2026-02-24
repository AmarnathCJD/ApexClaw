# 🐾 apexclaw

<p align="center">
  <img src="https://img.shields.io/github/stars/amarnathcjd/apexclaw?style=flat-square&color=pink" alt="stars">
  <img src="https://img.shields.io/github/forks/amarnathcjd/apexclaw?style=flat-square&color=purple" alt="forks">
  <img src="https://img.shields.io/github/downloads/amarnathcjd/apexclaw/total?style=flat-square&color=cyan" alt="downloads">
</p>

hi there! welcome to apexclaw 🌸 it's a soft but super capable ai companion that lives right inside your telegram. 
inspired by openclaw, it uses the z.ai engine to think, act, and make your digital life so much easier ✨

### 🎈 why you'll love it
instead of just an ai that talks, think of apexclaw as a tiny friend who actually does chores for you! you can:
- 🖼️ send it a photo and ask what's in it
- 🎙️ reply with a voice note and it will transcribe and do what you say (using free google voice-to-text!)
- 🗣️ have it talk back to you with text-to-speech
- 🌐 ask it to browse the web, click links, and read articles using a real headless browser
- 📧 tell it to read your latest gmail emails or send one to a friend
- 📝 ask it to remember your favorite coffee order for next time
- 🛠️ make it run python scripts, check flights, grab weather, convert timezones, track stocks, and way more!

it comes loaded with over 85 little tools to help out every day 🎀

### 🎀 all features & tools
apexclaw has a massive collection of 85+ built-in tools. here is everything it can do for you right now:

**💻 system & files**
running custom shell/python scripts, reading/writing files, creating/listing folders, fetching system info, managing processes, tracking system clipboard

**🌐 web & discovery**
fetching web pages, deep web searching, fully interacting with headless chrome (clicking, typing, screenshots, evaluating javascript), reading wikipedia, searching github repos, live news headlines, real-time reddit feeds, youtube search, and pulling pinterest boards

**✉️ chatting & mail**
reading incoming gmail, sending outbound email, sending and downloading telegram files, forwarding/pinning/deleting telegram messages, automatically updating its own profile picture, pulling group info, and deep-reading replies

**📆 tracking & life**
saving and recalling persistent facts, managing dynamic notes, running complex crontab scheduled tasks, creating one-off timers, building a complete to-do system, managing pomodoro study sessions, and setting up daily digests

**🌍 world utility**
live global flight tracking, airport info lookup, exact geocoding and live route planning, real-world sun shading calculation for drives, live weather mapping, real-time stock prices, translating languages, doing currency and timezone conversions, checking hex/rgb colors

**⚙️ misc & tools**
evaluating complex math, regex pattern matching, generating strong hash strings, base64 encoding/decoding, checking dns/ip data, grabbing raw http resources, pulling rss feeds, and sending voice notes via text-to-speech

### �🍼 quick start
getting your own apexclaw is super simple!

1. stuff you need: `go 1.22+`, `ffmpeg` (for voice notes), and your telegram api keys.
2. set up your `.env` file like this:
```ini
TELEGRAM_BOT_TOKEN="your_bot_token"
TELEGRAM_API_ID=123456
TELEGRAM_API_HASH="your_hash"
OWNER_ID=your_id # so it only listens to you!

# optional if you have a token
ZAI_TOKEN="your_token"
```

3. fire it up!
```bash
go run .
```

### 💌 setup: gmail integration
apexclaw can read and send emails right from your chat! since normal passwords don't work for bots, you just need a special google "app password".

1. go to your google account settings ➔ security.
2. make sure **2-step verification** is turned on.
3. search for "app passwords".
4. create a new one, name it "apexclaw", and copy the 16-letter password it gives you!
5. add these lines to your `.env` file:
```ini
EMAIL_ADDRESS="your.email@gmail.com"
EMAIL_PASSWORD="your-new-16-letter-password"

EMAIL_IMAP_HOST="imap.gmail.com"
EMAIL_IMAP_PORT="993"

EMAIL_SMTP_HOST="smtp.gmail.com"
EMAIL_SMTP_PORT="587"
```
restart the bot and ask it to *"read my last 3 emails!"* ✨

### 🧩 making it yours
want it to do something new? adding tools is so easy. just write a tiny go struct and drop it in! apexclaw will figure out how to use it instantly. 

### 🌱 what we're planning
- [ ] a pretty web dashboard for settings 
- [ ] connect other ai models 
- [ ] deep vector memory so it remembers everything 
- [ ] an easy plugin system for tools!

### 📜 license
all yours to play with under the mit license 💖
