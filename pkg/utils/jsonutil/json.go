// Package jsonutil provides JSON / JSONL helper functions.
// The package is named jsonutil (not json) to avoid collision with
// the stdlib "encoding/json" package.
package jsonutil

import (
	stdjson "encoding/json"
)

// MarshalLine serialises v to a single JSON line (no trailing newline).
func MarshalLine(v any) ([]byte, error) {
	return stdjson.Marshal(v)
}

// AppendLine serialises v and appends it as a JSONL line (with trailing '\n')
// to buf, returning the extended slice.
func AppendLine(buf []byte, v any) ([]byte, error) {
	line, err := stdjson.Marshal(v)
	if err != nil {
		return buf, err
	}
	buf = append(buf, line...)
	buf = append(buf, '\n')
	return buf, nil
}

// UnmarshalLine parses a single JSON line into v.
func UnmarshalLine(line []byte, v any) error {
	return stdjson.Unmarshal(line, v)
}
