package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const defaultLang = "eng"

type config struct {
	port      string
	lang      string
	psm       int
	maxBytes  int64
	timeout   time.Duration
	version   string
	startedAt time.Time
	requests  int
	mu        sync.Mutex
}

func main() {
	cfg := &config{
		port:      env("PORT", "8080"),
		lang:      env("OCR_LANG", defaultLang),
		psm:       atoi(env("OCR_PSM", "3"), 3),
		maxBytes:  parseInt64(env("MAX_UPLOAD_BYTES", "10485760"), 10485760),
		timeout:   time.Duration(atoi(env("OCR_TIMEOUT_SECONDS", "30"), 30)) * time.Second,
		version:   env("OCR_VERSION", "dev"),
		startedAt: time.Now(),
	}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /", cfg.handleIndex)
	mux.HandleFunc("GET /healthz", cfg.handleHealth)
	mux.HandleFunc("POST /ocr", cfg.handleOCR)

	log.Printf("tesseract %s, default lang %s, default psm %d, listening on :%s",
		tesseractVersion(), cfg.lang, cfg.psm, cfg.port)

	server := &http.Server{Addr: ":" + cfg.port, Handler: mux, ReadHeaderTimeout: 10 * time.Second}
	if err := server.ListenAndServe(); err != nil {
		log.Fatal(err)
	}
}

func (c *config) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.requests++
	return c.requests
}

func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func atoi(s string, fallback int) int {
	v, err := strconv.Atoi(s)
	if err != nil {
		return fallback
	}
	return v
}

func parseInt64(s string, fallback int64) int64 {
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return fallback
	}
	return v
}

func tesseractVersion() string {
	out, err := exec.Command("tesseract", "--version").Output()
	if err != nil {
		return "unknown"
	}
	fields := strings.Fields(strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0])
	if len(fields) < 2 {
		return "unknown"
	}
	return fields[1]
}

func (c *config) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	c.requests++
	writeJSON(w, map[string]any{
		"service":      "ocr",
		"tesseract":    tesseractVersion(),
		"default_lang": c.lang,
		"default_psm":  c.psm,
		"languages":    listLanguages(),
		"formats":      []string{"txt", "tsv", "json"},
		"uptime_s":     time.Since(c.startedAt).Seconds(),
		"requests":     c.count(),
	})
}

func (c *config) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, map[string]any{"ok": true})
}

func (c *config) handleOCR(w http.ResponseWriter, r *http.Request) {
	c.count()

	if err := r.ParseMultipartForm(c.maxBytes); err != nil && !errors.Is(err, http.ErrNotMultipart) {
		http.Error(w, "bad multipart form", http.StatusBadRequest)
		return
	}

	lang := r.FormValue("lang")
	if lang == "" {
		lang = c.lang
	}
	psm := c.psm
	if v := r.FormValue("psm"); v != "" {
		psm = atoi(v, c.psm)
	}
	format := r.FormValue("format")
	if format == "" {
		format = "txt"
	}
	switch format {
	case "txt", "tsv", "json":
	default:
		http.Error(w, "format must be txt, tsv or json", http.StatusBadRequest)
		return
	}

	raw, err := readRaw(r, c.maxBytes)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	tesseractFormat := format
	if format == "json" {
		tesseractFormat = "tsv"
	}

	out, err := runTesseract(raw, lang, psm, tesseractFormat, c.timeout)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	switch format {
	case "json":
		writeJSON(w, tsvToJSON(out))
	case "tsv":
		w.Header().Set("Content-Type", "text/tab-separated-values")
		w.Write(out)
	default:
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write(out)
	}
}

func readRaw(r *http.Request, maxBytes int64) ([]byte, error) {
	if f, _, err := r.FormFile("file"); err == nil {
		defer f.Close()
		return io.ReadAll(io.LimitReader(f, maxBytes))
	}

	if u := r.FormValue("url"); u != "" {
		client := &http.Client{Timeout: 15 * time.Second}
		resp, err := client.Get(u)
		if err != nil {
			return nil, fmt.Errorf("url fetch failed: %w", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("url returned %d", resp.StatusCode)
		}
		return io.ReadAll(io.LimitReader(resp.Body, maxBytes))
	}

	return nil, fmt.Errorf("post 'file' (multipart) or 'url'")
}

func runTesseract(raw []byte, lang string, psm int, format string, timeout time.Duration) ([]byte, error) {
	dir, err := os.MkdirTemp("", "ocr")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)

	in := filepath.Join(dir, "input")
	if err := os.WriteFile(in, raw, 0o600); err != nil {
		return nil, err
	}

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	args := []string{in, "-", format, "-l", lang, "--psm", strconv.Itoa(psm)}
	cmd := exec.CommandContext(ctx, "tesseract", args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("timed out after %s", timeout)
		}
		return nil, fmt.Errorf("tesseract failed: %s", strings.TrimSpace(stderr.String()))
	}
	return stdout.Bytes(), nil
}

func tsvToJSON(raw []byte) map[string]any {
	lines := strings.Split(strings.TrimRight(string(raw), "\n"), "\n")
	if len(lines) < 2 {
		return map[string]any{"text": "", "words": []any{}}
	}

	var words []map[string]any
	var text strings.Builder
	for _, line := range lines[1:] {
		fields := strings.Split(line, "\t")
		if len(fields) < 12 {
			continue
		}
		conf, _ := strconv.ParseFloat(fields[10], 64)
		word := strings.Join(fields[11:], "\t")
		if word == "" {
			continue
		}
		words = append(words, map[string]any{
			"text":  word,
			"conf":  conf,
			"bbox":  []int{atoi(fields[6], 0), atoi(fields[7], 0), atoi(fields[8], 0), atoi(fields[9], 0)},
			"block": atoi(fields[2], 0),
			"line":  atoi(fields[4], 0),
		})
		text.WriteString(word)
		text.WriteString(" ")
	}

	return map[string]any{
		"text":  strings.TrimSpace(text.String()),
		"words": words,
	}
}

func listLanguages() []string {
	out, err := exec.Command("tesseract", "--list-langs").Output()
	if err != nil {
		return nil
	}
	var langs []string
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if line == "" || strings.HasPrefix(line, "List of available") {
			continue
		}
		langs = append(langs, line)
	}
	return langs
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
