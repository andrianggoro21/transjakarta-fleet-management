package location

import (
	"errors"
	"strings"
	"testing"
)

func TestVehicleIDFromTopic(t *testing.T) {
	tests := []struct {
		topic string
		id    string
		ok    bool
	}{
		{"/fleet/vehicle/B1/location", "B1", true},
		{"fleet/vehicle/B1/location", "", false},
		{"/fleet/vehicle//location", "", false},
		{"/fleet/vehicle/B1/speed", "", false},
		{"/fleet/vehicle/a/b/location", "", false},
	}
	for _, tc := range tests {
		id, ok := VehicleIDFromTopic(tc.topic)
		if id != tc.id || ok != tc.ok {
			t.Errorf("%q → %q,%v want %q,%v", tc.topic, id, ok, tc.id, tc.ok)
		}
	}
}

const topic = "/fleet/vehicle/B1234XYZ/location"

func TestParse_Valid(t *testing.T) {
	p := `{"vehicle_id":"B1234XYZ","latitude":-6.2088,"longitude":106.8456,"timestamp":1715003456}`
	got, err := Parse(topic, []byte(p))
	if err != nil {
		t.Fatal(err)
	}
	want := Location{"B1234XYZ", -6.2088, 106.8456, 1715003456}
	if got != want {
		t.Fatalf("got %+v", got)
	}
	for _, p := range []string{
		`{"vehicle_id":"B1234XYZ","latitude":90,"longitude":180,"timestamp":1}`,
		`{"vehicle_id":"B1234XYZ","latitude":-90,"longitude":-180,"timestamp":1}`,
	} {
		if _, err := Parse(topic, []byte(p)); err != nil {
			t.Errorf("boundary %s: %v", p, err)
		}
	}
}

func TestParse(t *testing.T) {
	bad := map[string]string{
		"broken json":   `{`,
		"array":         `[]`,
		"no vehicle_id": `{"latitude":1,"longitude":1,"timestamp":1}`,
		"no latitude":   `{"vehicle_id":"B1234XYZ","longitude":1,"timestamp":1}`,
		"no longitude":  `{"vehicle_id":"B1234XYZ","latitude":1,"timestamp":1}`,
		"no timestamp":  `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1}`,
		"lat 91":        `{"vehicle_id":"B1234XYZ","latitude":91,"longitude":1,"timestamp":1}`,
		"lat -90.0001":  `{"vehicle_id":"B1234XYZ","latitude":-90.0001,"longitude":1,"timestamp":1}`,
		"lon 180.5":     `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":180.5,"timestamp":1}`,
		"lat string":    `{"vehicle_id":"B1234XYZ","latitude":"-6.2","longitude":1,"timestamp":1}`,
		"ts decimal":    `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":1715003456.5}`,
		"ts zero":       `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":0}`,
		"ts negative":   `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":-1}`,
		"empty id":      `{"vehicle_id":"","latitude":1,"longitude":1,"timestamp":1}`,
		"id 33 chars":   `{"vehicle_id":"` + strings.Repeat("A", 33) + `","latitude":1,"longitude":1,"timestamp":1}`,
		"id != topic":   `{"vehicle_id":"OTHER","latitude":1,"longitude":1,"timestamp":1}`,
		"extra field":   `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":1,"speed":5}`,
		"null lat":      `{"vehicle_id":"B1234XYZ","latitude":null,"longitude":1,"timestamp":1}`,
		"1025 bytes":    `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":1}` + strings.Repeat(" ", 1025),
		"two objects":   `{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":1}{"vehicle_id":"B1234XYZ","latitude":1,"longitude":1,"timestamp":1}`,
	}
	for name, p := range bad {
		if _, err := Parse(topic, []byte(p)); !errors.Is(err, ErrInvalid) {
			t.Errorf("%s: err = %v, want ErrInvalid", name, err)
		}
	}
	if _, err := Parse("/fleet/vehicle/B1234XYZ/speed", []byte(`{}`)); !errors.Is(err, ErrInvalid) {
		t.Errorf("bad topic: %v", err)
	}
}
