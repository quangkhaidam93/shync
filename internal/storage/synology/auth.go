package synology

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	"github.com/quangkhaidam93/shync/internal/config"
)

type authResponse struct {
	Success bool `json:"success"`
	Data    struct {
		SID      string `json:"sid"`
		DeviceID string `json:"did"`
	} `json:"data"`
	Error struct {
		Code int `json:"code"`
	} `json:"error"`
}

func newHTTPClient(cfg *config.SynologyConfig) *http.Client {
	transport := &http.Transport{}
	if !cfg.VerifySSL {
		transport.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
	}
	return &http.Client{Transport: transport}
}

func baseURL(cfg *config.SynologyConfig) string {
	scheme := "http"
	if cfg.HTTPS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d", scheme, cfg.Host, cfg.Port)
}

func login(client *http.Client, base string, cfg *config.Config) (string, error) {
	params := url.Values{
		"api":                 {"SYNO.API.Auth"},
		"version":             {"6"},
		"method":              {"login"},
		"account":             {cfg.Synology.Username},
		"passwd":              {cfg.Synology.Password},
		"format":              {"sid"},
		"enable_device_token": {"yes"},
		"device_name":         {"shync"},
	}

	if cfg.Synology.DeviceID != "" {
		params.Set("device_id", cfg.Synology.DeviceID)
	}

	sid, err := doLogin(client, base, params, cfg)
	if err != nil {
		return "", err
	}
	return sid, nil
}

func doLogin(client *http.Client, base string, params url.Values, cfg *config.Config) (string, error) {
	resp, err := client.Get(base + "/webapi/auth.cgi?" + params.Encode())
	if err != nil {
		return "", fmt.Errorf("login request failed: %w", err)
	}
	defer resp.Body.Close()

	var result authResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("parsing login response: %w", err)
	}

	if !result.Success {
		// Error 403 = 2FA required; prompt for OTP if we haven't already
		if result.Error.Code == 403 && params.Get("otp_code") == "" {
			return promptOTPAndRetry(client, base, params, cfg)
		}
		return "", fmt.Errorf("login failed (error code: %d)", result.Error.Code)
	}

	// Persist device_id if the server returned one
	if result.Data.DeviceID != "" && result.Data.DeviceID != cfg.Synology.DeviceID {
		cfg.Synology.DeviceID = result.Data.DeviceID
		if err := cfg.Save(); err != nil {
			fmt.Printf("Warning: failed to save device_id: %v\n", err)
		}
	}

	return result.Data.SID, nil
}

func promptOTPAndRetry(client *http.Client, base string, params url.Values, cfg *config.Config) (string, error) {
	fmt.Print("Enter OTP code: ")
	var otp string
	if _, err := fmt.Scanln(&otp); err != nil {
		return "", fmt.Errorf("reading OTP code: %w", err)
	}
	params.Set("otp_code", otp)
	return doLogin(client, base, params, cfg)
}
