# EPGo

subredit: [epgo](https://www.reddit.com/r/EpGo/)

# EpGo-Docker

## EpGo Docker Image

A robust, secure, and multi-arch Docker image for **[EpGo](https://github.com/Chuchodavids/EpGo)**, a command-line tool for downloading television listings from Schedules Direct.

This image is built from source, ensuring compatibility with any Docker host architecture. It includes an intelligent entrypoint script to handle initial configuration and can be run either on a cron schedule or as a one-time task.

---

## ✅ Features

- **Multi-Arch**: Built from source to run on any Docker host (amd64, arm64, etc.).
- **Flexible Execution**: Run `epgo` on a cron schedule or as a single, one-off task.
- **Secure**: Runs the application as a non-root `app` user.
- **Auto-Initialization**: Creates a default `config.yaml` on the first run.
- **Small Footprint**: Uses a multi-stage build to create a minimal final image.
- **Poster Aspect control**: Choose 2×3 / 4×3 / 16×9 / all for Schedules Direct images.
- **Sharper TMDb posters**: TMDb fallback returns **w500** posters by default.
- **NEW (v1.3) Cache expiry controls**: Configure how many days artwork stays cached before automatic refresh (0 keeps images indefinitely).
- **Smart Image Cache & Proxy (v1.2+)**: On-demand image caching with a built-in proxy that fetches artwork once from Schedules Direct and then serves it locally from disk—stable, fast, and fewer API calls.

---

## 🚀 Quick Start

This image is controlled via environment variables in your `docker-compose.yaml`.

1) Create a project directory:
```bash
mkdir epgo-stack
cd epgo-stack
```

2) Minimal `docker-compose.yaml`:
```yaml
services:
  epgo:
    image: nillivanilli0815/epgo:latest
    container_name: epgo
    environment:
      - TZ=Europe/Berlin
      - PUID=1000
      - PGID=1000

      # --- CHOOSE ONE EXECUTION MODE ---
      - CRON_SCHEDULE=0 2 * * *     # Example: run daily at 02:00
      # - RUN_ONCE=true             # Or run once and exit

    volumes:
      - ./epgo_data:/app            # persistent config/cache/XML + images

    restart: unless-stopped
```

3) Start it:
```bash
docker compose up -d
```

---

## ✨ NEW in v1.3 — Cache expiry controls

Keep your artwork fresh without hammering the API. Version **1.3** introduces a configurable cache lifetime via `Max Cache Age Days`—set it to the number of days you want to retain pinned images before a background refresh, or leave it at `0` to keep cached art indefinitely.

- Startup logs now confirm the configured lifetime so you can double-check your deployment.
- When an image is refreshed because it aged out, the proxy log line includes the configured maximum.
- Enable `Purge Stale Posters` to delete posters that haven’t been requested for **twice** the configured lifetime (e.g., 14 days when `Max Cache Age Days` is `7`).

## ✨ NEW in v1.2 — Smart Image Cache & Proxy

**What it does**
- When a client requests an image, EPGo fetches it **once** from Schedules Direct (SD), stores it under `/app/images/`, and serves it immediately.
- All subsequent requests are served straight from disk (no SD round-trip).
- Benefits: stable artwork over time, faster UIs, and fewer API requests.

**YAML additions (v1.2+)**
```yaml
Options:
  Images:
    Download Images from Schedules Direct: false   # set false to allow on-demand fetch, true will download all images on building epg
    Image Path: /app/images/                       # persistent cache directory
    Poster Aspect: 2x3                             # 2x3 | 4x3 | 16x9 | all
    Proxy Mode: true                               # enable built-in proxy
    Proxy Base URL:                                # optional; set if clients reach EPGo externally
    Max Cache Age Days: 0                          # 0 disables expiry; otherwise refresh pinned art after N days
    Purge Stale Posters: false                     # if true, remove posters untouched for 2× Max Cache Age Days
```

**Quick notes**
- Mount **`/app/images/`** as a persistent volume.
- If clients access the proxy from outside your LAN, set **`Proxy Base URL`** to your public base.
- With **`Download Images from Schedules Direct: false`**, only previously cached files are served.

**Under the hood (brief)**
- EPGo writes lightweight **JSON sidecars** next to cached images (index) and one for the **SD token**—used for quick lookups and to avoid excessive logins. (Automatic; no action required.)

---

## ⚙️ Execution Modes

Set exactly **one** of:

### Cron Schedule Mode
Run continuously with cron and execute on a schedule:
```env
CRON_SCHEDULE=0 2 * * *
```
Container logs receive the task output.

### Run Once Mode
```env
RUN_ONCE=true
```
Runs a single job and exits.  
> Tip: with `RUN_ONCE`, prefer `restart: "no"` to avoid restart loops.

---

## 🔧 Interactive Configuration

Run the wizard in a temporary container:
```bash
docker compose run --rm epgo epgo -configure /app/config.yaml
```

---

## ⚠️ Permissions

The entrypoint ensures `/app` is owned by the unprivileged `app` user; host-side you may see numeric UID/GID ownership—that’s expected.

---

## CONFIG

> Sample reflecting **Poster Aspect**, TMDb changes, and v1.2+ cache/proxy options (including v1.3 cache expiry).

```yaml
Account:
  Username: YOUR_USERNAME
  Password: YOUR_PASSWORD

Files:
  Cache: config_cache.json
  XMLTV: config.xml
  The MovieDB cache file: imdb_image_cache.json

Server:
  Enable: true                 # enable the built-in HTTP server
  Address: 0.0.0.0
  Port: "8765"

Options:
  Live and New icons: false
  Schedule Days: 1
  Subtitle into Description: false
  Insert credits tag into XML file: false

  Images:
    Download Images from Schedules Direct: false  # set false to allow on-demand fetch, true will download all images on building EPG (legacy)
    Image Path: /app/images/
    Poster Aspect: 2x3

    Proxy Mode: true                              # set false when using "Download Images from Schedules Direct: true"
    Proxy Base URL: ""                            # e.g., https://epgo.example.com if accessed externally
    Max Cache Age Days: 0                         # refresh artwork after N days (0 = disabled)
    Purge Stale Posters: false                    # remove posters untouched for 2× Max Cache Age Days

    The MovieDB:
      Enable: false                               # set true to enable TMDB-fallback on missing SD posters
      Api Key: ""                                 # the longer key from your TMDB-api page

  Rating:
    Insert rating tag into XML file: false
    Maximum rating entries. 0 for all entries: 1
    Preferred countries. ISO 3166-1 alpha-3 country code. Leave empty for all systems:
      - USA
      - COL
    Use country code as rating system: false

  Show download errors from Schedules Direct in the log: false

Station:
  - Name: MTV
    ID: "12345"
    Lineup: SAMPLE
```

*TMDb fallback:* if enabled and SD has no image, EPGo queries TMDb; poster URLs default to **`w500`** for sharper results.

---

## Using the CLI

Create or edit config:
```bash
epgo -configure MY_CONFIG_FILE.yaml
```

Generate XMLTV:
```bash
epgo -config MY_CONFIG_FILE.yaml
```

Help:
```bash
epgo -h
```

---

## Notes & Tips

- For v1.2 proxy mode, ensure the **server** is enabled and that `/app/images/` is persisted.
- If you previously relied on tight cron timing, v1.2’s scheduler logic is tuned for server mode and token lifetimes.
- Sidecars are managed automatically; no manual cleanup is required in normal operation.

---

# Original README with up to date information

## Features

* Cache function to download only new EPG data
* No database is required
* Update EPG with CLI command for using your own scripts

## Requirements

* [Schedules Direct](https://www.schedulesdirect.org/)
* 1–2 GB memory
* [Go](https://golang.org/) to build the binary
* (Optional) Docker

## Installation

### Option 1 — Build Binary

```bash
go mod tidy
go build epgo
```

#
### Option 2 — Download binary

See [releases](https://github.com/Chuchodavids/EpGo/releases).

## Using the APP

```
epgo -h
```

```
-config string     Get data from Schedules Direct with configuration file. [filename.yaml]
-configure string  Create or modify the configuration file. [filename.yaml]
-version           Show version
-h                 Show help
```

### Create a config file

```
epgo -configure MY_CONFIG_FILE.yaml
```

**Configuration file from version 1.0.6 or earlier is not compatible.**

---

## CONFIG

> This sample reflects the new **Poster Aspect** option and TMDb changes.

```yaml
Account:
  Username: YOUR_USERNAME
  Password: YOUR_PASSWORD

Files:
  Cache: config_cache.json
  XMLTV: config.xml
  The MovieDB cache file: imdb_image_cache.json

Server:
  Enable: false
  Address: localhost
  Port: "80"

Options:
  Live and New icons: false
  Schedule Days: 1
  Subtitle into Description: false
  Insert credits tag into XML file: false

  Images:
    Download Images from Schedules Direct: false
    Image Path: ""
    Poster Aspect: 2x3           # ← new (2x3 | 4x3 | 16x9 | all)

    The MovieDB:
      Enable: false
      Api Key: ""

  Rating:
    Insert rating tag into XML file: false
    Maximum rating entries. 0 for all entries: 1
    Preferred countries. ISO 3166-1 alpha-3 country code. Leave empty for all systems:
      - USA
      - COL
    Use country code as rating system: false

  Show download errors from Schedules Direct in the log: false

Station:
  - Name: MTV
    ID: "12345"
    Lineup: SAMPLE
```

### Files

```yaml
Cache: /app/file.json
XMLTV: /app/xml
```

### Server

```yaml
Enable: false
Address: localhost
Port: "80"
```

### Options

#### Subtitle into Description

When enabled, the subtitle is prepended to the description for clients that ignore `<sub-title>`.

#### Images

```yaml
Images:
  Download Images from Schedules Direct: false
  Image Path: ""
  Poster Aspect: 2x3
```

* **Download Images from Schedules Direct**:

  * `true` → images are downloaded and served locally from the built-in server.
  * `false` → XMLTV references SD/TMDb URLs directly.
* **Image Path**: local folder to store downloaded images (defaults to `images/` if empty).
* **Poster Aspect** (new): choose which Schedules Direct aspect to prefer:

  * `2x3` (portrait), `4x3`, `16x9`, or `all` (no filtering).
  * **Fallback logic** if the chosen aspect isn’t available: prefer poster-ish categories (Poster Art → Box Art → Banner-L1 → Banner-L2 → VOD Art), breaking ties by larger width.

#### The MovieDB (TMDb) fallback

```yaml
Images:
  The MovieDB:
    Enable: false
    Api Key: ""
```

* When enabled and **no SD image** is available, TMDb is queried.
* **Quality (new)**: TMDb poster URLs now default to **`w500`** (about 500×750) for sharper results.
  No config change needed. (If you upgraded from a very old cache that stored full TMDb URLs, you may delete `The MovieDB cache file` once; current versions store poster *paths* and generate w500 at read time.)

#### Insert credits / Ratings

(unchanged; left as in the original README)

---

### Create the XMLTV file using the CLI

```bash
epgo -config MY_CONFIG_FILE.yaml
```

**The configuration file must exist.**

---

If you want me to add a short “Upgrade notes” section (e.g., “v1.0: Poster Aspect support; TMDb posters now w500”), I can append that too.
