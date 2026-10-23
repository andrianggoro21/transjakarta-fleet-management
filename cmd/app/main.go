package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/gin-gonic/gin"

	"transjakarta-fleet-management/internal/api"
	"transjakarta-fleet-management/internal/config"
	"transjakarta-fleet-management/internal/geofence"
	"transjakarta-fleet-management/internal/ingest"
	"transjakarta-fleet-management/internal/mq"
	"transjakarta-fleet-management/internal/store"
)

const locationTopic = "/fleet/vehicle/+/location"

func fatal(msg string, err error) {
	slog.Error(msg, "error", err)
	os.Exit(1)
}

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(log)
	gin.SetMode(gin.ReleaseMode)

	cfg, err := config.Load()
	if err != nil {
		fatal("invalid config", err)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.DatabaseURL)
	if err != nil {
		fatal("connect postgres", err)
	}
	if err := st.Migrate(ctx); err != nil {
		fatal("migrate", err)
	}
	pub, err := mq.NewPublisher(ctx, cfg.RabbitMQURL)
	if err != nil {
		fatal("connect rabbitmq", err)
	}

	h := &ingest.Handler{
		Store: st,
		Pub:   pub,
		Fence: geofence.Fence{Lat: cfg.FenceLat, Lon: cfg.FenceLon, RadiusM: cfg.FenceRadiusM},
		Log:   log,
	}
	onMessage := func(_ mqtt.Client, m mqtt.Message) {
		if err := h.Handle(ctx, m.Topic(), m.Payload()); err != nil {
			log.Error("handle location", "topic", m.Topic(), "error", err)
		}
	}
	hostname, _ := os.Hostname()
	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID("fleet-app-" + hostname).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(2 * time.Second).
		SetCleanSession(true).
		SetOnConnectHandler(func(c mqtt.Client) {
			t := c.Subscribe(locationTopic, 1, onMessage)
			if t.WaitTimeout(10*time.Second) && t.Error() == nil {
				log.Info("mqtt subscribed", "topic", locationTopic)
				return
			}
			log.Error("mqtt subscribe failed", "topic", locationTopic, "error", t.Error())
		}).
		SetConnectionLostHandler(func(_ mqtt.Client, err error) {
			log.Warn("mqtt connection lost", "error", err)
		})
	client := mqtt.NewClient(opts)
	if t := client.Connect(); !t.WaitTimeout(30*time.Second) || t.Error() != nil {
		fatal("connect mqtt", errors.Join(errors.New("timeout or error"), t.Error()))
	}

	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: api.NewRouter(st, log), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("http listening", "addr", cfg.HTTPAddr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal("http server", err)
		}
	}()

	<-ctx.Done()
	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown", "error", err)
	}
	client.Disconnect(250)
	if err := pub.Close(); err != nil {
		log.Error("rabbitmq close", "error", err)
	}
	st.Close()
	log.Info("shutdown complete")
}
