package prefetch

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/Natarizki/flow/pkg/utils"
)

// WikipediaPrecacher pulls real article content from Wikipedia's public
// REST API (no scraping, this is the documented API) and feeds each
// article through the normal Fetcher pipeline so it ends up compressed,
// cached, and chunk-split like anything else FLOW caches. This isn't a
// full offline ZIM-format Wikipedia dump (those are multi-GB files
// needing a dedicated ZIM reader) — it's real, working "cache these
// specific/popular articles for offline reading", which is what's
// actually achievable without bundling a several-gigabyte binary asset.
type WikipediaPrecacher struct {
	client *http.Client
	fetch  func(url string) error // delegates to fetcher.Fetcher.Fetch
}

func NewWikipediaPrecacher(fetchFunc func(url string) error) *WikipediaPrecacher {
	return &WikipediaPrecacher{
		client: &http.Client{Timeout: 15 * time.Second},
		fetch:  fetchFunc,
	}
}

type wikiFeaturedResponse struct {
	MostRead struct {
		Articles []struct {
			Title string `json:"title"`
		} `json:"articles"`
	} `json:"mostread"`
}

// FetchMostReadTitles gets today's most-read article titles from
// Wikipedia's official Feed API — a real, documented, public endpoint
// (no API key needed), giving a genuinely useful default set to
// pre-cache rather than an arbitrary hardcoded list.
func (w *WikipediaPrecacher) FetchMostReadTitles(lang string) ([]string, error) {
	if lang == "" {
		lang = "en"
	}
	date := time.Now().UTC()
	url := fmt.Sprintf("https://api.wikimedia.org/feed/v1/wikipedia/%s/featured/%04d/%02d/%02d",
		lang, date.Year(), date.Month(), date.Day())

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "FLOW/0.1.0 (+https://github.com/Natarizki/flow)")

	resp, err := w.client.Do(req)
	if err != nil {
		return nil, utils.WrapError("WIKI_FEED_FETCH", "failed to fetch Wikipedia featured feed", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Wikipedia feed API returned status %d", resp.StatusCode)
	}

	var parsed wikiFeaturedResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return nil, utils.WrapError("WIKI_FEED_PARSE", "failed to parse Wikipedia feed response", err)
	}

	titles := make([]string, 0, len(parsed.MostRead.Articles))
	for _, a := range parsed.MostRead.Articles {
		titles = append(titles, a.Title)
	}
	return titles, nil
}

// articleURL builds the real Wikipedia article URL for a title — this
// is what actually gets fetched, compressed, and cached by the normal
// Fetcher, exactly like any browsed page.
func articleURL(lang, title string) string {
	return fmt.Sprintf("https://%s.wikipedia.org/wiki/%s", lang, title)
}

// PrecacheTitles fetches and caches a specific list of article titles.
// Tolerates individual failures (redirects, deleted articles, etc.)
// and returns how many succeeded.
func (w *WikipediaPrecacher) PrecacheTitles(lang string, titles []string) int {
	succeeded := 0
	for _, title := range titles {
		url := articleURL(lang, title)
		if err := w.fetch(url); err != nil {
			utils.LogWarn("wikipedia precache failed for %s: %v", title, err)
			continue
		}
		succeeded++
	}
	utils.LogInfo("wikipedia precache: cached %d/%d articles", succeeded, len(titles))
	return succeeded
}

// PrecacheMostRead is the one-call convenience path: fetch today's
// most-read list, then cache every one of them.
func (w *WikipediaPrecacher) PrecacheMostRead(lang string) (int, error) {
	titles, err := w.FetchMostReadTitles(lang)
	if err != nil {
		return 0, err
	}
	return w.PrecacheTitles(lang, titles), nil
}
