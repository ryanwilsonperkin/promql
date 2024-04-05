package promfiles

import (
	"encoding/json"
	"fmt"
)

type SLOFile struct {
	ID      string      `json:"id"`
	Metrics []SLIMetric `json:"sliMetrics"`
}

func NewSLOFile(filename string) SLOFile {
	var sloFile SLOFile
	bytes := loadFile(filename)
	json.Unmarshal(bytes, &sloFile)
	return sloFile
}

func (sloFile *SLOFile) Load(metrics *Metrics) LoadResult {
	var result LoadResult
	for _, metric := range sloFile.Metrics {
		name := metric.Name
		var labels []string
		for _, filter := range metric.Filters {
			labels = append(labels, filter.Key)
		}
		metrics.Add(sloFile.URL(), name, labels)
		result.Succeeded++
	}
	return result
}

func (sloFile *SLOFile) URL() string {
	return fmt.Sprintf("https://observe.shopify.io/a/observe/monitoring/slos/%s", sloFile.ID)
}

type SLIMetric struct {
	Name    string   `json:"metricName"`
	Filters []Filter `json:"filters"`
}

type Filter struct {
	Key string `json:"key"`
}
