package rtsp

import (
	"fmt"
	"strings"

	"stream-source-tester/internal/config"
)

func configuredStatusLine(cfg config.OutputConfig, method string) (string, bool) {
	if cfg.Options == nil {
		return "", false
	}
	key := "methodStatus." + strings.ToUpper(method)
	value, ok := cfg.Options[key]
	if !ok || strings.TrimSpace(value) == "" {
		return "", false
	}
	if strings.HasPrefix(value, "RTSP/1.0") {
		return value, true
	}
	return fmt.Sprintf("RTSP/1.0 %s", value), true
}
