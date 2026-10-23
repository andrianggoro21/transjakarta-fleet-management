package geofence

import (
	"math"
	"testing"

	"transjakarta-fleet-management/internal/location"
)

func TestDistance(t *testing.T) {
	if d := Distance(1, 2, 1, 2); d != 0 {
		t.Errorf("same point = %v", d)
	}
	if d := Distance(-6.1754, 106.8272, -6.1950, 106.8230); math.Abs(d-2200) > 50 {
		t.Errorf("Monas-HI = %v", d)
	}
	if d := Distance(0, 0, 0.001, 0); math.Abs(d-111.2) > 0.5 {
		t.Errorf("0.001deg = %v", d)
	}
}

func loc(lat, lon float64, ts int64) location.Location {
	return location.Location{VehicleID: "V", Latitude: lat, Longitude: lon, Timestamp: ts}
}

func TestFence_ContainsBoundary(t *testing.T) {
	f := Fence{-6.1754, 106.8272, 50}
	perDeg := EarthRadiusM * math.Pi / 180
	edge := loc(f.Lat+50/perDeg, f.Lon, 1)
	if d := Distance(f.Lat, f.Lon, edge.Latitude, edge.Longitude); math.Abs(d-50) > 0.01 {
		t.Fatalf("edge distance = %v", d)
	}
	if !f.Contains(loc(f.Lat+(50-1e-6)/perDeg, f.Lon, 1)) {
		t.Error("just inside must be contained")
	}
	if f.Contains(loc(f.Lat+50.5/perDeg, f.Lon, 1)) {
		t.Error("50.5 m must be outside")
	}
	if !(Fence{0, 0, 0}).Contains(loc(0, 0, 1)) {
		t.Error("distance 0 with radius 0 is inside (<=)")
	}
}

func TestFence_Entered(t *testing.T) {
	f := Fence{0, 0, 50}
	in, out := func(ts int64) location.Location { return loc(0, 0, ts) }, func(ts int64) location.Location { return loc(0.01, 0, ts) }
	p := func(l location.Location) *location.Location { return &l }
	tests := []struct {
		name string
		prev *location.Location
		cur  location.Location
		want bool
	}{
		{"nil prev, inside", nil, in(10), true},
		{"nil prev, outside", nil, out(10), false},
		{"out->in", p(out(10)), in(20), true},
		{"in->in", p(in(10)), in(20), false},
		{"in->out", p(in(10)), out(20), false},
		{"out->in older ts", p(out(20)), in(10), false},
		{"out->in same ts", p(out(10)), in(10), true},
	}
	for _, tc := range tests {
		if got := f.Entered(tc.prev, tc.cur); got != tc.want {
			t.Errorf("%s: got %v", tc.name, got)
		}
	}
}
