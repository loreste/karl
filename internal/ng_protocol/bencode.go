package ng_protocol

import (
	"bytes"
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// Bencode errors
var (
	ErrInvalidBencode   = errors.New("invalid bencode data")
	ErrUnexpectedEnd    = errors.New("unexpected end of data")
	ErrInvalidType      = errors.New("invalid type for bencode")
	ErrNegativeLength   = errors.New("negative string length")
)

// BencodeValue represents a decoded bencode value
type BencodeValue interface{}

// BencodeDict represents a bencode dictionary
type BencodeDict map[string]BencodeValue

// BencodeList represents a bencode list
type BencodeList []BencodeValue

// Encoder handles bencode encoding
type Encoder struct {
	buf *bytes.Buffer
}

// NewEncoder creates a new bencode encoder
func NewEncoder() *Encoder {
	return &Encoder{buf: new(bytes.Buffer)}
}

// Encode encodes a value to bencode format
func (e *Encoder) Encode(v interface{}) ([]byte, error) {
	e.buf.Reset()
	if err := e.encode(v); err != nil {
		return nil, err
	}
	return e.buf.Bytes(), nil
}

// encode recursively encodes values
func (e *Encoder) encode(v interface{}) error {
	switch val := v.(type) {
	case int:
		return e.encodeInt(int64(val))
	case int64:
		return e.encodeInt(val)
	case int32:
		return e.encodeInt(int64(val))
	case uint:
		return e.encodeInt(int64(val))
	case uint32:
		return e.encodeInt(int64(val))
	case uint64:
		return e.encodeInt(int64(val))
	case float32:
		// Bencode doesn't support floats, encode as string
		return e.encodeString(strconv.FormatFloat(float64(val), 'f', -1, 32))
	case float64:
		// Bencode doesn't support floats, encode as string
		return e.encodeString(strconv.FormatFloat(val, 'f', -1, 64))
	case bool:
		if val {
			return e.encodeInt(1)
		}
		return e.encodeInt(0)
	case string:
		return e.encodeString(val)
	case []byte:
		return e.encodeBytes(val)
	case []interface{}:
		return e.encodeList(val)
	case BencodeList:
		items := make([]interface{}, len(val))
		for i, v := range val {
			items[i] = v
		}
		return e.encodeList(items)
	case map[string]interface{}:
		return e.encodeDict(val)
	case BencodeDict:
		dict := make(map[string]interface{})
		for k, v := range val {
			dict[k] = v
		}
		return e.encodeDict(dict)
	case nil:
		// Encode nil as empty string
		return e.encodeString("")
	default:
		return fmt.Errorf("%w: %T", ErrInvalidType, v)
	}
}

// encodeInt encodes an integer
func (e *Encoder) encodeInt(i int64) error {
	e.buf.WriteByte('i')
	e.buf.WriteString(strconv.FormatInt(i, 10))
	e.buf.WriteByte('e')
	return nil
}

// encodeString encodes a string
func (e *Encoder) encodeString(s string) error {
	e.buf.WriteString(strconv.Itoa(len(s)))
	e.buf.WriteByte(':')
	e.buf.WriteString(s)
	return nil
}

// encodeBytes encodes bytes
func (e *Encoder) encodeBytes(b []byte) error {
	e.buf.WriteString(strconv.Itoa(len(b)))
	e.buf.WriteByte(':')
	e.buf.Write(b)
	return nil
}

// encodeList encodes a list
func (e *Encoder) encodeList(list []interface{}) error {
	e.buf.WriteByte('l')
	for _, item := range list {
		if err := e.encode(item); err != nil {
			return err
		}
	}
	e.buf.WriteByte('e')
	return nil
}

// encodeDict encodes a dictionary (keys must be sorted)
func (e *Encoder) encodeDict(dict map[string]interface{}) error {
	e.buf.WriteByte('d')

	// Sort keys
	keys := make([]string, 0, len(dict))
	for k := range dict {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		if err := e.encodeString(k); err != nil {
			return err
		}
		if err := e.encode(dict[k]); err != nil {
			return err
		}
	}
	e.buf.WriteByte('e')
	return nil
}

// Decoder handles bencode decoding
type Decoder struct {
	data []byte
	pos  int
}

// NewDecoder creates a new bencode decoder
func NewDecoder(data []byte) *Decoder {
	return &Decoder{data: data, pos: 0}
}

// Decode decodes bencode data
func (d *Decoder) Decode() (BencodeValue, error) {
	if d.pos >= len(d.data) {
		return nil, ErrUnexpectedEnd
	}

	switch d.data[d.pos] {
	case 'i':
		return d.decodeInt()
	case 'l':
		return d.decodeList()
	case 'd':
		return d.decodeDict()
	case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
		return d.decodeString()
	default:
		return nil, fmt.Errorf("%w: unexpected character %c at position %d", ErrInvalidBencode, d.data[d.pos], d.pos)
	}
}

// decodeInt decodes an integer
func (d *Decoder) decodeInt() (int64, error) {
	if d.data[d.pos] != 'i' {
		return 0, fmt.Errorf("%w: expected 'i'", ErrInvalidBencode)
	}
	d.pos++

	endPos := bytes.IndexByte(d.data[d.pos:], 'e')
	if endPos == -1 {
		return 0, ErrUnexpectedEnd
	}

	numStr := string(d.data[d.pos : d.pos+endPos])
	num, err := strconv.ParseInt(numStr, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("%w: invalid integer %s", ErrInvalidBencode, numStr)
	}

	d.pos += endPos + 1
	return num, nil
}

// decodeString decodes a string
func (d *Decoder) decodeString() (string, error) {
	colonPos := bytes.IndexByte(d.data[d.pos:], ':')
	if colonPos == -1 {
		return "", ErrUnexpectedEnd
	}

	lengthStr := string(d.data[d.pos : d.pos+colonPos])
	length, err := strconv.Atoi(lengthStr)
	if err != nil {
		return "", fmt.Errorf("%w: invalid string length %s", ErrInvalidBencode, lengthStr)
	}
	if length < 0 {
		return "", ErrNegativeLength
	}

	d.pos += colonPos + 1

	if d.pos+length > len(d.data) {
		return "", ErrUnexpectedEnd
	}

	str := string(d.data[d.pos : d.pos+length])
	d.pos += length
	return str, nil
}

// decodeList decodes a list
func (d *Decoder) decodeList() (BencodeList, error) {
	if d.data[d.pos] != 'l' {
		return nil, fmt.Errorf("%w: expected 'l'", ErrInvalidBencode)
	}
	d.pos++

	list := make(BencodeList, 0)
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		item, err := d.Decode()
		if err != nil {
			return nil, err
		}
		list = append(list, item)
	}

	if d.pos >= len(d.data) {
		return nil, ErrUnexpectedEnd
	}
	d.pos++ // skip 'e'
	return list, nil
}

// decodeDict decodes a dictionary
func (d *Decoder) decodeDict() (BencodeDict, error) {
	if d.data[d.pos] != 'd' {
		return nil, fmt.Errorf("%w: expected 'd'", ErrInvalidBencode)
	}
	d.pos++

	dict := make(BencodeDict)
	for d.pos < len(d.data) && d.data[d.pos] != 'e' {
		key, err := d.decodeString()
		if err != nil {
			return nil, fmt.Errorf("error decoding dict key: %w", err)
		}
		value, err := d.Decode()
		if err != nil {
			return nil, fmt.Errorf("error decoding dict value for key %s: %w", key, err)
		}
		dict[key] = value
	}

	if d.pos >= len(d.data) {
		return nil, ErrUnexpectedEnd
	}
	d.pos++ // skip 'e'
	return dict, nil
}

// Position returns the current position in the data
func (d *Decoder) Position() int {
	return d.pos
}

// Remaining returns remaining bytes
func (d *Decoder) Remaining() []byte {
	if d.pos >= len(d.data) {
		return nil
	}
	return d.data[d.pos:]
}

// Helper functions for type conversion

// GetString extracts a string from BencodeValue
func GetString(v BencodeValue) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

// GetInt extracts an int64 from BencodeValue
func GetInt(v BencodeValue) (int64, bool) {
	i, ok := v.(int64)
	return i, ok
}

// GetDict extracts a BencodeDict from BencodeValue
func GetDict(v BencodeValue) (BencodeDict, bool) {
	d, ok := v.(BencodeDict)
	return d, ok
}

// GetList extracts a BencodeList from BencodeValue
func GetList(v BencodeValue) (BencodeList, bool) {
	l, ok := v.(BencodeList)
	return l, ok
}

// DictGetString gets a string value from a dictionary
func DictGetString(d BencodeDict, key string) string {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// DictGetInt gets an int64 value from a dictionary
func DictGetInt(d BencodeDict, key string) int64 {
	if v, ok := d[key]; ok {
		if i, ok := v.(int64); ok {
			return i
		}
	}
	return 0
}

// DictGetBool gets a boolean value from a dictionary (treats non-empty string as true)
func DictGetBool(d BencodeDict, key string) bool {
	if v, ok := d[key]; ok {
		if s, ok := v.(string); ok {
			return s != "" && s != "0" && s != "false" && s != "no"
		}
		if i, ok := v.(int64); ok {
			return i != 0
		}
	}
	return false
}

// DictGetDict gets a nested dictionary from a dictionary
func DictGetDict(d BencodeDict, key string) BencodeDict {
	if v, ok := d[key]; ok {
		if dict, ok := v.(BencodeDict); ok {
			return dict
		}
	}
	return nil
}

// DictGetList gets a list from a dictionary
func DictGetList(d BencodeDict, key string) BencodeList {
	if v, ok := d[key]; ok {
		if list, ok := v.(BencodeList); ok {
			return list
		}
	}
	return nil
}

// DictGetStringList gets a list of strings from a dictionary
func DictGetStringList(d BencodeDict, key string) []string {
	list := DictGetList(d, key)
	if list == nil {
		return nil
	}
	result := make([]string, 0, len(list))
	for _, item := range list {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	return result
}

// EncodeBencode is a convenience function to encode a value
func EncodeBencode(v interface{}) ([]byte, error) {
	return NewEncoder().Encode(v)
}

// DecodeBencode is a convenience function to decode bencode data
func DecodeBencode(data []byte) (BencodeValue, error) {
	return NewDecoder(data).Decode()
}
