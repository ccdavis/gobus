// Builds a fixture GoBus database and serves it with the real binary in test
// mode (no GTFS download, no external realtime services). Used as the
// Playwright webServer command — it stays in the foreground while serving.
//
// Requires the gobus binary at ../gobus (built by `make test-e2e`).

import { spawn, spawnSync } from 'node:child_process';
import { mkdirSync, rmSync, existsSync } from 'node:fs';
import { DatabaseSync } from 'node:sqlite';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';

const here = dirname(fileURLToPath(import.meta.url));
const binary = join(here, '..', 'gobus');
const tmpDir = join(here, '.tmp');
const dbPath = join(tmpDir, 'test.db');
const PORT = 9990;

if (!existsSync(binary)) {
  console.error('gobus binary not found at ' + binary + ' — run `make build` first');
  process.exit(1);
}

rmSync(tmpDir, { recursive: true, force: true });
mkdirSync(tmpDir, { recursive: true });

// 1. Create the schema with the real migrations.
const init = spawnSync(binary, ['--init-db'], {
  env: { ...process.env, GOBUS_DB_PATH: dbPath },
  stdio: 'inherit',
});
if (init.status !== 0) {
  console.error('gobus --init-db failed');
  process.exit(1);
}

// 2. Seed fixture data. Departure times are generated relative to the current
// time in the agency's zone (America/Chicago), using the GTFS convention that
// hours may exceed 24 — so seeding is midnight-safe.
const db = new DatabaseSync(dbPath);

function chicagoSecondsOfDay() {
  const parts = new Intl.DateTimeFormat('en-US', {
    timeZone: 'America/Chicago',
    hour: 'numeric', minute: 'numeric', second: 'numeric', hour12: false,
  }).formatToParts(new Date());
  const get = (t) => parseInt(parts.find((p) => p.type === t).value, 10) % 24;
  return get('hour') * 3600 + get('minute') * 60 + get('second');
}

function gtfsTime(minutesAhead) {
  const s = chicagoSecondsOfDay() + minutesAhead * 60;
  const pad = (n) => String(n).padStart(2, '0');
  return `${pad(Math.floor(s / 3600))}:${pad(Math.floor((s % 3600) / 60))}:${pad(s % 60)}`;
}

db.exec(`INSERT INTO agency (agency_id, agency_name, agency_url, agency_timezone)
  VALUES ('MT', 'Metro Transit', 'https://example.test', 'America/Chicago')`);

db.exec(`INSERT INTO routes (route_id, route_short_name, route_long_name, route_type, route_color, route_text_color, route_sort_order)
  VALUES ('R10', '10', 'Central Ave', 3, '0053A0', 'FFFFFF', 1),
         ('R21', '21', 'Lake St', 3, 'ED1B2E', 'FFFFFF', 2)`);

// Service active every day of the week, wide date range.
db.exec(`INSERT INTO calendar (service_id, monday, tuesday, wednesday, thursday, friday, saturday, sunday, start_date, end_date)
  VALUES ('ALL', 1, 1, 1, 1, 1, 1, 1, '20200101', '20401231')`);

// Two stops near the query point (downtown Minneapolis), opposite directions
// of the same route for the direction toggle, plus a second route.
const stops = [
  ['A', 'Test St & 1st Ave', 44.9778, -93.265],
  ['B', 'Test St & 2nd Ave', 44.9782, -93.265],
];
const insStop = db.prepare(`INSERT INTO stops (stop_id, stop_code, stop_name, stop_desc, stop_lat, stop_lon, location_type, wheelchair_boarding)
  VALUES (?, ?, ?, '', ?, ?, 0, 0)`);
for (const [id, name, lat, lon] of stops) insStop.run(id, id, name, lat, lon);
db.exec(`INSERT INTO stops_rtree (id, min_lat, max_lat, min_lon, max_lon)
  SELECT rowid, stop_lat, stop_lat, stop_lon, stop_lon FROM stops`);

const insTrip = db.prepare(`INSERT INTO trips (trip_id, route_id, service_id, trip_headsign, direction_id, block_id, shape_id)
  VALUES (?, ?, 'ALL', ?, ?, '', '')`);
const insST = db.prepare(`INSERT INTO stop_times (trip_id, arrival_time, departure_time, stop_id, stop_sequence, pickup_type, drop_off_type)
  VALUES (?, ?, ?, ?, 1, 0, 0)`);

let tripN = 0;
function seedTrip(routeID, headsign, directionID, stopID, minutesAhead) {
  const id = `T${++tripN}`;
  insTrip.run(id, routeID, headsign, directionID);
  insST.run(id, gtfsTime(minutesAhead), gtfsTime(minutesAhead), stopID);
}

// Route 10 northbound at A (several times → later times + interval), and
// southbound at B (→ direction pairing).
for (const mins of [5, 20, 35, 50, 65, 80]) seedTrip('R10', 'Downtown', 0, 'A', mins);
for (const mins of [10, 25, 40, 55]) seedTrip('R10', 'Uptown', 1, 'B', mins);
// Route 21 at A.
for (const mins of [8, 38]) seedTrip('R21', 'Lake St East', 0, 'A', mins);

db.close();
console.log(`fixture DB ready at ${dbPath}`);

// 3. Serve. NexTrip URL points at a closed local port so realtime lookups
// fail fast and pages render schedule-only.
const server = spawn(binary, [], {
  env: {
    ...process.env,
    GOBUS_DB_PATH: dbPath,
    GOBUS_PORT: String(PORT),
    GOBUS_TEST_MODE: '1',
    GOBUS_NEXTRIP_URL: 'http://127.0.0.1:1',
  },
  stdio: 'inherit',
});

for (const sig of ['SIGINT', 'SIGTERM']) {
  process.on(sig, () => server.kill(sig));
}
server.on('exit', (code) => process.exit(code ?? 0));
