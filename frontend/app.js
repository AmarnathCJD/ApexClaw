document.addEventListener('DOMContentLoaded', () => {
    const authOverlay = document.getElementById('auth-overlay');
    const mainApp = document.getElementById('main-app');
    const authBtn = document.getElementById('auth-btn');
    const codeInput = document.getElementById('code-input');
    const authError = document.getElementById('auth-error');

    let accessToken = null;  // Stored in memory
    let refreshTokenID = null;  // Stored in httpOnly cookie

    // Try to restore session on page load
    restoreSession();

    authBtn.addEventListener('click', async () => {
        const code = codeInput.value.trim();
        if (!code) return;

        authBtn.disabled = true;
        const originalText = authBtn.textContent;
        authBtn.textContent = "Signing In...";

        try {
            const res = await fetch('/api/auth/login', {
                method: 'POST',
                headers: { 'Content-Type': 'application/json' },
                body: JSON.stringify({ code })
            });
            if (res.ok) {
                const data = await res.json();
                accessToken = data.accessToken;
                refreshTokenID = data.refreshToken;

                if (data.isFirstTime) {
                    // Show code change modal
                    unlockApp();
                    showChangeCodeModal();
                } else {
                    unlockApp();
                }
            } else {
                authError.textContent = "Invalid code. Please try again.";
                authBtn.disabled = false;
                authBtn.textContent = originalText;
            }
        } catch (e) {
            authError.textContent = "Connection error.";
            authBtn.disabled = false;
            authBtn.textContent = originalText;
        }
    });

    async function restoreSession() {
        try {
            const res = await fetch('/api/auth/refresh', {
                method: 'POST',
                credentials: 'include'  // Include httpOnly cookies
            });
            if (res.ok) {
                const data = await res.json();
                accessToken = data.accessToken;
                unlockApp();
            }
        } catch (e) {
            // Session not available, show login
        }
    }

    function unlockApp() {
        authOverlay.classList.add('hidden');
        mainApp.classList.remove('hidden');
    }

    function forceLogout() {
        accessToken = null;
        refreshTokenID = null;
        location.reload();
    }

    // ===== Change Code Modal =====
    const changeCodeModal = document.getElementById('change-code-modal');
    const changeCodeBtn = document.getElementById('change-code-btn');
    const changeCodeInput = document.getElementById('new-code-input');
    const changeCodeSubmit = document.getElementById('change-code-submit');
    const changeCodeSkip = document.getElementById('change-code-skip');
    const changeCodeError = document.getElementById('change-code-error');

    changeCodeSubmit.addEventListener('click', async () => {
        const newCode = changeCodeInput.value.trim();
        if (!/^\d{6}$/.test(newCode)) {
            changeCodeError.textContent = "Code must be 6 digits.";
            changeCodeError.style.color = "#d32f2f";
            return;
        }

        changeCodeSubmit.disabled = true;
        changeCodeSubmit.textContent = "Updating...";

        try {
            const res = await fetch('/api/auth/change-code', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + accessToken
                },
                body: JSON.stringify({ newCode })
            });
            if (res.ok) {
                changeCodeError.style.color = "#4caf50";
                changeCodeError.textContent = "✓ Code updated successfully";
                setTimeout(() => {
                    changeCodeModal.classList.add('hidden');
                    changeCodeInput.value = '';
                    changeCodeError.textContent = '';
                    changeCodeSubmit.disabled = false;
                    changeCodeSubmit.textContent = "Update Code";
                }, 1500);
            } else {
                changeCodeError.style.color = "#d32f2f";
                changeCodeError.textContent = "Failed to change code.";
                changeCodeSubmit.disabled = false;
                changeCodeSubmit.textContent = "Update Code";
            }
        } catch (e) {
            changeCodeError.style.color = "#d32f2f";
            changeCodeError.textContent = "Connection error.";
            changeCodeSubmit.disabled = false;
            changeCodeSubmit.textContent = "Update Code";
        }
    });

    changeCodeSkip.addEventListener('click', () => {
        changeCodeModal.classList.add('hidden');
    });

    function showChangeCodeModal() {
        changeCodeModal.classList.remove('hidden');
        changeCodeInput.focus();
    }

    // ===== Settings Panel =====
    const settingsBtn = document.getElementById('open-settings-btn');
    const settingsOverlay = document.getElementById('settings-overlay');
    const closeSettingsBtn = document.getElementById('close-settings-btn');
    const saveSettingsBtn = document.getElementById('save-settings-btn');
    const settingsForm = document.getElementById('settings-form');
    const settingsStatus = document.getElementById('settings-status');

    settingsBtn.addEventListener('click', async () => {
        settingsOverlay.classList.remove('hidden');
        settingsStatus.textContent = "Loading...";
        settingsForm.innerHTML = '';

        try {
            const res = await fetch('/api/settings', {
                headers: { 'Authorization': 'Bearer ' + accessToken }
            });
            if (!res.ok) {
                if (res.status === 401) {
                    await refreshAccessToken();
                    return settingsBtn.click();  // Retry
                }
                throw new Error("Failed to load configs.");
            }
            const data = await res.json();

            Object.keys(data).forEach(key => {
                const group = document.createElement('div');
                group.className = 'setting-group';

                const label = document.createElement('label');
                label.textContent = key;

                const input = document.createElement('input');
                input.type = 'text';
                input.value = data[key];
                input.dataset.key = key;

                group.appendChild(label);
                group.appendChild(input);
                settingsForm.appendChild(group);
            });
            settingsStatus.textContent = "";
        } catch (e) {
            settingsStatus.textContent = e.message;
        }
    });

    closeSettingsBtn.addEventListener('click', () => {
        settingsOverlay.classList.add('hidden');
    });

    saveSettingsBtn.addEventListener('click', async () => {
        settingsStatus.textContent = "Saving...";
        const newSettings = {};
        settingsForm.querySelectorAll('input').forEach(i => {
            newSettings[i.dataset.key] = i.value;
        });

        try {
            const res = await fetch('/api/settings', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + accessToken
                },
                body: JSON.stringify(newSettings)
            });
            if (!res.ok) {
                if (res.status === 401) {
                    await refreshAccessToken();
                    return saveSettingsBtn.click();  // Retry
                }
                throw new Error("Save failed");
            }
            settingsStatus.textContent = "Saved! Restart backend to apply.";
        } catch (e) {
            settingsStatus.textContent = e.message;
        }
    });

    // ===== Token Refresh =====
    async function refreshAccessToken() {
        try {
            const res = await fetch('/api/auth/refresh', {
                method: 'POST',
                credentials: 'include'
            });
            if (res.ok) {
                const data = await res.json();
                accessToken = data.accessToken;
                return true;
            }
        } catch (e) {
        }
        forceLogout();
        return false;
    }

    // ===== Chat Interface =====
    const chatInput = document.getElementById('chat-input');
    const sendBtn = document.getElementById('send-btn');
    const chatHistory = document.getElementById('chat-history');

    chatInput.addEventListener('input', function () {
        this.style.height = 'auto';
        this.style.height = (this.scrollHeight) + 'px';
        if (this.style.height === 'auto') {
            this.style.height = '24px';
        }

        if (this.value.trim() !== '') {
            sendBtn.removeAttribute('disabled');
            sendBtn.style.color = 'var(--bg-color)';
            sendBtn.style.background = 'var(--text-primary)';
        } else {
            sendBtn.setAttribute('disabled', 'true');
            sendBtn.style.color = 'var(--text-secondary)';
            sendBtn.style.background = 'transparent';
        }
    });

    chatInput.addEventListener('keydown', (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            if (chatInput.value.trim() !== '') {
                sendMessage();
            }
        }
    });

    sendBtn.addEventListener('click', () => {
        if (chatInput.value.trim() !== '') {
            sendMessage();
        }
    });

    marked.setOptions({
        breaks: true,
        highlight: function (code, lang) {
            if (lang && hljs.getLanguage(lang)) {
                return hljs.highlight(code, { language: lang }).value;
            }
            return hljs.highlightAuto(code).value;
        }
    });

    function createMessageElement(role, text) {
        const msgDiv = document.createElement('div');
        msgDiv.className = `message ${role}`;

        const sender = document.createElement('div');
        sender.className = 'message-sender';
        sender.textContent = role === 'user' ? 'You' : 'ApexClaw';

        const content = document.createElement('div');
        content.className = 'message-content';

        if (role === 'user') {
            const p = document.createElement('p');
            p.textContent = text;
            content.appendChild(p);
        } else {
            content.innerHTML = DOMPurify.sanitize(marked.parse(text));
        }

        msgDiv.appendChild(sender);
        msgDiv.appendChild(content);
        return msgDiv;
    }

    function insertToolBlock(container, toolName) {
        const block = document.createElement('div');
        block.className = `tool-block running tool-${toolName}`;
        block.innerHTML = `
            <div class="tool-icon">
                <div class="spinner"></div>
            </div>
            <div class="tool-text">Running tool '${toolName}'...</div>
        `;
        container.appendChild(block);
        scrollToBottom();
        return block;
    }

    function finishToolBlock(container, toolName) {
        const block = container.querySelector(`.tool-block.running.tool-${toolName}`);
        if (block) {
            block.classList.remove('running');
            block.innerHTML = `
                <div class="tool-icon">
                    <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="var(--accent-color)" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M22 11.08V12a10 10 0 1 1-5.93-9.14"></path><polyline points="22 4 12 14.01 9 11.01"></polyline></svg>
                </div>
                <div class="tool-text">Finished '${toolName}'</div>
            `;
            scrollToBottom();
        }
    }

    async function sendMessage() {
        const message = chatInput.value.trim();
        if (!message) return;

        const welcome = document.querySelector('.welcome-screen');
        if (welcome) welcome.style.display = 'none';

        const userMsg = createMessageElement('user', message);
        chatHistory.appendChild(userMsg);

        chatInput.value = '';
        chatInput.style.height = 'auto';
        sendBtn.setAttribute('disabled', 'true');
        sendBtn.style.color = 'var(--text-secondary)';
        sendBtn.style.background = 'transparent';

        const aiMsgDiv = document.createElement('div');
        aiMsgDiv.className = `message ai`;

        const sender = document.createElement('div');
        sender.className = 'message-sender';
        sender.textContent = 'ApexClaw';

        const content = document.createElement('div');
        content.className = 'message-content markdown-body';

        aiMsgDiv.appendChild(sender);
        aiMsgDiv.appendChild(content);
        chatHistory.appendChild(aiMsgDiv);
        scrollToBottom();

        let rawMarkdown = "";

        try {
            const response = await fetch('/api/chat', {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json',
                    'Authorization': 'Bearer ' + accessToken
                },
                body: JSON.stringify({
                    message: message,
                    user_id: ''
                })
            });

            if (!response.ok) {
                if (response.status === 401) {
                    if (await refreshAccessToken()) {
                        return sendMessage();  // Retry
                    }
                }
                throw new Error('Network error');
            }

            const reader = response.body.getReader();
            const decoder = new TextDecoder('utf-8');

            while (true) {
                const { value, done } = await reader.read();
                if (done) break;

                const chunk = decoder.decode(value, { stream: true });
                const lines = chunk.split('\n');

                for (const line of lines) {
                    if (line.startsWith('data: ')) {
                        const dataStr = line.replace('data: ', '');
                        if (dataStr === '[DONE]') break;
                        try {
                            const data = JSON.parse(dataStr);

                            if (data.type === 'tool_call') {
                                insertToolBlock(content, data.name);
                            } else if (data.type === 'tool_result') {
                                finishToolBlock(content, data.name);
                            } else if (data.type === 'chunk') {
                                rawMarkdown += data.chunk;

                                let textWrapper = content.querySelector('.markdown-text');
                                if (!textWrapper) {
                                    textWrapper = document.createElement('div');
                                    textWrapper.className = 'markdown-text';
                                    content.appendChild(textWrapper);
                                }
                                textWrapper.innerHTML = DOMPurify.sanitize(marked.parse(rawMarkdown));

                                textWrapper.querySelectorAll('pre code').forEach((block) => {
                                    hljs.highlightElement(block);
                                });
                                textWrapper.querySelectorAll('pre').forEach((pre) => {
                                    if (!pre.querySelector('.copy-btn')) {
                                        const btn = document.createElement('button');
                                        btn.className = 'copy-btn';
                                        btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><rect x="9" y="9" width="13" height="13" rx="2" ry="2"></rect><path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1"></path></svg> Copy';
                                        pre.appendChild(btn);

                                        btn.addEventListener('click', () => {
                                            const code = pre.querySelector('code');
                                            if (code) {
                                                navigator.clipboard.writeText(code.innerText).then(() => {
                                                    const originalHtml = btn.innerHTML;
                                                    btn.innerHTML = '<svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><polyline points="20 6 9 17 4 12"></polyline></svg> Copied!';
                                                    setTimeout(() => btn.innerHTML = originalHtml, 2000);
                                                });
                                            }
                                        });
                                    }
                                });
                                scrollToBottom();
                            } else if (data.type === 'error') {
                                const errDiv = document.createElement('div');
                                errDiv.style.color = "red";
                                errDiv.textContent = "**Error:** " + data.error;
                                content.appendChild(errDiv);
                                scrollToBottom();
                            }
                        } catch (e) {
                        }
                    }
                }
            }

        } catch (error) {
            console.error('Failed to get response:', error);
            const errP = document.createElement('p');
            errP.style.color = 'red';
            errP.textContent = 'Connection to backend lost.';
            content.appendChild(errP);
        }
    }

    function scrollToBottom() {
        window.scrollTo({
            top: document.body.scrollHeight,
            behavior: 'smooth'
        });
    }
});
