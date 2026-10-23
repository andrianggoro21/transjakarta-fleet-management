package mq

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/retry"
)

const (
	Exchange           = "fleet.events"
	Queue              = "geofence_alerts"
	RoutingKey         = "geofence.entry"
	BindingKey         = "geofence.#"
	EventGeofenceEntry = "geofence_entry"
)

type Point struct {
	Latitude  float64 `json:"latitude"`
	Longitude float64 `json:"longitude"`
}

type Event struct {
	VehicleID string `json:"vehicle_id"`
	Event     string `json:"event"`
	Location  Point  `json:"location"`
	Timestamp int64  `json:"timestamp"`
}

func NewEntryEvent(l location.Location) Event {
	return Event{
		VehicleID: l.VehicleID,
		Event:     EventGeofenceEntry,
		Location:  Point{Latitude: l.Latitude, Longitude: l.Longitude},
		Timestamp: l.Timestamp,
	}
}

func Dial(ctx context.Context, url string) (*amqp.Connection, error) {
	var conn *amqp.Connection
	err := retry.Do(ctx, 15, 2*time.Second, func(context.Context) error {
		c, err := amqp.Dial(url)
		conn = c
		return err
	})
	return conn, err
}

func Declare(ch *amqp.Channel) error {
	if err := ch.ExchangeDeclare(Exchange, "topic", true, false, false, false, nil); err != nil {
		return err
	}
	if _, err := ch.QueueDeclare(Queue, true, false, false, false, nil); err != nil {
		return err
	}
	return ch.QueueBind(Queue, BindingKey, Exchange, false, nil)
}

type Publisher struct {
	url  string
	mu   sync.Mutex
	conn *amqp.Connection
	ch   *amqp.Channel
}

func NewPublisher(ctx context.Context, url string) (*Publisher, error) {
	conn, err := Dial(ctx, url)
	if err != nil {
		return nil, err
	}
	p := &Publisher{url: url, conn: conn}
	if err := p.openChannel(); err != nil {
		conn.Close()
		return nil, err
	}
	return p, nil
}

func (p *Publisher) openChannel() error {
	ch, err := p.conn.Channel()
	if err != nil {
		return err
	}
	if err := ch.Confirm(false); err != nil {
		ch.Close()
		return err
	}
	if err := Declare(ch); err != nil {
		ch.Close()
		return err
	}
	p.ch = ch
	return nil
}

func (p *Publisher) redial() error {
	if p.conn != nil && !p.conn.IsClosed() {
		p.conn.Close()
	}
	conn, err := amqp.Dial(p.url)
	if err != nil {
		return err
	}
	p.conn = conn
	return p.openChannel()
}

func (p *Publisher) PublishEntry(ctx context.Context, l location.Location) error {
	body, err := json.Marshal(NewEntryEvent(l))
	if err != nil {
		return err
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil || p.conn.IsClosed() || p.ch == nil || p.ch.IsClosed() {
		if err := p.redial(); err != nil {
			return fmt.Errorf("redial: %w", err)
		}
	}
	dc, err := p.ch.PublishWithDeferredConfirmWithContext(ctx, Exchange, RoutingKey, false, false, amqp.Publishing{
		ContentType:  "application/json",
		DeliveryMode: amqp.Persistent,
		Timestamp:    time.Now(),
		Body:         body,
	})
	if err != nil {
		return err
	}
	ok, err := dc.WaitContext(ctx)
	if err != nil {
		return err
	}
	if !ok {
		return errors.New("publish nacked by broker")
	}
	return nil
}

func (p *Publisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.conn == nil {
		return nil
	}
	return p.conn.Close()
}
