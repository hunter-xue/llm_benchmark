package bench

import (
	"context"
	"errors"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const (
	ErrorCategoryRateLimit   = "rate_limit"
	ErrorCategoryAuth        = "auth"
	ErrorCategoryBadRequest  = "bad_request"
	ErrorCategoryServer      = "server_error"
	ErrorCategoryTimeout     = "timeout"
	ErrorCategoryConnection  = "connection"
	ErrorCategoryEmptyOutput = "empty_output"
	ErrorCategoryStream      = "stream_error"
	ErrorCategoryParse       = "parse_error"
	ErrorCategoryClient      = "client_error"
	ErrorCategoryOther       = "other"
)

var httpStatusPattern = regexp.MustCompile(`\bhttp\s+([0-9]{3})\b`)

// ClassifyError maps benchmark errors into stable categories for reporting.
func ClassifyError(err error) string {
	if err == nil {
		return ErrorCategoryOther
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return ErrorCategoryTimeout
	}

	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrorCategoryTimeout
	}

	msg := strings.ToLower(err.Error())
	if status, ok := httpStatusCode(msg); ok {
		switch {
		case status == 429:
			return ErrorCategoryRateLimit
		case status == 401 || status == 403:
			return ErrorCategoryAuth
		case status == 400:
			return ErrorCategoryBadRequest
		case status >= 500 && status <= 599:
			return ErrorCategoryServer
		}
	}

	switch {
	case strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded"):
		return ErrorCategoryTimeout
	case strings.Contains(msg, "no output tokens received") || strings.Contains(msg, "no answer content"):
		return ErrorCategoryEmptyOutput
	case strings.Contains(msg, "failed to read stream"):
		return ErrorCategoryStream
	case strings.Contains(msg, "failed to parse response json"):
		return ErrorCategoryParse
	case strings.Contains(msg, "failed to marshal") || strings.Contains(msg, "failed to create request"):
		return ErrorCategoryClient
	case isConnectionError(err, msg):
		return ErrorCategoryConnection
	}

	return ErrorCategoryOther
}

func recordError(details map[string]int, categories map[string]int, err error) {
	if err == nil {
		return
	}
	details[err.Error()]++
	categories[ClassifyError(err)]++
}

func httpStatusCode(msg string) (int, bool) {
	match := httpStatusPattern.FindStringSubmatch(msg)
	if len(match) != 2 {
		return 0, false
	}
	status, err := strconv.Atoi(match[1])
	if err != nil {
		return 0, false
	}
	return status, true
}

func isConnectionError(err error, msg string) bool {
	var urlErr *url.Error
	if errors.As(err, &urlErr) && urlErr.Err != nil {
		return true
	}

	keywords := []string{
		"connection refused",
		"connection reset",
		"connection aborted",
		"no such host",
		"network is unreachable",
		"broken pipe",
	}
	for _, keyword := range keywords {
		if strings.Contains(msg, keyword) {
			return true
		}
	}
	return false
}
