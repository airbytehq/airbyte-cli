package client

import (
	"encoding/json"
	"fmt"
)

const (
	ExitSuccess    = 0
	ExitGeneral    = 1
	ExitAuth       = 2
	ExitNotFound   = 3
	ExitValidation = 4
)

type APIError struct {
	Type       string          `json:"type"`
	Message    string          `json:"message"`
	StatusCode int             `json:"status_code"`
	Retryable  bool            `json:"retryable"`
	Detail     json.RawMessage `json:"detail,omitempty"`
	Hint       string          `json:"hint,omitempty"`
}

func (e *APIError) Error() string {
	return fmt.Sprintf("%s (status %d): %s", e.Type, e.StatusCode, e.Message)
}

func (e *APIError) ExitCode() int {
	switch {
	case e.StatusCode == 401 || e.StatusCode == 403:
		return ExitAuth
	case e.StatusCode == 404:
		return ExitNotFound
	case e.StatusCode == 400 || e.StatusCode == 422:
		return ExitValidation
	default:
		return ExitGeneral
	}
}

func NewValidationError(message, hint string) *APIError {
	return &APIError{
		Type:       "validation_error",
		Message:    message,
		StatusCode: 400,
		Hint:       hint,
	}
}

func NewNotFoundError(message, hint string) *APIError {
	return &APIError{
		Type:       "not_found",
		Message:    message,
		StatusCode: 404,
		Hint:       hint,
	}
}

func newAPIError(statusCode int, message string, body []byte) *APIError {
	typ := errorType(statusCode)
	e := &APIError{
		Type:       typ,
		Message:    message,
		StatusCode: statusCode,
		Retryable:  isRetryable(statusCode),
	}
	if len(body) > 0 && json.Valid(body) {
		e.Detail = json.RawMessage(body)
	}
	return e
}

func errorType(statusCode int) string {
	switch {
	case statusCode == 401:
		return "unauthorized"
	case statusCode == 403:
		return "forbidden"
	case statusCode == 404:
		return "not_found"
	case statusCode == 400 || statusCode == 422:
		return "validation_error"
	case statusCode == 429:
		return "rate_limited"
	case statusCode >= 500:
		return "server_error"
	default:
		return "api_error"
	}
}

func isRetryable(statusCode int) bool {
	switch statusCode {
	case 429, 502, 503, 504:
		return true
	default:
		return false
	}
}
