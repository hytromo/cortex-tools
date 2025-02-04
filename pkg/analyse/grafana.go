package analyse

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	log "github.com/sirupsen/logrus"

	"github.com/grafana-tools/sdk"
	"github.com/pkg/errors"
	"github.com/prometheus/prometheus/promql/parser"
)

type MetricsInGrafana struct {
	MetricsUsed    []string            `json:"metricsUsed"`
	OverallMetrics map[string]struct{} `json:"-"`
	Dashboards     []DashboardMetrics  `json:"dashboards"`
}

type DashboardMetrics struct {
	Slug        string            `json:"slug"`
	UID         string            `json:"uid,omitempty"`
	Title       string            `json:"title"`
	Metrics     map[string]Metric `json:"metrics"`
	ParseErrors []string          `json:"parse_errors"`
}

func ParseMetricsInBoard(mig *MetricsInGrafana, board sdk.Board) {
	var parseErrors []error
	metrics := make(map[string]Metric)

	// Iterate through all the panels and collect metrics
	for _, panel := range board.Panels {
		parseErrors = append(parseErrors, metricsFromPanel(*panel, metrics)...)
		if panel.RowPanel != nil {
			for _, subPanel := range panel.RowPanel.Panels {
				parseErrors = append(parseErrors, metricsFromPanel(subPanel, metrics)...)
			}
		}
	}

	// Iterate through all the rows and collect metrics
	for _, row := range board.Rows {
		for _, panel := range row.Panels {
			parseErrors = append(parseErrors, metricsFromPanel(panel, metrics)...)
		}
	}

	// Process metrics in templating
	parseErrors = append(parseErrors, metricsFromTemplating(board.Templating, metrics)...)

	var parseErrs []string
	for _, err := range parseErrors {
		parseErrs = append(parseErrs, err.Error())
	}

	var metricsInBoard []string
	for metric := range metrics {
		if metric == "" {
			continue
		}

		metricsInBoard = append(metricsInBoard, metric)
		mig.OverallMetrics[metric] = struct{}{}
	}
	sort.Strings(metricsInBoard)

	mig.Dashboards = append(mig.Dashboards, DashboardMetrics{
		Slug:        board.Slug,
		UID:         board.UID,
		Title:       board.Title,
		Metrics:     metrics,
		ParseErrors: parseErrs,
	})

}

func metricsFromTemplating(templating sdk.Templating, metrics map[string]Metric) []error {
	parseErrors := []error{}
	for _, templateVar := range templating.List {
		if templateVar.Type != "query" {
			continue
		}
		if query, ok := templateVar.Query.(string); ok {
			// label_values
			if strings.Contains(query, "label_values") {
				re := regexp.MustCompile(`label_values\(([a-zA-Z0-9_]+)`)
				sm := re.FindStringSubmatch(query)
				// In case of really gross queries, like - https://github.com/grafana/jsonnet-libs/blob/e97ab17f67ab40d5fe3af7e59151dd43be03f631/hass-mixin/dashboard.libsonnet#L93
				if len(sm) > 0 {
					query = sm[1]
				}
			}
			// query_result
			if strings.Contains(query, "query_result") {
				re := regexp.MustCompile(`query_result\((.+)\)`)
				query = re.FindStringSubmatch(query)[1]
			}
			err := parseQuery(query, metrics)
			if err != nil {
				parseErrors = append(parseErrors, errors.Wrapf(err, "query=%v", query))
				log.Debugln("msg", "promql parse error", "err", err, "query", query)
				continue
			}
		} else {
			err := fmt.Errorf("templating type error: name=%v", templateVar.Name)
			parseErrors = append(parseErrors, err)
			log.Debugln("msg", "templating parse error", "err", err)
			continue
		}
	}
	return parseErrors
}

func metricsFromPanel(panel sdk.Panel, metrics map[string]Metric) []error {
	var parseErrors []error

	targets := panel.GetTargets()
	if targets == nil {
		parseErrors = append(parseErrors, fmt.Errorf("unsupported panel type: %q", panel.CommonPanel.Type))
		return parseErrors
	}

	for _, target := range *targets {
		// Prometheus has this set.
		if target.Expr == "" {
			continue
		}
		query := target.Expr
		err := parseQuery(query, metrics)
		if err != nil {
			parseErrors = append(parseErrors, errors.Wrapf(err, "query=%v", query))
			log.Debugln("msg", "promql parse error", "err", err, "query", query)
			continue
		}
	}

	return parseErrors
}

type Metric struct {
	Labels []string
}

func unique(arr []string) []string {
	occurred := map[string]bool{}
	result := []string{}
	for e := range arr {

		// check if already the mapped
		// variable is set to true or not
		if occurred[arr[e]] != true {
			occurred[arr[e]] = true

			// Append to result slice.
			result = append(result, arr[e])
		}
	}

	return result
}

func parseQuery(query string, metrics map[string]Metric) error {
	query = strings.ReplaceAll(query, `$__interval`, "5m")
	query = strings.ReplaceAll(query, `$interval`, "5m")
	query = strings.ReplaceAll(query, `$resolution`, "5s")
	query = strings.ReplaceAll(query, "$__rate_interval", "15s")
	query = strings.ReplaceAll(query, "$__range", "1d")
	query = strings.ReplaceAll(query, "${__range_s:glob}", "30")
	query = strings.ReplaceAll(query, "${__range_s}", "30")
	expr, err := parser.ParseExpr(query)
	if err != nil {
		return err
	}

	parser.Inspect(expr, func(node parser.Node, path []parser.Node) error {
		if n, ok := node.(*parser.VectorSelector); ok {
			// fmt.Println("-------------------")
			// fmt.Printf("METRIC NAME %v\n", n.Name)

			var labels []string
			for _, l := range n.LabelMatchers {
				labels = append(labels, l.Name)
			}

			if _, exists := metrics[n.Name]; exists {
				oldCount := len(metrics[n.Name].Labels)
				newLabels := unique(append(metrics[n.Name].Labels, labels...))
				newCount := len(newLabels)

				if oldCount != newCount {
					// fmt.Printf("old labels: %v\n", metrics[n.Name].Labels)
					// fmt.Printf("new labels: %v\n", newLabels)
				}

				metrics[n.Name] = Metric{
					Labels: newLabels,
				}
			} else {
				// fmt.Printf("new metric: %v\n", n.Name)
				metrics[n.Name] = Metric{
					Labels: labels,
				}
			}

			// fmt.Println("-------------------")
		}

		return nil
	})

	return nil
}
