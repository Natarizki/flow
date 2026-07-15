package utils

import "fmt"

type FlowError struct {
	Code    string
	Message string
	Cause   error
}

func (e *FlowError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

func NewError(code, message string) *FlowError {
	return &FlowError{Code: code, Message: message}
}

func WrapError(code, message string, cause error) *FlowError {
	return &FlowError{Code: code, Message: message, Cause: cause}
}

var (
	ErrPeerNotFound     = NewError("PEER_NOT_FOUND", "peer does not exist")
	ErrAuthFailed       = NewError("AUTH_FAILED", "authentication failed")
	ErrInvalidFormat    = NewError("INVALID_FORMAT", "invalid .flow file format")
	ErrCacheFull        = NewError("CACHE_FULL", "cache storage limit reached")
	ErrConnectionFailed = NewError("CONNECTION_FAILED", "failed to connect to peer")
)
