package mgrs

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
)

// WGS84 Ellipsoid Parameters
const (
	a         = 6378137.0         // Semi-major axis
	f         = 1 / 298.257223563 // Flattening
	k0        = 0.9996            // UTM scale factor on central meridian
	falseE    = 500000.0          // UTM false easting
	falseN_sh = 10000000.0        // UTM false northing for southern hemisphere
)

// Derived WGS84 parameters
var (
	e2  = f * (2 - f)   // Eccentricity squared (e²)
	ep2 = e2 / (1 - e2) // Second eccentricity squared (e'²)
)

// MGRS specific constants
const (
	mgrsLetters    = "ABCDEFGHJKLMNPQRSTUVWXYZ" // Omits I and O
	latBandLetters = "CDEFGHJKLMNPQRSTUVWX"     // Lat band letters C-X (omitting I, O)
	degToRad       = math.Pi / 180.0
	radToDeg       = 180.0 / math.Pi
)

// LatLngToMGRS converts decimal Latitude and Longitude to an MGRS string.
// Precision determines the number of digits for easting/northing (1-5):
// 1: 10km, 2: 1km, 3: 100m, 4: 10m, 5: 1m
func LatLngToMGRS(lat, lon float64, precision int) (string, error) {
	// Explicitly reject polar regions outside MGRS coverage
	if lat < -80.0 || lat > 84.0 {
		return "", fmt.Errorf("latitude %f is outside MGRS coverage (-80 to 84 degrees)", lat)
	}
	if lon < -180.0 || lon > 180.0 {
		return "", fmt.Errorf("longitude %f out of range (-180 to 180)", lon)
	}
	if precision < 1 || precision > 5 {
		return "", fmt.Errorf("precision %d out of range (1-5)", precision)
	}

	// Rest of the function remains the same
	zone, _, easting, northing, err := latLngToUTM(lat, lon)
	if err != nil {
		return "", fmt.Errorf("failed converting to UTM: %w", err)
	}

	latBand := getLatBand(lat)
	colLetter, rowLetter := get100kID(easting, northing, zone)

	// Calculate MGRS numeric portion
	easting100k := math.Mod(easting, 100000.0)
	northing100k := math.Mod(northing, 100000.0)

	// Adjust northing for southern hemisphere 100k grid
	// The row letter calculation already accounts for the hemisphere offset structure,
	// but the final numeric northing needs to be relative to the 100k square's origin
	// within the 2,000,000m northing cycle.

	// Format numeric part based on precision
	divisor := math.Pow10(5 - precision)
	eastingStr := fmt.Sprintf("%0*d", precision, int(math.Floor(easting100k/divisor)))
	northingStr := fmt.Sprintf("%0*d", precision, int(math.Floor(northing100k/divisor)))

	mgrs := fmt.Sprintf("%d%c%c%c%s%s", zone, latBand, colLetter, rowLetter, eastingStr, northingStr)

	return mgrs, nil
}

// latLngToUTM converts Lat/Lng to UTM coordinates (Zone, Hemisphere, Easting, Northing)
func latLngToUTM(lat, lon float64) (zone int, hemisphere rune, easting, northing float64, err error) {
	if lon == 180.0 { // Handle 180 degree longitude
		lon = -180.0
	}
	latRad := lat * degToRad
	lonRad := lon * degToRad

	// Calculate UTM zone
	zone = int(math.Floor((lon+180)/6) + 1)
	// Special case for Norway/Svalbard could be added here if needed

	// Central meridian for the zone
	lon0 := float64((zone-1)*6-180+3) * degToRad

	// Determine hemisphere
	if lat >= 0 {
		hemisphere = 'N'
	} else {
		hemisphere = 'S'
	}

	// Transverse Mercator projection calculations
	N := a / math.Sqrt(1-e2*math.Pow(math.Sin(latRad), 2))
	T := math.Pow(math.Tan(latRad), 2)
	C := ep2 * math.Pow(math.Cos(latRad), 2)
	A := (lonRad - lon0) * math.Cos(latRad)

	// Meridional Arc calculation (M)
	M := a * ((1-e2/4-3*e2*e2/64-5*e2*e2*e2/256)*latRad -
		(3*e2/8+3*e2*e2/32+45*e2*e2*e2/1024)*math.Sin(2*latRad) +
		(15*e2*e2/256+45*e2*e2*e2/1024)*math.Sin(4*latRad) -
		(35*e2*e2*e2/3072)*math.Sin(6*latRad))

	// Calculate Easting
	easting = falseE + k0*N*(A+(1-T+C)*(A*A*A)/6+(5-18*T+T*T+72*C-58*ep2)*math.Pow(A, 5)/120)

	// Calculate Northing
	northing = k0 * (M + N*math.Tan(latRad)*(A*A/2+(5-T+9*C+4*C*C)*math.Pow(A, 4)/24+
		(61-58*T+T*T+600*C-330*ep2)*math.Pow(A, 6)/720))

	// Add false northing for southern hemisphere
	if hemisphere == 'S' {
		northing += falseN_sh
	}

	return zone, hemisphere, easting, northing, nil
}

// getLatBand returns the MGRS latitude band letter for a given latitude.
func getLatBand(lat float64) rune {
	if lat < -80 || lat > 84 {
		panic("Latitude out of MGRS range") // Should be caught earlier
	}
	// Index into latBandLetters C..X (excluding I, O) corresponds to bands -80..-72, -72..-64, ..., 72..80, 80..84
	// Band C starts at index 0 (-80 deg), band X is at index 19 (72 deg).
	// Latitude 80.0 to 84.0 is band X.
	if lat >= 80.0 && lat <= 84.0 { // Special case for X band (only 4 degrees high)
		return 'X'
	}
	// Calculate index: (lat + 80) / 8 degrees per band
	index := int(math.Floor((lat + 80.0) / 8.0))
	// Clamp index to valid range 0-19
	if index < 0 {
		index = 0
	}
	if index > 19 {
		index = 19
	}
	return rune(latBandLetters[index])
}

// get100kID calculates the two-letter 100km square ID.
func get100kID(easting, northing float64, zone int) (colLetter, rowLetter rune) {
	// Determine the 100k grid set based on the zone number (1-60)
	// set := (zone - 1) % 6 // Sets 0-5

	// Column Letters (Easting)
	// Column sets repeat every 3 zones (A-H, J-N, P-Z)
	// Set 1 (Zones 1, 4, 7...): A..H, J..N, P..Z (starts A) -> Offset 0
	// Set 2 (Zones 2, 5, 8...): J..N, P..Z, A..H (starts J) -> Offset 8
	// Set 3 (Zones 3, 6, 9...): S..Z, A..H, J..N, P..R (starts S) -> Offset 16
	colSetOffset := []int{0, 8, 16}[(zone-1)%3]
	easting100k := math.Floor(easting / 100000.0)
	// Add large offset to handle potential negative results of mod near grid boundaries
	// The index cycles A-Z (excluding I, O) every 2,000,000 meters (20 * 100k blocks)
	// The index within the letter sequence depends on the set offset
	colIndex := (colSetOffset + int(easting100k) - 1 + len(mgrsLetters)*10) % len(mgrsLetters) // -1 because MGRS easting 100k values are 1-8
	colLetter = rune(mgrsLetters[colIndex])

	// Row Letters (Northing)
	// Row sets alternate between odd and even zones
	// Odd zones (1, 3, 5...): A..H, J..N, P..V (starts A) -> Offset 0
	// Even zones (2, 4, 6...): F..H, J..N, P..V, A..E (starts F) -> Offset 5
	var rowSetOffset int
	if zone%2 == 0 { // Even zone
		rowSetOffset = 5
	} else { // Odd zone
		rowSetOffset = 0
	}
	northing100k := math.Floor(math.Mod(northing, 2000000.0) / 100000.0) // Northing cycle repeats every 2,000,000m (20 * 100k blocks)
	// Add large offset for modulo safety
	rowIndex := (rowSetOffset + int(northing100k) + len(mgrsLetters)*10) % len(mgrsLetters)
	rowLetter = rune(mgrsLetters[rowIndex])

	return colLetter, rowLetter
}

// MGRSToLatLng converts an MGRS string to decimal Latitude and Longitude.
// The returned coordinates represent the center of the MGRS grid square.
func MGRSToLatLng(mgrs string) (lat, lon float64, err error) {
	zone, latBand, colLetter, rowLetter, eastingNum, northingNum, precision, err := parseMGRS(mgrs)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid MGRS string: %w", err)
	}

	// Check if the latitude band is valid (not in polar regions)
	minLat, maxLat := getLatBandBounds(latBand)
	if minLat < -80.0 || maxLat > 84.0 {
		return 0, 0, fmt.Errorf("MGRS coordinate in latitude band %c is outside supported range (-80 to 84 degrees)", latBand)
	}

	// Rest of the function remains the same
	hemisphere := 'N'
	if latBand < 'N' {
		hemisphere = 'S'
	}

	// Get 100k grid offsets
	colIdx := strings.IndexRune(mgrsLetters, colLetter)
	rowIdx := strings.IndexRune(mgrsLetters, rowLetter)
	if colIdx == -1 || rowIdx == -1 {
		return 0, 0, fmt.Errorf("invalid 100k grid letters: %c%c", colLetter, rowLetter)
	}

	// Determine the set for column and row based on zone
	colSetOffset := []int{0, 8, 16}[(zone-1)%3]
	var rowSetOffset int
	if zone%2 == 0 { // Even zone
		rowSetOffset = 5
	} else { // Odd zone
		rowSetOffset = 0
	}

	// Calculate the 100k grid index relative to the set origin (0-based)
	relColIdx := (colIdx - colSetOffset + len(mgrsLetters)) % len(mgrsLetters) // 0-based index within the 24 letters, relative to set start
	relRowIdx := (rowIdx - rowSetOffset + len(mgrsLetters)) % len(mgrsLetters) // 0-based index within the 20 letters, relative to set start

	// --- Calculate Base UTM Northing for the 100km Grid Square ---
	// Estimate northing at the center latitude and zone's central meridian to find the correct 2,000,000m cycle
	bandIdx := strings.IndexRune(latBandLetters, latBand)
	var approxCenterLat float64
	if bandIdx < 0 {
		return 0, 0, fmt.Errorf("internal error: invalid lat band %c", latBand) // Should be caught by parser
	}
	if latBand == 'X' { // Band X is 72-84
		approxCenterLat = 78.0
	} else { // Bands C-W are 8 degrees high
		approxCenterLat = float64(bandIdx)*8.0 - 80.0 + 4.0 // Midpoint of 8-degree band
	}
	zoneCentralMeridianLon := float64((zone-1)*6 - 180 + 3)
	_, _, _, approxNorthing, _ := latLngToUTM(approxCenterLat, zoneCentralMeridianLon) // Use existing UTM conversion for estimation

	// Base northing of the 100k row within the 2M cycle
	gridNorthingBaseInCycle := float64(relRowIdx) * 100000.0

	// Find the correct 2,000,000m northing cycle base
	northingCycleBase := math.Round((approxNorthing-gridNorthingBaseInCycle)/2000000.0) * 2000000.0

	// Adjust cycle base if calculation seems off near poles or hemisphere boundaries
	// This part remains heuristic and might need refinement for edge cases.
	// Test values around the calculated base to find the best fit.
	minDiff := math.Abs(approxNorthing - (northingCycleBase + gridNorthingBaseInCycle))
	bestNorthingCycleBase := northingCycleBase

	for i := -2; i <= 2; i++ { // Check a wider range just in case estimate was off
		if i == 0 {
			continue
		}
		nbTest := northingCycleBase + float64(i)*2000000.0
		currentTotalNorthing := nbTest + gridNorthingBaseInCycle

		// Basic sanity check for northing range based on hemisphere
		if hemisphere == 'S' && currentTotalNorthing > falseN_sh {
			continue // Northing in SH UTM cannot exceed 10M
		}
		if hemisphere == 'N' && currentTotalNorthing < 0 {
			continue // Northing in NH UTM cannot be negative
		}
		// Added check for SH: Should be less than approx 10M
		if hemisphere == 'S' && currentTotalNorthing < 100000 { // Very low northing unlikely far from south pole band C
			continue
		}

		diff := math.Abs(approxNorthing - currentTotalNorthing)
		if diff < minDiff {
			minDiff = diff
			bestNorthingCycleBase = nbTest
		}
	}
	northingCycleBase = bestNorthingCycleBase
	// Ensure final base isn't negative in NH, or resulting northing too high in SH
	if hemisphere == 'N' && northingCycleBase < 0 {
		northingCycleBase = 0
	}
	baseNorthingSW := northingCycleBase + gridNorthingBaseInCycle
	if hemisphere == 'S' && baseNorthingSW > falseN_sh {
		// This should ideally not happen if cycle base calculation is correct
		// Attempt correction by going down one cycle
		northingCycleBase -= 2000000.0
		baseNorthingSW = northingCycleBase + gridNorthingBaseInCycle
		if baseNorthingSW > falseN_sh { // Still wrong? Error.
			return 0, 0, fmt.Errorf("calculated northing %.0f exceeds 10M for Southern Hemisphere", baseNorthingSW)
		}
	}
	if baseNorthingSW < 0 { // Northing should never be negative in final UTM
		return 0, 0, fmt.Errorf("calculated negative base northing %.0f", baseNorthingSW)
	}

	// --- Calculate Base UTM Easting for the 100km Grid Square ---
	// MGRS 100k easting grid origin is offset. Value 1 corresponds to 100km E, 2 to 200km E etc.
	// Easting number (1-based index for the 100k block)
	easting100kNum := float64(relColIdx + 1)
	baseEastingSW := easting100kNum * 100000.0

	// --- Calculate Full UTM coordinates for the SW Corner ---
	scale := math.Pow10(5 - precision)
	eastingSW := baseEastingSW + eastingNum*scale
	northingSW := baseNorthingSW + northingNum*scale

	// --- Calculate Center Coordinates ---
	// Add half the grid size represented by the precision
	offset := scale / 2.0
	centerEasting := eastingSW + offset
	centerNorthing := northingSW + offset

	// --- Convert Center UTM back to Lat/Lng ---
	latRad, lonRad := utmToLatLng(zone, hemisphere, centerEasting, centerNorthing)

	lat = latRad * radToDeg
	lon = lonRad * radToDeg

	return lat, lon, nil
}

// Helper function to get approximate latitude bounds for a band letter
func getLatBandBounds(latBand rune) (minLat, maxLat float64) {
	idx := strings.IndexRune(latBandLetters, latBand)
	if idx == -1 {
		return -90, 90 // Should not happen
	}
	minLat = float64(idx)*8.0 - 80.0
	if latBand == 'X' {
		maxLat = 84.0 // Band X is special 72-84
	} else {
		maxLat = minLat + 8.0
	}
	// Clamp to standard MGRS range
	if minLat < -80.0 {
		minLat = -80.0
	}
	if maxLat > 84.0 {
		maxLat = 84.0
	}
	return minLat, maxLat
}

// parseMGRS extracts components from an MGRS string.
func parseMGRS(mgrs string) (zone int, latBand, colLetter, rowLetter rune, eastingNum, northingNum float64, precision int, err error) {
	mgrs = strings.ToUpper(strings.ReplaceAll(mgrs, " ", ""))
	// Regex to match GZD (1-2 digits, 1 letter), 100k ID (2 letters), Easting/Northing (2, 4, 6, 8, or 10 digits)
	re := regexp.MustCompile(`^(\d{1,2})([C-HJ-NP-X])([A-HJ-NP-Z])([A-HJ-NP-Z])(\d+)$`)
	matches := re.FindStringSubmatch(mgrs)

	if len(matches) != 6 {
		err = fmt.Errorf("invalid format, does not match GZD + 100kID + Digits pattern")
		return
	}

	zone, err = strconv.Atoi(matches[1])
	if err != nil || zone < 1 || zone > 60 {
		err = fmt.Errorf("invalid zone number: %s", matches[1])
		return
	}

	latBand = rune(matches[2][0])
	colLetter = rune(matches[3][0])
	rowLetter = rune(matches[4][0])

	digits := matches[5]
	if len(digits)%2 != 0 {
		err = fmt.Errorf("numeric part must have even number of digits: %s", digits)
		return
	}
	precision = len(digits) / 2
	if precision < 1 || precision > 5 {
		err = fmt.Errorf("invalid precision (digits=%d, precision=%d)", len(digits), precision)
		return
	}

	eastingStr := digits[:precision]
	northingStr := digits[precision:]

	eastingNum, err = strconv.ParseFloat(eastingStr, 64)
	if err != nil {
		err = fmt.Errorf("invalid numeric easting: %s", eastingStr)
		return
	}
	northingNum, err = strconv.ParseFloat(northingStr, 64)
	if err != nil {
		err = fmt.Errorf("invalid numeric northing: %s", northingStr)
		return
	}

	return
}

// utmToLatLng converts UTM coordinates back to Lat/Lng (in radians).
func utmToLatLng(zone int, hemisphere rune, easting, northing float64) (latRad, lonRad float64) {
	// Central meridian for the zone
	lon0 := float64((zone-1)*6-180+3) * degToRad

	// Adjust easting relative to central meridian
	x := easting - falseE

	// Adjust northing relative to equator (remove false northing for SH)
	y := northing
	if hemisphere == 'S' {
		y -= falseN_sh
	}

	// Calculate Footprint Latitude (latitude at the central meridian with the same northing) - M'
	M := y / k0

	// Calculate mu (μ)
	mu := M / (a * (1 - e2/4 - 3*e2*e2/64 - 5*e2*e2*e2/256))

	// Calculate footprint latitude (phi1) using series expansion
	e1 := (1 - math.Sqrt(1-e2)) / (1 + math.Sqrt(1-e2))
	phi1 := mu + (3*e1/2-27*e1*e1*e1/32)*math.Sin(2*mu) +
		(21*e1*e1/16-55*e1*e1*e1*e1/32)*math.Sin(4*mu) +
		(151*e1*e1*e1/96)*math.Sin(6*mu) +
		(1097*e1*e1*e1*e1/512)*math.Sin(8*mu)

	// Pre-calculate terms based on phi1
	sinPhi1 := math.Sin(phi1)
	cosPhi1 := math.Cos(phi1)
	tanPhi1 := sinPhi1 / cosPhi1
	N1 := a / math.Sqrt(1-e2*sinPhi1*sinPhi1)                // Radius of curvature in prime vertical
	T1 := tanPhi1 * tanPhi1                                  // Tan squared of phi1
	C1 := ep2 * cosPhi1 * cosPhi1                            // Second eccentricity squared times cos squared
	R1 := a * (1 - e2) / math.Pow(1-e2*sinPhi1*sinPhi1, 1.5) // Radius of curvature in meridian
	D := x / (N1 * k0)

	// Calculate Latitude (latRad)
	latRad = phi1 - (N1*tanPhi1/R1)*(D*D/2-
		(5+3*T1+10*C1-4*C1*C1-9*ep2)*math.Pow(D, 4)/24+
		(61+90*T1+298*C1+45*T1*T1-252*ep2-3*C1*C1)*math.Pow(D, 6)/720)

	// Calculate Longitude (lonRad)
	lonRad = lon0 + (D-(1+2*T1+C1)*(D*D*D)/6+(5-2*C1+28*T1-3*C1*C1+8*ep2+24*T1*T1)*math.Pow(D, 5)/120)/cosPhi1
	return latRad, lonRad
}
