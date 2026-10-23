package ingest

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"

	"transjakarta-fleet-management/internal/geofence"
	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/store"
)

type fakeStore struct {
	rows      []location.Location
	insertErr error
	latestErr error
}

func (f *fakeStore) Latest(_ context.Context, id string) (location.Location, error) {
	if f.latestErr != nil {
		return location.Location{}, f.latestErr
	}
	var best *location.Location
	for i, r := range f.rows {
		if r.VehicleID == id && (best == nil || r.Timestamp > best.Timestamp) {
			best = &f.rows[i]
		}
	}
	if best == nil {
		return location.Location{}, store.ErrNotFound
	}
	return *best, nil
}

func (f *fakeStore) Insert(_ context.Context, l location.Location) error {
	if f.insertErr != nil {
		return f.insertErr
	}
	f.rows = append(f.rows, l)
	return nil
}

type fakePub struct {
	sent []location.Location
	err  error
}

func (f *fakePub) PublishEntry(_ context.Context, l location.Location) error {
	f.sent = append(f.sent, l)
	return f.err
}

const topic = "/fleet/vehicle/B1/location"

func newHandler() (*Handler, *fakeStore, *fakePub) {
	s, p := &fakeStore{}, &fakePub{}
	return &Handler{Store: s, Pub: p, Fence: geofence.Fence{Lat: 0, Lon: 0, RadiusM: 50}, Log: slog.New(slog.NewTextHandler(io.Discard, nil))}, s, p
}

func msg(inside bool, ts int64) []byte {
	lat := 0.01
	if inside {
		lat = 0
	}
	return []byte(fmt.Sprintf(`{"vehicle_id":"B1","latitude":%v,"longitude":0,"timestamp":%d}`, lat, ts))
}

func feed(t *testing.T, h *Handler, msgs ...[]byte) {
	t.Helper()
	for _, m := range msgs {
		if err := h.Handle(context.Background(), topic, m); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHandle_ValidStored(t *testing.T) {
	h, s, _ := newHandler()
	feed(t, h, msg(false, 1))
	if len(s.rows) != 1 || s.rows[0].Timestamp != 1 {
		t.Fatalf("rows = %+v", s.rows)
	}
}

func TestHandle_ValidLogsSaved(t *testing.T) {
	h, _, _ := newHandler()
	var buf bytes.Buffer
	h.Log = slog.New(slog.NewTextHandler(&buf, nil))
	feed(t, h, msg(false, 7))
	out := buf.String()
	for _, want := range []string{"location saved", "vehicle_id=B1", "timestamp=7"} {
		if !strings.Contains(out, want) {
			t.Fatalf("log missing %q: %s", want, out)
		}
	}
}

func TestHandle_StoreErrorNoSavedLog(t *testing.T) {
	h, s, _ := newHandler()
	var buf bytes.Buffer
	h.Log = slog.New(slog.NewTextHandler(&buf, nil))
	s.insertErr = errors.New("db down")
	_ = h.Handle(context.Background(), topic, msg(false, 1))
	if strings.Contains(buf.String(), "location saved") {
		t.Fatalf("unexpected saved log: %s", buf.String())
	}
}

func TestHandle_InvalidPayloadNotStored(t *testing.T) {
	h, s, p := newHandler()
	if err := h.Handle(context.Background(), topic, []byte(`{`)); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(s.rows) != 0 || len(p.sent) != 0 {
		t.Fatalf("rows=%d sent=%d", len(s.rows), len(p.sent))
	}
}

func TestHandle_EntryPublishes(t *testing.T) {
	h, _, p := newHandler()
	feed(t, h, msg(false, 1), msg(true, 2))
	if len(p.sent) != 1 || p.sent[0].Timestamp != 2 {
		t.Fatalf("sent = %+v", p.sent)
	}
}

func TestHandle_FirstMessageInsidePublishes(t *testing.T) {
	h, _, p := newHandler()
	feed(t, h, msg(true, 1))
	if len(p.sent) != 1 {
		t.Fatalf("sent = %d", len(p.sent))
	}
}

func TestHandle_StayInsideNoEvent(t *testing.T) {
	h, _, p := newHandler()
	feed(t, h, msg(false, 1), msg(true, 2), msg(true, 3), msg(true, 4))
	if len(p.sent) != 1 {
		t.Fatalf("sent = %d", len(p.sent))
	}
}

func TestHandle_ExitAndReenter(t *testing.T) {
	h, _, p := newHandler()
	feed(t, h, msg(false, 1), msg(true, 2), msg(false, 3), msg(true, 4))
	if len(p.sent) != 2 {
		t.Fatalf("sent = %d", len(p.sent))
	}
}

func TestHandle_DuplicateEntryPublishesOnce(t *testing.T) {
	h, s, p := newHandler()
	feed(t, h, msg(false, 1), msg(true, 2), msg(true, 2))
	if len(s.rows) != 3 || len(p.sent) != 1 {
		t.Fatalf("rows=%d sent=%d", len(s.rows), len(p.sent))
	}
}

func TestHandle_OutOfOrderNoEvent(t *testing.T) {
	h, s, p := newHandler()
	feed(t, h, msg(false, 200), msg(true, 100))
	if len(s.rows) != 2 || len(p.sent) != 0 {
		t.Fatalf("rows=%d sent=%d", len(s.rows), len(p.sent))
	}
}

func TestHandle_PublishErrorStillStored(t *testing.T) {
	h, s, p := newHandler()
	p.err = errors.New("broker down")
	if err := h.Handle(context.Background(), topic, msg(true, 1)); err != nil {
		t.Fatalf("err = %v", err)
	}
	if len(s.rows) != 1 {
		t.Fatalf("rows = %d", len(s.rows))
	}
}

func TestHandle_StoreErrorReturned(t *testing.T) {
	h, s, p := newHandler()
	s.insertErr = errors.New("db down")
	if err := h.Handle(context.Background(), topic, msg(true, 1)); err == nil {
		t.Fatal("want insert error")
	}
	s.insertErr, s.latestErr = nil, errors.New("db down")
	if err := h.Handle(context.Background(), topic, msg(true, 1)); err == nil {
		t.Fatal("want latest error")
	}
	if len(p.sent) != 0 {
		t.Fatalf("sent = %d", len(p.sent))
	}
}
