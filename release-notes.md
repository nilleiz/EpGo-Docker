# Release Notes — 1.3.1

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
