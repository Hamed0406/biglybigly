package dnsfilter

import (
	"bufio"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// BlocklistManager downloads, parses, and caches blocklist domains in memory
type BlocklistManager struct {
	mu       sync.RWMutex
	blocked  map[string]bool // domain → blocked
	allowed  map[string]bool // explicit allow rules override blocks
	total    int
	logger   *slog.Logger
	client   *http.Client
}

func NewBlocklistManager(logger *slog.Logger) *BlocklistManager {
	return &BlocklistManager{
		blocked: make(map[string]bool),
		allowed: make(map[string]bool),
		logger:  logger,
		client:  &http.Client{Timeout: 60 * time.Second},
	}
}

// IsBlocked checks if a domain should be blocked
func (bm *BlocklistManager) IsBlocked(domain string) bool {
	domain = normalizeDomain(domain)

	bm.mu.RLock()
	defer bm.mu.RUnlock()

	// Explicit allow always wins
	if bm.allowed[domain] {
		return false
	}

	// Check exact match
	if bm.blocked[domain] {
		return true
	}

	// Check parent domains (e.g., block "ads.example.com" if "example.com" is blocked)
	parts := strings.Split(domain, ".")
	for i := 1; i < len(parts)-1; i++ {
		parent := strings.Join(parts[i:], ".")
		if bm.allowed[parent] {
			return false
		}
		if bm.blocked[parent] {
			return true
		}
	}

	return false
}

// TotalBlocked returns count of domains in blocklist
func (bm *BlocklistManager) TotalBlocked() int {
	bm.mu.RLock()
	defer bm.mu.RUnlock()
	return bm.total
}

// LoadFromDB loads all enabled blocklists and custom rules from the database
func (bm *BlocklistManager) LoadFromDB(db *sql.DB) error {
	blocked := make(map[string]bool)
	allowed := make(map[string]bool)

	// Load enabled blocklists
	rows, err := db.Query(`SELECT id, url, name FROM dnsfilter_blocklists WHERE enabled = 1`)
	if err != nil {
		return fmt.Errorf("query blocklists: %w", err)
	}

	type listInfo struct {
		id   int
		url  string
		name string
	}
	var lists []listInfo
	for rows.Next() {
		var l listInfo
		if err := rows.Scan(&l.id, &l.url, &l.name); err != nil {
			continue
		}
		lists = append(lists, l)
	}
	rows.Close()

	// Download and parse each list
	for _, l := range lists {
		domains, err := bm.downloadList(l.url)
		if err != nil {
			bm.logger.Warn("Failed to download blocklist", "name", l.name, "url", l.url, "err", err)
			continue
		}

		for _, d := range domains {
			blocked[d] = true
		}

		// Update entry count and timestamp
		now := time.Now().Unix()
		db.Exec(`UPDATE dnsfilter_blocklists SET entry_count = ?, last_updated = ? WHERE id = ?`,
			len(domains), now, l.id)

		bm.logger.Info("Loaded blocklist", "name", l.name, "domains", len(domains))
	}

	// Load custom rules
	ruleRows, err := db.Query(`SELECT domain, action FROM dnsfilter_custom_rules`)
	if err != nil {
		return fmt.Errorf("query custom rules: %w", err)
	}

	for ruleRows.Next() {
		var domain, action string
		if err := ruleRows.Scan(&domain, &action); err != nil {
			continue
		}
		d := normalizeDomain(domain)
		if action == "allow" {
			allowed[d] = true
		} else {
			blocked[d] = true
		}
	}
	ruleRows.Close()

	// Swap in the new data
	bm.mu.Lock()
	bm.blocked = blocked
	bm.allowed = allowed
	bm.total = len(blocked)
	bm.mu.Unlock()

	bm.logger.Info("Blocklist loaded", "blocked_domains", len(blocked), "allow_rules", len(allowed))
	return nil
}

// downloadList fetches a hosts-file format blocklist and returns domain list
func (bm *BlocklistManager) downloadList(url string) ([]string, error) {
	resp, err := bm.client.Get(url)
	if err != nil {
		return nil, fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}

	return parseHostsFile(resp.Body)
}

// parseHostsFile parses a hosts-file or domain-list format
func parseHostsFile(r io.Reader) ([]string, error) {
	var domains []string
	scanner := bufio.NewScanner(r)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip empty lines and comments
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}

		// Handle hosts-file format: "0.0.0.0 domain.com" or "127.0.0.1 domain.com"
		fields := strings.Fields(line)
		var domain string

		if len(fields) >= 2 && (fields[0] == "0.0.0.0" || fields[0] == "127.0.0.1") {
			domain = fields[1]
		} else if len(fields) == 1 {
			// Plain domain list format
			domain = fields[0]
		} else {
			continue
		}

		// Skip localhost entries
		domain = normalizeDomain(domain)
		if domain == "" || domain == "localhost" || domain == "localhost.localdomain" {
			continue
		}
		if strings.HasPrefix(domain, "localhost") {
			continue
		}

		// Basic domain validation
		if !strings.Contains(domain, ".") {
			continue
		}

		domains = append(domains, domain)
	}

	return domains, scanner.Err()
}

// normalizeDomain lowercases and trims trailing dots
func normalizeDomain(d string) string {
	d = strings.ToLower(strings.TrimSpace(d))
	d = strings.TrimSuffix(d, ".")
	return d
}
