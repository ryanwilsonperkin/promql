package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type DashboardFile struct {
	Dashboard Dashboard `json:"dashboard"`
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

type Target struct {
	Expr string `json:"expr"`
}

type Metrics map[string][]string

type Variable struct {
	Name  string
	Value string
}

var REPLACE_XRATE_EXPR = regexp.MustCompile("xrate\\(")
var REPLACE_XINCREASE_EXPR = regexp.MustCompile("xincrease\\(")

var IGNORED_TYPES = []string{
	"text",
	"logs",
	"news",
	"canvas",
	"dashlist",
	"table",
}

var GLOBAL_VARIABLES = []Variable{
	{Name: "__rate_interval", Value: "1m"},
	{Name: "__interval_ms", Value: "60000"},
	{Name: "__interval", Value: "1m"},
	{Name: "interval", Value: "1m"},
	{Name: "__range", Value: "1m"},
	{Name: "__auto_interval_interval", Value: "1m"},
	{Name: "__all", Value: "ALL"},
	// Commonly used as refId for combining series
	{Name: "A", Value: "A"},
	{Name: "B", Value: "B"},
	{Name: "C", Value: "C"},
	{Name: "D", Value: "D"},
}

var ERRORS = 0
var SUCCESS = 0
var SKIPPED = 0

func main() {
	if len(os.Args) < 2 {
		log.Fatal("usage: command_name FILE [...FILE]")
	}
	filenames := os.Args[1:]
	dashboards := Map(filenames, loadDashboard)
	metrics := loadMetrics(dashboards)
	for metric, labels := range metrics {
		fmt.Printf("%s %s\n", metric, strings.Join(labels, " "))
	}
	fmt.Fprintln(os.Stderr, "Success: ", SUCCESS)
	fmt.Fprintln(os.Stderr, "Errors:  ", ERRORS)
	fmt.Fprintln(os.Stderr, "Skipped: ", SKIPPED)
}

func normalizeExpression(expr string, variables []Variable) string {
	var normalized = expr

	// Expand dashboard template variables
	for _, variable := range variables {
		pattern1 := fmt.Sprintf("$%s", variable.Name)
		pattern2 := fmt.Sprintf("${%s}", variable.Name)
		pattern3 := fmt.Sprintf("${%s:value}", variable.Name)
		pattern4 := fmt.Sprintf("[[%s]]", variable.Name)
		unquoted, err := strconv.Unquote(variable.Value)
		if err != nil {
			unquoted = variable.Value
		}
		// Special case, replace "by ($variable)" with "by value"
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("by (%s)", pattern1), fmt.Sprintf("by (%s)", unquoted))
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("by (%s)", pattern2), fmt.Sprintf("by (%s)", unquoted))
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("by (%s)", pattern3), fmt.Sprintf("by (%s)", unquoted))
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("by (%s)", pattern4), fmt.Sprintf("by (%s)", unquoted))

		// Otherwise, quote the variable on insertion
		valueIsNumeric := false
		if _, err := strconv.ParseFloat(unquoted, 10); err == nil {
			valueIsNumeric = true
		}

		if valueIsNumeric {
			normalized = strings.ReplaceAll(normalized, pattern1, unquoted)
			normalized = strings.ReplaceAll(normalized, pattern2, unquoted)
			normalized = strings.ReplaceAll(normalized, pattern3, unquoted)
			normalized = strings.ReplaceAll(normalized, pattern4, unquoted)
		} else {
			normalized = strings.ReplaceAll(normalized, pattern1, escape(variable.Value))
			normalized = strings.ReplaceAll(normalized, pattern2, escape(variable.Value))
			normalized = strings.ReplaceAll(normalized, pattern3, escape(variable.Value))
			normalized = strings.ReplaceAll(normalized, pattern4, escape(variable.Value))
		}
	}

	// Expand global variables
	for _, variable := range GLOBAL_VARIABLES {
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("$%s", variable.Name), variable.Value)
		normalized = strings.ReplaceAll(normalized, fmt.Sprintf("${%s}", variable.Name), variable.Value)
	}

	// Replace xrate function with rate function
	normalized = REPLACE_XRATE_EXPR.ReplaceAllString(normalized, "rate(")
	// Replace xincrease function with increase function
	normalized = REPLACE_XINCREASE_EXPR.ReplaceAllString(normalized, "increase(")
	return normalized
}

func parseSelectors(input string) ([][]*labels.Matcher, error) {
	expr, err := parser.ParseExpr(input)
	if err != nil {
		return nil, err
	}
	return parser.ExtractSelectors(expr), nil
}

func loadDashboard(filename string) Dashboard {
	jsonFile, err := os.Open(filename)
	defer jsonFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Fatal(err)
	}

	var dashboardFile DashboardFile
	json.Unmarshal(byteValue, &dashboardFile)

	return dashboardFile.Dashboard
}

func loadVariables(dashboard Dashboard) []Variable {
	var variables []Variable
	for _, template := range dashboard.Templating.List {
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

func loadMetrics(dashboards []Dashboard) Metrics {
	var metrics = make(Metrics)
	for _, dashboard := range dashboards {
		variables := loadVariables(dashboard)

		for _, panel := range dashboard.Panels {
			if slices.Contains(IGNORED_TYPES, panel.Type) {
				SKIPPED++
				continue
			}

			for _, target := range panel.Targets {
				expression := normalizeExpression(target.Expr, variables)
				if expression == "" {
					SKIPPED++
					continue
				}

				selectors, err := parseSelectors(expression)
				if err != nil {
					ERRORS++
					fmt.Fprintf(
						os.Stderr,
						"\033[31mDashboard '%s', Panel '%d'\033[0m\n"+
							"\033[31m%s\033[0m\nOriginal:\t%s\nNormalized:\t%s\n\n",
						dashboard.UID,
						panel.ID,
						err.Error(),
						target.Expr,
						expression,
					)
				}

				for _, selector := range selectors {
					name, labels := loadMetric(selector)
					metrics[name] = merge(metrics[name], labels)
				}
				SUCCESS++
			}
		}
	}
	return metrics
}

// Format is like [[__name__="MetricName" label1="value" label2="value"]]
func loadMetric(selector []*labels.Matcher) (string, []string) {
	var name string
	var labels []string
	for _, element := range selector {
		if element.Name == "__name__" {
			name = element.Value
		} else {
			labels = append(labels, element.Name)
		}
	}
	return name, labels
}

func merge(arr1 []string, arr2 []string) []string {
	var set = make(map[string]bool)
	var arr3 []string
	for _, val := range arr1 {
		set[val] = true
	}
	for _, val := range arr2 {
		set[val] = true
	}
	for key := range set {
		arr3 = append(arr3, key)
	}
	return arr3
}

func Map[T, U any](ts []T, f func(T) U) []U {
	us := make([]U, len(ts))
	for i := range ts {
		us[i] = f(ts[i])
	}
	return us
}

func firstNonEmptyString(strings ...string) string {
	for _, s := range strings {
		if len(s) != 0 {
			return s
		}
	}
	return ""
}

// Escape any non-escaped double quotes within the string
func escape(s string) string {
	return regexp.MustCompile(`([^\\])"`).ReplaceAllString(s, `$1\"`)
}

func safeIndex(strings []string, idx int) string {
	if idx < len(strings) {
		return strings[idx]
	}
	return ""
}
