package api

import "encoding/json"

// jsonUnmarshal is a thin wrapper around json.Unmarshal for use within the package.
func jsonUnmarshal(data json.RawMessage, v any) error {
	return json.Unmarshal(data, v)
}
