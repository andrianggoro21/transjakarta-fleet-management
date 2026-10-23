package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"transjakarta-fleet-management/internal/config"
	"transjakarta-fleet-management/internal/mq"
)

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

	conn, err := mq.Dial(ctx, cfg.RabbitMQURL)
	if err != nil {
		fatal("connect rabbitmq", err)
	}
	ch, err := conn.Channel()
	if err != nil {
		fatal("open channel", err)
	}
	if err := mq.Declare(ch); err != nil {
		fatal("declare topology", err)
	}
	if err := ch.Qos(10, 0, false); err != nil {
		fatal("set qos", err)
	}
	deliveries, err := ch.ConsumeWithContext(ctx, mq.Queue, "fleet-worker", false, false, false, false, nil)
	if err != nil {
		fatal("consume", err)
	}
	log.Info("worker consuming", "queue", mq.Queue)

	for d := range deliveries {
		var e mq.Event
		if err := json.Unmarshal(d.Body, &e); err != nil {
			log.Warn("invalid event dropped", "error", err, "body", string(d.Body))
			_ = d.Nack(false, false)
			continue
		}
		log.Info("geofence alert",
			"vehicle_id", e.VehicleID,
			"event", e.Event,
			"latitude", e.Location.Latitude,
			"longitude", e.Location.Longitude,
			"timestamp", e.Timestamp)
		_ = d.Ack(false)
	}

	if ctx.Err() == nil {
		fatal("delivery channel closed", conn.Close())
	}
	_ = conn.Close()
	log.Info("shutdown complete")
}
