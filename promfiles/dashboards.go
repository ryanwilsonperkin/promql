package promfiles

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
)

type DashboardFile struct {
	Dashboard Dashboard `json:"dashboard"`
}

func NewDashboardFile(filename string) DashboardFile {
	var dashboardFile DashboardFile
	bytes := loadFile(filename)
	json.Unmarshal(bytes, &dashboardFile)
	return dashboardFile
}

func (dashboardFile *DashboardFile) Load(metrics Metrics) LoadResult {
	var result LoadResult
	variables := dashboardFile.LoadVariables()

	for _, panel := range dashboardFile.Dashboard.Panels {
		if panel.Ignored() {
			result.Skipped++
			continue
		}

		for _, target := range panel.Targets {
			if target.Ignored() {
				result.Skipped++
				continue
			}

			expression := normalizeExpression(target.Expr, variables)
			targetMetrics, err := parseExpression(expression)
			if err != nil {
				result.Failed++
				fmt.Fprintf(os.Stderr, "Dashboard '%s', Panel '%d'\n%s\nOriginal:\t%s\nNormalized:\t%s\n\n", dashboardFile.Dashboard.UID, panel.ID, err.Error(), target.Expr, expression)
			}

			metrics.Add(targetMetrics)
			result.Succeeded++
		}
	}
	return result
}

func (dashboardFile *DashboardFile) LoadVariables() []Variable {
	var variables []Variable

	for _, template := range dashboardFile.Dashboard.Templating.List {
		value := firstNonEmptyString(
			safeIndex(template.Current.Values.TemplateValues, 0),
			template.Query,
		)
		if value == "?" {
			value = template.AllValue
		}
		variables = append(variables, Variable{
			Name:  template.Name,
			Value: value,
		})
	}
	return variables
}

type Dashboard struct {
	ID         int        `json:"id"`
	UID        string     `json:"uid"`
	Templating Templating `json:"templating"`
	Panels     []Panel    `json:"panels"`
}

type Templating struct {
	List []Template `json:"list"`
}

type Template struct {
	Name     string          `json:"name"`
	Current  TemplateCurrent `json:"current"`
	Query    string          `json:"query"`
	AllValue string          `json:"allValue"`
}

type TemplateCurrent struct {
	Values TemplateValuesWrapper `json:"value"`
}

type TemplateValuesWrapper struct {
	TemplateValues
	Partial bool `json:"-"`
}

type TemplateValues []string

// Override unmarshalling to allow dashboad.templating.list[].current.value to be either a string or list of strings
func (w *TemplateValuesWrapper) UnmarshalJSON(data []byte) error {
	if data[0] == '"' {
		s := string(data)
		unquoted := s[1 : len(s)-1]
		w.Partial = true
		w.TemplateValues = []string{unquoted}
		return nil
	}
	return json.Unmarshal(data, &w.TemplateValues)
}

type Panel struct {
	ID      int      `json:"id"`
	Type    string   `json:"type"`
	Targets []Target `json:"targets"`
}

func (panel *Panel) Ignored() bool {
	return slices.Contains(
		[]string{
			"text",
			"logs",
			"news",
			"canvas",
			"dashlist",
			"table",
		},
		panel.Type,
	)
}

type Target struct {
	Expr string `json:"expr"`
}

func (target *Target) Ignored() bool {
	return target.Expr == ""
}
