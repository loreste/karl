package internal

import (
	"errors"
	"fmt"
	"net"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Input validation errors
var (
	ErrInputTooLong       = errors.New("input exceeds maximum length")
	ErrInputTooShort      = errors.New("input below minimum length")
	ErrInvalidCharacters  = errors.New("input contains invalid characters")
	ErrInvalidFormat      = errors.New("input has invalid format")
	ErrInvalidIPAddress   = errors.New("invalid IP address")
	ErrInvalidPort        = errors.New("invalid port number")
	ErrInvalidCallID      = errors.New("invalid call ID format")
	ErrInvalidSDP         = errors.New("invalid SDP format")
	ErrControlCharacters  = errors.New("input contains control characters")
	ErrNullBytes          = errors.New("input contains null bytes")
	ErrInvalidUTF8        = errors.New("input contains invalid UTF-8")
)

// InputValidator provides input validation functions
type InputValidator struct {
	maxCallIDLength   int
	maxSDPLength      int
	maxTagLength      int
	maxAddressLength  int
	allowedSDPChars   *regexp.Regexp
	callIDPattern     *regexp.Regexp
	tagPattern        *regexp.Regexp
}

// InputValidatorConfig holds configuration for input validation
type InputValidatorConfig struct {
	MaxCallIDLength  int
	MaxSDPLength     int
	MaxTagLength     int
	MaxAddressLength int
}

// DefaultInputValidatorConfig returns sensible defaults
func DefaultInputValidatorConfig() *InputValidatorConfig {
	return &InputValidatorConfig{
		MaxCallIDLength:  256,
		MaxSDPLength:     65536,
		MaxTagLength:     128,
		MaxAddressLength: 256,
	}
}

// NewInputValidator creates a new input validator
func NewInputValidator(config *InputValidatorConfig) *InputValidator {
	if config == nil {
		config = DefaultInputValidatorConfig()
	}

	return &InputValidator{
		maxCallIDLength:  config.MaxCallIDLength,
		maxSDPLength:     config.MaxSDPLength,
		maxTagLength:     config.MaxTagLength,
		maxAddressLength: config.MaxAddressLength,
		allowedSDPChars:  regexp.MustCompile(`^[\x20-\x7E\r\n\t]*$`),
		callIDPattern:    regexp.MustCompile(`^[a-zA-Z0-9._~:/?#\[\]@!$&'()*+,;=-]+$`),
		tagPattern:       regexp.MustCompile(`^[a-zA-Z0-9._~-]+$`),
	}
}

// ValidateCallID validates a SIP Call-ID
func (v *InputValidator) ValidateCallID(callID string) error {
	if callID == "" {
		return fmt.Errorf("%w: call ID is empty", ErrInvalidCallID)
	}

	if len(callID) > v.maxCallIDLength {
		return fmt.Errorf("%w: call ID length %d exceeds max %d", ErrInputTooLong, len(callID), v.maxCallIDLength)
	}

	if err := v.checkBasicSafety(callID); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidCallID, err)
	}

	if !v.callIDPattern.MatchString(callID) {
		return fmt.Errorf("%w: contains invalid characters", ErrInvalidCallID)
	}

	return nil
}

// ValidateTag validates a SIP From/To tag
func (v *InputValidator) ValidateTag(tag string) error {
	if tag == "" {
		return nil // Empty tags are allowed
	}

	if len(tag) > v.maxTagLength {
		return fmt.Errorf("%w: tag length %d exceeds max %d", ErrInputTooLong, len(tag), v.maxTagLength)
	}

	if err := v.checkBasicSafety(tag); err != nil {
		return err
	}

	if !v.tagPattern.MatchString(tag) {
		return fmt.Errorf("%w: tag contains invalid characters", ErrInvalidFormat)
	}

	return nil
}

// ValidateSDP validates SDP content
func (v *InputValidator) ValidateSDP(sdp string) error {
	if sdp == "" {
		return nil // Empty SDP allowed for some operations
	}

	if len(sdp) > v.maxSDPLength {
		return fmt.Errorf("%w: SDP length %d exceeds max %d", ErrInputTooLong, len(sdp), v.maxSDPLength)
	}

	if err := v.checkBasicSafety(sdp); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSDP, err)
	}

	// SDP should only contain printable ASCII plus CRLF and tab
	if !v.allowedSDPChars.MatchString(sdp) {
		return fmt.Errorf("%w: contains non-printable characters", ErrInvalidSDP)
	}

	// Basic SDP structure validation
	lines := strings.Split(sdp, "\n")
	hasVersion := false
	hasOrigin := false
	hasSession := false

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		if len(line) < 2 || line[1] != '=' {
			continue // Skip malformed lines
		}

		switch line[0] {
		case 'v':
			hasVersion = true
		case 'o':
			hasOrigin = true
		case 's':
			hasSession = true
		}
	}

	if !hasVersion || !hasOrigin || !hasSession {
		return fmt.Errorf("%w: missing required fields (v=, o=, s=)", ErrInvalidSDP)
	}

	return nil
}

// ValidateIPAddress validates an IP address
func (v *InputValidator) ValidateIPAddress(addr string) error {
	if addr == "" {
		return fmt.Errorf("%w: address is empty", ErrInvalidIPAddress)
	}

	if len(addr) > v.maxAddressLength {
		return fmt.Errorf("%w: address length %d exceeds max %d", ErrInputTooLong, len(addr), v.maxAddressLength)
	}

	ip := net.ParseIP(addr)
	if ip == nil {
		return fmt.Errorf("%w: '%s' is not a valid IP", ErrInvalidIPAddress, addr)
	}

	return nil
}

// ValidatePort validates a port number
func (v *InputValidator) ValidatePort(port int) error {
	if port < 1 || port > 65535 {
		return fmt.Errorf("%w: port %d out of range 1-65535", ErrInvalidPort, port)
	}
	return nil
}

// ValidateHostPort validates a host:port combination
func (v *InputValidator) ValidateHostPort(hostPort string) error {
	if hostPort == "" {
		return errors.New("host:port is empty")
	}

	host, portStr, err := net.SplitHostPort(hostPort)
	if err != nil {
		return fmt.Errorf("invalid host:port format: %w", err)
	}

	// Validate host (can be IP or hostname)
	if ip := net.ParseIP(host); ip == nil {
		// Try as hostname
		if !isValidHostname(host) {
			return fmt.Errorf("invalid hostname: %s", host)
		}
	}

	// Validate port
	var port int
	if _, err := fmt.Sscanf(portStr, "%d", &port); err != nil {
		return fmt.Errorf("invalid port: %s", portStr)
	}
	return v.ValidatePort(port)
}

// ValidateCodec validates a codec name
func (v *InputValidator) ValidateCodec(codec string) error {
	if codec == "" {
		return nil
	}

	if len(codec) > 64 {
		return fmt.Errorf("%w: codec name too long", ErrInputTooLong)
	}

	// Codec names should start with a letter and be alphanumeric with some allowed special chars
	pattern := regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9/_.-]*$`)
	if !pattern.MatchString(codec) {
		return fmt.Errorf("%w: invalid codec name format", ErrInvalidFormat)
	}

	return nil
}

// checkBasicSafety performs basic safety checks on input
func (v *InputValidator) checkBasicSafety(s string) error {
	// Check for valid UTF-8
	if !utf8.ValidString(s) {
		return ErrInvalidUTF8
	}

	// Check for null bytes
	if strings.ContainsRune(s, 0) {
		return ErrNullBytes
	}

	// Check for control characters (except common whitespace)
	for _, r := range s {
		if unicode.IsControl(r) && r != '\n' && r != '\r' && r != '\t' {
			return ErrControlCharacters
		}
	}

	return nil
}

// SanitizeString removes potentially dangerous characters
func (v *InputValidator) SanitizeString(s string, maxLen int) string {
	// Truncate if too long
	if len(s) > maxLen {
		s = s[:maxLen]
	}

	// Remove null bytes
	s = strings.ReplaceAll(s, "\x00", "")

	// Remove control characters except whitespace
	var result strings.Builder
	for _, r := range s {
		if !unicode.IsControl(r) || r == '\n' || r == '\r' || r == '\t' {
			result.WriteRune(r)
		}
	}

	return result.String()
}

// SanitizeCallID sanitizes a call ID
func (v *InputValidator) SanitizeCallID(callID string) string {
	// Remove any invalid characters
	var result strings.Builder
	for _, r := range callID {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == '.' || r == '_' ||
			r == '-' || r == '@' {
			result.WriteRune(r)
		}
	}

	s := result.String()
	if len(s) > v.maxCallIDLength {
		s = s[:v.maxCallIDLength]
	}
	return s
}

// RequestValidator validates complete requests
type RequestValidator struct {
	inputValidator *InputValidator
	maxRequestSize int64
	maxBodySize    int64
}

// NewRequestValidator creates a new request validator
func NewRequestValidator(maxRequestSize, maxBodySize int64) *RequestValidator {
	return &RequestValidator{
		inputValidator: NewInputValidator(nil),
		maxRequestSize: maxRequestSize,
		maxBodySize:    maxBodySize,
	}
}

// NGRequestValidation holds validation rules for NG protocol requests
type NGRequestValidation struct {
	CallID    string
	FromTag   string
	ToTag     string
	SDP       string
	Command   string
	Interface string
	Address   string
}

// ValidateNGRequest validates an NG protocol request
func (v *RequestValidator) ValidateNGRequest(req *NGRequestValidation) []error {
	var errs []error

	if req.CallID != "" {
		if err := v.inputValidator.ValidateCallID(req.CallID); err != nil {
			errs = append(errs, fmt.Errorf("call-id: %w", err))
		}
	}

	if req.FromTag != "" {
		if err := v.inputValidator.ValidateTag(req.FromTag); err != nil {
			errs = append(errs, fmt.Errorf("from-tag: %w", err))
		}
	}

	if req.ToTag != "" {
		if err := v.inputValidator.ValidateTag(req.ToTag); err != nil {
			errs = append(errs, fmt.Errorf("to-tag: %w", err))
		}
	}

	if req.SDP != "" {
		if err := v.inputValidator.ValidateSDP(req.SDP); err != nil {
			errs = append(errs, fmt.Errorf("sdp: %w", err))
		}
	}

	if req.Address != "" {
		if err := v.inputValidator.ValidateIPAddress(req.Address); err != nil {
			errs = append(errs, fmt.Errorf("address: %w", err))
		}
	}

	return errs
}

// SDPValidator provides specialized SDP validation
type SDPValidator struct {
	maxMediaSections   int
	maxAttributeLength int
	maxCandidates      int
}

// NewSDPValidator creates a new SDP validator
func NewSDPValidator() *SDPValidator {
	return &SDPValidator{
		maxMediaSections:   50,
		maxAttributeLength: 4096,
		maxCandidates:      100,
	}
}

// ValidateSDP performs detailed SDP validation
func (v *SDPValidator) ValidateSDP(sdp string) error {
	if sdp == "" {
		return nil
	}

	lines := strings.Split(sdp, "\n")
	mediaSections := 0
	candidates := 0

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}

		// Check line length
		if len(line) > v.maxAttributeLength {
			return fmt.Errorf("SDP line exceeds maximum length")
		}

		// Count media sections
		if strings.HasPrefix(line, "m=") {
			mediaSections++
			if mediaSections > v.maxMediaSections {
				return fmt.Errorf("too many media sections (max %d)", v.maxMediaSections)
			}
		}

		// Count ICE candidates
		if strings.HasPrefix(line, "a=candidate:") {
			candidates++
			if candidates > v.maxCandidates {
				return fmt.Errorf("too many ICE candidates (max %d)", v.maxCandidates)
			}
		}
	}

	return nil
}

// BencodeValidator validates bencode input
type BencodeValidator struct {
	maxDepth      int
	maxStringLen  int
	maxListLen    int
	maxDictKeys   int
}

// NewBencodeValidator creates a new bencode validator
func NewBencodeValidator() *BencodeValidator {
	return &BencodeValidator{
		maxDepth:     20,
		maxStringLen: 1024 * 1024, // 1MB
		maxListLen:   1000,
		maxDictKeys:  500,
	}
}

// ValidateBencodeDepth checks bencode nesting depth
func (v *BencodeValidator) ValidateBencodeDepth(data []byte) error {
	depth := 0
	maxSeen := 0

	for _, b := range data {
		switch b {
		case 'l', 'd': // list or dict start
			depth++
			if depth > maxSeen {
				maxSeen = depth
			}
			if depth > v.maxDepth {
				return fmt.Errorf("bencode nesting depth exceeds maximum %d", v.maxDepth)
			}
		case 'e': // end
			depth--
		}
	}

	return nil
}

// Global validator
var globalInputValidator = NewInputValidator(nil)

// ValidateCallID validates a call ID using the global validator
func ValidateCallID(callID string) error {
	return globalInputValidator.ValidateCallID(callID)
}

// ValidateSDP validates SDP using the global validator
func ValidateSDP(sdp string) error {
	return globalInputValidator.ValidateSDP(sdp)
}

// ValidateTag validates a tag using the global validator
func ValidateTag(tag string) error {
	return globalInputValidator.ValidateTag(tag)
}

// ValidateIPAddress validates an IP using the global validator
func ValidateIPAddress(addr string) error {
	return globalInputValidator.ValidateIPAddress(addr)
}
