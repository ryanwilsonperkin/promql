package promfiles

import (
	"fmt"
	"io"
	"log"
	"os"
	"regexp"
	"strconv"
	"strings"

	"github.com/prometheus/prometheus/model/labels"
	"github.com/prometheus/prometheus/promql/parser"
)

type Metrics map[string][]string

type Variable struct {
	Name  string
	Value string
}

type LoadResult struct {
	Skipped   int
	Succeeded int
	Failed    int
}

func (result1 *LoadResult) Add(result2 LoadResult) {
	result1.Failed += result2.Failed
	result1.Skipped += result2.Skipped
	result1.Succeeded += result2.Succeeded
}

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
	normalized = regexp.MustCompile("xrate\\(").ReplaceAllString(normalized, "rate(")
	// Replace xincrease function with increase function
	normalized = regexp.MustCompile("xincrease\\(").ReplaceAllString(normalized, "increase(")
	return normalized
}

func parseSelectors(input string) ([][]*labels.Matcher, error) {
	expr, err := parser.ParseExpr(input)
	if err != nil {
		return nil, err
	}
	return parser.ExtractSelectors(expr), nil
}

func loadFile(filename string) []byte {
	jsonFile, err := os.Open(filename)
	defer jsonFile.Close()
	if err != nil {
		log.Fatal(err)
	}

	byteValue, err := io.ReadAll(jsonFile)
	if err != nil {
		log.Fatal(err)
	}
	return byteValue
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
