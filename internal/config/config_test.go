package config

import (
	"strings"
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	for _, k := range []string{"MQTT_BROKER_URL", "DATABASE_URL", "RABBITMQ_URL", "HTTP_ADDR", "GEOFENCE_LAT", "GEOFENCE_LON", "GEOFENCE_RADIUS_M", "PUBLISH_INTERVAL"} {
		t.Setenv(k, "")
	}
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	want := Config{
		MQTTBrokerURL: "tcp://mosquitto:1883",
		DatabaseURL:   "postgres://fleet:fleet@postgres:5432/fleet?sslmode=disable",
		RabbitMQURL:   "amqp://fleet:fleet@rabbitmq:5672/",
		HTTPAddr:      ":8080",
		FenceLat:      -6.1754, FenceLon: 106.8272, FenceRadiusM: 50,
		PublishInterval: 2 * time.Second,
	}
	if c != want {
		t.Fatalf("got %+v want %+v", c, want)
	}
}

func TestLoad_InvalidFloat(t *testing.T) {
	t.Setenv("GEOFENCE_LAT", "abc")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "GEOFENCE_LAT") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_InvalidDuration(t *testing.T) {
	t.Setenv("PUBLISH_INTERVAL", "soon")
	if _, err := Load(); err == nil || !strings.Contains(err.Error(), "PUBLISH_INTERVAL") {
		t.Fatalf("err = %v", err)
	}
}

func TestLoad_RejectsNonPositive(t *testing.T) {
	t.Setenv("GEOFENCE_RADIUS_M", "-5")
	if _, err := Load(); err == nil {
		t.Fatal("want error for negative radius")
	}
	t.Setenv("GEOFENCE_RADIUS_M", "50")
	t.Setenv("PUBLISH_INTERVAL", "0s")
	if _, err := Load(); err == nil {
		t.Fatal("want error for zero interval")
	}
}
