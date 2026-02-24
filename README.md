# ApexClaw

ApexClaw is a soft but powerful AI companion that lives inside your Telegram. Using the Z.AI engine, it thinks, acts, and manages your digital life with total autonomy. It is designed to be your friendly command center, taking care of the heavy lifting so you can focus on being you.

> **Current State:** v0.1 | **Tool Count:** 85 Functional Tools | **Language:** Go

### Key Capabilities

**Vision and Voice**
*   **Visual Intelligence.** Send a photo and ApexClaw will instantly tell you what is happening in the image.
*   **Natural Voice Messages.** Talk to ApexClaw in DMs or groups (just reply to the bot!). It transcribes your voice via a free Google pipeline and acts on your commands immediately.
*   **Speech Output.** ApexClaw can speak back to you using high-quality synthesized voices for a truly hands-free experience.

**Web and Browser Control**
*   **Chrome Engine Native.** ApexClaw uses a real browser to click buttons, type text, and navigate sites precisely.
*   **Content Discovery.** Integrated support for YouTube, Reddit, Pinterest, Wikipedia, and Google News.
*   **Advanced Research.** Browses the web using DuckDuckGo and GitHub to find specific code or information for you.

**Communication and Productivity**
*   **Complete Email Control.** Manage your Gmail or any IMAP/SMTP inbox. ApexClaw can read your mail, summarize threads, and send replies.
*   **Telegram Superpowers.** It can pin messages, manage group members, react to posts, and download media to its own local storage.
*   **Memory and To-Dos.** It builds a persistent knowledge base of your facts and manages a full-featured To-Do system.

---

### Expanding the Toolkit

One of the best things about ApexClaw is how friendly it is to developers. Adding a new tool is as simple as creating a small Go struct. You don't need to worry about complex API wiring—just define what your tool needs and what it does.

If you have an idea for a tool that helps with your workflow, you can drop it in and ApexClaw will start using it immediately to solve problems.

---

### The Roadmap

Here is what is planned for the future of ApexClaw:

- [ ] **Web Dashboard.** A beautiful browser interface to configure your bot, view logs, and manage tools visually.
- [ ] **More LLM Providers.** Expansion to support local models and other major AI providers.
- [ ] **Vector Memory.** Advanced long-term memory using vector embeddings for even better fact retrieval.
- [ ] **Plugin System.** A community-driven way to share and install new tools instantly.
- [ ] **Voice-to-Voice.** Real-time audio conversations without needing to click "Send."

---

### Quick Start

**Required Ingredients**
*   Go 1.22+ installed on your machine.
*   ffmpeg on your system path (needed for voice processing).
*   Telegram API ID and Hash from my.telegram.org.
*   Telegram Bot Token from @BotFather.

**Setup**
Create a `.env` file with your details:
```ini
TELEGRAM_BOT_TOKEN="your_token"
TELEGRAM_API_ID=123456
TELEGRAM_API_HASH="your_hash"
OWNER_ID=your_telegram_id

# Optional Token
ZAI_TOKEN="your_token"
```

**Running**
```bash
go run .
```

---

### License

ApexClaw is released under the **MIT License**. You are free to use, modify, and distribute it however you like.
