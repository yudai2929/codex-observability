package dashboardgen

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/yudai2929/codex-observability/internal/dashboards"

	"github.com/grafana/grafana-foundation-sdk/go/resource"
)

type Config struct {
	CodexPath string
}

type Generator struct {
	Config Config
}

type output struct {
	path  string
	build func() (resource.Manifest, error)
}

func (generator Generator) Run() error {
	manifest, err := dashboards.BuildCodexDashboard()
	if err != nil {
		return fmt.Errorf("build %s: %w", generator.Config.CodexPath, err)
	}
	if err := write(generator.Config.CodexPath, manifest); err != nil {
		return fmt.Errorf("write %s: %w", generator.Config.CodexPath, err)
	}
	return nil
}

func write(path string, manifest resource.Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}
