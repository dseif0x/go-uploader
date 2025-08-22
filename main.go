package main

import (
	"bytes"
	"context"
	"embed"
	"errors"
	"fmt"
	"github.com/meyskens/go-turnstile"
	store "go-uploader/storage"
	"html/template"
	"io"
	"io/fs"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

//go:embed public
var staticFiles embed.FS

var storage store.Backend
var turnstileSecret string

func main() {
	err := godotenv.Load()
	if err != nil {
		log.Println("No .env file found, continuing...")
	}
	turnstileSecret = os.Getenv("TURNSTILE_SECRET")
	if turnstileSecret == "" {
		log.Fatal("TURNSTILE_SECRET environment variable is not set")
	}

	err = setupStorage()
	if err != nil {
		log.Fatalf("Failed to setup storage: %v", err)
	}

	indexCache, files, err := buildIndexPage()
	if err != nil {
		log.Fatalf("Failed to build index page: %v", err)
	}

	// Serve static files
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" || r.URL.Path == "/index.html" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(indexCache))
			return
		}

		fileServer := http.FileServer(http.FS(files))
		fileServer.ServeHTTP(w, r)
	})

	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	// Create server with timeouts to handle slow/interrupted uploads
	server := &http.Server{
		Addr:         ":8080",
		ReadTimeout:  5 * time.Minute,  // Allow up to 5 minutes for reading request body
		WriteTimeout: 30 * time.Second, // Response timeout
		IdleTimeout:  60 * time.Second, // Keep-alive timeout
	}

	log.Println("Server started on :8080")
	log.Fatal(server.ListenAndServe())
}

func setupStorage() error {
	backend := os.Getenv("BACKEND")
	if backend == "" {
		log.Println("BACKEND environment variable not set, using local backend")
		backend = "local"
	}

	var err error
	if backend == "local" {
		log.Println("Using local storage backend")
		uploadDir := os.Getenv("LOCAL_PATH")
		if uploadDir == "" {
			log.Println("LOCAL_PATH environment variable not set, using default: ./uploads")
			uploadDir = "./uploads"
		}
		storage, err = store.NewLocalStorage(uploadDir)
	} else if backend == "s3" {
		log.Println("Using S3 storage backend")
		storage, err = store.NewS3Storage("go-upload", "uploads")
	}
	if err != nil {
		return err
	}
	return nil
}

func buildIndexPage() (string, fs.FS, error) {
	siteKey := os.Getenv("TURNSTILE_SITEKEY")
	if siteKey == "" {
		log.Fatal("TURNSTILE_SITEKEY is not set")
	}

	contentFS, err := fs.Sub(staticFiles, "public")
	if err != nil {
		return "", nil, err
	}

	tmpl, err := template.ParseFS(contentFS, "index.html")
	if err != nil {
		return "", nil, err
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, map[string]string{
		"SiteKey": siteKey,
	})
	if err != nil {
		return "", nil, err
	}
	return buf.String(), contentFS, nil
}

func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	// Add context with timeout for the upload operation
	ctx, cancel := context.WithTimeout(r.Context(), 4*time.Minute)
	defer cancel()
	r = r.WithContext(ctx)

	contentType := r.Header.Get("Content-Type")
	mediaType, params, err := mime.ParseMediaType(contentType)
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		http.Error(w, "Invalid Content-Type", http.StatusBadRequest)
		return
	}

	ts := turnstile.New(turnstileSecret)
	token := r.Header.Get("X-Turnstile-Token")
	resp, err := ts.Verify(token, r.RemoteAddr)
	if err != nil || !resp.Success {
		http.Error(w, "CAPTCHA verification failed", http.StatusForbidden)
		return
	}

	mr := multipart.NewReader(r.Body, params["boundary"])
	saved := 0
	failed := 0
	var lastError error

	now := time.Now()
	subfolder := now.Format("2006-01-02_15-04-05.000")

	log.Printf("Starting upload session: %s", subfolder)

	for {
		// Check context for timeout/cancellation
		select {
		case <-ctx.Done():
			log.Printf("Upload cancelled or timed out for session %s: %v", subfolder, ctx.Err())
			if saved > 0 {
				// Partial success - inform client
				w.WriteHeader(http.StatusPartialContent)
				w.Write([]byte(fmt.Sprintf("Upload partially completed: %d file(s) uploaded, %d failed due to timeout", saved, failed)))
			} else {
				http.Error(w, "Upload timed out", http.StatusRequestTimeout)
			}
			return
		default:
		}

		part, err := mr.NextPart()
		if err == io.EOF {
			log.Printf("Upload session %s completed normally", subfolder)
			break
		}
		if err != nil {
			log.Printf("Error reading multipart data in session %s: %v", subfolder, err)
			lastError = err

			// Check if this is an unexpected EOF (connection dropped)
			if errors.Is(err, io.ErrUnexpectedEOF) || strings.Contains(err.Error(), "unexpected EOF") {
				failed++
				log.Printf("Connection interrupted during upload in session %s", subfolder)
				// Don't break immediately - there might be more data
				continue
			}

			// For other errors, break the loop
			break
		}
		defer part.Close()

		if part.FileName() == "" {
			continue
		}

		filename := filepath.Join(subfolder, sanitizeFilename(part.FileName()))
		log.Printf("Saving file: %s", filename)

		if err := storage.SaveFile(filename, part); err != nil {
			log.Printf("Error saving file %s in session %s: %v", filename, subfolder, err)
			failed++
			lastError = err
			continue
		}
		saved++
		log.Printf("Successfully saved file: %s", filename)
	}

	log.Printf("Upload session %s summary: %d saved, %d failed", subfolder, saved, failed)

	if saved == 0 {
		if lastError != nil {
			if errors.Is(lastError, io.ErrUnexpectedEOF) || strings.Contains(lastError.Error(), "unexpected EOF") {
				http.Error(w, "Upload failed due to connection issues. Please check your internet connection and try again.", http.StatusBadRequest)
			} else {
				http.Error(w, fmt.Sprintf("Upload failed: %v", lastError), http.StatusBadRequest)
			}
		} else {
			http.Error(w, "No files uploaded", http.StatusBadRequest)
		}
		return
	}

	if failed > 0 {
		// Partial success
		w.WriteHeader(http.StatusPartialContent)
		w.Write([]byte(fmt.Sprintf("Partially successful: %d file(s) uploaded, %d failed", saved, failed)))
	} else {
		// Complete success
		w.WriteHeader(http.StatusCreated)
		w.Write([]byte(fmt.Sprintf("Uploaded %d file(s)", saved)))
	}
}

func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return -1
		}
		return r
	}, name)
}
