package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// OrderedMap is a JSON object that preserves key insertion order across a
// parse/marshal round-trip.  The MCP CLI contract is order-sensitive: `mcp
// list` renders servers in storage insertion order and `mcp get` renders env
// and header maps the same way, so the generic map[string]any (which Go sorts
// on marshal, and iterates randomly) cannot round-trip these files.
type OrderedMap struct {
	keys []string
	vals map[string]any
}

// NewOrderedMap returns an empty ordered object.
func NewOrderedMap() *OrderedMap {
	return &OrderedMap{vals: map[string]any{}}
}

// Len returns the number of keys.
func (m *OrderedMap) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

// Keys returns the keys in insertion order.
func (m *OrderedMap) Keys() []string {
	if m == nil {
		return nil
	}
	return append([]string(nil), m.keys...)
}

// Get returns the value for key.
func (m *OrderedMap) Get(key string) (any, bool) {
	if m == nil {
		return nil, false
	}
	v, ok := m.vals[key]
	return v, ok
}

// GetString returns the string value for key, or "" when absent or not a
// string.
func (m *OrderedMap) GetString(key string) string {
	v, _ := m.Get(key)
	s, _ := v.(string)
	return s
}

// GetMap returns the nested object for key.
func (m *OrderedMap) GetMap(key string) (*OrderedMap, bool) {
	v, ok := m.Get(key)
	if !ok {
		return nil, false
	}
	om, ok := v.(*OrderedMap)
	return om, ok
}

// Set stores key→val, appending new keys at the end and replacing existing
// keys in place (order preserved).
func (m *OrderedMap) Set(key string, val any) {
	if _, exists := m.vals[key]; !exists {
		m.keys = append(m.keys, key)
	}
	m.vals[key] = val
}

// Delete removes key.  Deleting an absent key is a no-op.
func (m *OrderedMap) Delete(key string) {
	if _, exists := m.vals[key]; !exists {
		return
	}
	delete(m.vals, key)
	for i, k := range m.keys {
		if k == key {
			m.keys = append(m.keys[:i], m.keys[i+1:]...)
			return
		}
	}
}

// MarshalJSON renders the object with keys in insertion order.
func (m *OrderedMap) MarshalJSON() ([]byte, error) {
	if m == nil {
		return []byte("null"), nil
	}
	var b bytes.Buffer
	b.WriteByte('{')
	for i, k := range m.keys {
		if i > 0 {
			b.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		b.Write(kb)
		b.WriteByte(':')
		vb, err := json.Marshal(m.vals[k])
		if err != nil {
			return nil, err
		}
		b.Write(vb)
	}
	b.WriteByte('}')
	return b.Bytes(), nil
}

// UnmarshalJSON parses a JSON object into the map, recording key order and
// converting nested objects to *OrderedMap so their order survives too.
// Numbers decode as json.Number so integer fields (e.g. callbackPort 8080)
// re-marshal without float artifacts.
func (m *OrderedMap) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	v, err := decodeOrderedValue(dec)
	if err != nil {
		return err
	}
	om, ok := v.(*OrderedMap)
	if !ok {
		return fmt.Errorf("config: not a JSON object")
	}
	m.keys = om.keys
	m.vals = om.vals
	return nil
}

// decodeOrderedValue decodes one JSON value from dec, building ordered maps
// for objects.
func decodeOrderedValue(dec *json.Decoder) (any, error) {
	tok, err := dec.Token()
	if err != nil {
		return nil, err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return tok, nil // string | json.Number | bool | nil
	}
	switch delim {
	case '{':
		om := NewOrderedMap()
		for dec.More() {
			keyTok, err := dec.Token()
			if err != nil {
				return nil, err
			}
			key, ok := keyTok.(string)
			if !ok {
				return nil, fmt.Errorf("config: unexpected object key %v", keyTok)
			}
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			om.Set(key, val)
		}
		if _, err := dec.Token(); err != nil { // consume '}'
			return nil, err
		}
		return om, nil
	case '[':
		arr := []any{}
		for dec.More() {
			val, err := decodeOrderedValue(dec)
			if err != nil {
				return nil, err
			}
			arr = append(arr, val)
		}
		if _, err := dec.Token(); err != nil { // consume ']'
			return nil, err
		}
		return arr, nil
	}
	return nil, fmt.Errorf("config: unexpected delimiter %v", delim)
}

// LoadOrderedJSON reads path as an ordered JSON object.  A missing file
// yields an empty map, not an error — callers decide whether to create it.
func LoadOrderedJSON(path string) (*OrderedMap, error) {
	doc := NewOrderedMap()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return doc, nil
		}
		return nil, fmt.Errorf("config: read %s: %w", path, err)
	}
	if len(bytes.TrimSpace(data)) == 0 {
		return doc, nil
	}
	if err := json.Unmarshal(data, doc); err != nil {
		return nil, fmt.Errorf("config: parse %s: %w", path, err)
	}
	return doc, nil
}

// WriteOrderedJSONAtomic serialises doc as 2-space indented JSON (keys in
// insertion order, HTML escaping disabled to match JSON.stringify output) and
// replaces path via temp-file + rename, mirroring writeSettingsAtomic.
func WriteOrderedJSONAtomic(path string, doc *OrderedMap) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("config: create %s: %w", dir, err)
	}

	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(doc); err != nil {
		return fmt.Errorf("config: marshal %s: %w", path, err)
	}
	// The oracle's config files end at the closing brace with no trailing
	// newline (JSON.stringify semantics); Encoder.Encode appends one.
	out := bytes.TrimRight(buf.Bytes(), "\n")

	tmp, err := os.CreateTemp(dir, ".mcp-*.json")
	if err != nil {
		return fmt.Errorf("config: create temp file in %s: %w", dir, err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("config: write %s: %w", tmpName, err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: close %s: %w", tmpName, err)
	}
	if err := os.Chmod(tmpName, 0o644); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: chmod %s: %w", tmpName, err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("config: rename %s → %s: %w", tmpName, path, err)
	}
	return nil
}
