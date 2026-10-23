package store

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/retry"
)

var ErrNotFound = errors.New("location not found")

const ddl = `
CREATE TABLE IF NOT EXISTS vehicle_locations (
  id         BIGSERIAL PRIMARY KEY,
  vehicle_id TEXT             NOT NULL,
  latitude   DOUBLE PRECISION NOT NULL,
  longitude  DOUBLE PRECISION NOT NULL,
  timestamp  BIGINT           NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_vehicle_locations_vehicle_ts
  ON vehicle_locations (vehicle_id, timestamp DESC);`

type Store struct{ pool *pgxpool.Pool }

func New(ctx context.Context, url string) (*Store, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	if err := retry.Do(ctx, 15, 2*time.Second, pool.Ping); err != nil {
		pool.Close()
		return nil, err
	}
	return &Store{pool: pool}, nil
}

func (s *Store) Migrate(ctx context.Context) error {
	_, err := s.pool.Exec(ctx, ddl)
	return err
}

func (s *Store) Insert(ctx context.Context, l location.Location) error {
	_, err := s.pool.Exec(ctx,
		`INSERT INTO vehicle_locations (vehicle_id, latitude, longitude, timestamp) VALUES ($1, $2, $3, $4)`,
		l.VehicleID, l.Latitude, l.Longitude, l.Timestamp)
	return err
}

func (s *Store) Latest(ctx context.Context, vehicleID string) (location.Location, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT vehicle_id, latitude, longitude, timestamp FROM vehicle_locations
		 WHERE vehicle_id = $1 ORDER BY timestamp DESC, id DESC LIMIT 1`, vehicleID)
	l, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByPos[location.Location])
	if errors.Is(err, pgx.ErrNoRows) {
		return l, ErrNotFound
	}
	return l, err
}

func (s *Store) History(ctx context.Context, vehicleID string, start, end int64) ([]location.Location, error) {
	rows, _ := s.pool.Query(ctx,
		`SELECT vehicle_id, latitude, longitude, timestamp FROM vehicle_locations
		 WHERE vehicle_id = $1 AND timestamp BETWEEN $2 AND $3 ORDER BY timestamp, id`, vehicleID, start, end)
	ls, err := pgx.CollectRows(rows, pgx.RowToStructByPos[location.Location])
	if err != nil {
		return nil, err
	}
	if ls == nil {
		ls = []location.Location{}
	}
	return ls, nil
}

func (s *Store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

func (s *Store) Close() { s.pool.Close() }
