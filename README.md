```
# build
make cross

# export vars before use
export GRAFANA_ADDRESS=<address>
export GRAFANA_API_KEY=<api key>
export ASSUME_NO_MOVING_GC_UNSAFE_RISK_IT_WITH=go1.18

# create metrics-in-grafana.json
./dist/cortextool-linux-amd64 analyse grafana

# extract all dashboards and (*optionally*) filter out some
cat metrics-in-grafana.json | jq '.dashboards[] | select(.title|test(".*WIP"))' | jq -s > app-filtered.json

# repeat the above process for all the organizations

# and then...
node combine.js
# prints out ready-to-use prometheus configuration to filter out metrics and labels
```