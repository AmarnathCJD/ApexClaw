package tools

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"
)

// ---------- ArxivSearch ----------

// arxivAtomFeed mirrors the subset of the arXiv Atom response we care about.
type arxivAtomFeed struct {
	XMLName xml.Name     `xml:"feed"`
	Entries []arxivEntry `xml:"entry"`
}

type arxivEntry struct {
	Title     string        `xml:"title"`
	Summary   string        `xml:"summary"`
	Published string        `xml:"published"`
	ID        string        `xml:"id"`
	Authors   []arxivAuthor `xml:"author"`
}

type arxivAuthor struct {
	Name string `xml:"name"`
}

var ArxivSearch = &ToolDef{
	Name:        "arxiv_search",
	Description: "Search arXiv (academic paper preprint server) for papers matching a query. Returns title, authors, abstract excerpt, publication date, and arXiv URL. Use for ML/physics/math/CS research.",
	MaxOutput:   16 * 1024,
	Timeout:     30 * time.Second,
	Args: []ToolArg{
		{Name: "query", Type: ArgString, Description: "Search query (matches title, abstract, authors).", Required: true},
		{Name: "max_results", Type: ArgInt, Description: "How many results to return (default 5, max 20).", Required: false, Default: 5},
		{Name: "sort_by", Type: ArgString, Description: "Sort order: relevance, lastUpdatedDate, submittedDate.", Required: false, Default: "relevance", Enum: []string{"relevance", "lastUpdatedDate", "submittedDate"}},
	},
	Execute: func(args map[string]any) string {
		query := String(args, "query")
		if query == "" {
			return "Error: query is required"
		}
		maxResults := IntOr(args, "max_results", 5)
		if maxResults < 1 {
			maxResults = 1
		}
		if maxResults > 20 {
			maxResults = 20
		}
		sortBy := StringOr(args, "sort_by", "relevance")
		switch sortBy {
		case "relevance", "lastUpdatedDate", "submittedDate":
		default:
			sortBy = "relevance"
		}

		apiURL := fmt.Sprintf(
			"http://export.arxiv.org/api/query?search_query=all:%s&start=0&max_results=%d&sortBy=%s",
			url.QueryEscape(query),
			maxResults,
			sortBy,
		)

		if err := ValidateExternalURL(apiURL); err != nil {
			return fmt.Sprintf("Error: %v", err)
		}

		client := &http.Client{Timeout: 20 * time.Second}
		req, err := http.NewRequest("GET", apiURL, nil)
		if err != nil {
			return fmt.Sprintf("Error building request: %v", err)
		}
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching arXiv: %v", err)
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(io.LimitReader(resp.Body, 512*1024))
		if err != nil {
			return fmt.Sprintf("Error reading arXiv response: %v", err)
		}

		var feed arxivAtomFeed
		if err := xml.Unmarshal(body, &feed); err != nil {
			return fmt.Sprintf("Error parsing arXiv XML: %v", err)
		}

		if len(feed.Entries) == 0 {
			return fmt.Sprintf("No arXiv results for %q.", query)
		}

		var sb strings.Builder
		fmt.Fprintf(&sb, "arXiv search: %q (sorted by %s)\n\n", query, sortBy)

		for i, entry := range feed.Entries {
			title := strings.TrimSpace(collapseSpaces(entry.Title))
			authors := make([]string, 0, len(entry.Authors))
			for _, a := range entry.Authors {
				name := strings.TrimSpace(a.Name)
				if name != "" {
					authors = append(authors, name)
				}
			}
			published := strings.TrimSpace(entry.Published)
			if len(published) >= 10 {
				published = published[:10]
			}
			link := strings.TrimSpace(entry.ID)
			abstract := strings.TrimSpace(collapseSpaces(entry.Summary))
			if len(abstract) > 400 {
				abstract = abstract[:400] + "..."
			}

			fmt.Fprintf(&sb, "[%d] %s\n", i+1, title)
			fmt.Fprintf(&sb, "Authors: %s\n", strings.Join(authors, ", "))
			fmt.Fprintf(&sb, "Published: %s\n", published)
			fmt.Fprintf(&sb, "URL: %s\n", link)
			fmt.Fprintf(&sb, "Abstract: %s\n", abstract)
			if i < len(feed.Entries)-1 {
				sb.WriteString("\n")
			}
		}

		return strings.TrimRight(sb.String(), "\n")
	},
}

// collapseSpaces flattens any sequence of whitespace (including newlines)
// into a single space so abstracts/titles read cleanly on one line.
func collapseSpaces(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// ---------- HackerNewsTop ----------

type hnItem struct {
	ID          int    `json:"id"`
	Title       string `json:"title"`
	URL         string `json:"url"`
	Score       int    `json:"score"`
	Descendants int    `json:"descendants"`
	By          string `json:"by"`
	Type        string `json:"type"`
	Time        int64  `json:"time"`

	rank int // original position in topstories list, for stable ordering
}

var HackerNewsTop = &ToolDef{
	Name:        "hackernews_top",
	Description: "Fetch the top N HackerNews stories with title, URL, score, comments count, and author. Use for tech industry news.",
	MaxOutput:   12 * 1024,
	Timeout:     30 * time.Second,
	Args: []ToolArg{
		{Name: "count", Type: ArgInt, Description: "How many stories to return (default 10, max 30).", Required: false, Default: 10},
		{Name: "include_self_posts", Type: ArgBool, Description: "Include Ask HN / Show HN style self-posts (no URL). Default true.", Required: false, Default: true},
	},
	Execute: func(args map[string]any) string {
		count := IntOr(args, "count", 10)
		if count < 1 {
			count = 1
		}
		if count > 30 {
			count = 30
		}
		includeSelf := BoolOr(args, "include_self_posts", true)

		client := &http.Client{Timeout: 20 * time.Second}

		// 1. Fetch the topstories index.
		idsURL := "https://hacker-news.firebaseio.com/v0/topstories.json"
		req, err := http.NewRequest("GET", idsURL, nil)
		if err != nil {
			return fmt.Sprintf("Error building request: %v", err)
		}
		req.Header.Set("User-Agent", "ApexClaw/1.0")
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Sprintf("Error fetching HN topstories: %v", err)
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return fmt.Sprintf("Error reading HN topstories: %v", err)
		}

		var ids []int
		if err := json.Unmarshal(body, &ids); err != nil {
			return fmt.Sprintf("Error parsing HN topstories: %v", err)
		}
		if len(ids) == 0 {
			return "No HN top stories returned."
		}

		// 2. Decide how many candidates to pull (count*1.5, capped at len(ids)).
		want := int(float64(count) * 1.5)
		if want < count {
			want = count
		}
		if want > len(ids) {
			want = len(ids)
		}
		candidates := ids[:want]

		// 3. Fan-out fetch, capped at 8 concurrent.
		const maxConcurrent = 8
		sem := make(chan struct{}, maxConcurrent)
		var wg sync.WaitGroup
		var mu sync.Mutex
		items := make([]hnItem, 0, want)

		for rank, id := range candidates {
			wg.Add(1)
			sem <- struct{}{}
			go func(rank, id int) {
				defer wg.Done()
				defer func() { <-sem }()

				itemURL := fmt.Sprintf("https://hacker-news.firebaseio.com/v0/item/%d.json", id)
				ireq, err := http.NewRequest("GET", itemURL, nil)
				if err != nil {
					return
				}
				ireq.Header.Set("User-Agent", "ApexClaw/1.0")
				iresp, err := client.Do(ireq)
				if err != nil {
					return
				}
				defer iresp.Body.Close()
				ibody, err := io.ReadAll(io.LimitReader(iresp.Body, 256*1024))
				if err != nil {
					return
				}
				var item hnItem
				if err := json.Unmarshal(ibody, &item); err != nil {
					return
				}
				item.rank = rank
				mu.Lock()
				items = append(items, item)
				mu.Unlock()
			}(rank, id)
		}
		wg.Wait()

		// Restore original topstories order (goroutines complete out of order).
		sort.Slice(items, func(i, j int) bool { return items[i].rank < items[j].rank })

		// 4. Filter to story type and self-post preference.
		filtered := make([]hnItem, 0, count)
		for _, it := range items {
			if it.Type != "" && it.Type != "story" {
				continue
			}
			if it.URL == "" && !includeSelf {
				continue
			}
			filtered = append(filtered, it)
			if len(filtered) >= count {
				break
			}
		}

		if len(filtered) == 0 {
			return "No HN stories matched the filters."
		}

		var sb strings.Builder
		now := time.Now()
		for i, it := range filtered {
			ago := humanAgo(now, time.Unix(it.Time, 0))
			title := strings.TrimSpace(it.Title)
			by := strings.TrimSpace(it.By)
			if by == "" {
				by = "unknown"
			}
			fmt.Fprintf(&sb, "[%d] %s\n", i+1, title)
			fmt.Fprintf(&sb, "By @%s · %d points · %d comments · %s\n", by, it.Score, it.Descendants, ago)
			if it.URL != "" {
				fmt.Fprintf(&sb, "URL: %s\n", it.URL)
			} else {
				fmt.Fprintf(&sb, "URL: (self-post)\n")
			}
			fmt.Fprintf(&sb, "HN: https://news.ycombinator.com/item?id=%d\n", it.ID)
			if i < len(filtered)-1 {
				sb.WriteString("\n")
			}
		}

		return strings.TrimRight(sb.String(), "\n")
	},
}

// humanAgo returns a short human-friendly duration like "3h ago", "2d ago".
func humanAgo(now, then time.Time) string {
	if then.IsZero() || then.Unix() <= 0 {
		return "unknown"
	}
	d := now.Sub(then)
	if d < 0 {
		d = 0
	}
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds ago", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm ago", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh ago", int(d.Hours()))
	default:
		return fmt.Sprintf("%dd ago", int(d.Hours()/24))
	}
}

// ---------- DNSLookup ----------

var DNSLookup = &ToolDef{
	Name:        "dns_lookup",
	Description: "Look up DNS records for a hostname. Returns A, AAAA, MX, TXT, CNAME, NS records. Useful for debugging deploys, checking domain config, sanity-checking before SSH/HTTP.",
	MaxOutput:   8 * 1024,
	Timeout:     15 * time.Second,
	Args: []ToolArg{
		{Name: "host", Type: ArgString, Description: "Hostname to resolve (e.g. example.com).", Required: true},
		{Name: "record_types", Type: ArgList, Description: "Record types to query (default: A, AAAA, MX, TXT, CNAME, NS).", Required: false},
	},
	Execute: func(args map[string]any) string {
		host := String(args, "host")
		if host == "" {
			return "Error: host is required"
		}
		// Strip any accidental scheme/path so the model can paste a URL too.
		if i := strings.Index(host, "://"); i >= 0 {
			host = host[i+3:]
		}
		if i := strings.IndexAny(host, "/?#"); i >= 0 {
			host = host[:i]
		}
		host = strings.TrimSpace(host)
		if host == "" {
			return "Error: host is required"
		}

		types := List(args, "record_types")
		if len(types) == 0 {
			types = []string{"A", "AAAA", "MX", "TXT", "CNAME", "NS"}
		}
		// Normalize.
		for i, t := range types {
			types[i] = strings.ToUpper(strings.TrimSpace(t))
		}

		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		resolver := net.DefaultResolver

		var sb strings.Builder
		fmt.Fprintf(&sb, "Host: %s\n", host)

		for _, t := range types {
			sb.WriteString("\n")
			sb.WriteString(t)
			sb.WriteString(":\n")

			lines, err := lookupRecord(ctx, resolver, host, t)
			if err != nil {
				if isNoSuchRecordErr(err) {
					sb.WriteString("  (none)\n")
					continue
				}
				fmt.Fprintf(&sb, "  Error: %v\n", err)
				continue
			}
			if len(lines) == 0 {
				sb.WriteString("  (none)\n")
				continue
			}
			for _, l := range lines {
				fmt.Fprintf(&sb, "  %s\n", l)
			}
		}

		return strings.TrimRight(sb.String(), "\n")
	},
}

// lookupRecord performs the resolver call for one record type and returns
// already-formatted result lines. Returns an error only for hard failures;
// "no records of this type" comes back as (nil, nil) so callers print "(none)".
func lookupRecord(ctx context.Context, r *net.Resolver, host, recordType string) ([]string, error) {
	switch recordType {
	case "A":
		ips, err := r.LookupIP(ctx, "ip4", host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ips))
		for _, ip := range ips {
			out = append(out, ip.String())
		}
		return out, nil
	case "AAAA":
		ips, err := r.LookupIP(ctx, "ip6", host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(ips))
		for _, ip := range ips {
			out = append(out, ip.String())
		}
		return out, nil
	case "MX":
		mxs, err := r.LookupMX(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			out = append(out, fmt.Sprintf("%d %s", mx.Pref, strings.TrimSuffix(mx.Host, ".")))
		}
		return out, nil
	case "TXT":
		txts, err := r.LookupTXT(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(txts))
		for _, t := range txts {
			out = append(out, fmt.Sprintf("%q", t))
		}
		return out, nil
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, host)
		if err != nil {
			return nil, err
		}
		cname = strings.TrimSuffix(strings.TrimSpace(cname), ".")
		// LookupCNAME echoes back the host itself when no CNAME exists.
		if cname == "" || strings.EqualFold(cname, host) {
			return nil, nil
		}
		return []string{cname}, nil
	case "NS":
		nss, err := r.LookupNS(ctx, host)
		if err != nil {
			return nil, err
		}
		out := make([]string, 0, len(nss))
		for _, ns := range nss {
			out = append(out, strings.TrimSuffix(ns.Host, "."))
		}
		return out, nil
	default:
		return nil, fmt.Errorf("unsupported record type %q", recordType)
	}
}

// isNoSuchRecordErr returns true for "no records of this type" style errors
// that should render as "(none)" rather than a hard error. Hard failures
// (timeout, server unreachable) fall through and surface to the user.
func isNoSuchRecordErr(err error) bool {
	if err == nil {
		return false
	}
	var dnsErr *net.DNSError
	if !errorsAs(err, &dnsErr) {
		// Fall back to substring sniffing if the error didn't unwrap cleanly.
		msg := strings.ToLower(err.Error())
		return strings.Contains(msg, "no such host") ||
			strings.Contains(msg, "no records") ||
			strings.Contains(msg, "not found")
	}
	if dnsErr.IsNotFound {
		return true
	}
	msg := strings.ToLower(dnsErr.Err)
	return strings.Contains(msg, "no such host") ||
		strings.Contains(msg, "no records") ||
		strings.Contains(msg, "not found")
}

// errorsAs is a thin shim so we don't need to import "errors" alongside everything
// else; it forwards to the stdlib helper via type assertion on *net.DNSError.
func errorsAs(err error, target **net.DNSError) bool {
	for e := err; e != nil; {
		if de, ok := e.(*net.DNSError); ok {
			*target = de
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := e.(unwrapper)
		if !ok {
			return false
		}
		e = u.Unwrap()
	}
	return false
}
