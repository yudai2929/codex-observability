package main

import (
	"flag"
	"fmt"
	"os"

	"github.com/yudai2929/codex-observability/internal/dashboardgen"
)

func main() {
	var config dashboardgen.Config
	flag.StringVar(&config.CodexPath, "codex-out", "grafana/dashboards/codex.json", "Codex OTel dashboard JSON path")
	flag.Parse()

	if err := (dashboardgen.Generator{Config: config}).Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
