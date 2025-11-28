# Release Notes — 1.3.3-RC

## Highlights
- TMDb fallback now supports both v3 API keys and v4 bearer tokens, caches missing-poster lookups, and reuses an in-memory cache to reduce repeated disk reads.
- SD poster preindexing can be disabled with the new `Preindex SD Posters` option for faster refreshes on large caches; the proxy will build the mapping lazily at runtime instead.
- Proxy robustness improvements: non-SD image IDs are rejected, cached metadata is loaded even after a failed refresh, resolved art can still be served during upstream pauses, and SD tokens are only requested when a download is required.

## Bugfixes
- TMDb lookups no longer fail with unexpected EOF when using v3 keys, and negative results are memoised to avoid repeatedly hammering TMDb when no poster exists.
- The SD proxy avoids caching or serving non-SD image IDs and can continue serving already-cached resolved posters during global download blocks.

# Release Notes — 1.3.1

## Highlights
- Cached-artwork lifetime controls introduced in 1.3 let you refresh or purge images based on `Max Cache Age Days` and `Purge Stale Posters` settings.
- Cached posters within the configured `Max Cache Age Days` window are served directly from disk without contacting Schedules Direct, reducing login churn when artwork is still fresh.
- A skip refresh window lets you bypass Schedules Direct downloads when your XMLTV file is newer than a configurable hour threshold.

## Bugfixes
- Poster overrides now match titles case-insensitively and fall back to schedule titles when programme metadata is missing, so pinned art still resolves during upstream outages.
- The proxy loads cached EPG data at startup and, during Schedules Direct pauses, serves already-downloaded resolved images and updates the programme→image index without contacting the API.
- Schedules Direct tokens are fetched only when a download is required; cached posters that are still within the `Max Cache Age Days` window are delivered straight from disk to avoid unnecessary requests.

# Release Notes — 1.3

## Highlights
- Added **poster override** support: map Title120 values to preferred Schedules Direct image IDs via an `overrides.txt` file stored alongside the cache/index files. Overrides apply to the proxy and XML output so you can pin the art you want.
- Override poster links in XML now use program-only proxy URLs (no image IDs), keeping the override intact without exposing specific IDs.
- Override images are immune to stale-cache purges, ensuring hand-picked posters stay available even when automatic cleanup is enabled.
- Cached-artwork lifetime controls introduced in 1.3 let you refresh or purge images based on `Max Cache Age Days` and `Purge Stale Posters` settings.
- Cached posters within the configured `Max Cache Age Days` window are served directly from disk without contacting Schedules Direct, reducing login churn when artwork is still fresh.
- New **skip refresh window** lets you bypass Schedules Direct downloads when your XMLTV file is newer than a configurable hour threshold.

## Using poster overrides
1. Create `overrides.txt` next to `config_cache.json` (the same folder that holds `config_cache.imgindex.json`).
2. Add one CSV line per show, using the Title120 value followed by the SD image ID:
   - `The Simpsons,199655_i`
   - `"Law & Order: Special Victims Unit",301122_i`
3. Enable the built-in server + proxy mode if you want XML icons to flow through `/proxy/sd/{programID}` automatically.

## Upgrade notes
- No configuration keys are required for overrides; the feature activates when the file is present and readable.
- Keep `Proxy Mode` enabled to take advantage of program-only URLs for overrides and to benefit from cache-refresh logging added in 1.3.

## Skip refresh when XMLTV is recent
- Set **Skip EPG refresh if XMLTV younger than hours** in your config to reuse a previously generated XMLTV file. EPGo checks the XMLTV modification time at startup and skips the download if it’s newer than the threshold you specify.
