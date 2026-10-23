package api

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/store"
)

type fakeReader struct {
	latest     location.Location
	latestErr  error
	history    []location.Location
	historyErr error
	pingErr    error
	gotID      string
	gotStart   int64
	gotEnd     int64
}

func (f *fakeReader) Latest(_ context.Context, id string) (location.Location, error) {
	f.gotID = id
	return f.latest, f.latestErr
}

func (f *fakeReader) History(_ context.Context, id string, start, end int64) ([]location.Location, error) {
	f.gotID, f.gotStart, f.gotEnd = id, start, end
	return f.history, f.historyErr
}

func (f *fakeReader) Ping(context.Context) error { return f.pingErr }

func do(t *testing.T, r *fakeReader, path string) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	NewRouter(r, slog.New(slog.NewTextHandler(io.Discard, nil))).ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func expect(t *testing.T, w *httptest.ResponseRecorder, code int, body string) {
	t.Helper()
	if w.Code != code || strings.TrimSpace(w.Body.String()) != body {
		t.Fatalf("got %d %s, want %d %s", w.Code, w.Body.String(), code, body)
	}
}

var sample = location.Location{VehicleID: "B1234XYZ", Latitude: -6.2088, Longitude: 106.8456, Timestamp: 1715003456}

const sampleJSON = `{"vehicle_id":"B1234XYZ","latitude":-6.2088,"longitude":106.8456,"timestamp":1715003456}`

func TestLocation_OK(t *testing.T) {
	r := &fakeReader{latest: sample}
	expect(t, do(t, r, "/vehicles/B1234XYZ/location"), 200, sampleJSON)
	if r.gotID != "B1234XYZ" {
		t.Fatalf("id = %q", r.gotID)
	}
}

func TestLocation_NotFound(t *testing.T) {
	expect(t, do(t, &fakeReader{latestErr: store.ErrNotFound}, "/vehicles/X/location"), 404, `{"error":"location not found"}`)
}

func TestLocation_InternalError(t *testing.T) {
	expect(t, do(t, &fakeReader{latestErr: errors.New("secret db detail")}, "/vehicles/X/location"), 500, `{"error":"internal error"}`)
}

func TestHistory_OK(t *testing.T) {
	r := &fakeReader{history: []location.Location{sample, sample}}
	expect(t, do(t, r, "/vehicles/B1234XYZ/history?start=1715000000&end=1715009999"), 200, "["+sampleJSON+","+sampleJSON+"]")
	if r.gotID != "B1234XYZ" || r.gotStart != 1715000000 || r.gotEnd != 1715009999 {
		t.Fatalf("got %q %d %d", r.gotID, r.gotStart, r.gotEnd)
	}
}

func TestHistory_EmptyIsArray(t *testing.T) {
	expect(t, do(t, &fakeReader{history: []location.Location{}}, "/vehicles/X/history?start=1&end=2"), 200, `[]`)
	expect(t, do(t, &fakeReader{}, "/vehicles/X/history?start=1&end=2"), 200, `[]`)
}

func TestHistory_BadParams(t *testing.T) {
	for _, q := range []string{"end=5", "start=5", "start=abc&end=5", "start=1&end=1.5", "start=10&end=5", "start=&end="} {
		w := do(t, &fakeReader{}, "/vehicles/X/history?"+q)
		var body map[string]string
		if w.Code != 400 || json.Unmarshal(w.Body.Bytes(), &body) != nil || body["error"] == "" {
			t.Errorf("%s: got %d %s", q, w.Code, w.Body.String())
		}
	}
}

func TestHistory_StartEqualsEnd(t *testing.T) {
	expect(t, do(t, &fakeReader{history: []location.Location{sample}}, "/vehicles/X/history?start=5&end=5"), 200, "["+sampleJSON+"]")
}

func TestHistory_InternalError(t *testing.T) {
	expect(t, do(t, &fakeReader{historyErr: errors.New("boom")}, "/vehicles/X/history?start=1&end=2"), 500, `{"error":"internal error"}`)
}

func TestHealth_OK(t *testing.T) {
	expect(t, do(t, &fakeReader{}, "/health"), 200, `{"status":"ok"}`)
}

func TestHealth_Unhealthy(t *testing.T) {
	expect(t, do(t, &fakeReader{pingErr: errors.New("down")}, "/health"), 503, `{"status":"unhealthy"}`)
}
