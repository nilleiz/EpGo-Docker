---

# EPGo

subredit: [epgo](https://www.reddit.com/r/EpGo/)

# EpGo-Docker

## EpGo Docker Image

A robust, secure, and multi-arch Docker image for **[EpGo](https://github.com/Chuchodavids/EpGo)**, a command-line tool for downloading television listings from Schedules Direct.

This image is built from source, ensuring compatibility with any Docker host architecture. It includes an intelligent entrypoint script to handle initial configuration and can be run either on a cron schedule or as a one-time task.

---

## ✅ Features

* **Multi-Arch**: Built from source to run on any Docker host (amd64, arm64, etc.).
* **Flexible Execution**: Run `epgo` on a cron schedule or as a single, one-off task.
* **Secure**: Runs the application as a non-root `app` user.
* **Auto-Initialization**: Creates a default `config.yaml` on the first run.
* **Small Footprint**: Uses a multi-stage build to create a minimal final image.
* **Poster Aspect control (new)**: Choose 2×3 / 4×3 / 16×9 / all for Schedules Direct images.
* **Sharper TMDb posters (new)**: TMDb fallback now returns **w500** posters by default.

---

## 🚀 Quick Start

This image is controlled via environment variables in your `docker-compose.yaml` file.

1. Create a directory for your project:

   ```bash
   mkdir epgo-stack
   cd epgo-stack
   ```

2. Create a `docker-compose.yaml` file with the following content. **Remember to change `yourusername/epgo:latest`** to your image name.

   ```yaml
   services:
     epgo:
       image: yourusername/epgo:latest  # ← change this
       container_name: epgo
       environment:
         - TZ=America/Chicago
         - PUID=1000
         - PGID=1000

         # --- CHOOSE ONE EXECUTION MODE ---
         - CRON_SCHEDULE=0 2 * * *   # Example: Run daily at 2:00 AM
         # - RUN_ONCE=true            # Or run once and exit

       volumes:
         - ./epgo_data:/app   # persistent config/cache/XML output
       
       restart: unless-stopped
   ```

3. Start the container:

   ```bash
   docker compose up -d
   ```

---

## ⚙️ Execution Modes

You must set one of the following environment variables. `RUN_ONCE` takes priority if both are set.

### Cron Schedule Mode

Set the `CRON_SCHEDULE` variable to run `epgo` on a schedule. The container will run continuously as a cron daemon. Any output from `epgo` will be sent to the container's logs.

* **Example**: `CRON_SCHEDULE: "0 2 * * *"` runs the task every day at 2:00 AM.

### Run Once Mode

Set `RUN_ONCE: "true"` to execute the `epgo` command one time. The container will exit after the task is complete.

> **Note on `restart` policy**: With `RUN_ONCE`, use `restart: "no"` to avoid restart loops.

---

## 🔧 Interactive Configuration

Run the built-in wizard in a temporary container:

```bash
docker compose run --rm epgo epgo -configure /app/config.yaml
```

---

## ⚠️ Permissions

The entrypoint adjusts ownership of `/app` to the non-root `app` user. On the host, the folder may appear owned by a numeric UID/GID — that’s expected and required for writes.

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
