package geofence

import (
	"math"

	"transjakarta-fleet-management/internal/location"
)

const EarthRadiusM = 6371000.0

type Fence struct{ Lat, Lon, RadiusM float64 }

func rad(d float64) float64 { return d * math.Pi / 180 }

func Distance(lat1, lon1, lat2, lon2 float64) float64 {
	dLat, dLon := rad(lat2-lat1), rad(lon2-lon1)
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(rad(lat1))*math.Cos(rad(lat2))*math.Sin(dLon/2)*math.Sin(dLon/2)
	return 2 * EarthRadiusM * math.Asin(math.Min(1, math.Sqrt(a)))
}

func (f Fence) Contains(l location.Location) bool {
	return Distance(f.Lat, f.Lon, l.Latitude, l.Longitude) <= f.RadiusM
}

func (f Fence) Entered(prev *location.Location, cur location.Location) bool {
	if !f.Contains(cur) {
		return false
	}
	return prev == nil || (!f.Contains(*prev) && cur.Timestamp >= prev.Timestamp)
}
