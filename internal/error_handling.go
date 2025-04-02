package internal

import (
	"errors"
	"fmt"
	"runtime"
	"strings"
)

// KarlError is a custom error type that includes contextual information
type KarlError struct {
	Err       error  // The underlying error
	Code      string // Error code for categorization
	Component string // The component where the error occurred
	Op        string // The operation being performed
	File      string // The file where the error occurred
	Line      int    // The line where the error occurred
	Context   string // Additional contextual information
}

// Error returns the error message
func (e *KarlError) Error() string {
	var sb strings.Builder
	
	sb.WriteString(fmt.Sprintf("[%s] %s in %s: ", e.Code, e.Op, e.Component))
	
	if e.Err != nil {
		sb.WriteString(e.Err.Error())
	} else {
		sb.WriteString("unknown error")
	}
	
	if e.Context != "" {
		sb.WriteString(fmt.Sprintf(" (%s)", e.Context))
	}
	
	if e.File != "" && e.Line > 0 {
		sb.WriteString(fmt.Sprintf(" at %s:%d", e.File, e.Line))
	}
	
	return sb.String()
}

// Unwrap returns the underlying error
func (e *KarlError) Unwrap() error {
	return e.Err
}

// Is reports whether any error in err's chain matches target.
func (e *KarlError) Is(target error) bool {
	if target == nil {
		return e.Err == target
	}
	
	var karlErr *KarlError
	if errors.As(target, &karlErr) {
		return e.Code == karlErr.Code
	}
	
	return errors.Is(e.Err, target)
}

// ErrorCode constants for categorization
const (
	// Infrastructure errors
	ErrCodeNetwork        = "NETWORK_ERROR"
	ErrCodeIO             = "IO_ERROR"
	ErrCodeConfiguration  = "CONFIG_ERROR"
	ErrCodeDatabase       = "DB_ERROR"
	
	// Media processing errors
	ErrCodeRTP            = "RTP_ERROR"
	ErrCodeSRTP           = "SRTP_ERROR"
	ErrCodeCodec          = "CODEC_ERROR"
	ErrCodeTranscoding    = "TRANSCODING_ERROR"
	
	// Protocol errors
	ErrCodeSIP            = "SIP_ERROR"
	ErrCodeSDP            = "SDP_ERROR"
	ErrCodeWebRTC         = "WEBRTC_ERROR"
	ErrCodeDTLS           = "DTLS_ERROR"
	
	// Internal errors
	ErrCodeInternal       = "INTERNAL_ERROR"
	ErrCodeResourceLimit  = "RESOURCE_LIMIT"
	ErrCodeTimeout        = "TIMEOUT"
)

// NewError creates a new error with contextual information
func NewError(err error, code string, component string, op string) *KarlError {
	_, file, line, _ := runtime.Caller(1)
	
	// Extract just the filename from the full path
	fileParts := strings.Split(file, "/")
	shortFile := fileParts[len(fileParts)-1]
	
	// Increment the appropriate error metric
	IncrementErrorMetric(code)
	
	return &KarlError{
		Err:       err,
		Code:      code,
		Component: component,
		Op:        op,
		File:      shortFile,
		Line:      line,
	}
}

// WithContext adds contextual information to the error
func (e *KarlError) WithContext(ctx string) *KarlError {
	e.Context = ctx
	return e
}

// IsNetworkError checks if an error is a network error
func IsNetworkError(err error) bool {
	var karlErr *KarlError
	if errors.As(err, &karlErr) {
		return karlErr.Code == ErrCodeNetwork
	}
	return false
}

// IsRTPError checks if an error is an RTP error
func IsRTPError(err error) bool {
	var karlErr *KarlError
	if errors.As(err, &karlErr) {
		return karlErr.Code == ErrCodeRTP || karlErr.Code == ErrCodeSRTP
	}
	return false
}

// IsTemporary indicates if an error is likely temporary and retryable
func IsTemporary(err error) bool {
	var karlErr *KarlError
	if errors.As(err, &karlErr) {
		return karlErr.Code == ErrCodeNetwork || 
		       karlErr.Code == ErrCodeTimeout || 
		       strings.Contains(karlErr.Error(), "temporary")
	}
	return false
}