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

func download(url, artefact, downloadPath string) error {
	needDownload := true

	localFilePath := filepath.Join(downloadPath, artefact)
	log.Printf("Processing artefact: %s", artefact)

	if fi, err := os.Stat(localFilePath); err == nil {
		localModTime := fi.ModTime()

		req, err := http.NewRequest("HEAD", url, nil)
		if err != nil {
			log.Printf("Error creating HEAD request for %s: %v", url, err)
		} else {
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				log.Printf("Error performing HEAD request for %s: %v", url, err)
			} else {
				resp.Body.Close() // No content expected.
				lastModified := resp.Header.Get("Last-Modified")
				if lastModified != "" {
					remoteModTime, err := time.Parse(http.TimeFormat, lastModified)
					if err != nil {
						log.Printf("Error parsing Last-Modified header for %s: %v", url, err)
					} else if !remoteModTime.After(localModTime) {
						log.Printf("No new version available for %s (remote mod time: %s, local mod time: %s)",
							artefact, remoteModTime, localModTime)
						needDownload = false
					}
				} else {
					log.Printf("No Last-Modified header for %s; proceeding to download", url)
				}
			}
		}
	}

	// Download the file if needed.
	if needDownload {
		log.Printf("Downloading %s from %s", artefact, url)
		resp, err := http.Get(url)
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
			return fmt.Errorf("error creating file %s: %v", tmpFile, err)
		}
		// small buffer but honestly that's okay-ish.
		_, err = io.CopyBuffer(out, resp.Body, make([]byte, 1024))
		out.Close()
		if err != nil {
			return fmt.Errorf("error saving file %s: %v", tmpFile, err)
		}
		log.Printf("Successfully downloaded %s", artefact)

		if err := os.Rename(tmpFile, localFilePath); err != nil {
			return fmt.Errorf("error moving file %s to %s: %v", tmpFile, localFilePath, err)
		}
		log.Printf("Successfully move tmp file %s to %s", tmpFile, localFilePath)

		// Update the local file's modification time with the remote header (if available).
		if lm := resp.Header.Get("Last-Modified"); lm != "" {
			if remoteModTime, err := time.Parse(http.TimeFormat, lm); err == nil {
				err := os.Chtimes(localFilePath, time.Now(), remoteModTime)
				if err != nil {
					return fmt.Errorf("error changing last-modified header for %s: %v", artefact, err)
				}
			} else {
				return fmt.Errorf("error parsing Last-Modified header for %s: %v", artefact, err)
			}
		}
	}
	return nil
}

// checkAndDownload processes each artefact: it downloads the asset from GitHub if the
// remote file is newer than the local copy or if the file does not exist locally.
func checkAndDownload(owner, repo, artefacts, downloadPath string) {
	// Ensure the download directory exists.
	if err := os.MkdirAll(downloadPath, 0755); err != nil {
		log.Printf("Failed to create download directory %q: %v", downloadPath, err)
		return
	}

	artefactList := strings.Split(artefacts, ",")
	for _, artefact := range artefactList {
		artefact = strings.TrimSpace(artefact)
		if artefact == "" {
			continue
		}

		url := fmt.Sprintf("https://github.com/%s/%s/releases/latest/download/%s", owner, repo, artefact)
		err := download(url, artefact, downloadPath)
		if err != nil {
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
	checkInterval := time.Hour
	runOnce := false
	if checkIntervalStr == "0" || checkIntervalStr == "" {
		runOnce = true
		log.Printf("Check interval set to 0 or empty; running only once")
	} else {
		var err error
		checkInterval, err = time.ParseDuration(checkIntervalStr)
		if err != nil {
			log.Fatalf("Invalid CHECK_INTERVAL %q; error: %v", checkIntervalStr, err)
		}
	}

	log.Println("Starting scheduled download check...")

	checkAndDownload(owner, repo, artefacts, downloadPath)

	if runOnce {
		log.Println("Run once mode enabled; exiting after initial check.")
		return
	}

	// Setup signal handling for graceful shutdown.
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
