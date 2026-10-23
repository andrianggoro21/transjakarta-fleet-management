package main

import (
	"math"
	"testing"

	"transjakarta-fleet-management/internal/geofence"
	"transjakarta-fleet-management/internal/location"
)

var fence = geofence.Fence{Lat: -6.1754, Lon: 106.8272, RadiusM: 50}

func at(lat, lon float64, ts int64) location.Location {
	return location.Location{VehicleID: "B1234XYZ", Latitude: lat, Longitude: lon, Timestamp: ts}
}

func TestRoute_CrossesFence(t *testing.T) {
	var prev *location.Location
	entries := 0
	for tick := 0; tick < 48; tick++ {
		lat, lon := crossingRoute(fence, tick)
		cur := at(lat, lon, int64(tick+1))
		if prev != nil {
			if d := geofence.Distance(prev.Latitude, prev.Longitude, lat, lon); d > 26 {
				t.Fatalf("tick %d step %.2f m > 26", tick, d)
			}
		}
		if fence.Entered(prev, cur) {
			entries++
		}
		prev = &cur
	}
	if entries < 2 {
		t.Fatalf("entries = %d, want >= 2", entries)
	}
}

func TestOrbit_NeverInside(t *testing.T) {
	for tick := 0; tick < 240; tick++ {
		lat, lon := orbitRoute(fence, tick)
		if fence.Contains(at(lat, lon, 1)) {
			t.Fatalf("tick %d inside fence", tick)
		}
		if d := geofence.Distance(fence.Lat, fence.Lon, lat, lon); math.Abs(d-1000) > 1 {
			t.Fatalf("tick %d distance %.2f", tick, d)
		}
	}
}
