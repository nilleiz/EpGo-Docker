package main

import "sync"

var (
	downloadMu        sync.Mutex
	downloadsInFlight = make(map[string]*sync.WaitGroup)
)

// acquireImageDownload returns a WaitGroup for an image download. The boolean
// result indicates whether the caller is responsible for performing the
// download (true) or should wait for an in-flight download to finish (false).
func acquireImageDownload(imageID string) (*sync.WaitGroup, bool) {
	if imageID == "" {
		// Nothing to guard; treat caller as owner
		return nil, true
	}

	downloadMu.Lock()
	defer downloadMu.Unlock()

	if wg, ok := downloadsInFlight[imageID]; ok {
		return wg, false
	}

	wg := &sync.WaitGroup{}
	wg.Add(1)
	downloadsInFlight[imageID] = wg
	return wg, true
}

// releaseImageDownload marks the end of the guarded download.
func releaseImageDownload(imageID string) {
	if imageID == "" {
		return
	}

	downloadMu.Lock()
	defer downloadMu.Unlock()

	if wg, ok := downloadsInFlight[imageID]; ok {
		wg.Done()
		delete(downloadsInFlight, imageID)
	}
}
