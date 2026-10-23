package ingest

import (
	"context"
	"errors"
	"log/slog"

	"transjakarta-fleet-management/internal/geofence"
	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/store"
)

type Store interface {
	Latest(ctx context.Context, vehicleID string) (location.Location, error)
	Insert(ctx context.Context, l location.Location) error
}

type Publisher interface {
	PublishEntry(ctx context.Context, l location.Location) error
}

type Handler struct {
	Store Store
	Pub   Publisher
	Fence geofence.Fence
	Log   *slog.Logger
}

func (h *Handler) Handle(ctx context.Context, topic string, payload []byte) error {
	l, err := location.Parse(topic, payload)
	if err != nil {
		h.Log.Warn("invalid location dropped", "topic", topic, "error", err)
		return nil
	}
	var prev *location.Location
	last, err := h.Store.Latest(ctx, l.VehicleID)
	switch {
	case err == nil:
		prev = &last
	case !errors.Is(err, store.ErrNotFound):
		return err
	}
	if err := h.Store.Insert(ctx, l); err != nil {
		return err
	}
	h.Log.Info("location saved", "vehicle_id", l.VehicleID, "timestamp", l.Timestamp)
	if !h.Fence.Entered(prev, l) {
		return nil
	}
	if err := h.Pub.PublishEntry(ctx, l); err != nil {
		h.Log.Error("publish geofence event failed", "vehicle_id", l.VehicleID, "timestamp", l.Timestamp, "error", err)
		return nil
	}
	h.Log.Info("geofence entry published", "vehicle_id", l.VehicleID, "timestamp", l.Timestamp)
	return nil
}
