package mq

import (
	"encoding/json"
	"testing"

	"transjakarta-fleet-management/internal/location"
)

func TestNewEntryEvent_JSON(t *testing.T) {
	l := location.Location{VehicleID: "B1234XYZ", Latitude: -6.2088, Longitude: 106.8456, Timestamp: 1715003456}
	b, err := json.Marshal(NewEntryEvent(l))
	if err != nil {
		t.Fatal(err)
	}
	want := `{"vehicle_id":"B1234XYZ","event":"geofence_entry","location":{"latitude":-6.2088,"longitude":106.8456},"timestamp":1715003456}`
	if string(b) != want {
		t.Fatalf("got  %s\nwant %s", b, want)
	}
}
