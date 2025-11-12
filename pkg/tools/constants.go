package tools

// Emissions and cost constants
const (
	// CO2 emissions in kg per km
	CarCO2PerKm         = 0.12 // Average car
	TransitCO2PerKm     = 0.05 // Public transit (bus, tram, etc.)
	ElectricCarCO2PerKm = 0.05 // Electric car
	BikeCO2PerKm        = 0.0  // No direct emissions
	WalkingCO2PerKm     = 0.0  // No direct emissions

	// Cost in currency units per km
	CarCostPerKm         = 0.20 // Average cost (fuel, depreciation, maintenance)
	TransitCostPerKm     = 0.10 // Public transport fare
	ElectricCarCostPerKm = 0.05 // Electricity
	BikeCostPerKm        = 0.0  // No direct cost
	WalkingCostPerKm     = 0.0  // No direct cost

	// Calorie burn in kcal per km
	BikeCaloriesPerKm    = 50.0 // Average calorie burn cycling
	WalkingCaloriesPerKm = 80.0 // Average calorie burn walking
)
