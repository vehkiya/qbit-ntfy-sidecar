package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"
)

// --- Configuration ---
var (
	qbitHost       string
	qbitUser       string
	qbitPass       string
	ntfyServer     string
	ntfyTopic      string
	ntfyPrioProg   string
	ntfyPrioComp   string
	notifyComplete bool
	progressFormat string
	pollInt        = 5 * time.Second
)

// --- State ---
var (
	activeMonitors = make(map[string]bool)
	mutex          sync.Mutex
)

// Torrent struct for JSON parsing
type Torrent struct {
	Hash     string  `json:"hash"`
	Name     string  `json:"name"`
	Progress float64 `json:"progress"`
	Eta      int     `json:"eta"`
	DlSpeed  int     `json:"dlspeed"`
	State    string  `json:"state"`
}

func main() {
	log.SetFlags(0) // K8s handles timestamps

	// 1. Config Check
	qbitHost = getEnv("QBIT_HOST", "http://localhost:8080")
	qbitUser = getEnv("QBIT_USER", "")
	qbitPass = getEnv("QBIT_PASS", "")

	ntfyServer = strings.TrimRight(getEnv("NTFY_SERVER", "https://ntfy.sh"), "/")
	ntfyTopic = mustGetEnv("NTFY_TOPIC")
	ntfyPrioProg = getEnv("NTFY_PRIORITY_PROGRESS", "2") // Default: Low (no sound/vibe)
	ntfyPrioComp = getEnv("NTFY_PRIORITY_COMPLETE", "3") // Default: Default (sound/vibe)

	notifyComplete = getEnvBool("NOTIFY_COMPLETE", true)
	progressFormat = getEnv("PROGRESS_FORMAT", "bar") // "bar" or "percent"

	// 2. Start Trigger Server
	http.HandleFunc("/track", handleTrackRequest)

	port := "9090"
	log.Printf("Sidecar listening on :%s", port)
	log.Printf("Config: Host=%s Auth=%v Topic=%s/%s", qbitHost, qbitUser != "", ntfyServer, ntfyTopic)

	// 3. Run Startup Scan (Background)
	go startupScan()

	log.Fatal(http.ListenAndServe(":"+port, nil))
}

func startupScan() {
	// Retry loop to wait for qBittorrent to be ready
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}

	for {
		log.Println("Startup: Attempting to connect to qBittorrent...")

		// 1. Auth (if required)
		if qbitUser != "" && qbitPass != "" {
			if err := login(client); err != nil {
				log.Printf("Startup: Auth failed (%v). Retrying in 10s...", err)
				time.Sleep(10 * time.Second)
				continue
			}
		}

		// 2. Fetch Active Torrents
		// "downloading" filter covers: downloading, metaDL, stalledDL, checkingDL, forcedDL
		resp, err := client.Get(qbitHost + "/api/v2/torrents/info?filter=downloading")
		if err != nil {
			log.Printf("Startup: Connection failed (%v). Retrying in 10s...", err)
			time.Sleep(10 * time.Second)
			continue
		}

		if resp.StatusCode != 200 {
			log.Printf("Startup: API returned %d. Retrying in 10s...", resp.StatusCode)
			_ = resp.Body.Close()
			time.Sleep(10 * time.Second)
			continue
		}

		var torrents []Torrent
		if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
			log.Printf("Startup: JSON decode error (%v). Retrying in 10s...", err)
			_ = resp.Body.Close()
			time.Sleep(10 * time.Second)
			continue
		}
		_ = resp.Body.Close()

		// 3. Sync
		log.Printf("Startup: Found %d active downloads. Syncing...", len(torrents))
		for _, t := range torrents {
			mutex.Lock()
			if !activeMonitors[t.Hash] {
				activeMonitors[t.Hash] = true
				mutex.Unlock()
				log.Printf("Startup: Resuming monitor for %s (%s)", t.Name, t.Hash)
				go trackTorrent(t.Hash)
			} else {
				mutex.Unlock()
			}
		}

		// Success! Exit the loop (or we could keep this running periodically as a failsafe)
		log.Println("Startup: Sync complete.")
		return
	}
}

func handleTrackRequest(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}

	hash := r.URL.Query().Get("hash")
	if hash == "" {
		http.Error(w, "Missing 'hash' query parameter", 400)
		return
	}

	mutex.Lock()
	if activeMonitors[hash] {
		mutex.Unlock()
		_, _ = fmt.Fprintf(w, "Already tracking %s", hash)
		return
	}
	activeMonitors[hash] = true
	mutex.Unlock()

	go trackTorrent(hash)

	w.WriteHeader(200)
	_, _ = fmt.Fprintf(w, "Tracking started for %s", hash)
}

func trackTorrent(hash string) {
	defer func() {
		mutex.Lock()
		delete(activeMonitors, hash)
		mutex.Unlock()
	}()

	log.Printf("[%s] Monitor started", hash)

	// Per-routine client to handle independent auth sessions cleanly
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 5 * time.Second}

	// Login only if credentials are provided
	if qbitUser != "" && qbitPass != "" {
		if err := login(client); err != nil {
			log.Printf("[%s] Auth failed: %v", hash, err)
			return
		}
	}

	ticker := time.NewTicker(pollInt)
	defer ticker.Stop()

	lastPct := -1

	for range ticker.C {
		t, err := getTorrentInfo(client, hash)
		if err != nil {
			log.Printf("[%s] Error: %v", hash, err)
			continue
		}
		if t == nil {
			log.Printf("[%s] Torrent removed. Stopping.", hash)
			return
		}

		pct := int(t.Progress * 100)

		// Update Notification if progress changed
		if pct > lastPct {
			lastPct = pct
			sendUpdate(t, pct)
		}

		// Check Completion
		// qBittorrent states: upload, uploading, upLO, pausedUP, completed, etc.
		if pct >= 100 || strings.Contains(t.State, "up") || t.State == "completed" {
			if notifyComplete {
				sendComplete(t)
			}
			return
		}
	}
}

func sendUpdate(t *Torrent, pct int) {
	speed := float64(t.DlSpeed) / 1024 / 1024
	eta := formatDuration(t.Eta)

	var msg string
	if progressFormat == "percent" {
		msg = fmt.Sprintf("Progress: %d%%\nSpeed: %.1f MB/s\nETA: %s", pct, speed, eta)
	} else {
		bar := drawProgressBar(pct)
		msg = fmt.Sprintf("%d%% %s\nSpeed: %.1f MB/s\nETA: %s", pct, bar, speed, eta)
	}

	sendNtfy(t.Name, msg, "arrow_down", "qbit-"+t.Hash, ntfyPrioProg)
}

func sendComplete(t *Torrent) {
	sendNtfy("Download Complete", t.Name+" has finished downloading.", "white_check_mark", "qbit-"+t.Hash, ntfyPrioComp)
}

func sendNtfy(title, msg, tag, id, priority string) {
	url := fmt.Sprintf("%s/%s", ntfyServer, ntfyTopic)
	req, _ := http.NewRequest("POST", url, strings.NewReader(msg))
	req.Header.Set("Title", title)
	req.Header.Set("Tags", tag)
	req.Header.Set("Priority", priority)
	req.Header.Set("X-Notification-ID", id) // Updates in-place

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("Failed to send ntfy notification: %v", err)
		return
	}
	defer func() { _ = resp.Body.Close() }()
}

func getTorrentInfo(client *http.Client, hash string) (*Torrent, error) {
	resp, err := client.Get(qbitHost + "/api/v2/torrents/info?hashes=" + hash)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("qBit API returned status: %d", resp.StatusCode)
	}

	var torrents []Torrent
	if err := json.NewDecoder(resp.Body).Decode(&torrents); err != nil {
		return nil, err
	}

	if len(torrents) == 0 {
		return nil, nil
	}
	return &torrents[0], nil
}

func login(client *http.Client) error {
	data := url.Values{}
	data.Set("username", qbitUser)
	data.Set("password", qbitPass)

	resp, err := client.PostForm(qbitHost+"/api/v2/auth/login", data)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || strings.Contains(string(body), "Fails.") {
		return fmt.Errorf("bad credentials or connection failed")
	}
	return nil
}

func drawProgressBar(pct int) string {
	width := 10
	filled := int(math.Round(float64(pct) / 10.0))
	if filled > width {
		filled = width
	}
	if filled < 0 {
		filled = 0
	}
	empty := width - filled
	return "[" + strings.Repeat("█", filled) + strings.Repeat("░", empty) + "]"
}

func formatDuration(sec int) string {
	if sec >= 8640000 {
		return "∞"
	}
	return (time.Duration(sec) * time.Second).String()
}

func mustGetEnv(k string) string {
	v := os.Getenv(k)
	if v == "" {
		log.Fatalf("Missing ENV: %s", k)
	}
	return v
}

func getEnv(k, fallback string) string {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvBool(k string, fallback bool) bool {
	v := os.Getenv(k)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
