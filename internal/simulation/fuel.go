package simulation

// Fuel: a rider's carbohydrate store (glycogen), burned at a rate that rises
// steeply with intensity and topped up by feeding. Durability (fatigue.go)
// fades threshold with the work done whatever its intensity; fuel adds what
// intensity costs: an hour at 90% of threshold burns about three times the
// carbohydrate of an hour sitting in the bunch at 70%, and a rider whose
// store runs low loses threshold fast (the bonk), down to what fat alone
// sustains.
//
// Numbers for trained pros: ~600 g of glycogen when carbo-loaded (~10.5 MJ
// of metabolic energy) for a 68 kg rider, ~90 g/h of carbohydrate in the
// race (~1.5 MJ/h), gross efficiency ~23%, and a carbohydrate share of
// energy of ~40% at 60% of threshold, ~55% at 75%, ~80% at 90% and all of it
// above threshold (fat oxidation peaks well below threshold in trained
// riders). Below 30% of the store threshold falls linearly to 70% at empty.
const (
	fuelStoreKJPer68kg = 10500 // metabolic kJ of carbohydrate, full
	fuelFeedKJPerHour  = 1500  // eaten and drunk in the race
	grossEfficiency    = 0.23
	fuelLowShare       = 0.30 // below this share of the store, threshold falls
	fuelEmptyFactor    = 0.70 // threshold at an empty store (fat alone)

	// fuelLimitsThreshold: off until races cost realistic energy. With it on,
	// almost the whole field bonks in Flanders and Roubaix (median fuel ~10%
	// at 60 km to go; Milan-San Remo is fine) because the sim's riders do
	// ~20-30% too much work in the cobbled classics (~7200 kJ vs a real
	// 5500-6500). Fuel is tracked either way.
	fuelLimitsThreshold = false
)

// carbShare is the share of energy from carbohydrate at an intensity
// (power / threshold).
func carbShare(intensity float64) float64 {
	pts := [][2]float64{{0, 0.15}, {0.45, 0.25}, {0.6, 0.4}, {0.75, 0.55}, {0.9, 0.8}, {1.0, 1.0}}
	if intensity >= 1 {
		return 1
	}
	for i := 1; i < len(pts); i++ {
		if intensity <= pts[i][0] {
			a, b := pts[i-1], pts[i]
			return a[1] + (b[1]-a[1])*(intensity-a[0])/(b[0]-a[0])
		}
	}
	return 1
}

// fuelMax is a rider's full store (it scales with body mass).
func fuelMax(r *RiderState) float64 {
	m := r.Rider.WeightKg
	if m <= 0 {
		m = 68
	}
	return fuelStoreKJPer68kg * m / 68
}

// burnFuel spends and replaces carbohydrate for power held over seconds.
func (rs *RiderState) burnFuel(powerWatts, seconds, threshold float64) {
	if seconds <= 0 {
		return
	}
	if !rs.fuelSet {
		rs.FuelKJ, rs.fuelSet = fuelMax(rs), true
	}
	intensity := 0.0
	if threshold > 0 {
		intensity = powerWatts / threshold
	}
	burn := carbShare(intensity) * powerWatts / grossEfficiency * seconds / 1000
	feed := fuelFeedKJPerHour * seconds / 3600
	rs.FuelKJ += feed - burn
	if mx := fuelMax(rs); rs.FuelKJ > mx {
		rs.FuelKJ = mx
	}
	if rs.FuelKJ < 0 {
		rs.FuelKJ = 0
	}
}

// FuelShare is the share of a full store left (1 before the race starts).
func (rs *RiderState) FuelShare() float64 {
	if !rs.fuelSet {
		return 1
	}
	return rs.FuelKJ / fuelMax(rs)
}

// fuelFactor is the share of threshold the fuel left allows.
func (rs *RiderState) fuelFactor() float64 {
	if !fuelLimitsThreshold {
		return 1
	}
	return fuelFactorAt(rs.FuelShare())
}

// fuelFactorAt is the share of threshold a fuel share allows.
func fuelFactorAt(f float64) float64 {
	if f >= fuelLowShare {
		return 1
	}
	return fuelEmptyFactor + (1-fuelEmptyFactor)*f/fuelLowShare
}
