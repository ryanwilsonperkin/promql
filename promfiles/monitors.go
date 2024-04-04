package promfiles

import (
	"encoding/json"
	"fmt"
	"os"
)

type MonitorFile struct {
	ID         string `json:"id"`
	Expression string `json:"expression"`
}

func NewMonitorFile(filename string) MonitorFile {
	var monitorFile MonitorFile
	bytes := loadFile(filename)
	json.Unmarshal(bytes, &monitorFile)
	return monitorFile
}

func (monitorFile *MonitorFile) Load(metrics Metrics) LoadResult {
	var result LoadResult
	var variables []Variable
	expression := normalizeExpression(monitorFile.Expression, variables)
	if expression == "" {
		result.Skipped++
		return result
	}

	monitorMetrics, err := parseExpression(expression)
	if err != nil {
		result.Failed++
		fmt.Fprintf(os.Stderr, "Monitor '%s'\n%s\nOriginal:\t%s\nNormalized:\t%s\n\n", monitorFile.ID, err.Error(), monitorFile.Expression, expression)
	}

	metrics.Add(monitorMetrics)
	result.Succeeded++
	return result
}
