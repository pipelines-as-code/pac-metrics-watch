package metrics

import (
	"fmt"
	"math"
	"strings"
)

const SparkChars = "▁▂▃▄▅▆▇█"

func Sparkline(values []float64) string {
	if len(values) == 0 {
		return ""
	}

	minValue, maxValue := values[0], values[0]
	for _, value := range values {
		if value < minValue {
			minValue = value
		}
		if value > maxValue {
			maxValue = value
		}
	}

	chars := []rune(SparkChars)
	maxIndex := float64(len(chars) - 1)
	var builder strings.Builder
	for _, value := range values {
		index := 0
		if maxValue > minValue {
			index = int(math.Round((value - minValue) / (maxValue - minValue) * maxIndex))
		}
		if index < 0 {
			index = 0
		}
		if index >= len(chars) {
			index = len(chars) - 1
		}
		builder.WriteRune(chars[index])
	}
	return builder.String()
}

func FormatMetricNumber(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%g", value)
	}
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%.0f", value)
	}
	return fmt.Sprintf("%.3f", value)
}

func FormatDelta(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%+g", value)
	}
	if math.Abs(value-math.Round(value)) < 1e-9 {
		return fmt.Sprintf("%+.0f", value)
	}
	return fmt.Sprintf("%+.3f", value)
}

func FormatBytes(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%g", value)
	}
	if value == 0 {
		return "0B"
	}

	abs := math.Abs(value)
	units := []string{"B", "KiB", "MiB", "GiB", "TiB"}
	unit := 0
	for abs >= 1024 && unit < len(units)-1 {
		abs /= 1024
		unit++
	}

	scaled := value / math.Pow(1024, float64(unit))
	if unit == 0 || math.Abs(scaled) >= 10 {
		return fmt.Sprintf("%.0f%s", scaled, units[unit])
	}
	return fmt.Sprintf("%.1f%s", scaled, units[unit])
}

func FormatBytesDelta(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%+g", value)
	}
	sign := "+"
	if value < 0 {
		sign = "-"
	}
	return sign + FormatBytes(math.Abs(value))
}

func FormatMillicores(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%g", value)
	}
	if math.Abs(value) >= 1000 {
		return fmt.Sprintf("%.2f cores", value/1000)
	}
	return fmt.Sprintf("%.0fm", value)
}

func FormatMillicoresDelta(value float64) string {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return fmt.Sprintf("%+g", value)
	}
	sign := "+"
	if value < 0 {
		sign = "-"
	}
	return sign + FormatMillicores(math.Abs(value))
}
