package location

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

type Location struct {
	VehicleID string  `json:"vehicle_id"`
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
	Timestamp int64   `json:"timestamp"`
}

var ErrInvalid = errors.New("invalid location")

const (
	MaxPayloadBytes = 1024
	maxIDLen        = 32
	topicPrefix     = "/fleet/vehicle/"
	topicSuffix     = "/location"
)

func VehicleIDFromTopic(topic string) (string, bool) {
	if !strings.HasPrefix(topic, topicPrefix) || !strings.HasSuffix(topic, topicSuffix) ||
		len(topic) <= len(topicPrefix)+len(topicSuffix) {
		return "", false
	}
	id := topic[len(topicPrefix) : len(topic)-len(topicSuffix)]
	if strings.Contains(id, "/") {
		return "", false
	}
	return id, true
}

type raw struct {
	VehicleID *string  `json:"vehicle_id"`
	Latitude  *float64 `json:"latitude"`
	Longitude *float64 `json:"longitude"`
	Timestamp *int64   `json:"timestamp"`
}

func invalid(format string, a ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, a...))
}

func Parse(topic string, payload []byte) (Location, error) {
	if len(payload) > MaxPayloadBytes {
		return Location{}, invalid("payload %d bytes > %d", len(payload), MaxPayloadBytes)
	}
	topicID, ok := VehicleIDFromTopic(topic)
	if !ok {
		return Location{}, invalid("bad topic %q", topic)
	}
	var r raw
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&r); err != nil {
		return Location{}, invalid("%v", err)
	}
	if dec.More() {
		return Location{}, invalid("trailing data")
	}
	if r.VehicleID == nil || r.Latitude == nil || r.Longitude == nil || r.Timestamp == nil {
		return Location{}, invalid("missing field")
	}
	l := Location{*r.VehicleID, *r.Latitude, *r.Longitude, *r.Timestamp}
	switch {
	case l.VehicleID == "" || len(l.VehicleID) > maxIDLen:
		return Location{}, invalid("vehicle_id length")
	case l.VehicleID != topicID:
		return Location{}, invalid("vehicle_id %q != topic id %q", l.VehicleID, topicID)
	case l.Latitude < -90 || l.Latitude > 90:
		return Location{}, invalid("latitude out of range")
	case l.Longitude < -180 || l.Longitude > 180:
		return Location{}, invalid("longitude out of range")
	case l.Timestamp <= 0:
		return Location{}, invalid("timestamp must be > 0")
	}
	return l, nil
}
