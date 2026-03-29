#!/usr/bin/env bash
# =============================================================================
# take_screenshots.sh
# Takes screenshots of all Slurm Grafana dashboards via Playwright in Docker.
# No local browser installation required.
#
# Usage: ./scripts/take_screenshots.sh [output_dir]
#   output_dir: where to save screenshots (default: /tmp/screenshots)
#
# Environment variables (optional, override defaults):
#   GRAFANA_URL         http://localhost:3000
#   GRAFANA_USER        admin
#   GRAFANA_PASS        admin
#   GRAFANA_DOCKER_HOST grafana  (hostname inside Docker network)
#   DOCKER_NETWORK      slurm_slurm-network
#   PLAYWRIGHT_IMAGE    mcr.microsoft.com/playwright:v1.58.2-noble
#   PLAYWRIGHT_VERSION  1.58.2
# =============================================================================
set -euo pipefail

OUTPUT_DIR="${1:-/tmp/screenshots}"
GRAFANA_URL="${GRAFANA_URL:-http://localhost:3000}"
GRAFANA_USER="${GRAFANA_USER:-admin}"
GRAFANA_PASS="${GRAFANA_PASS:-admin}"
GRAFANA_DOCKER_HOST="${GRAFANA_DOCKER_HOST:-grafana}"
DOCKER_NETWORK="${DOCKER_NETWORK:-slurm_slurm-network}"
PLAYWRIGHT_IMAGE="${PLAYWRIGHT_IMAGE:-mcr.microsoft.com/playwright:v1.58.2-noble}"
PLAYWRIGHT_VERSION="${PLAYWRIGHT_VERSION:-1.58.2}"

mkdir -p "$OUTPUT_DIR"

echo "Getting Grafana session..."
GSESSION=$(curl -s -D - -X POST "${GRAFANA_URL}/login" \
    -H "Content-Type: application/json" \
    -d "{\"user\":\"${GRAFANA_USER}\",\"password\":\"${GRAFANA_PASS}\"}" \
    | grep -i 'set-cookie' | grep 'grafana_session=' | grep -v 'expiry' \
    | sed 's/.*grafana_session=\([^;]*\).*/\1/' | tr -d '\r\n')

[ -z "$GSESSION" ] && echo "ERROR: Could not get Grafana session" && exit 1
echo "Session obtained. Running Playwright (${PLAYWRIGHT_IMAGE})..."

docker run --rm \
    --network "$DOCKER_NETWORK" \
    -w /work \
    -e "GSESSION=$GSESSION" \
    -e "GRAFANA_BASE=http://${GRAFANA_DOCKER_HOST}:3000" \
    -e "LANG=en_US.UTF-8" \
    -v "$OUTPUT_DIR:/screenshots" \
    "$PLAYWRIGHT_IMAGE" \
    bash -c "
npm init -y > /dev/null 2>&1
npm install playwright@${PLAYWRIGHT_VERSION} > /dev/null 2>&1

cat > /work/script.js << 'JSEOF'
const { chromium } = require('playwright');
const SESSION = process.env.GSESSION;
const BASE = process.env.GRAFANA_BASE || 'http://grafana:3000';
const DASHBOARDS = [
  { uid: 'slurm-overview',     name: 'overview',     sections: [0, 1300, 2500, 3600] },
  { uid: 'slurm-jobs',         name: 'jobs',         sections: [0, 1200, 2400] },
  { uid: 'slurm-nodes',        name: 'nodes',        sections: [0, 1200, 2400, 3600] },
  { uid: 'slurm-usage',        name: 'usage',        sections: [0, 1200, 2400, 3600, 4800] },
  { uid: 'slurm-scheduler',    name: 'scheduler',    sections: [0, 1200, 2400, 3400] },
  { uid: 'slurm-health',       name: 'health',       sections: [0, 1200, 2400] },
  { uid: 'slurm-reservations', name: 'reservations', sections: [0, 1200] },
  { uid: 'slurm-all-metrics',  name: 'all-metrics',  sections: [0, 1200, 2400, 3600, 4800] },
];
(async () => {
  const browser = await chromium.launch({
    headless: true,
    args: ['--no-sandbox', '--disable-setuid-sandbox', '--disable-dev-shm-usage', '--lang=en-US']
  });
  const ctx = await browser.newContext({ locale: 'en-US' });
  await ctx.addCookies([{
    name: 'grafana_session', value: SESSION,
    url: BASE + '/', httpOnly: false, sameSite: 'Lax'
  }]);
  const page = await ctx.newPage();
  await page.setViewportSize({ width: 1920, height: 1080 });
  for (const db of DASHBOARDS) {
    console.log('→ ' + db.name);
    try {
      await page.goto(BASE + '/d/' + db.uid + '?orgId=1&from=now-1h&to=now&kiosk',
        { waitUntil: 'networkidle', timeout: 35000 });
      await page.waitForTimeout(6000);
      for (let i = 0; i < db.sections.length; i++) {
        await page.evaluate(y => window.scrollTo(0, y), db.sections[i]);
        await page.waitForTimeout(i === 0 ? 2500 : 1800);
        await page.screenshot({ path: '/screenshots/' + db.name + '-' + (i+1) + '.png' });
      }
      console.log('  ok (' + db.sections.length + ' screenshots)');
    } catch(e) { console.error('  ERR ' + db.name + ': ' + e.message); }
  }
  await browser.close();
  console.log('All done. Screenshots saved to /screenshots/');
})().catch(e => { console.error('Fatal:', e.message); process.exit(1); });
JSEOF

node /work/script.js
"

echo ""
echo "Screenshots saved to: $OUTPUT_DIR"
ls "$OUTPUT_DIR"/*.png 2>/dev/null | wc -l | xargs -I{} echo "  {} files"
