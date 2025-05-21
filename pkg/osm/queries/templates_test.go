package queries

import "testing"

func TestOverpassBuilder_Simple(t *testing.T) {
	q := NewOverpassBuilder().
		WithNodeInBbox(1, 2, 3, 4, map[string]string{"amenity": "cafe"}).
		End().
		Build()
	expected := "[out:json];(node(1.000000,2.000000,3.000000,4.000000)[amenity=cafe];);out body;"
	if q != expected {
		t.Errorf("unexpected query: %s", q)
	}
}

func TestOverpassBuilder_CustomOutput(t *testing.T) {
	q := NewOverpassBuilder().
		WithWayInBbox(0, 0, 1, 1, map[string]string{"highway": "bus_stop"}).
		End().
		WithOutput("geom").
		Build()
	expected := "[out:json];(way(0.000000,0.000000,1.000000,1.000000)[highway=bus_stop];);out geom;"
	if q != expected {
		t.Errorf("unexpected query: %s", q)
	}
}
