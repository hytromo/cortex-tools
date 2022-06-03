// cat metrics-in-grafana.json | jq '.dashboards[] | select(.title|test(".*WIP"))' | jq -s > app-filtered.json

const fs = require('fs')

const dashboards = [...JSON.parse(fs.readFileSync('infra-filtered.json')), ...JSON.parse(fs.readFileSync('app-filtered.json'))]

console.log(dashboards.length)

const metrics = {}
for (const dashboard of dashboards) {
	for (let [metric, { Labels: labels }] of Object.entries(dashboard.metrics)) {
		if (!metrics[metric]) {
			metrics[metric] = { labels: new Set() }
		}

		for (const label of labels) {
			metrics[metric].labels.add(label)
		}
	}
}

for (const metric in metrics) {
	metrics[metric].labels = [...metrics[metric].labels]
}

// metric->labels
// console.log(metrics)


const allLabels = new Set()
for (const metric in metrics) {
	metrics[metric].labels = [...metrics[metric].labels]
	for (const label of metrics[metric].labels) {
		allLabels.add(label)
	}
}

console.log(`
- source_labels: [__name__]
	regex: '(${Object.keys(metrics).sort().join('|')}})'
	action: keep

- regex: '(${[...allLabels].sort().join('|')}})'
  action: labelkeep
`)
