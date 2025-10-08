package main

import (
	"net/url"
	"strings"
)

// sdImageID extracts the SD image ID from either a bare ID or a full SD URL.
// It also strips an optional ".jpg" suffix so we always store by plain ID.
//
// Example full URL:
//   https://json.schedulesdirect.org/20141201/image/<id>.jpg?token=...
func sdImageID(uri string) string {
	if strings.HasPrefix(uri, "http://") || strings.HasPrefix(uri, "https://") {
		if u, err := url.Parse(uri); err == nil {
			parts := strings.Split(strings.Trim(u.Path, "/"), "/")
			if n := len(parts); n > 0 {
				id := parts[n-1]
				return strings.TrimSuffix(id, ".jpg")
			}
		}
	}
	return strings.TrimSuffix(uri, ".jpg")
}
