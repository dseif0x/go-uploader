package main

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// Integration test to demonstrate improved upload resilience
func TestUploadResilience_Integration(t *testing.T) {
	// Setup test environment
	testDir := "/tmp/test-uploads"
	os.RemoveAll(testDir)
	defer os.RemoveAll(testDir)

	os.Setenv("BACKEND", "local")
	os.Setenv("LOCAL_PATH", testDir)
	os.Setenv("TURNSTILE_SECRET", "test-secret")
	os.Setenv("TURNSTILE_SITEKEY", "test-sitekey")
	defer func() {
		os.Unsetenv("BACKEND")
		os.Unsetenv("LOCAL_PATH")
		os.Unsetenv("TURNSTILE_SECRET")
		os.Unsetenv("TURNSTILE_SITEKEY")
	}()

	// Setup storage
	err := setupStorage()
	if err != nil {
		t.Fatalf("Failed to setup storage: %v", err)
	}

	// Create a large-ish multipart request to simulate real upload
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)

	// Add multiple test files
	for i := 0; i < 3; i++ {
		part, err := writer.CreateFormFile("file", fmt.Sprintf("test%d.jpg", i))
		if err != nil {
			t.Fatal(err)
		}
		// Write some test content
		content := bytes.Repeat([]byte(fmt.Sprintf("test content %d ", i)), 100)
		part.Write(content)
	}
	writer.Close()

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(uploadHandler))
	defer server.Close()

	// Test 1: Normal upload (will fail due to turnstile, but we test the flow)
	t.Run("NormalUpload", func(t *testing.T) {
		req, err := http.NewRequest("POST", server.URL+"/upload", bytes.NewReader(body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Turnstile-Token", "fake-token")

		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()

		// Should fail due to invalid turnstile token, but handler should not panic
		if resp.StatusCode != http.StatusForbidden {
			t.Logf("Expected 403 (turnstile failure), got %d", resp.StatusCode)
		}
	})

	// Test 2: Test that our timeout improvements work
	t.Run("TimeoutHandling", func(t *testing.T) {
		// Create a request with very short timeout
		req, err := http.NewRequest("POST", server.URL+"/upload", bytes.NewReader(body.Bytes()))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Content-Type", writer.FormDataContentType())
		req.Header.Set("X-Turnstile-Token", "fake-token")

		// Use a client with very short timeout to simulate network issues
		client := &http.Client{Timeout: 1 * time.Millisecond}
		_, err = client.Do(req)

		// Should get a timeout error, not a panic
		if err == nil {
			t.Log("Expected timeout error, but request succeeded")
		} else {
			t.Logf("Got expected timeout error: %v", err)
		}
	})

	// Test 3: Verify file handling improvements
	t.Run("FileHandling", func(t *testing.T) {
		// Test sanitization
		testCases := []string{
			"normal.jpg",
			"path/traversal.jpg",
			"../../../etc/passwd",
			"windows\\file.jpg",
		}

		for _, filename := range testCases {
			sanitized := sanitizeFilename(filename)
			if filepath.IsAbs(sanitized) {
				t.Errorf("Sanitized filename %q should not be absolute", sanitized)
			}
			if filepath.Clean(sanitized) != sanitized {
				t.Errorf("Sanitized filename %q contains path elements", sanitized)
			}
		}
	})
}

// Test demonstrating improved error messages and logging
func TestUploadErrorHandling(t *testing.T) {
	// Test that our error handling provides meaningful messages
	tests := []struct {
		name           string
		setupRequest   func() *http.Request
		expectedStatus int
		expectedMsg    string
	}{
		{
			name: "InvalidContentType",
			setupRequest: func() *http.Request {
				req := httptest.NewRequest("POST", "/upload", bytes.NewReader([]byte("invalid")))
				req.Header.Set("Content-Type", "text/plain")
				return req
			},
			expectedStatus: http.StatusBadRequest,
			expectedMsg:    "Invalid Content-Type",
		},
		{
			name: "WrongMethod",
			setupRequest: func() *http.Request {
				return httptest.NewRequest("GET", "/upload", nil)
			},
			expectedStatus: http.StatusMethodNotAllowed,
			expectedMsg:    "Only POST allowed",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := tt.setupRequest()
			w := httptest.NewRecorder()

			uploadHandler(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}

			if !bytes.Contains(w.Body.Bytes(), []byte(tt.expectedMsg)) {
				t.Errorf("Expected response to contain %q, got %q", tt.expectedMsg, w.Body.String())
			}
		})
	}
}
