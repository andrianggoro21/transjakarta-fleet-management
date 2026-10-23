package store

import (
	"context"
	"errors"
	"os"
	"testing"

	"transjakarta-fleet-management/internal/location"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL not set")
	}
	ctx := context.Background()
	s, err := New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(s.Close)
	if err := s.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := s.pool.Exec(ctx, "DELETE FROM vehicle_locations WHERE vehicle_id = $1", t.Name()); err != nil {
		t.Fatal(err)
	}
	return s
}

func insert(t *testing.T, s *Store, id string, ts ...int64) {
	t.Helper()
	for _, x := range ts {
		if err := s.Insert(context.Background(), location.Location{VehicleID: id, Latitude: -6.2, Longitude: 106.8, Timestamp: x}); err != nil {
			t.Fatal(err)
		}
	}
}

func timestamps(ls []location.Location) []int64 {
	out := []int64{}
	for _, l := range ls {
		out = append(out, l.Timestamp)
	}
	return out
}

func TestStore_InsertLatestHistory(t *testing.T) {
	s, ctx, id := newTestStore(t), context.Background(), t.Name()
	insert(t, s, id, 100, 200, 300)
	l, err := s.Latest(ctx, id)
	if err != nil || l.Timestamp != 300 || l.VehicleID != id || l.Latitude != -6.2 || l.Longitude != 106.8 {
		t.Fatalf("latest = %+v, %v", l, err)
	}
	cases := []struct {
		start, end int64
		want       []int64
	}{
		{150, 300, []int64{200, 300}},
		{200, 200, []int64{200}},
		{400, 500, []int64{}},
	}
	for _, c := range cases {
		h, err := s.History(ctx, id, c.start, c.end)
		if err != nil {
			t.Fatal(err)
		}
		if h == nil {
			t.Fatalf("history(%d,%d) is nil", c.start, c.end)
		}
		got := timestamps(h)
		if len(got) != len(c.want) {
			t.Fatalf("history(%d,%d) = %v want %v", c.start, c.end, got, c.want)
		}
		for i := range got {
			if got[i] != c.want[i] {
				t.Fatalf("history(%d,%d) = %v want %v", c.start, c.end, got, c.want)
			}
		}
	}
}

func TestStore_LatestIsMaxTimestamp(t *testing.T) {
	s, id := newTestStore(t), t.Name()
	insert(t, s, id, 300, 200)
	l, err := s.Latest(context.Background(), id)
	if err != nil || l.Timestamp != 300 {
		t.Fatalf("latest = %+v, %v", l, err)
	}
}

func TestStore_LatestNotFound(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.Latest(context.Background(), t.Name()); !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v", err)
	}
}

func TestStore_MigrateIdempotent(t *testing.T) {
	s := newTestStore(t)
	if err := s.Migrate(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}
