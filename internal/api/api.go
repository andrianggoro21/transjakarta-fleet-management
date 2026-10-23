package api

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"transjakarta-fleet-management/internal/location"
	"transjakarta-fleet-management/internal/store"
)

type Reader interface {
	Latest(ctx context.Context, vehicleID string) (location.Location, error)
	History(ctx context.Context, vehicleID string, start, end int64) ([]location.Location, error)
	Ping(ctx context.Context) error
}

type handler struct {
	r   Reader
	log *slog.Logger
}

func NewRouter(r Reader, log *slog.Logger) *gin.Engine {
	h := handler{r: r, log: log}
	e := gin.New()
	e.Use(gin.Recovery(), h.logRequest)
	e.GET("/vehicles/:vehicle_id/location", h.location)
	e.GET("/vehicles/:vehicle_id/history", h.history)
	e.GET("/health", h.health)
	return e
}

func (h handler) logRequest(c *gin.Context) {
	start := time.Now()
	c.Next()
	h.log.Info("http request", "method", c.Request.Method, "path", c.Request.URL.Path, "status", c.Writer.Status(), "duration_ms", time.Since(start).Milliseconds())
}

func (h handler) internalError(c *gin.Context, err error) {
	h.log.Error("request failed", "path", c.Request.URL.Path, "error", err)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal error"})
}

func (h handler) location(c *gin.Context) {
	l, err := h.r.Latest(c.Request.Context(), c.Param("vehicle_id"))
	switch {
	case errors.Is(err, store.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "location not found"})
	case err != nil:
		h.internalError(c, err)
	default:
		c.JSON(http.StatusOK, l)
	}
}

func queryInt(c *gin.Context, key string) (int64, bool) {
	v, err := strconv.ParseInt(c.Query(key), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": key + " must be an integer unix timestamp"})
		return 0, false
	}
	return v, true
}

func (h handler) history(c *gin.Context) {
	start, ok := queryInt(c, "start")
	if !ok {
		return
	}
	end, ok := queryInt(c, "end")
	if !ok {
		return
	}
	if start > end {
		c.JSON(http.StatusBadRequest, gin.H{"error": "start must be <= end"})
		return
	}
	ls, err := h.r.History(c.Request.Context(), c.Param("vehicle_id"), start, end)
	if err != nil {
		h.internalError(c, err)
		return
	}
	if ls == nil {
		ls = []location.Location{}
	}
	c.JSON(http.StatusOK, ls)
}

func (h handler) health(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.r.Ping(ctx); err != nil {
		h.log.Warn("health check failed", "error", err)
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unhealthy"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
