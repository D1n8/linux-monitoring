CREATE TABLE IF NOT EXISTS samples (
 time timestamptz PRIMARY KEY,
 data jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS service_settings (
 id integer PRIMARY KEY CHECK (id=1),
 monitoring_enabled boolean NOT NULL DEFAULT true
);
INSERT INTO service_settings(id) VALUES (1) ON CONFLICT DO NOTHING;
-- Legacy rules/events tables from 0.1 are left untouched; the service no longer uses them.
