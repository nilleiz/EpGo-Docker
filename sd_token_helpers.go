package main

import "time"

var lastTokenTime time.Time

// ensureFreshToken returns a valid SD token, renewing it if needed.
// It does NOT rely on a global Token variable, to avoid conflicts;
// it calls sd.Init + sd.Login and returns sd.Token.
func ensureFreshToken() (string, error) {
	// Refresh every ~23h to be safe
	if time.Since(lastTokenTime) > 23*time.Hour {
		var s SD
		if err := s.Init(); err != nil {
			return "", err
		}
		if err := s.Login(); err != nil {
			return "", err
		}
		lastTokenTime = time.Now()
		return s.Token, nil
	}

	// If we are within 23h, still do a lightweight login to fetch a fresh token
	// (keeps behavior simple and avoids relying on hidden globals).
	var s SD
	if err := s.Init(); err != nil {
		return "", err
	}
	if err := s.Login(); err != nil {
		return "", err
	}
	lastTokenTime = time.Now()
	return s.Token, nil
}
