package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	MQTTBrokerURL, DatabaseURL, RabbitMQURL, HTTPAddr string
	FenceLat, FenceLon, FenceRadiusM                  float64
	PublishInterval                                   time.Duration
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getFloat(key string, def float64) (float64, error) {
	v := getenv(key, "")
	if v == "" {
		return def, nil
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, fmt.Errorf("%s: invalid float %q", key, v)
	}
	return f, nil
}

func Load() (Config, error) {
	c := Config{
		MQTTBrokerURL: getenv("MQTT_BROKER_URL", "tcp://mosquitto:1883"),
		DatabaseURL:   getenv("DATABASE_URL", "postgres://fleet:fleet@postgres:5432/fleet?sslmode=disable"),
		RabbitMQURL:   getenv("RABBITMQ_URL", "amqp://fleet:fleet@rabbitmq:5672/"),
		HTTPAddr:      getenv("HTTP_ADDR", ":8080"),
	}
	var err error
	if c.FenceLat, err = getFloat("GEOFENCE_LAT", -6.1754); err != nil {
		return c, err
	}
	if c.FenceLon, err = getFloat("GEOFENCE_LON", 106.8272); err != nil {
		return c, err
	}
	if c.FenceRadiusM, err = getFloat("GEOFENCE_RADIUS_M", 50); err != nil {
		return c, err
	}
	if c.FenceRadiusM <= 0 {
		return c, fmt.Errorf("GEOFENCE_RADIUS_M: must be > 0")
	}
	c.PublishInterval = 2 * time.Second
	if v := os.Getenv("PUBLISH_INTERVAL"); v != "" {
		if c.PublishInterval, err = time.ParseDuration(v); err != nil {
			return c, fmt.Errorf("PUBLISH_INTERVAL: %w", err)
		}
		if c.PublishInterval <= 0 {
			return c, fmt.Errorf("PUBLISH_INTERVAL: must be > 0")
		}
	}
	return c, nil
}
