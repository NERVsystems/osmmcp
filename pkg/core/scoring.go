// Package core provides shared utilities for the OpenStreetMap MCP tools.
package core

import (
	"math"
)

// ScoreWeight represents a weight for a specific category in a scoring algorithm
type ScoreWeight struct {
	Category string  // Name of the category
	Weight   float64 // Weight multiplier
}

// WeightedScore calculates a score based on counts and weights
// It returns a value between 0 and maxScore
func WeightedScore(counts map[string]int, weights []ScoreWeight, maxScore float64) int {
	if len(counts) == 0 || len(weights) == 0 {
		return 0
	}

	var score float64
	for _, w := range weights {
		if count, ok := counts[w.Category]; ok {
			score += float64(count) * w.Weight
		}
	}

	// Bound score between 0 and maxScore
	boundedScore := math.Min(score, maxScore)
	boundedScore = math.Max(boundedScore, 0)

	return int(math.Round(boundedScore))
}

// NormalizedWeightedScore calculates a score based on counts and weights,
// then normalizes it to a value between 0 and maxScore
func NormalizedWeightedScore(counts map[string]int, weights []ScoreWeight, maxScore float64, normalizationFactor float64) int {
	if len(counts) == 0 || len(weights) == 0 {
		return 0
	}

	var score float64
	for _, w := range weights {
		if count, ok := counts[w.Category]; ok {
			score += float64(count) * w.Weight
		}
	}

	// Normalize the score
	normalizedScore := score / normalizationFactor * maxScore

	// Bound score between 0 and maxScore
	boundedScore := math.Min(normalizedScore, maxScore)
	boundedScore = math.Max(boundedScore, 0)

	return int(math.Round(boundedScore))
}

// CalculateOverallScore calculates an overall score as a weighted average of component scores
func CalculateOverallScore(scores map[string]int, weights map[string]float64) int {
	if len(scores) == 0 {
		return 0
	}

	// If no weights are provided, use equal weights
	if len(weights) == 0 {
		totalScore := 0
		for _, score := range scores {
			totalScore += score
		}
		return totalScore / len(scores)
	}

	// Calculate weighted score
	var totalScore float64
	var totalWeight float64

	for category, score := range scores {
		// Use default weight of 1.0 if not specified
		weight := 1.0
		if w, ok := weights[category]; ok {
			weight = w
		}
		totalScore += float64(score) * weight
		totalWeight += weight
	}

	// Avoid division by zero
	if totalWeight == 0 {
		return 0
	}

	return int(math.Round(totalScore / totalWeight))
}

// DistanceBiasedScore adjusts a base score based on distance
// The score decreases as distance increases
func DistanceBiasedScore(baseScore int, distance, maxDistance float64, maxScore float64) int {
	if distance <= 0 {
		return baseScore
	}

	// Normalize distance between 0 and 1
	normalizedDistance := math.Min(distance/maxDistance, 1.0)

	// Apply inverse effect (closer = higher score)
	distanceFactor := 1.0 - normalizedDistance

	// Calculate final score
	adjustedScore := float64(baseScore) * distanceFactor

	// Bound score between 0 and maxScore
	boundedScore := math.Min(adjustedScore, maxScore)
	boundedScore = math.Max(boundedScore, 0)

	return int(math.Round(boundedScore))
}

// ThresholdScores converts raw scores to categorical scores based on thresholds
// Example thresholds: map[string][]int{"low": {0, 30}, "medium": {31, 70}, "high": {71, 100}}
func ThresholdScores(scores map[string]int, thresholds map[string][]int) map[string]string {
	result := make(map[string]string)

	for category, score := range scores {
		// Default to "unknown"
		categoryScore := "unknown"

		for name, threshold := range thresholds {
			if len(threshold) == 2 && score >= threshold[0] && score <= threshold[1] {
				categoryScore = name
				break
			}
		}

		result[category] = categoryScore
	}

	return result
}
