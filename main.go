package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

var (
	client *http.Client
	buffer = make([]byte, 32*1024)
)

func download(url, artefact, downloadPath string) error {
	localFilePath := filepath.Join(downloadPath, artefact)
	log.Printf("Processing artefact: %s", artefact)

	needDownload := true
	if fi, err := os.Stat(localFilePath); err == nil {
		localModTime := fi.ModTime()

		req, err := http.NewRequest("HEAD", url, nil)
		if err != nil {
			return fmt.Errorf("error creating HEAD request for %s: %v", url, err)
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("error performing HEAD request for %s: %v", url, err)
		}
		resp.Body.Close()

		if lastModified := resp.Header.Get("Last-Modified"); lastModified != "" {
			remoteModTime, err := time.Parse(http.TimeFormat, lastModified)
			if err != nil {
				log.Printf("Error parsing Last-Modified header for %s: %v", url, err)
			} else if !remoteModTime.After(localModTime) {
				log.Printf("No new version available for %s (remote: %s, local: %s)",
					artefact, remoteModTime, localModTime)
				needDownload = false
			}
		} else {
			log.Printf("No Last-Modified header for %s; proceeding to download", url)
		}
	}

	if needDownload {
		log.Printf("Downloading %s from %s", artefact, url)
		resp, err := client.Get(url)
		if err != nil {
			return fmt.Errorf("error downloading %s: %v", artefact, err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("failed to download %s: HTTP status %s", artefact, resp.Status)
		}

		tmpFile := filepath.Join(downloadPath, fmt.Sprintf(".tmp-%s", artefact))
		out, err := os.Create(tmpFile)
		if err != nil {
			os.Remove(tmpFile)
			return fmt.Errorf("error creating file %s: %v", tmpFile, err)
		}

		if _, err := io.CopyBuffer(out, resp.Body, buffer); err != nil {
			out.Close()
			return fmt.Errorf("error saving file %s: %v", tmpFile, err)
		}
		out.Close()
		log.Printf("Successfully downloaded %s", artefact)

		if err := os.Rename(tmpFile, localFilePath); err != nil {
			return fmt.Errorf("error moving file %s to %s: %v", tmpFile, localFilePath, err)
		}
		log.Printf("Moved tmp file %s to %s", tmpFile, localFilePath)

		if lm := resp.Header.Get("Last-Modified"); lm != "" {
			if remoteModTime, err := time.Parse(http.TimeFormat, lm); err == nil {
				if err := os.Chtimes(localFilePath, time.Now(), remoteModTime); err != nil {
					return fmt.Errorf("error updating mod time for %s: %v", artefact, err)
				}
			} else {
				return fmt.Errorf("error parsing Last-Modified header for %s: %v", artefact, err)
			}
		}
	}
	return nil
}

func checkAndDownload(owner, repo, artefacts, downloadPath string) {
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		log.Printf("Failed to create download directory %q: %v", downloadPath, err)
		return
	}

	for _, artefact := range strings.Split(artefacts, ",") {
		artefact = strings.TrimSpace(artefact)
		if artefact == "" {
			continue
		}
		url := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", owner, repo, artefact)
		if err := download(url, artefact, downloadPath); err != nil {
			log.Printf("Failed to download artefact %s: %v", artefact, err)
		}
	}
}

func main() {
	owner := os.Getenv("GITHUB_OWNER")
	repo := os.Getenv("GITHUB_REPOSITORY")
	artefacts := os.Getenv("GITHUB_ARTEFACTS")
	downloadPath := os.Getenv("DOWNLOAD_PATH")

	if owner == "" || repo == "" || artefacts == "" || downloadPath == "" {
		log.Fatal("Missing required environment variables. Ensure GITHUB_OWNER, GITHUB_REPOSITORY, GITHUB_ARTEFACTS, and DOWNLOAD_PATH are set.")
	}

	checkIntervalStr := os.Getenv("CHECK_INTERVAL")
	runOnce := false
	checkInterval := time.Hour

	if checkIntervalStr == "" || checkIntervalStr == "0" {
		runOnce = true
		log.Println("Check interval set to 0 or empty; running only once")
	} else {
		var err error
		checkInterval, err = time.ParseDuration(checkIntervalStr)
		if err != nil {
			log.Fatalf("Invalid CHECK_INTERVAL %q; error: %v", checkIntervalStr, err)
		}
	}

	client = &http.Client{
		Transport: &http.Transport{
			MaxIdleConns:    5,
			IdleConnTimeout: 30 * time.Second,
			MaxConnsPerHost: 2,
		},
	}

	if runOnce {
		checkAndDownload(owner, repo, artefacts, downloadPath)
		log.Println("Run once mode enabled; exiting after initial check.")
		return
	}

	log.Println("Starting scheduled download check...")

	sigs := make(chan os.Signal, 1)
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(checkInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			checkAndDownload(owner, repo, artefacts, downloadPath)
		case sig := <-sigs:
			log.Printf("Received signal %s, shutting down gracefully", sig)
			return
		}
	}
}
