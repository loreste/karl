package ng_protocol

import (
	"bytes"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"
)

// Message parsing errors
var (
	ErrNoCookie       = errors.New("no cookie in message")
	ErrNoCommand      = errors.New("no command in message")
	ErrMalformedMsg   = errors.New("malformed message")
	ErrInvalidMessage = errors.New("invalid NG protocol message")
)

// NGMessage represents a raw NG protocol message
type NGMessage struct {
	Cookie   string
	Data     BencodeDict
	RawBytes []byte
	From     *net.UDPAddr
}

// ParseMessage parses a raw NG protocol message
// Format: <cookie> <bencode-dict>
func ParseMessage(data []byte, from *net.UDPAddr) (*NGMessage, error) {
	// Find the space separating cookie from bencode data
	spaceIdx := bytes.IndexByte(data, ' ')
	if spaceIdx == -1 {
		return nil, ErrNoCookie
	}

	cookie := string(data[:spaceIdx])
	if cookie == "" {
		return nil, ErrNoCookie
	}

	// Parse the bencode dictionary
	bencodeData := data[spaceIdx+1:]
	decoder := NewDecoder(bencodeData)
	decoded, err := decoder.Decode()
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrMalformedMsg, err)
	}

	dict, ok := decoded.(BencodeDict)
	if !ok {
		return nil, ErrInvalidMessage
	}

	return &NGMessage{
		Cookie:   cookie,
		Data:     dict,
		RawBytes: data,
		From:     from,
	}, nil
}

// ToRequest converts an NGMessage to an NGRequest
func (m *NGMessage) ToRequest() (*NGRequest, error) {
	command := DictGetString(m.Data, "command")
	if command == "" {
		return nil, ErrNoCommand
	}

	req := &NGRequest{
		Cookie:    m.Cookie,
		Command:   command,
		CallID:    DictGetString(m.Data, "call-id"),
		FromTag:   DictGetString(m.Data, "from-tag"),
		ToTag:     DictGetString(m.Data, "to-tag"),
		ViaBranch: DictGetString(m.Data, "via-branch"),
		SDP:       DictGetString(m.Data, "sdp"),
		ReceivedFrom: m.From,
		Timestamp: time.Now(),
		RawParams: m.Data,
	}

	// Parse flags array
	if flags := DictGetList(m.Data, "flags"); flags != nil {
		for _, f := range flags {
			if s, ok := f.(string); ok {
				req.Flags = append(req.Flags, s)
			}
		}
	}

	// Parse replace array
	if replace := DictGetList(m.Data, "replace"); replace != nil {
		for _, r := range replace {
			if s, ok := r.(string); ok {
				req.Replace = append(req.Replace, s)
			}
		}
	}

	// Parse direction
	if direction := DictGetList(m.Data, "direction"); direction != nil {
		for _, d := range direction {
			if s, ok := d.(string); ok {
				req.Direction = append(req.Direction, s)
			}
		}
	}

	// Parse ICE options
	req.ICE = DictGetString(m.Data, "ICE")

	// Parse DTLS options
	req.DTLS = DictGetString(m.Data, "DTLS")

	// Parse SDES
	if sdes := DictGetList(m.Data, "SDES"); sdes != nil {
		for _, s := range sdes {
			if str, ok := s.(string); ok {
				req.SDES = append(req.SDES, str)
			}
		}
	}

	// Parse transport protocol
	req.Transport = DictGetString(m.Data, "transport-protocol")

	// Parse media address
	req.MediaAddress = DictGetString(m.Data, "media address")

	// Parse address family
	req.AddressFamily = DictGetString(m.Data, "address family")

	// Parse codec options
	if codec := DictGetList(m.Data, "codec"); codec != nil {
		for _, c := range codec {
			if s, ok := c.(string); ok {
				req.Codec = append(req.Codec, s)
			}
		}
	}

	// Parse transcode options
	if transcode := DictGetList(m.Data, "transcode"); transcode != nil {
		for _, t := range transcode {
			if s, ok := t.(string); ok {
				req.Transcode = append(req.Transcode, s)
			}
		}
	}

	// Parse ptime
	if ptime := DictGetInt(m.Data, "ptime"); ptime > 0 {
		req.Ptime = int(ptime)
	}

	// Parse label options
	req.Label = DictGetString(m.Data, "label")
	req.SetLabel = DictGetString(m.Data, "set-label")
	req.FromLabel = DictGetString(m.Data, "from-label")
	req.ToLabel = DictGetString(m.Data, "to-label")

	// Parse DTMF options
	req.DTMFDigit = DictGetString(m.Data, "digit")
	if duration := DictGetInt(m.Data, "duration"); duration > 0 {
		req.DTMFDuration = int(duration)
	}

	// Parse forwarding options
	req.ForwardAddress = DictGetString(m.Data, "forward-address")
	if port := DictGetInt(m.Data, "forward-port"); port > 0 {
		req.ForwardPort = int(port)
	}

	// Parse recording options
	req.RecordCall = containsFlag(req.Flags, "record-call")
	if recordMeta := DictGetDict(m.Data, "record-meta"); recordMeta != nil {
		req.RecordingMeta = make(map[string]string)
		for k, v := range recordMeta {
			if s, ok := v.(string); ok {
				req.RecordingMeta[k] = s
			}
		}
	}

	return req, nil
}

// containsFlag checks if a flag is present in the flags list
func containsFlag(flags []string, flag string) bool {
	for _, f := range flags {
		if strings.EqualFold(f, flag) {
			return true
		}
	}
	return false
}

// BuildResponse creates a bencode-encoded response
func BuildResponse(cookie string, resp *NGResponse) ([]byte, error) {
	dict := make(map[string]interface{})

	dict["result"] = resp.Result

	if resp.ErrorReason != "" {
		dict["error-reason"] = resp.ErrorReason
	}

	if resp.SDP != "" {
		dict["sdp"] = resp.SDP
	}

	if resp.Warning != "" {
		dict["warning"] = resp.Warning
	}

	// Add stream info
	if len(resp.Streams) > 0 {
		streams := make([]interface{}, len(resp.Streams))
		for i, s := range resp.Streams {
			stream := map[string]interface{}{
				"local address": s.LocalIP,
				"local port":    s.LocalPort,
				"media type":    s.MediaType,
				"protocol":      s.Protocol,
				"index":         s.Index,
			}
			if s.LocalRTCPPort > 0 {
				stream["local RTCP port"] = s.LocalRTCPPort
			}
			if len(s.ICECandidates) > 0 {
				candidates := make([]interface{}, len(s.ICECandidates))
				for j, c := range s.ICECandidates {
					candidates[j] = map[string]interface{}{
						"foundation": c.Foundation,
						"component":  c.Component,
						"protocol":   c.Protocol,
						"priority":   c.Priority,
						"address":    c.IP,
						"port":       c.Port,
						"type":       c.Type,
					}
				}
				stream["ICE-candidates"] = candidates
			}
			if s.ICEUfrag != "" {
				stream["ICE-ufrag"] = s.ICEUfrag
			}
			if s.ICEPwd != "" {
				stream["ICE-pwd"] = s.ICEPwd
			}
			if s.Fingerprint != "" {
				stream["fingerprint"] = s.Fingerprint
				stream["fingerprint-hash"] = s.FingerprintHash
			}
			if s.Setup != "" {
				stream["setup"] = s.Setup
			}
			streams[i] = stream
		}
		dict["streams"] = streams
	}

	// Add tag info
	if len(resp.Tag) > 0 {
		tags := make(map[string]interface{})
		for name, info := range resp.Tag {
			tagDict := map[string]interface{}{
				"tag":         info.Tag,
				"in dialogue": info.InDialogue,
				"created":     info.Created,
				"media count": info.MediaCount,
			}
			if info.Label != "" {
				tagDict["label"] = info.Label
			}
			if len(info.Medias) > 0 {
				medias := make([]interface{}, len(info.Medias))
				for i, m := range info.Medias {
					medias[i] = map[string]interface{}{
						"index":      m.Index,
						"type":       m.Type,
						"protocol":   m.Protocol,
						"local IP":   m.LocalIP,
						"local port": m.LocalPort,
					}
				}
				tagDict["medias"] = medias
			}
			tags[name] = tagDict
		}
		dict["tags"] = tags
	}

	// Add statistics if present
	if resp.Stats != nil {
		stats := map[string]interface{}{
			"created":      resp.Stats.CreatedAt.Unix(),
			"duration":     int64(resp.Stats.Duration.Seconds()),
			"packets sent": resp.Stats.PacketsSent,
			"packets recv": resp.Stats.PacketsRecv,
			"bytes sent":   resp.Stats.BytesSent,
			"bytes recv":   resp.Stats.BytesRecv,
			"packet loss":  resp.Stats.PacketLoss,
			"jitter":       resp.Stats.Jitter,
			"rtt":          resp.Stats.RTT,
			"MOS":          resp.Stats.MOS,
		}
		dict["stats"] = stats

		// Add per-leg stats
		if len(resp.Stats.Legs) > 0 {
			legs := make([]interface{}, len(resp.Stats.Legs))
			for i, leg := range resp.Stats.Legs {
				legs[i] = map[string]interface{}{
					"tag":          leg.Tag,
					"SSRC":         leg.SSRC,
					"packets sent": leg.PacketsSent,
					"packets recv": leg.PacketsRecv,
					"bytes sent":   leg.BytesSent,
					"bytes recv":   leg.BytesRecv,
					"packet loss":  leg.PacketLoss,
					"jitter":       leg.Jitter,
					"rtt":          leg.RTT,
				}
			}
			dict["legs"] = legs
		}
	}

	// Add query response fields
	if resp.Created > 0 {
		dict["created"] = resp.Created
	}
	if resp.LastSignal > 0 {
		dict["last signal"] = resp.LastSignal
	}

	// Add extra fields
	for k, v := range resp.Extra {
		dict[k] = v
	}

	// Encode the response
	encoder := NewEncoder()
	encoded, err := encoder.Encode(dict)
	if err != nil {
		return nil, err
	}

	// Prepend cookie
	result := make([]byte, 0, len(cookie)+1+len(encoded))
	result = append(result, []byte(cookie)...)
	result = append(result, ' ')
	result = append(result, encoded...)

	return result, nil
}

// ErrorResponse creates an error response
func ErrorResponse(cookie string, reason string) ([]byte, error) {
	return BuildResponse(cookie, &NGResponse{
		Result:      ResultError,
		ErrorReason: reason,
	})
}

// PongResponse creates a pong response
func PongResponse(cookie string) ([]byte, error) {
	return BuildResponse(cookie, &NGResponse{
		Result: ResultPong,
	})
}

// OKResponse creates a simple OK response
func OKResponse(cookie string) ([]byte, error) {
	return BuildResponse(cookie, &NGResponse{
		Result: ResultOK,
	})
}

// OKResponseWithSDP creates an OK response with SDP
func OKResponseWithSDP(cookie string, sdp string, streams []StreamInfo) ([]byte, error) {
	return BuildResponse(cookie, &NGResponse{
		Result:  ResultOK,
		SDP:     sdp,
		Streams: streams,
	})
}

// ParseCallID extracts call-id from various formats
func ParseCallID(callID string) string {
	// Remove any branch suffix
	if idx := strings.Index(callID, ";"); idx != -1 {
		return callID[:idx]
	}
	return callID
}

// ParseDirection extracts direction info from direction array
func ParseDirection(directions []string) (external, internal string) {
	if len(directions) >= 1 {
		external = directions[0]
	}
	if len(directions) >= 2 {
		internal = directions[1]
	}
	return
}
