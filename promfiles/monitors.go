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

	selectors, err := parseSelectors(expression)
	if err != nil {
		result.Failed++
		fmt.Fprintf(
			os.Stderr,
			"\033[31mMonitor '%s'\033[0m\n"+
				"\033[31m%s\033[0m\nOriginal:\t%s\nNormalized:\t%s\n\n",
			monitorFile.ID,
			err.Error(),
			monitorFile.Expression,
			expression,
		)
	}

	for _, selector := range selectors {
		name, labels := loadMetric(selector)
		metrics[name] = merge(metrics[name], labels)
	}
	result.Succeeded++
	return result
}
