package main

import (
	"math"

	"transjakarta-fleet-management/internal/geofence"
)

const (
	stepM        = 25.0
	halfSpanM    = 300.0
	orbitM       = 1000.0
	orbitTicks   = 120
	metersPerDeg = geofence.EarthRadiusM * math.Pi / 180
)

func offset(f geofence.Fence, northM, eastM float64) (lat, lon float64) {
	return f.Lat + northM/metersPerDeg, f.Lon + eastM/(metersPerDeg*math.Cos(f.Lat*math.Pi/180))
}

func crossingRoute(f geofence.Fence, tick int) (lat, lon float64) {
	half := int(2 * halfSpanM / stepM)
	p := tick % (2 * half)
	east := -halfSpanM + stepM*float64(p)
	if p >= half {
		east = halfSpanM - stepM*float64(p-half)
	}
	return offset(f, 0, east)
}

func orbitRoute(f geofence.Fence, tick int) (lat, lon float64) {
	a := 2 * math.Pi * float64(tick%orbitTicks) / orbitTicks
	return offset(f, orbitM*math.Cos(a), orbitM*math.Sin(a))
}
