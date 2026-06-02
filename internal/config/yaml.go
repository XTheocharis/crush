package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/charmbracelet/crush/internal/fsext"
	"gopkg.in/yaml.v3"
)

// xrushConfig mirrors the .xrush/config.yml structure from the XRUSH
// implementation spec. Fields that map to existing Config struct fields are
// converted by xrushConfig.toConfig. Fields without a direct mapping are
// parsed and stored for future use.
type xrushConfig struct {
	Model       *xrushModelConfig       `yaml:"model,omitempty"`
	Context     *xrushContextConfig     `yaml:"context,omitempty"`
	Observation *xrushObservationConfig `yaml:"observation,omitempty"`
	DCP         *xrushDCPConfig         `yaml:"dcp,omitempty"`
	Quality     *xrushQualityConfig     `yaml:"quality,omitempty"`
	LSP         *xrushLSPConfig         `yaml:"lsp,omitempty"`
}

type xrushModelConfig struct {
	Architect string             `yaml:"architect,omitempty"`
	Editor    string             `yaml:"editor,omitempty"`
	Router    *xrushRouterConfig `yaml:"router,omitempty"`
}

type xrushRouterConfig struct {
	Tiers []xrushRouterTier `yaml:"tiers,omitempty"`
}

type xrushRouterTier struct {
	UpTo  int    `yaml:"up_to,omitempty"`
	Model string `yaml:"model,omitempty"`
}

type xrushContextConfig struct {
	AutoCompact *xrushAutoCompactConfig `yaml:"autoCompact,omitempty"`
}

type xrushAutoCompactConfig struct {
	BufferTokens      int                     `yaml:"buffer_tokens,omitempty"`
	OutputReservation int                     `yaml:"output_reservation,omitempty"`
	PostCompact       *xrushPostCompactConfig `yaml:"post_compact,omitempty"`
}

type xrushPostCompactConfig struct {
	MaxFiles    int `yaml:"max_files,omitempty"`
	TokenBudget int `yaml:"token_budget,omitempty"`
}

type xrushObservationConfig struct {
	Observer  *xrushObserverConfig  `yaml:"observer,omitempty"`
	Reflector *xrushReflectorConfig `yaml:"reflector,omitempty"`
}

type xrushObserverConfig struct {
	MessageTokens int     `yaml:"message_tokens,omitempty"`
	BufferRatio   float64 `yaml:"buffer_ratio,omitempty"`
	Model         string  `yaml:"model,omitempty"`
}

type xrushReflectorConfig struct {
	ObservationTokens int     `yaml:"observation_tokens,omitempty"`
	BufferActivation  float64 `yaml:"buffer_activation,omitempty"`
	Model             string  `yaml:"model,omitempty"`
}

type xrushDCPConfig struct {
	Compress   *xrushDCPCompressConfig   `yaml:"compress,omitempty"`
	Strategies *xrushDCPStrategiesConfig `yaml:"strategies,omitempty"`
}

type xrushDCPCompressConfig struct {
	MaxContextLimit int `yaml:"maxContextLimit,omitempty"`
	MinContextLimit int `yaml:"minContextLimit,omitempty"`
	NudgeFrequency  int `yaml:"nudgeFrequency,omitempty"`
}

type xrushDCPStrategiesConfig struct {
	Deduplication bool `yaml:"deduplication,omitempty"`
	PurgeErrors   bool `yaml:"purgeErrors,omitempty"`
}

type xrushQualityConfig struct {
	LintOnWrite bool `yaml:"lint_on_write,omitempty"`
	AutoCommit  bool `yaml:"auto_commit,omitempty"`
	MaxRetries  int  `yaml:"max_retries,omitempty"`
}

type xrushLSPConfig struct {
	Mode           string   `yaml:"mode,omitempty"`
	StartupTimeout string   `yaml:"startup_timeout,omitempty"`
	Languages      []string `yaml:"languages,omitempty"`
}

// toConfig converts the xrush YAML structure into the existing Config struct.
// Only fields with a direct mapping are translated; everything else is
// gracefully ignored and will be handled by future tasks.
func (x *xrushConfig) toConfig() *Config {
	cfg := &Config{}

	if x.Model == nil {
		return cfg
	}

	if x.Model.Architect != "" || x.Model.Editor != "" || x.Model.Router != nil {
		cfg.Options = &Options{}
	}

	if x.Model.Architect != "" {
		provider, model := parseModelString(x.Model.Architect)
		cfg.Options.ArchitectModel = &SelectedModel{
			Provider: provider,
			Model:    model,
		}
	}

	if x.Model.Editor != "" {
		provider, model := parseModelString(x.Model.Editor)
		cfg.Options.EditorModel = &SelectedModel{
			Provider: provider,
			Model:    model,
		}
	}

	if x.Model.Router != nil && len(x.Model.Router.Tiers) > 0 {
		tiers := make([]RoutingTier, 0, len(x.Model.Router.Tiers))
		for _, dt := range x.Model.Router.Tiers {
			tiers = append(tiers, RoutingTier{
				UpToTokens: dt.UpTo,
				ModelType:  SelectedModelType(dt.Model),
			})
		}
		cfg.Options.RouterTiers = tiers
	}

	// Wire DCP compress config → LCM nudge options.
	if x.DCP != nil && x.DCP.Compress != nil {
		dcp := x.DCP.Compress
		if dcp.MaxContextLimit > 0 || dcp.MinContextLimit > 0 || dcp.NudgeFrequency > 0 {
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			if cfg.Options.LCM == nil {
				cfg.Options.LCM = &LCMOptions{}
			}
			cfg.Options.LCM.Nudge = &NudgeOptions{
				MaxContextLimit: int64(dcp.MaxContextLimit),
				MinContextLimit: int64(dcp.MinContextLimit),
				NudgeFrequency:  dcp.NudgeFrequency,
			}
		}
	}

	// Wire observation config → LCM observation options.
	if x.Observation != nil {
		obs := x.Observation
		hasObserver := obs.Observer != nil && (obs.Observer.MessageTokens > 0 || obs.Observer.BufferRatio > 0 || obs.Observer.Model != "")
		hasReflector := obs.Reflector != nil && (obs.Reflector.ObservationTokens > 0 || obs.Reflector.BufferActivation > 0 || obs.Reflector.Model != "")
		if hasObserver || hasReflector {
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			if cfg.Options.LCM == nil {
				cfg.Options.LCM = &LCMOptions{}
			}
			observation := &ObservationOptions{}
			if obs.Observer != nil {
				observation.ObserverMessageTokens = obs.Observer.MessageTokens
				observation.ObserverBufferRatio = obs.Observer.BufferRatio
				observation.ObserverModel = obs.Observer.Model
			}
			if obs.Reflector != nil {
				observation.ReflectorObservationTokens = obs.Reflector.ObservationTokens
				observation.ReflectorBufferActivation = obs.Reflector.BufferActivation
				observation.ReflectorModel = obs.Reflector.Model
			}
			cfg.Options.LCM.Observation = observation
		}
	}

	// Wire quality config → validation options.
	if x.Quality != nil {
		if x.Quality.LintOnWrite || x.Quality.AutoCommit || x.Quality.MaxRetries > 0 {
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			if cfg.Options.Validation == nil {
				cfg.Options.Validation = &ValidationOptions{}
			}
			cfg.Options.Validation.AutoFix = x.Quality.LintOnWrite
			cfg.Options.Validation.AutoFixLoopEnabled = x.Quality.AutoCommit
			cfg.Options.Validation.MaxAutoFixRetries = x.Quality.MaxRetries
		}
	}

	// Wire context autoCompact config → LCM options.
	if x.Context != nil && x.Context.AutoCompact != nil {
		ac := x.Context.AutoCompact
		if ac.BufferTokens > 0 || ac.OutputReservation > 0 || ac.PostCompact != nil {
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			if cfg.Options.LCM == nil {
				cfg.Options.LCM = &LCMOptions{}
			}
			cfg.Options.LCM.LargeToolOutputTokenThreshold = ac.BufferTokens
			cfg.Options.LCM.CtxCutoffThreshold = float64(ac.OutputReservation) / 100000
			if ac.PostCompact != nil {
				cfg.Options.LCM.PostCompactMaxFiles = ac.PostCompact.MaxFiles
				cfg.Options.LCM.PostCompactTokenBudget = int64(ac.PostCompact.TokenBudget)
			}
		}
	}

	// Wire DCP strategies config → LCM options.
	if x.DCP != nil && x.DCP.Strategies != nil {
		s := x.DCP.Strategies
		if s.Deduplication || s.PurgeErrors {
			if cfg.Options == nil {
				cfg.Options = &Options{}
			}
			if cfg.Options.LCM == nil {
				cfg.Options.LCM = &LCMOptions{}
			}
			cfg.Options.LCM.DeduplicationEnabled = s.Deduplication
			cfg.Options.LCM.PurgeErrorsEnabled = s.PurgeErrors
		}
	}

	return cfg
}

// parseModelString splits a "provider/model" string into its components.
// If there is no slash, the entire string is treated as the model name with
// an empty provider.
func parseModelString(s string) (provider, model string) {
	parts := strings.SplitN(s, "/", 2)
	if len(parts) == 2 {
		return parts[0], parts[1]
	}
	return "", s
}

// loadYAMLConfig reads and parses a YAML config file, converting it to a
// Config struct.
func loadYAMLConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config %s: %w", path, err)
	}

	var dc xrushConfig
	if err := yaml.Unmarshal(data, &dc); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config %s: %w", path, err)
	}

	return dc.toConfig(), nil
}

// lookupYAMLConfig searches for .xrush/config.yml starting from cwd and
// walking up the directory tree. Returns the path and true if found.
func lookupYAMLConfig(cwd string) (string, bool) {
	path, ok := fsext.LookupClosest(cwd, filepath.Join(".xrush", "config.yml"))
	return path, ok
}

// isYAMLFile returns true if the file path has a .yml or .yaml extension.
func isYAMLFile(path string) bool {
	ext := strings.ToLower(filepath.Ext(path))
	return ext == ".yml" || ext == ".yaml"
}

// yamlConfigToJSON converts a YAML config file into JSON bytes that can be
// fed into the standard loadFromBytes merge pipeline. This allows YAML
// configs to participate in the same layered merge as JSON configs without
// modifying the merge logic.
func yamlConfigToJSON(yamlData []byte) ([]byte, error) {
	var dc xrushConfig
	if err := yaml.Unmarshal(yamlData, &dc); err != nil {
		return nil, fmt.Errorf("failed to parse YAML config: %w", err)
	}

	cfg := dc.toConfig()
	jsonData, err := json.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal YAML config to JSON: %w", err)
	}

	return jsonData, nil
}

func modelString(m *SelectedModel) string {
	if m == nil {
		return ""
	}
	if m.Provider != "" {
		return m.Provider + "/" + m.Model
	}
	return m.Model
}

// fromConfig converts a Config struct into a xrushConfig for YAML
// serialization. It is the inverse of toConfig.
func fromConfig(cfg *Config) *xrushConfig {
	dc := &xrushConfig{}

	if cfg.Options != nil {
		archStr := modelString(cfg.Options.ArchitectModel)
		editStr := modelString(cfg.Options.EditorModel)
		if archStr != "" || editStr != "" {
			dc.Model = &xrushModelConfig{
				Architect: archStr,
				Editor:    editStr,
			}
		}
	}

	if cfg.Options != nil && cfg.Options.LCM != nil {
		lcm := cfg.Options.LCM
		if lcm.CtxCutoffThreshold > 0 || lcm.LargeToolOutputTokenThreshold > 0 || lcm.PostCompactMaxFiles > 0 || lcm.PostCompactTokenBudget > 0 {
			ac := &xrushAutoCompactConfig{
				BufferTokens:      lcm.LargeToolOutputTokenThreshold,
				OutputReservation: int(lcm.CtxCutoffThreshold * 100000),
			}
			if lcm.PostCompactMaxFiles > 0 || lcm.PostCompactTokenBudget > 0 {
				ac.PostCompact = &xrushPostCompactConfig{
					MaxFiles:    lcm.PostCompactMaxFiles,
					TokenBudget: int(lcm.PostCompactTokenBudget),
				}
			}
			dc.Context = &xrushContextConfig{
				AutoCompact: ac,
			}
		}

		if lcm.Observation != nil {
			obs := lcm.Observation
			xobs := &xrushObservationConfig{}
			hasObserver := obs.ObserverMessageTokens > 0 || obs.ObserverBufferRatio > 0 || obs.ObserverModel != ""
			hasReflector := obs.ReflectorObservationTokens > 0 || obs.ReflectorBufferActivation > 0 || obs.ReflectorModel != ""
			if hasObserver {
				xobs.Observer = &xrushObserverConfig{
					MessageTokens: obs.ObserverMessageTokens,
					BufferRatio:   obs.ObserverBufferRatio,
					Model:         obs.ObserverModel,
				}
			}
			if hasReflector {
				xobs.Reflector = &xrushReflectorConfig{
					ObservationTokens: obs.ReflectorObservationTokens,
					BufferActivation:  obs.ReflectorBufferActivation,
					Model:             obs.ReflectorModel,
				}
			}
			if hasObserver || hasReflector {
				dc.Observation = xobs
			}
		}

		if lcm.DeduplicationEnabled || lcm.PurgeErrorsEnabled {
			if dc.DCP == nil {
				dc.DCP = &xrushDCPConfig{}
			}
			dc.DCP.Strategies = &xrushDCPStrategiesConfig{
				Deduplication: lcm.DeduplicationEnabled,
				PurgeErrors:   lcm.PurgeErrorsEnabled,
			}
		}

		if lcm.Nudge != nil && (lcm.Nudge.MaxContextLimit > 0 || lcm.Nudge.MinContextLimit > 0 || lcm.Nudge.NudgeFrequency > 0) {
			if dc.DCP == nil {
				dc.DCP = &xrushDCPConfig{}
			}
			dc.DCP.Compress = &xrushDCPCompressConfig{
				MaxContextLimit: int(lcm.Nudge.MaxContextLimit),
				MinContextLimit: int(lcm.Nudge.MinContextLimit),
				NudgeFrequency:  lcm.Nudge.NudgeFrequency,
			}
		}

		if cfg.Options.Validation != nil && (cfg.Options.Validation.AutoFix || cfg.Options.Validation.AutoFixLoopEnabled || cfg.Options.Validation.MaxAutoFixRetries > 0) {
			dc.Quality = &xrushQualityConfig{
				LintOnWrite: cfg.Options.Validation.AutoFix,
				AutoCommit:  cfg.Options.Validation.AutoFixLoopEnabled,
				MaxRetries:  cfg.Options.Validation.MaxAutoFixRetries,
			}
		}
	}

	if len(cfg.LSP) > 0 {
		languages := make([]string, 0, len(cfg.LSP))
		for name := range cfg.LSP {
			languages = append(languages, name)
		}
		slices.Sort(languages)
		dc.LSP = &xrushLSPConfig{
			Mode:      "auto",
			Languages: languages,
		}
	}

	return dc
}

// MarshalYAML serializes the xrushConfig to YAML bytes.
func (x *xrushConfig) MarshalYAML() ([]byte, error) {
	data, err := yaml.Marshal(x)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal xrush config to YAML: %w", err)
	}
	return data, nil
}

func (x *xrushConfig) UnmarshalYAML(data []byte) error {
	if err := yaml.Unmarshal(data, x); err != nil {
		return fmt.Errorf("failed to unmarshal xrush config: %w", err)
	}
	return nil
}

// WriteYAMLConfig writes a xrushConfig as YAML to the given file path,
// creating parent directories as needed.
func WriteYAMLConfig(path string, dc *xrushConfig) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	data, err := dc.MarshalYAML()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("failed to write YAML config: %w", err)
	}
	return nil
}

// ReadYAMLConfig reads a YAML file into a xrushConfig struct.
// [XRUSH: begin: YAML dual-read layer for .xrush/config.yml]
func lookupYAMLConfigPaths(cwd string, foundConfigs []string) []string {
	yamlConfigs, _ := fsext.LookupBounded(cwd, projectBoundary(cwd), filepath.Join(".xrush", "config.yml"))
	return append(foundConfigs, yamlConfigs...)
}

// [XRUSH: end]

// [XRUSH: begin: YAML config file support in load pipeline]
func parseConfigData(path string, data []byte) ([]byte, error) {
	if isYAMLFile(path) {
		jsonData, yamlErr := yamlConfigToJSON(data)
		if yamlErr != nil {
			return nil, fmt.Errorf("failed to convert YAML config %s: %w", path, yamlErr)
		}
		return jsonData, nil
	}
	if !json.Valid(data) {
		return nil, fmt.Errorf("invalid JSON in config file %s", path)
	}

	// Process @include directives in JSON config.
	processed, err := processJSONIncludes(data, filepath.Dir(path))
	if err != nil {
		return nil, fmt.Errorf("@include in %s: %w", path, err)
	}

	return processed, nil
}

// [XRUSH: end]

func ReadYAMLConfig(path string) (*xrushConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read YAML config: %w", err)
	}
	var dc xrushConfig
	if err := dc.UnmarshalYAML(data); err != nil {
		return nil, err
	}
	return &dc, nil
}
