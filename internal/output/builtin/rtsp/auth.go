package rtsp

import (
	"encoding/base64"
	"fmt"
	"strings"

	"stream-source-tester/internal/config"
)

type authConfig struct {
	mode     string
	username string
	password string
	realm    string
}

func loadAuthConfig(cfg config.OutputConfig) authConfig {
	opts := cfg.Options
	ac := authConfig{
		mode:  "none",
		realm: "stream-source-tester",
	}
	if opts == nil {
		return ac
	}
	if v := strings.TrimSpace(opts["auth.mode"]); v != "" {
		ac.mode = strings.ToLower(v)
	}
	if v := opts["auth.username"]; v != "" {
		ac.username = v
	}
	if v := opts["auth.password"]; v != "" {
		ac.password = v
	}
	if v := strings.TrimSpace(opts["auth.realm"]); v != "" {
		ac.realm = v
	}
	return ac
}

func (a authConfig) enabled() bool {
	return a.mode == "basic"
}

// requiresAuth reports whether the given RTSP method must be authenticated.
// OPTIONS stays anonymous so clients can discover server capabilities.
func requiresAuth(method string) bool {
	switch strings.ToUpper(method) {
	case "OPTIONS":
		return false
	default:
		return true
	}
}

// checkBasicAuth validates the Authorization header against the configured
// credentials. It returns true when the request is authorized.
func (a authConfig) checkBasicAuth(authorization string) bool {
	if !a.enabled() {
		return true
	}
	const prefix = "Basic "
	if !strings.HasPrefix(authorization, prefix) {
		return false
	}
	decoded, err := base64.StdEncoding.DecodeString(strings.TrimSpace(authorization[len(prefix):]))
	if err != nil {
		return false
	}
	user, pass, ok := strings.Cut(string(decoded), ":")
	if !ok {
		return false
	}
	return user == a.username && pass == a.password
}

func (a authConfig) challengeHeader() map[string]string {
	return map[string]string{
		"WWW-Authenticate": fmt.Sprintf("Basic realm=%q", a.realm),
	}
}
