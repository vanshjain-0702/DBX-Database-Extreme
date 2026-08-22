package engine

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"github.com/dbx/dbx/internal/protocol"
	"github.com/dbx/dbx/internal/util"
)

// GeoPoint stores a geographic location.
type GeoPoint struct {
	Name      string
	Longitude float64
	Latitude  float64
	Score     float64 // geohash score for sorted ordering
}

// dbxGeo stores geo points as a sorted set (by geohash score).
type dbxGeo struct {
	points map[string]*GeoPoint
	sorted []*GeoPoint
}

func newGeo() *dbxGeo {
	return &dbxGeo{points: make(map[string]*GeoPoint)}
}

// GeoStore provides geo operations.
type GeoStore struct{ kv *KVStore }

func NewGeoStore(kv *KVStore) *GeoStore { return &GeoStore{kv: kv} }

func (g *GeoStore) getOrCreate(key string) (*dbxGeo, func(), error) {
	e, unlock := g.kv.GetForWrite(key)
	if e == nil {
		geo := newGeo()
		sh := g.kv.shard(key)
		sh.data[key] = &Entry{Value: geo, Type: protocol.TypeGeo}
		return geo, unlock, nil
	}
	if e.Type != protocol.TypeGeo {
		unlock()
		return nil, func() {}, util.ErrWrongType
	}
	return e.Value.(*dbxGeo), unlock, nil
}

func (g *GeoStore) getReadOnly(key string) (*dbxGeo, func(), error) {
	e, unlock := g.kv.GetForRead(key)
	if e == nil {
		return nil, func(){}, nil
	}
	if e.Type != protocol.TypeGeo {
		unlock()
		return nil, func(){}, util.ErrWrongType
	}
	return e.Value.(*dbxGeo), unlock, nil
}

// GeoAdd adds geo members. Returns count added.
func (g *GeoStore) GeoAdd(key string, points []GeoPoint) (int, error) {
	geo, unlock, err := g.getOrCreate(key)
	defer unlock()
	if err != nil {
		return 0, err
	}
	added := 0
	for _, p := range points {
		score := encodeGeohash(p.Longitude, p.Latitude)
		pt := &GeoPoint{Name: p.Name, Longitude: p.Longitude, Latitude: p.Latitude, Score: score}
		if _, exists := geo.points[p.Name]; !exists {
			added++
		}
		geo.points[p.Name] = pt
	}
	// Re-sort
	geo.sorted = make([]*GeoPoint, 0, len(geo.points))
	for _, pt := range geo.points {
		geo.sorted = append(geo.sorted, pt)
	}
	sort.Slice(geo.sorted, func(i, j int) bool {
		return geo.sorted[i].Score < geo.sorted[j].Score
	})
	return added, nil
}

// GeoPos returns the position of members.
func (g *GeoStore) GeoPos(key string, members []string) ([]*GeoPoint, error) {
	geo, unlock, err := g.getReadOnly(key)
	defer unlock()
	if err != nil || geo == nil {
		return nil, err
	}
	result := make([]*GeoPoint, len(members))
	for i, m := range members {
		result[i] = geo.points[m]
	}
	return result, nil
}

// GeoDist calculates distance between two members.
func (g *GeoStore) GeoDist(key, m1, m2, unit string) (float64, error) {
	geo, unlock, err := g.getReadOnly(key)
	defer unlock()
	if err != nil || geo == nil {
		return 0, err
	}
	p1, ok1 := geo.points[m1]
	p2, ok2 := geo.points[m2]
	if !ok1 || !ok2 {
		return 0, fmt.Errorf("member not found")
	}
	dist := haversine(p1.Latitude, p1.Longitude, p2.Latitude, p2.Longitude)
	switch strings.ToLower(unit) {
	case "km":
		dist /= 1000
	case "mi":
		dist /= 1609.344
	case "ft":
		dist *= 3.28084
	}
	return dist, nil
}

// GeoRadius returns members within radius meters of (lon, lat).
func (g *GeoStore) GeoRadius(key string, lon, lat, radius float64, unit string) ([]*GeoPoint, error) {
	geo, unlock, err := g.getReadOnly(key)
	defer unlock()
	if err != nil || geo == nil {
		return nil, err
	}
	var radiusM float64
	switch strings.ToLower(unit) {
	case "km":
		radiusM = radius * 1000
	case "mi":
		radiusM = radius * 1609.344
	case "ft":
		radiusM = radius / 3.28084
	default:
		radiusM = radius
	}
	var result []*GeoPoint
	for _, pt := range geo.points {
		d := haversine(lat, lon, pt.Latitude, pt.Longitude)
		if d <= radiusM {
			result = append(result, pt)
		}
	}
	return result, nil
}

// haversine calculates distance between two lat/lon pairs in meters.
func haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const R = 6372797.560856
	dLat := (lat2 - lat1) * math.Pi / 180
	dLon := (lon2 - lon1) * math.Pi / 180
	a := math.Sin(dLat/2)*math.Sin(dLat/2) +
		math.Cos(lat1*math.Pi/180)*math.Cos(lat2*math.Pi/180)*
			math.Sin(dLon/2)*math.Sin(dLon/2)
	return R * 2 * math.Atan2(math.Sqrt(a), math.Sqrt(1-a))
}

// encodeGeohash returns a float64 geohash for sorting.
func encodeGeohash(lon, lat float64) float64 {
	// Simple interleaved bit encoding for ordering purposes
	lonN := (lon + 180) / 360
	latN := (lat + 90) / 180
	var score float64
	for i := 0; i < 26; i++ {
		bit := 1.0 / math.Pow(2, float64(i+1))
		if lonN >= 0.5 {
			score += bit
			lonN = (lonN - 0.5) * 2
		} else {
			lonN *= 2
		}
		if latN >= 0.5 {
			score += bit / 2
			latN = (latN - 0.5) * 2
		} else {
			latN *= 2
		}
	}
	return score
}
