# Dashboards tooling

Utilities for the Grafana dashboards that live under
[`monitoring/grafana/dashboards/`](../../monitoring/grafana/dashboards/).

All three scripts are **idempotent** — running them on already-processed
dashboards is a no-op. The current ten dashboards have already been passed
through `add_drilldown_links.py` and `add_export_format.py`; the scripts
exist so any future dashboard gets the same treatment without having to
remember the boilerplate.

## `add_drilldown_links.py`

Adds a "Slurm Dashboards" dropdown link to each dashboard's top bar so
users can jump between dashboards without going back to the home menu.
The link uses Grafana's `type: "dashboards"` + `asDropdown: true` schema,
which auto-populates from the `tags: ["slurm"]` already present on every
dashboard in this repo.

```bash
python3 scripts/dashboards/add_drilldown_links.py
```

Existing `links[]` entries (like the GitHub link) are preserved. Requires
Grafana 12+ panel/dashboard JSON schema.

## `add_export_format.py`

Adds the `__inputs` / `__elements` / `__requires` sections so each
dashboard passes the grafana.com upload validator. Without them, the
validator rejects with "Old dashboard JSON format".

```bash
python3 scripts/dashboards/add_export_format.py
```

The rest of each file is preserved byte-for-byte: the script parses the
JSON only to detect which panel types are in use, then inserts the new
sections as text right after the opening `{`.

## `take_screenshots.sh`

Drives Playwright in a headless Chromium container to grab a PNG of each
dashboard. Used by the test cluster (`make -C scripts/testing screenshots`)
and by maintainers when refreshing the README screenshots.

```bash
./scripts/dashboards/take_screenshots.sh /tmp/screenshots
```

Requires Grafana running on `localhost:3000` with the dashboards already
provisioned.

## When adding a new dashboard

1. Export the JSON from Grafana with **"Export for sharing externally"** off.
2. Add `tags: ["slurm"]` so it appears in the dropdown.
3. Drop the file under `monitoring/grafana/dashboards/`.
4. Run both Python scripts to normalize it:
   ```bash
   python3 scripts/dashboards/add_drilldown_links.py
   python3 scripts/dashboards/add_export_format.py
   ```
5. Refresh the screenshots if needed:
   ```bash
   ./scripts/dashboards/take_screenshots.sh /tmp/screenshots
   ```

Update [`monitoring/grafana/dashboards/README.md`](../../monitoring/grafana/dashboards/README.md)
with the new dashboard description.
