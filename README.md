# 🐾 Apexclaw

<p align="center">
  <img src="https://img.shields.io/github/stars/amarnathcjd/apexclaw?style=flat-square&color=pink" alt="stars">
  <img src="https://img.shields.io/github/forks/amarnathcjd/apexclaw?style=flat-square&color=purple" alt="forks">
  <img src="https://img.shields.io/github/downloads/amarnathcjd/apexclaw/total?style=flat-square&color=cyan" alt="downloads">
</p>

A personal AI assistant that lives inside your Telegram. Powered by the z.ai engine, it can think, use tools, and actually get things done — not just talk about them.

### Why you'll love it

Instead of a chatbot that just replies, Apexclaw is more like a capable assistant that acts. You can:

- 🖼️ Send it a photo and ask what's in it
- 🎙️ Reply with a voice note and it will transcribe and act on what you say
- 🗣️ Have it talk back with text-to-speech
- 🌐 Ask it to browse the web, click links, and read articles using a real headless browser
- 📧 Tell it to read your Gmail or send emails for you
- 📝 Ask it to remember facts and notes across sessions
- 🎬 Fetch IMDB info, download Instagram reels, search YouTube
- 🛠️ Run Python scripts, check flights, grab weather, convert timezones, track stocks, and much more

It comes loaded with **94 tools** ready to use.

---

### All tools

#### System & execution
| Tool | What it does |
|---|---|
| `exec` | Run a shell command |
| `run_python` | Execute a Python script |
| `system_info` | Get CPU, RAM, disk info |
| `process_list` | List running processes |
| `kill_process` | Kill a process by PID |
| `clipboard_get` | Read the clipboard |
| `clipboard_set` | Write to the clipboard |

#### Files
| Tool | What it does |
|---|---|
| `read_file` | Read a file's contents |
| `write_file` | Write content to a file |
| `append_file` | Append content to a file |
| `list_dir` | List a directory's contents |
| `create_dir` | Create a new directory |
| `delete_file` | Delete a file |
| `move_file` | Move or rename a file |
| `search_files` | Find files by name pattern |

#### Memory & notes
| Tool | What it does |
|---|---|
| `save_fact` | Persist a key-value fact |
| `recall_fact` | Retrieve a saved fact |
| `list_facts` | List all saved facts |
| `delete_fact` | Delete a saved fact |
| `update_note` | Create or overwrite a named note |

#### Web & search
| Tool | What it does |
|---|---|
| `web_fetch` | Fetch and read a webpage |
| `web_search` | Search the web |
| `http_request` | Make a raw HTTP request |
| `rss_feed` | Pull and parse an RSS feed |
| `wikipedia` | Search and read Wikipedia |
| `news_headlines` | Get live news headlines |
| `reddit_feed` | Read a subreddit's top posts |
| `youtube_search` | Search YouTube for videos |

#### IMDB
| Tool | What it does |
|---|---|
| `imdb_search` | Search movies, shows, and actors |
| `imdb_title` | Get detailed info about a title by IMDB ID |

#### Browser automation
| Tool | What it does |
|---|---|
| `browser_open` | Open a URL in headless Chrome |
| `browser_click` | Click an element by CSS selector |
| `browser_type` | Type text into an input |
| `browser_get_text` | Extract text from the page |
| `browser_eval` | Run JavaScript on the page |
| `browser_screenshot` | Take a screenshot of the page |

#### GitHub
| Tool | What it does |
|---|---|
| `github_search` | Search GitHub repositories |
| `github_read_file` | Read a file from a GitHub repo |

#### Scheduling
| Tool | What it does |
|---|---|
| `schedule_task` | Schedule a one-off or repeating task |
| `cancel_task` | Cancel a scheduled task |
| `list_tasks` | List all scheduled tasks |

#### Travel & navigation
| Tool | What it does |
|---|---|
| `flight_airport_search` | Look up airport info |
| `flight_route_search` | Search flight routes |
| `flight_countries` | List supported countries |
| `nav_geocode` | Geocode an address to coordinates |
| `nav_route` | Get directions between two points |
| `nav_sunshade` | Calculate sun shading for a drive |

#### Utilities
| Tool | What it does |
|---|---|
| `datetime` | Get the current date and time |
| `timer` | Set a countdown timer |
| `echo` | Echo back a message |
| `calculate` | Evaluate a math expression |
| `random` | Generate a random number |
| `text_process` | Trim, split, replace, or transform text |
| `hash_text` | Hash a string (md5, sha256, etc.) |
| `encode_decode` | Base64 encode or decode |
| `regex_match` | Test a regex pattern against text |
| `color_info` | Get info about a hex or RGB color |

#### Network & data
| Tool | What it does |
|---|---|
| `weather` | Get live weather for a location |
| `ip_lookup` | Look up info about an IP address |
| `dns_lookup` | Resolve a domain's DNS records |
| `stock_price` | Get a live stock price |
| `currency_convert` | Convert between currencies |
| `unit_convert` | Convert units (length, weight, temp, etc.) |
| `timezone_convert` | Convert a time between timezones |
| `translate` | Translate text to another language |

#### Telegram
| Tool | What it does |
|---|---|
| `tg_send_message` | Send a text message |
| `tg_send_file` | Send a file or photo |
| `tg_send_message_buttons` | Send a message with inline buttons |
| `tg_download` | Download a Telegram media file |
| `tg_get_chat_info` | Get info about a chat or user |
| `tg_forward` | Forward a message to another chat |
| `tg_delete_msg` | Delete a message |
| `tg_pin_msg` | Pin a message in a chat |
| `tg_react` | React to a message with an emoji |
| `tg_get_reply` | Get the message being replied to |
| `set_bot_dp` | Update the bot's profile picture |

#### Email & voice
| Tool | What it does |
|---|---|
| `read_email` | Read emails from Gmail |
| `send_email` | Send an email |
| `text_to_speech` | Convert text to a voice note |

#### Productivity
| Tool | What it does |
|---|---|
| `todo_add` | Add a to-do item |
| `todo_list` | List all to-do items |
| `todo_done` | Mark a to-do item as done |
| `todo_delete` | Delete a to-do item |
| `pomodoro` | Start a Pomodoro focus session |
| `daily_digest` | Set up a daily briefing |
| `cron_status` | Check scheduled task status |

#### Downloads
| Tool | What it does |
|---|---|
| `download_ytdlp` | Download video/audio via yt-dlp |
| `download_aria2c` | Download files via aria2c |

#### Documents
| Tool | What it does |
|---|---|
| `read_document` | Read a stored document |
| `list_documents` | List all stored documents |
| `summarize_document` | Summarize a document |

#### Social
| Tool | What it does |
|---|---|
| `pinterest_search` | Search Pinterest boards and pins |
| `pinterest_get_pin` | Get details about a Pinterest pin |

---

### Quick start

#### One-line install (Linux/macOS)

```bash
curl -fsSL https://claw.gogram.fun | bash
apexclaw
```

This will:
1. Download the latest binary for your OS/architecture
2. Install to `/usr/local/bin` or `~/.local/bin`
3. Launch an interactive setup wizard on first run
4. Ask for your Telegram credentials
5. Save to `.env` and start

#### Manual setup

1. You need: `go 1.22+`, `ffmpeg` (for voice)
2. Clone and build:

```bash
git clone https://github.com/amarnathcjd/apexclaw
cd apexclaw
go build -o apexclaw .
./apexclaw
```

3. On first run, you'll be prompted for:
   - Telegram API ID
   - Telegram API Hash
   - Telegram Bot Token
   - Owner ID (your Telegram Chat ID)

---

### Gmail setup

Apexclaw can read and send emails. Since regular passwords don't work for bots, you need a Google app password:

1. Go to your Google account → Security
2. Make sure 2-step verification is on
3. Search for "App passwords"
4. Create one named "apexclaw" and copy the 16-character password
5. Add to `.env`:

```ini
EMAIL_ADDRESS="your.email@gmail.com"
EMAIL_PASSWORD="your-16-char-app-password"
EMAIL_IMAP_HOST="imap.gmail.com"
EMAIL_IMAP_PORT="993"
EMAIL_SMTP_HOST="smtp.gmail.com"
EMAIL_SMTP_PORT="587"
```

Restart and ask it to *"read my last 3 emails"*.

---

### Adding your own tools

Drop a new `ToolDef` struct into any file in the `tools/` directory and register it in `tools/tools.go`. Apexclaw will pick it up automatically.

---

### Planned features

- [ ] Web dashboard for settings
- [ ] Support for other AI models
- [ ] Vector memory for long-term recall
- [ ] Plugin system for third-party tools

---

### License

MIT
