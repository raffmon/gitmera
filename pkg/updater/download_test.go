package updater

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDownload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("archive-bytes"))
	}))
	defer server.Close()

	got, err := Download(context.Background(), server.Client(), server.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(got) != "archive-bytes" {
		t.Errorf("got %q, want %q", got, "archive-bytes")
	}
}

func TestDownload_NonOKStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := Download(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected an error for a 404 response, got nil")
	}
}

func TestDownload_InvalidURL(t *testing.T) {
	_, err := Download(context.Background(), &http.Client{}, "://invalid")
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
	if !strings.Contains(err.Error(), "failed to build request") {
		t.Errorf("expected 'failed to build request', got: %v", err)
	}
}

func TestDownload_BodyReadFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hj, ok := w.(http.Hijacker)
		if !ok {
			http.Error(w, "hijacking not supported", http.StatusInternalServerError)
			return
		}
		conn, buf, err := hj.Hijack()
		if err != nil {
			return
		}
		_, _ = buf.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 1000\r\n\r\npartial")
		_ = buf.Flush()
		_ = conn.Close()
	}))
	defer server.Close()

	_, err := Download(context.Background(), server.Client(), server.URL)
	if err == nil {
		t.Fatal("expected error when response body read fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to read response body") {
		t.Errorf("expected 'failed to read response body', got: %v", err)
	}
}

func TestDownload_DialFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := server.URL
	server.Close()

	_, err := Download(context.Background(), http.DefaultClient, url)
	if err == nil {
		t.Fatal("expected error when server is unreachable, got nil")
	}
	if !strings.Contains(err.Error(), "failed to download") {
		t.Errorf("expected 'failed to download' in error, got: %v", err)
	}
}

func TestReadLimited(t *testing.T) {
	t.Parallel()

	t.Run("under limit", func(t *testing.T) {
		t.Parallel()
		got, err := readLimited(strings.NewReader("hello"), 16)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if string(got) != "hello" {
			t.Errorf("got %q, want %q", got, "hello")
		}
	})

	t.Run("exactly at limit succeeds", func(t *testing.T) {
		t.Parallel()
		body := strings.Repeat("a", 16)
		got, err := readLimited(strings.NewReader(body), 16)
		if err != nil {
			t.Fatalf("unexpected error for input exactly at the cap: %v", err)
		}
		if string(got) != body {
			t.Errorf("got %d bytes, want %d", len(got), len(body))
		}
	})

	t.Run("one byte over limit fails", func(t *testing.T) {
		t.Parallel()
		_, err := readLimited(strings.NewReader(strings.Repeat("a", 17)), 16)
		if err == nil {
			t.Fatal("expected error for input one byte over the cap, got nil")
		}
		if !strings.Contains(err.Error(), "exceeds maximum allowed size") {
			t.Errorf("expected 'exceeds maximum allowed size', got: %v", err)
		}
	})
}

func TestVerifyChecksum(t *testing.T) {
	data := []byte("archive-bytes")
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	otherHash := strings.Repeat("0", 64)
	checksums := []byte(fmt.Sprintf("%s  gitmera_0.2.0_linux_amd64.tar.gz\n%s  checksums-other.tar.gz\n", hash, otherHash))

	if err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", data); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVerifyChecksum_Mismatch(t *testing.T) {
	checksums := []byte(strings.Repeat("0", 64) + "  gitmera_0.2.0_linux_amd64.tar.gz\n")

	err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", []byte("archive-bytes"))
	if err == nil {
		t.Fatal("expected a checksum mismatch error, got nil")
	}
}

func TestVerifyChecksum_MissingEntry(t *testing.T) {
	checksums := []byte(strings.Repeat("0", 64) + "  some-other-file.tar.gz\n")

	err := VerifyChecksum(checksums, "gitmera_0.2.0_linux_amd64.tar.gz", []byte("archive-bytes"))
	if err == nil {
		t.Fatal("expected a missing-entry error, got nil")
	}
}
