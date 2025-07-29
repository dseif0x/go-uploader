package main

import (
	"embed"
	"fmt"
	"github.com/meyskens/go-turnstile"
	store "go-uploader/storage"
	"io"
	"io/fs"
	"log"
	"mime"
	"mime/multipart"
	"net/http"
	"os"
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

	backend := os.Getenv("BACKEND")
	if backend == "" {
		log.Println("BACKEND environment variable not set, using local backend")
		backend = "local"
	}

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
		log.Fatalf("Failed to init storage backend: %v", err)
	}

	var staticFS = fs.FS(staticFiles)
	htmlContent, err := fs.Sub(staticFS, "public")
	if err != nil {
		log.Fatal(err)
	}
	fs := http.FileServer(http.FS(htmlContent))

	// Serve static files
	http.Handle("/", fs)

	http.HandleFunc("/upload", uploadHandler)
	http.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Println("Server started on :8080")
	log.Fatal(http.ListenAndServe(":8080", nil))
}
func uploadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
		return
	}

	//r.Body = http.MaxBytesReader(w, r.Body, maxUploadSize)

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

	for {
		part, err := mr.NextPart()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		defer part.Close()

		if part.FileName() == "" {
			continue
		}

		filename := fmt.Sprintf("%d_%s", time.Now().UnixNano(), sanitizeFilename(part.FileName()))
		if err := storage.SaveFile(filename, part); err != nil {
			log.Printf("error saving file: %v", err)
			continue
		}
		saved++
	}

	if saved == 0 {
		http.Error(w, "No files uploaded", http.StatusBadRequest)
		return
	}

	w.WriteHeader(http.StatusCreated)
	w.Write([]byte(fmt.Sprintf("Uploaded %d file(s)", saved)))
}

func sanitizeFilename(name string) string {
	return strings.Map(func(r rune) rune {
		if r == '/' || r == '\\' || r == ':' {
			return -1
		}
		return r
	}, name)
}
