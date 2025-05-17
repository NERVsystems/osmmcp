package core

// StandardQueries contains common Overpass API query templates
var StandardQueries = map[string]string{
	"amenity": `
		[out:json][timeout:25];
		(
			node["amenity"="{{.Value}}"]({{.BBox}});
			way["amenity"="{{.Value}}"]({{.BBox}});
			relation["amenity"="{{.Value}}"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
	"building": `
		[out:json][timeout:25];
		(
			way["building"]({{.BBox}});
			relation["building"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
	"highway": `
		[out:json][timeout:25];
		(
			way["highway"]({{.BBox}});
			relation["highway"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
	"leisure": `
		[out:json][timeout:25];
		(
			node["leisure"="{{.Value}}"]({{.BBox}});
			way["leisure"="{{.Value}}"]({{.BBox}});
			relation["leisure"="{{.Value}}"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
	"shop": `
		[out:json][timeout:25];
		(
			node["shop"="{{.Value}}"]({{.BBox}});
			way["shop"="{{.Value}}"]({{.BBox}});
			relation["shop"="{{.Value}}"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
	"tourism": `
		[out:json][timeout:25];
		(
			node["tourism"="{{.Value}}"]({{.BBox}});
			way["tourism"="{{.Value}}"]({{.BBox}});
			relation["tourism"="{{.Value}}"]({{.BBox}});
		);
		out body;
		>;
		out skel qt;
	`,
}
