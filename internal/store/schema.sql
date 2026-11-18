CREATE TABLE IF NOT EXISTS samples (
 time timestamptz PRIMARY KEY,
 data jsonb NOT NULL
);
CREATE TABLE IF NOT EXISTS rules (
 metric text PRIMARY KEY CHECK (metric IN ('cpu','memory','disk','rx','tx')),
 threshold double precision NOT NULL CHECK (threshold >= 0),
 duration integer NOT NULL CHECK (duration BETWEEN 1 AND 3600),
 enabled boolean NOT NULL DEFAULT false,
 revision bigint NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS events (
 id bigserial PRIMARY KEY,
 metric text NOT NULL REFERENCES rules(metric),
 threshold double precision NOT NULL,
 started timestamptz NOT NULL,
 ended timestamptz,
 state text NOT NULL CHECK (state IN ('active','resolved','interrupted'))
);
CREATE INDEX IF NOT EXISTS events_started_idx ON events(started DESC);
CREATE UNIQUE INDEX IF NOT EXISTS events_one_active ON events(metric) WHERE state = 'active';
INSERT INTO rules(metric,threshold,duration) VALUES ('cpu',85,30),('memory',90,30),('disk',90,60) ON CONFLICT DO NOTHING;
