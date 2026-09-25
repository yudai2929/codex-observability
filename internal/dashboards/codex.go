package dashboards

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	viz "github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/resource"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/tempo"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

// Codex's OTLP counters are cumulative after ingestion into Prometheus. A
// short-lived session can report only once, so its last observed value is
// shown within the selected range rather than an exact range increase.
func codexLastReported(metric string) string {
	return fmt.Sprintf("sum(last_over_time(%s[$__range]))", metric)
}

func codexRequests() string {
	return `{__name__=~"codex_(api|websocket)_request_total"}`
}

func codexRequestDuration(suffix string) string {
	return fmt.Sprintf(`{__name__=~"codex_(api|websocket)_request_duration_ms_milliseconds_%s"}`, suffix)
}

func codexDonut(id int, position Position, title, description, expr, legend string) PanelDefinition {
	return PanelDefinition{
		ID: id, Title: title, Description: description, Position: position,
		Visualization: piechart.NewVisualizationV2Builder().Unit("short").
			PieType(piechart.PieChartTypeDonut).
			ReduceOptions(viz.NewReduceDataOptionsBuilder().Calcs([]string{"lastNotNull"})).
			Legend(piechart.NewPieChartLegendOptionsBuilder().ShowLegend(true).
				DisplayMode(viz.LegendDisplayModeTable).Placement(viz.LegendPlacementRight).
				Values([]piechart.PieChartLegendValues{piechart.PieChartLegendValuesValue, piechart.PieChartLegendValuesPercent})),
		Queries: []cog.Builder[dashboardv2.PanelQueryKind]{promQuery(expr, "A", legend, true)},
	}
}

func codexTimeSeries(id int, position Position, title, description, unit string, queries ...cog.Builder[dashboardv2.PanelQueryKind]) PanelDefinition {
	return PanelDefinition{
		ID: id, Title: title, Description: description, Position: position,
		Visualization: timeseries.NewVisualizationV2Builder().Unit(unit).Min(0).
			Legend(viz.NewVizLegendOptionsBuilder().ShowLegend(true).
				DisplayMode(viz.LegendDisplayModeTable).Placement(viz.LegendPlacementRight).
				Calcs([]string{"lastNotNull"})),
		Queries: queries,
	}
}

func BuildCodexDashboard() (resource.Manifest, error) {
	const reported = "Last cumulative value observed in the selected time range; this is not the increase during that range."
	definition := Definition{
		UID:         "codex-otel-local",
		Title:       "OpenAI Codex overview",
		Description: "A local view inspired by Grafana's OpenAI Codex Overview. Codex OTel metrics do not include team member identities, so the dashboard shows the originating client instead. Stat panels and breakdowns show the last cumulative value observed in the selected time range.",
		Tags:        []string{"codex", "opentelemetry", "local"},
		Panels: []PanelDefinition{
			statPanel(1, Position{0, 0, 6, 4}, "Total requests", codexLastReported(codexRequests()), "short", reported+" Combined HTTP and WebSocket requests."),
			statPanel(2, Position{6, 0, 6, 4}, "Total tokens", codexLastReported(`codex_turn_token_usage_sum{token_type="total"}`), "short", reported+" Input and output tokens combined."),
			statPanel(3, Position{12, 0, 6, 4}, "Conversations", codexLastReported("codex_thread_started_total"), "short", reported+" Number of Codex threads started."),
			statPanel(4, Position{18, 0, 6, 4}, "Active users", `clamp_max(`+codexLastReported("codex_thread_started_total")+`, 1)`, "short", "Shows 0 or 1 depending on whether Codex was used locally. The OTel metrics do not identify multiple users."),
			codexTimeSeries(5, Position{0, 4, 12, 8}, "Request rate by model", "Model requests per minute. Requires at least two consecutive metric reports.", "short",
				promQuery(`sum by (model) (rate(`+codexRequests()+`[$__rate_interval])) * 60`, "A", "{{model}}", false)),
			codexDonut(6, Position{12, 4, 12, 8}, "Requests by model", reported,
				`sum by (model) (last_over_time(`+codexRequests()+`[$__range])) > 0`, "{{model}}"),
			codexTimeSeries(7, Position{0, 12, 12, 8}, "API request latency", "Average and estimated 95th percentile latency across HTTP and WebSocket requests. Requires consecutive reports.", "ms",
				promQuery(`sum(rate(`+codexRequestDuration("sum")+`[$__rate_interval])) / sum(rate(`+codexRequestDuration("count")+`[$__rate_interval]))`, "A", "Avg", false),
				promQuery(`histogram_quantile(0.95, sum by (le) (rate(`+codexRequestDuration("bucket")+`[$__rate_interval])))`, "B", "P95", false)),
			codexDonut(8, Position{12, 12, 12, 8}, "Requests by client", "This panel replaces the team member breakdown in the hosted dashboard. It shows requests by the originating Codex client. "+reported,
				`sum by (originator) (last_over_time(`+codexRequests()+`[$__range])) > 0`, "{{originator}}"),
			statPanel(9, Position{0, 20, 6, 5}, "Avg response duration", `sum(last_over_time(codex_turn_e2e_duration_ms_milliseconds_sum[$__range])) / sum(last_over_time(codex_turn_e2e_duration_ms_milliseconds_count[$__range]))`, "ms", "Average end-to-end turn duration, calculated as the cumulative histogram sum divided by count."),
			codexDonut(10, Position{6, 20, 6, 5}, "Auth mode", "Authentication mode recorded when a thread starts. "+reported,
				`sum by (auth_mode) (last_over_time(codex_thread_started_total[$__range])) > 0`, "{{auth_mode}}"),
			codexDonut(11, Position{12, 20, 6, 5}, "Terminal type", "Client type that started the thread, using the Codex OTel session_source attribute. "+reported,
				`sum by (session_source) (last_over_time(codex_thread_started_total[$__range])) > 0`, "{{session_source}}"),
			codexDonut(12, Position{18, 20, 6, 5}, "Codex version", "Codex application version that started the thread. "+reported,
				`sum by (app_version) (last_over_time(codex_thread_started_total[$__range])) > 0`, "{{app_version}}"),
			{
				ID: 13, Title: "Codex traces", Position: Position{0, 25, 24, 9},
				Description:   "Traces for tasks in the selected time range. Select a row to inspect it in Tempo.",
				Visualization: table.NewVisualizationV2Builder(),
				Queries: []cog.Builder[dashboardv2.PanelQueryKind]{
					dashboardv2.NewTargetBuilder().RefId("A").Query(tempo.NewQueryV2Builder().
						Datasource(dashboardv2.NewDashboardv2DataQueryKindDatasourceBuilder().Name("tempo")).
						QueryType("traceql").Query(`{resource.service.name=~"[Cc]odex.*"}`).
						Limit(100).TableType(tempo.SearchTableTypeTraces)),
				},
			},
		},
	}
	return definition.Build()
}
