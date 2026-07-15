package prefetch

import (
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

var (
	videoSrcRe = regexp.MustCompile(`<video[^>]*>[\s\S]*?<source[^>]+src="([^"]+)"[^>]*>[\s\S]*?</video>`)
	videoTagRe = regexp.MustCompile(`<video[^>]+src="([^"]+)"`)
)

const prerollBytes = 512 * 1024 // 512KB — enough for most video containers' initial moov/keyframe data

// VideoPreroller scans HTML for <video> tags and issues real HTTP Range
// requests for just the first N bytes of each video source — this is
// what actually makes playback start instantly from cache: the browser
// gets the header/first-keyframe data immediately while the rest either
// streams normally or gets pulled from peers as playback continues.
type VideoPreroller struct {
	client *http.Client
	fetch  func(url string, data []byte, contentType string) error // callback into Fetcher-like storage
}

func NewVideoPreroller(storeFunc func(url string, data []byte, contentType string) error) *VideoPreroller {
	return &VideoPreroller{
		client: &http.Client{Timeout: 10 * time.Second},
		fetch:  storeFunc,
	}
}

// ExtractVideoURLs finds all <video src="..."> and <video><source
// src="..."></video> references in HTML content, resolving relative
// URLs against the page's base URL.
func ExtractVideoURLs(htmlContent []byte, baseURL string) []string {
	content := string(htmlContent)
	var urls []string
	seen := make(map[string]bool)

	for _, m := range videoTagRe.FindAllStringSubmatch(content, -1) {
		resolved := resolveURL(m[1], baseURL)
		if resolved != "" && !seen[resolved] {
			seen[resolved] = true
			urls = append(urls, resolved)
		}
	}
	for _, m := range videoSrcRe.FindAllStringSubmatch(content, -1) {
		resolved := resolveURL(m[1], baseURL)
		if resolved != "" && !seen[resolved] {
			seen[resolved] = true
			urls = append(urls, resolved)
		}
	}
	return urls
}

func resolveURL(ref, base string) string {
	if strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://") {
		return ref
	}
	if strings.HasPrefix(ref, "//") {
		if strings.HasPrefix(base, "https:") {
			return "https:" + ref
		}
		return "http:" + ref
	}
	if strings.HasPrefix(ref, "/") {
		idx := strings.Index(base[8:], "/")
		if idx == -1 {
			return base + ref
		}
		return base[:8+idx] + ref
	}
	return "" // relative-to-current-path refs skipped, too ambiguous to resolve reliably
}

// PrerollVideo issues an HTTP Range request for the first prerollBytes
// of a video URL and stores just that prefix. If the server doesn't
// support ranges (no 206 response), it falls back to skip (rather than
// downloading the whole video, which would defeat the point of "preroll
// only").
func (p *VideoPreroller) PrerollVideo(videoURL string) error {
	req, err := http.NewRequest(http.MethodGet, videoURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Range", fmt.Sprintf("bytes=0-%d", prerollBytes-1))

	resp, err := p.client.Do(req)
	if err != nil {
		return utils.WrapError("PREROLL_FETCH", fmt.Sprintf("failed to preroll %s", videoURL), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPartialContent {
		return fmt.Errorf("server does not support range requests for %s (status %d)", videoURL, resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, prerollBytes))
	if err != nil {
		return utils.WrapError("PREROLL_READ", "failed to read preroll bytes", err)
	}

	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "video/mp4"
	}

	if err := p.fetch(videoURL, data, contentType); err != nil {
		return err
	}

	utils.LogInfo("prerolled %d bytes for video %s", len(data), videoURL)
	return nil
}

// PrerollAllInPage extracts every video URL from HTML content and
// prerolls each one, tolerating individual failures (a page might
// reference videos from CDNs that don't support ranges).
func (p *VideoPreroller) PrerollAllInPage(htmlContent []byte, baseURL string) int {
	urls := ExtractVideoURLs(htmlContent, baseURL)
	count := 0
	for _, u := range urls {
		if err := p.PrerollVideo(u); err != nil {
			utils.LogWarn("video preroll skipped for %s: %v", u, err)
			continue
		}
		count++
	}
	return count
}
