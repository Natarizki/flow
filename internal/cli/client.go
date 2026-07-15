package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
)

// apiBaseURL bisa di-override lewat `flc config set api_url <url>`
const defaultAPIURL = "http://localhost:7677"

func apiBaseURL() string {
	if v := os.Getenv("FLC_API_URL"); v != "" {
		return v
	}
	return defaultAPIURL
}

func tokenFilePath() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".flow", "token")
}

func SaveToken(token string) error {
	dir := filepath.Dir(tokenFilePath())
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	return os.WriteFile(tokenFilePath(), []byte(token), 0600)
}

func LoadToken() (string, error) {
	data, err := os.ReadFile(tokenFilePath())
	if err != nil {
		return "", fmt.Errorf("not logged in (no token found), run: flc auth login")
	}
	return string(data), nil
}

func ClearToken() error {
	return os.Remove(tokenFilePath())
}

func apiPost(path string, body interface{}, out interface{}) error {
	data, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", apiBaseURL()+path, jsonReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	if token, err := LoadToken(); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach flow daemon at %s: %w", apiBaseURL(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, errBody["error"])
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func apiGet(path string, out interface{}) error {
	req, err := http.NewRequest("GET", apiBaseURL()+path, nil)
	if err != nil {
		return err
	}
	if token, err := LoadToken(); err == nil {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach flow daemon at %s: %w", apiBaseURL(), err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		var errBody map[string]string
		json.NewDecoder(resp.Body).Decode(&errBody)
		return fmt.Errorf("server error (%d): %s", resp.StatusCode, errBody["error"])
	}

	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}
