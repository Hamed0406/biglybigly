package urlcheck

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"
)

// URL is one tracked URL with the result of its most recent check (if any).
type URL struct {
	ID        int    `json:"id"`
	URL       string `json:"url"`
	Status    *int   `json:"status"`
	LastCheck *int64 `json:"last_check"`
	CreatedAt int64  `json:"created_at"`
	UpdatedAt int64  `json:"updated_at"`
}

// AddURLRequest is the body accepted by POST /api/urlcheck/urls.
type AddURLRequest struct {
	URL string `json:"url"`
}

// CheckResult is the response payload from GET /api/urlcheck/check/{id}.
// Status is 0 when the request failed before a status code was received.
type CheckResult struct {
	Status       int    `json:"status"`
	ResponseTime int64  `json:"response_time"`
	Error        string `json:"error,omitempty"`
}

// HistoryEntry is one row from urlcheck_history.
type HistoryEntry struct {
	ID           int    `json:"id"`
	Status       int    `json:"status"`
	ResponseTime int64  `json:"response_time"`
	Error        string `json:"error,omitempty"`
	CheckedAt    int64  `json:"checked_at"`
}

// handleListURLs returns all tracked URLs in newest-first order.
func (m *Module) handleListURLs(w http.ResponseWriter, r *http.Request) {
	db := m.p.DB()
	rows, err := db.Query(`
		SELECT id, url, status, last_check, created_at, updated_at
		FROM urlcheck_urls
		ORDER BY created_at DESC
	`)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var urls []URL
	for rows.Next() {
		var u URL
		if err := rows.Scan(&u.ID, &u.URL, &u.Status, &u.LastCheck, &u.CreatedAt, &u.UpdatedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		urls = append(urls, u)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(urls)
}

// handleAddURL inserts a new URL record and returns its assigned ID.
func (m *Module) handleAddURL(w http.ResponseWriter, r *http.Request) {
	var req AddURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	if req.URL == "" {
		http.Error(w, "url required", http.StatusBadRequest)
		return
	}

	db := m.p.DB()
	now := time.Now().Unix()
	result, err := db.Exec(`
		INSERT INTO urlcheck_urls (url, created_at, updated_at)
		VALUES (?, ?, ?)
	`, req.URL, now, now)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	id, _ := result.LastInsertId()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{"id": id})
}

// handleDeleteURL removes a URL and (via FK cascade) its history.
func (m *Module) handleDeleteURL(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	db := m.p.DB()
	_, err := db.Exec(`DELETE FROM urlcheck_urls WHERE id = ?`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
}

// handleCheckURL performs a synchronous HEAD request against the stored URL,
// records the result in urlcheck_history, and updates the URL's last-known
// status. Returns the CheckResult to the caller.
func (m *Module) handleCheckURL(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	db := m.p.DB()
	var urlStr string
	err = db.QueryRow(`SELECT url FROM urlcheck_urls WHERE id = ?`, id).Scan(&urlStr)
	if err != nil {
		http.Error(w, "url not found", http.StatusNotFound)
		return
	}

	// Check the URL
	start := time.Now()
	resp, err := http.Head(urlStr)
	elapsed := time.Since(start).Milliseconds()

	result := CheckResult{
		ResponseTime: elapsed,
	}

	if err != nil {
		result.Error = err.Error()
		result.Status = 0
	} else {
		defer resp.Body.Close()
		result.Status = resp.StatusCode
	}

	// Store in history
	now := time.Now().Unix()
	if _, err := db.Exec(`
		INSERT INTO urlcheck_history (url_id, status, response_time, error, checked_at)
		VALUES (?, ?, ?, ?, ?)
	`, id, result.Status, elapsed, result.Error, now); err != nil {
		http.Error(w, "failed to save history", http.StatusInternalServerError)
		return
	}

	// Update last check time
	if _, err := db.Exec(`
		UPDATE urlcheck_urls SET last_check = ?, status = ?, updated_at = ?
		WHERE id = ?
	`, now, result.Status, now, id); err != nil {
		http.Error(w, "failed to update status", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(result)
}

// handleGetHistory returns the most recent 100 check results for a URL.
func (m *Module) handleGetHistory(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		http.Error(w, "invalid id", http.StatusBadRequest)
		return
	}

	db := m.p.DB()
	rows, err := db.Query(`
		SELECT id, status, response_time, error, checked_at
		FROM urlcheck_history
		WHERE url_id = ?
		ORDER BY checked_at DESC
		LIMIT 100
	`, id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	var history []HistoryEntry
	for rows.Next() {
		var h HistoryEntry
		if err := rows.Scan(&h.ID, &h.Status, &h.ResponseTime, &h.Error, &h.CheckedAt); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		history = append(history, h)
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(history)
}
