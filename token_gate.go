package main

// applyCachedToken obtains a valid token from the shared token cache (with
// disk persistence) and assigns it to the given SD client. It never logs in
// directly; it delegates to getSDToken() which rate-limits and persists.
func applyCachedToken(sd *SD) error {
	tok, err := getSDToken()
	if err != nil {
		return err
	}
	sd.Token = tok
	return nil
}
