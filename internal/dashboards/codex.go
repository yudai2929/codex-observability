package dashboards

import (
	"fmt"

	"github.com/grafana/grafana-foundation-sdk/go/cog"
	viz "github.com/grafana/grafana-foundation-sdk/go/common"
	"github.com/grafana/grafana-foundation-sdk/go/dashboardv2"
	"github.com/grafana/grafana-foundation-sdk/go/piechart"
	"github.com/grafana/grafana-foundation-sdk/go/resource"
	"github.com/grafana/grafana-foundation-sdk/go/stat"
	"github.com/grafana/grafana-foundation-sdk/go/table"
	"github.com/grafana/grafana-foundation-sdk/go/tempo"
	"github.com/grafana/grafana-foundation-sdk/go/timeseries"
)

const codexWebsocketRequests = "codex_websocket_request_total"
const codexAPIRequests = "codex_api_request_total"

// Codex exports cumulative OTLP instruments. A short-lived CLI session may
// report only once, so use the latest snapshot in the selected range instead
// of increase(), which requires multiple samples.
func codexLatest(metric string) string {
	return fmt.Sprintf("sum(last_over_time(%s[$__range])) or vector(0)", metric)
}

func codexBreakdown(metric, label string) string {
	return fmt.Sprintf(
		`sum by (%[1]s) (last_over_time(%[2]s[$__range])) or on() label_replace(vector(0), "%[1]s", "No activity", "", "")`,
		label, metric,
	)
}

func codexRequestCounts() string {
	return fmt.Sprintf(
		`label_replace(last_over_time(%s[$__range]), "request_transport", "API", "__name__", ".*") or label_replace(last_over_time(%s[$__range]), "request_transport", "WebSocket", "__name__", ".*")`,
		codexAPIRequests,
		codexWebsocketRequests,
	)
}

func codexRequestTotal() string {
	return fmt.Sprintf(
		`(sum(last_over_time(%s[$__range])) or vector(0)) + (sum(last_over_time(%s[$__range])) or vector(0))`,
		codexAPIRequests,
		codexWebsocketRequests,
	)
}

func codexRequestBreakdown(label string) string {
	return fmt.Sprintf(
		`sum by (%[1]s) (%[2]s) or on() label_replace(vector(0), "%[1]s", "No activity", "", "")`,
		label,
		codexRequestCounts(),
	)
}

func codexAverage(sumMetric, countMetric string) string {
	return fmt.Sprintf(
		"(sum(last_over_time(%s[$__range])) / clamp_min(sum(last_over_time(%s[$__range])), 1)) or vector(0)",
		sumMetric,
		countMetric,
	)
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

func codexStat(id int, position Position, title, expr, unit, description string) PanelDefinition {
	return PanelDefinition{
		ID: id, Title: title, Description: description, Position: position,
		Visualization: stat.NewVisualizationV2Builder().Unit(unit).ColorMode(viz.BigValueColorModeNone).NoValue("0"),
		Queries:       []cog.Builder[dashboardv2.PanelQueryKind]{promQuery(expr, "A", "", true)},
	}
}

func BuildCodexDashboard() (resource.Manifest, error) {
	const cumulativeDescription = "Latest cumulative Codex OTel snapshot found in the selected time range. Choose a shorter range to focus on recent activity."
	const averageDescription = "Average from the latest cumulative histogram snapshots in the selected time range. Displays 0 until Codex reports a measurement."
	definition := Definition{
		UID:         "codex-otel-local",
		Title:       "OpenAI Codex overview",
		Description: "Local Codex OpenTelemetry activity. Metric panels use the latest cumulative snapshots reported during the selected range; missing metric samples display as zero. Model and app version labels come from the Codex client that emitted each sample.",
		Tags:        []string{"codex", "opentelemetry", "local"},
		Panels: []PanelDefinition{
			codexStat(1, Position{0, 0, 6, 4}, "API + WebSocket requests", codexRequestTotal(), "short", "Sum of the latest cumulative API and WebSocket request counters in the selected time range."),
			codexStat(2, Position{6, 0, 6, 4}, "Conversations", codexLatest("codex_thread_started_total"), "short", "Codex threads started, from the latest cumulative snapshot."),
			codexStat(3, Position{12, 0, 6, 4}, "Turns", codexLatest("codex_conversation_turn_count_total"), "short", "Conversation turns reported by Codex."),
			codexStat(4, Position{18, 0, 6, 4}, "Tokens", codexLatest(`codex_turn_token_usage_sum{token_type="total"}`), "short", "Total input and output tokens reported by Codex."),

			codexTimeSeries(5, Position{0, 4, 12, 8}, "Cumulative requests by model", "Latest cumulative API and WebSocket request snapshots, grouped by the model label reported by Codex. This remains useful after a short session that reports only once; select a shorter time range to focus on recent model activity.", "short",
				promQuery(codexRequestBreakdown("model"), "A", "{{model}}", false)),
			codexDonut(6, Position{12, 4, 12, 8}, "Requests by model", "Latest cumulative API and WebSocket requests grouped by the model label reported by Codex.",
				codexRequestBreakdown("model"), "{{model}}"),

			codexTimeSeries(7, Position{0, 12, 12, 8}, "Token usage by model and type", "Latest cumulative token counts reported by Codex. The selected time range controls which model snapshots are included.", "short",
				promQuery(`sum by (model, token_type) (last_over_time(codex_turn_token_usage_sum[$__range])) or on() label_replace(vector(0), "token_type", "No activity", "", "")`, "A", "{{model}} · {{token_type}}", false)),
			codexDonut(8, Position{12, 12, 12, 8}, "Requests by client", "API and WebSocket requests grouped by the Codex client that emitted the samples. This is a local-client breakdown, not a team-member breakdown.",
				codexRequestBreakdown("originator"), "{{originator}}"),

			codexStat(9, Position{0, 20, 6, 4}, "Avg request latency", codexAverage("codex_websocket_request_duration_ms_milliseconds_sum", "codex_websocket_request_duration_ms_milliseconds_count"), "ms", averageDescription),
			codexStat(10, Position{6, 20, 6, 4}, "P95 request latency", `histogram_quantile(0.95, sum by (le) (last_over_time(codex_websocket_request_duration_ms_milliseconds_bucket[$__range]))) or vector(0)`, "ms", "95th percentile from the latest cumulative request-latency histogram snapshots in the selected time range."),
			codexStat(11, Position{12, 20, 6, 4}, "Avg turn duration", codexAverage("codex_turn_e2e_duration_ms_milliseconds_sum", "codex_turn_e2e_duration_ms_milliseconds_count"), "ms", averageDescription),
			codexStat(12, Position{18, 20, 6, 4}, "Tool calls", codexLatest("codex_tool_call_total"), "short", "Tool calls reported by Codex. Displays 0 when no tool-call sample has been reported."),

			codexDonut(13, Position{0, 24, 8, 6}, "Threads by session source", "Where a Codex session started, such as the desktop app or CLI.",
				codexBreakdown("codex_thread_started_total", "session_source"), "{{session_source}}"),
			codexDonut(14, Position{8, 24, 8, 6}, "Threads by authentication mode", "Authentication mode reported when a thread starts.",
				codexBreakdown("codex_thread_started_total", "auth_mode"), "{{auth_mode}}"),
			codexDonut(15, Position{16, 24, 8, 6}, "Threads by Codex version", "Version of the Codex client that emitted each thread-start sample.",
				codexBreakdown("codex_thread_started_total", "app_version"), "{{app_version}}"),
			{
				ID: 16, Title: "Codex traces", Position: Position{0, 30, 24, 9},
				Description:   "Codex spans in the selected time range. This table fills when Codex has emitted traces; metric panels remain useful even when there are no spans.",
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
