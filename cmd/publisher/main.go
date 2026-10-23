package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"

	"transjakarta-fleet-management/internal/config"
	"transjakarta-fleet-management/internal/geofence"
	"transjakarta-fleet-management/internal/location"
)

type vehicle struct {
	id    string
	route func(geofence.Fence, int) (float64, float64)
}

var vehicles = []vehicle{
	{"B1234XYZ", crossingRoute},
	{"B5678ABC", orbitRoute},
}

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)

	cfg, err := config.Load()
	if err != nil {
		fatal("invalid config", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	hostname, _ := os.Hostname()
	client := mqtt.NewClient(mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID("fleet-publisher-" + hostname).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second))
	if t := client.Connect(); !t.WaitTimeout(30*time.Second) || t.Error() != nil {
		fatal("connect mqtt", errors.Join(errors.New("timeout or error"), t.Error()))
	}
	log.Info("publisher started", "interval", cfg.PublishInterval.String())

	fence := geofence.Fence{Lat: cfg.FenceLat, Lon: cfg.FenceLon, RadiusM: cfg.FenceRadiusM}
	ticker := time.NewTicker(cfg.PublishInterval)
	defer ticker.Stop()
	for tick := 0; ; tick++ {
		for _, v := range vehicles {
			lat, lon := v.route(fence, tick)
			l := location.Location{VehicleID: v.id, Latitude: lat, Longitude: lon, Timestamp: time.Now().Unix()}
			body, _ := json.Marshal(l)
			topic := "/fleet/vehicle/" + v.id + "/location"
			t := client.Publish(topic, 1, false, body)
			if !t.WaitTimeout(5*time.Second) || t.Error() != nil {
				log.Error("publish failed", "topic", topic, "error", t.Error())
				continue
			}
			log.Info("location published", "vehicle_id", l.VehicleID, "latitude", l.Latitude, "longitude", l.Longitude, "timestamp", l.Timestamp)
		}
		select {
		case <-ctx.Done():
			client.Disconnect(250)
			log.Info("shutdown complete")
			return
		case <-ticker.C:
		}
	}
}
