package tools

import (
	"bufio"
	"crypto/tls"
	"fmt"
	"net"
	"net/smtp"
	"os"
	"strconv"
	"strings"
	"time"
)

type imapClient struct {
	conn   net.Conn
	reader *bufio.Reader
	seq    int
}

func dialIMAP(host, port string) (*imapClient, error) {
	addr := net.JoinHostPort(host, port)
	conn, err := tls.Dial("tcp", addr, &tls.Config{ServerName: host})
	if err != nil {
		return nil, err
	}
	c := &imapClient{conn: conn, reader: bufio.NewReader(conn)}

	if _, err := c.readline(); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *imapClient) send(cmd string) error {
	c.seq++
	line := fmt.Sprintf("A%04d %s\r\n", c.seq, cmd)
	_, err := c.conn.Write([]byte(line))
	return err
}

func (c *imapClient) readline() (string, error) {
	return c.reader.ReadString('\n')
}

func (c *imapClient) tag() string {
	return fmt.Sprintf("A%04d", c.seq)
}

func (c *imapClient) readUntilTagged() ([]string, error) {
	t := c.tag()
	var lines []string
	for {
		line, err := c.readline()
		if err != nil {
			return lines, err
		}
		line = strings.TrimRight(line, "\r\n")
		lines = append(lines, line)
		if strings.HasPrefix(line, t+" ") {
			break
		}
	}
	return lines, nil
}

func (c *imapClient) login(user, pass string) error {
	if err := c.send(fmt.Sprintf("LOGIN %q %q", user, pass)); err != nil {
		return err
	}
	lines, err := c.readUntilTagged()
	if err != nil {
		return err
	}
	for _, l := range lines {
		if strings.HasPrefix(l, c.tag()+" NO") || strings.HasPrefix(l, c.tag()+" BAD") {
			return fmt.Errorf("login failed: %s", l)
		}
	}
	return nil
}

func (c *imapClient) selectFolder(folder string) (int, error) {
	if err := c.send(fmt.Sprintf("SELECT %q", folder)); err != nil {
		return 0, err
	}
	lines, err := c.readUntilTagged()
	if err != nil {
		return 0, err
	}
	exists := 0
	for _, l := range lines {
		if strings.Contains(l, " EXISTS") {
			fmt.Sscanf(l, "* %d EXISTS", &exists)
		}
	}
	return exists, nil
}

func (c *imapClient) fetchHeaders(seqRange string) ([]map[string]string, error) {
	if err := c.send(fmt.Sprintf("FETCH %s (FLAGS ENVELOPE)", seqRange)); err != nil {
		return nil, err
	}
	lines, err := c.readUntilTagged()
	if err != nil {
		return nil, err
	}

	var results []map[string]string
	for _, l := range lines {
		if !strings.HasPrefix(l, "* ") || !strings.Contains(l, "FETCH") {
			continue
		}
		m := map[string]string{"raw": l}

		if idx := strings.Index(l, "ENVELOPE ("); idx != -1 {
			env := l[idx+10:]

			parts := parseIMAPEnvelope(env)
			if len(parts) >= 1 {
				m["date"] = parts[0]
			}
			if len(parts) >= 2 {
				m["subject"] = decodeIMAPString(parts[1])
			}
			if len(parts) >= 3 {
				m["from"] = parseIMAPAddress(parts[2])
			}
		}

		if strings.Contains(l, "\\Seen") {
			m["seen"] = "true"
		}
		results = append(results, m)
	}
	return results, nil
}

func parseIMAPEnvelope(s string) []string {

	var parts []string
	s = strings.TrimPrefix(s, "(")
	i := 0
	for i < len(s) {
		switch s[i] {
		case '"':

			j := i + 1
			for j < len(s) {
				if s[j] == '\\' {
					j += 2
					continue
				}
				if s[j] == '"' {
					break
				}
				j++
			}
			parts = append(parts, s[i+1:j])
			i = j + 1
		case '(':

			depth := 1
			j := i + 1
			for j < len(s) && depth > 0 {
				switch s[j] {
				case '(':
					depth++
				case ')':
					depth--
				}
				j++
			}
			parts = append(parts, s[i:j])
			i = j
		case 'N', 'n':

			if i+3 <= len(s) && strings.EqualFold(s[i:i+3], "NIL") {
				parts = append(parts, "")
				i += 3
			} else {
				i++
			}
		case ' ':
			i++
		default:
			i++
		}
	}
	return parts
}

func parseIMAPAddress(s string) string {

	s = strings.Trim(s, "()")
	parts := parseIMAPEnvelope(s)
	name := ""
	if len(parts) > 0 {
		name = parts[0]
	}
	user := ""
	if len(parts) > 2 {
		user = parts[2]
	}
	host := ""
	if len(parts) > 3 {
		host = parts[3]
	}
	email := ""
	if user != "" && host != "" {
		email = user + "@" + host
	}
	if name != "" && email != "" {
		return name + " <" + email + ">"
	}
	if email != "" {
		return email
	}
	return name
}

func decodeIMAPString(s string) string {

	return s
}

func (c *imapClient) close() {
	_ = c.send("LOGOUT")
	c.conn.Close()
}

var ReadEmail = &ToolDef{
	Name: "read_email",
	Description: "Read recent emails from an inbox. Requires env vars: EMAIL_IMAP_HOST, EMAIL_IMAP_PORT (default 993), EMAIL_ADDRESS, EMAIL_PASSWORD. " +
		"Returns subject, sender, and date of each email.",
	Secure: true,
	Args: []ToolArg{
		{Name: "count", Description: "Number of recent emails to fetch (default 5, max 20)", Required: false},
		{Name: "folder", Description: "Mailbox folder to read (default 'INBOX')", Required: false},
	},
	Execute: func(args map[string]string) string {
		host := os.Getenv("EMAIL_IMAP_HOST")
		if host == "" {
			return "Error: EMAIL_IMAP_HOST environment variable not set"
		}
		port := os.Getenv("EMAIL_IMAP_PORT")
		if port == "" {
			port = "993"
		}
		addr := os.Getenv("EMAIL_ADDRESS")
		pass := os.Getenv("EMAIL_PASSWORD")
		if addr == "" || pass == "" {
			return "Error: EMAIL_ADDRESS and EMAIL_PASSWORD must be set"
		}

		count := 5
		if v := strings.TrimSpace(args["count"]); v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				count = n
				if count > 20 {
					count = 20
				}
			}
		}
		folder := strings.TrimSpace(args["folder"])
		if folder == "" {
			folder = "INBOX"
		}

		c, err := dialIMAP(host, port)
		if err != nil {
			return fmt.Sprintf("Error connecting to %s:%s — %v", host, port, err)
		}
		defer c.close()

		if err := c.login(addr, pass); err != nil {
			return fmt.Sprintf("Login failed: %v", err)
		}

		exists, err := c.selectFolder(folder)
		if err != nil {
			return fmt.Sprintf("Error selecting %s: %v", folder, err)
		}
		if exists == 0 {
			return "Inbox is empty."
		}

		start := max(exists-count+1, 1)
		seqRange := fmt.Sprintf("%d:%d", start, exists)

		headers, err := c.fetchHeaders(seqRange)
		if err != nil {
			return fmt.Sprintf("Error fetching: %v", err)
		}
		if len(headers) == 0 {
			return "No emails fetched."
		}

		for i, j := 0, len(headers)-1; i < j; i, j = i+1, j-1 {
			headers[i], headers[j] = headers[j], headers[i]
		}

		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("📬 Last %d email(s) in %s:\n\n", len(headers), folder))
		for i, h := range headers {
			subj := h["subject"]
			if subj == "" {
				subj = "(no subject)"
			}
			from := h["from"]
			if from == "" {
				from = "unknown"
			}
			date := h["date"]
			seen := ""
			if h["seen"] != "true" {
				seen = " 🔵"
			}
			sb.WriteString(fmt.Sprintf("%d.%s %s\n   From: %s\n   Date: %s\n\n", i+1, seen, subj, from, date))
		}
		return strings.TrimRight(sb.String(), "\n")
	},
}

var SendEmail = &ToolDef{
	Name:        "send_email",
	Description: "Send an email via SMTP. Requires env vars: EMAIL_SMTP_HOST, EMAIL_SMTP_PORT (default 587), EMAIL_ADDRESS, EMAIL_PASSWORD.",
	Secure:      true,
	Args: []ToolArg{
		{Name: "to", Description: "Recipient email address", Required: true},
		{Name: "subject", Description: "Email subject line", Required: true},
		{Name: "body", Description: "Email body (plain text)", Required: true},
		{Name: "cc", Description: "Optional CC address(es), comma-separated", Required: false},
	},
	Execute: func(args map[string]string) string {
		host := os.Getenv("EMAIL_SMTP_HOST")
		if host == "" {
			return "Error: EMAIL_SMTP_HOST environment variable not set"
		}
		port := os.Getenv("EMAIL_SMTP_PORT")
		if port == "" {
			port = "587"
		}
		from := os.Getenv("EMAIL_ADDRESS")
		pass := os.Getenv("EMAIL_PASSWORD")
		if from == "" || pass == "" {
			return "Error: EMAIL_ADDRESS and EMAIL_PASSWORD must be set"
		}

		to := strings.TrimSpace(args["to"])
		subject := strings.TrimSpace(args["subject"])
		body := strings.TrimSpace(args["body"])
		cc := strings.TrimSpace(args["cc"])

		if to == "" || subject == "" || body == "" {
			return "Error: to, subject, and body are required"
		}

		var msgBuilder strings.Builder
		msgBuilder.WriteString("From: " + from + "\r\n")
		msgBuilder.WriteString("To: " + to + "\r\n")
		if cc != "" {
			msgBuilder.WriteString("Cc: " + cc + "\r\n")
		}
		msgBuilder.WriteString("Subject: " + subject + "\r\n")
		msgBuilder.WriteString("Date: " + time.Now().Format(time.RFC1123Z) + "\r\n")
		msgBuilder.WriteString("MIME-Version: 1.0\r\n")
		msgBuilder.WriteString("Content-Type: text/plain; charset=UTF-8\r\n")
		msgBuilder.WriteString("\r\n")
		msgBuilder.WriteString(body)

		auth := smtp.PlainAuth("", from, pass, host)
		toList := []string{to}
		if cc != "" {
			for _, a := range strings.Split(cc, ",") {
				if a = strings.TrimSpace(a); a != "" {
					toList = append(toList, a)
				}
			}
		}

		smtpAddr := net.JoinHostPort(host, port)
		if err := smtp.SendMail(smtpAddr, auth, from, toList, []byte(msgBuilder.String())); err != nil {
			return fmt.Sprintf("Error sending email: %v", err)
		}
		return fmt.Sprintf("✉️ Email sent to %s — Subject: %q", to, subject)
	},
}
