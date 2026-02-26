package runtimewire

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sync"

	"github.com/Gurpartap/agentframe/agent"
	"github.com/Gurpartap/agentframe/agentreact"
	eventinginmem "github.com/Gurpartap/agentframe/eventing/inmem"
	runstoreinmem "github.com/Gurpartap/agentframe/runstore/inmem"

	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/config"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/modelopenai"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runstream"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/runtimewire/mocks"
	"github.com/Gurpartap/agentframe/examples/coding-agent/internal/toolset"
)

// Runtime contains the composed runtime dependencies for the server.
type Runtime struct {
	Runner          *agent.Runner
	RunStore        *runstoreinmem.Store
	EventSink       *eventinginmem.Sink
	StreamBroker    *runstream.Broker
	ToolDefinitions []agent.ToolDefinition
}

func New(cfg config.Config) (*Runtime, error) {
	return newRuntime(cfg, nil)
}

func NewWithLogger(cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	return newRuntime(cfg, logger)
}

func newRuntime(cfg config.Config, logger *slog.Logger) (*Runtime, error) {
	if err := cfg.Validate(); err != nil {
		return nil, fmt.Errorf("new runtime config: %w", err)
	}

	store := runstoreinmem.New()
	events := eventinginmem.New()
	streamBroker := runstream.New(runstream.DefaultHistoryLimit)
	eventLogger := newRuntimeEventLogSink(logger, cfg.LogFormat)
	fanout := newFanoutSink(events, streamBroker, eventLogger)

	model, err := buildModel(cfg)
	if err != nil {
		return nil, fmt.Errorf("new runtime model: %w", err)
	}
	tools, toolDefinitions, err := buildTools(cfg)
	if err != nil {
		return nil, fmt.Errorf("new runtime tools: %w", err)
	}
	loop, err := agentreact.New(model, tools, fanout)
	if err != nil {
		return nil, fmt.Errorf("new runtime loop: %w", err)
	}

	runner, err := agent.NewRunner(agent.Dependencies{
		IDGenerator: newSequenceIDGenerator(),
		RunStore:    store,
		Engine:      loop,
		EventSink:   fanout,
	})
	if err != nil {
		return nil, fmt.Errorf("new runtime runner: %w", err)
	}

	return &Runtime{
		Runner:          runner,
		RunStore:        store,
		EventSink:       events,
		StreamBroker:    streamBroker,
		ToolDefinitions: toolDefinitions,
	}, nil
}

func buildModel(cfg config.Config) (agentreact.Model, error) {
	switch cfg.ModelMode {
	case config.ModelModeMock:
		return mocks.NewModel(), nil
	case config.ModelModeProvider:
		httpClient := &http.Client{Timeout: cfg.ProviderTimeout}
		providerModel, err := modelopenai.New(modelopenai.Config{
			APIKey:     cfg.ProviderAPIKey,
			Model:      cfg.ProviderModel,
			BaseURL:    cfg.ProviderBaseURL,
			HTTPClient: httpClient,
		})
		if err != nil {
			return nil, err
		}
		return providerModel, nil
	default:
		return nil, fmt.Errorf("unsupported model mode %q", cfg.ModelMode)
	}
}

func buildTools(cfg config.Config) (agentreact.ToolExecutor, []agent.ToolDefinition, error) {
	switch cfg.ToolMode {
	case config.ToolModeMock:
		return mocks.NewTools(), mocks.Definitions(), nil
	case config.ToolModeReal:
		policy, err := toolset.NewPolicy(cfg.WorkspaceRoot, cfg.BashTimeout)
		if err != nil {
			return nil, nil, err
		}
		return toolset.NewExecutor(policy), toolset.Definitions(), nil
	default:
		return nil, nil, fmt.Errorf("unsupported tool mode %q", cfg.ToolMode)
	}
}

type fanoutSink struct {
	sinks []agent.EventSink
}

func newFanoutSink(sinks ...agent.EventSink) fanoutSink {
	filtered := make([]agent.EventSink, 0, len(sinks))
	for _, sink := range sinks {
		if sink != nil {
			filtered = append(filtered, sink)
		}
	}
	return fanoutSink{sinks: filtered}
}

func (s fanoutSink) Publish(ctx context.Context, event agent.Event) error {
	var result error
	for _, sink := range s.sinks {
		if err := sink.Publish(ctx, event); err != nil {
			result = errors.Join(result, err)
		}
	}
	return result
}

type sequenceIDGenerator struct {
	mu   sync.Mutex
	next uint64
}

func newSequenceIDGenerator() *sequenceIDGenerator {
	return &sequenceIDGenerator{}
}

func (g *sequenceIDGenerator) NewRunID(_ context.Context) (agent.RunID, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	g.next++
	return agent.RunID(fmt.Sprintf("run-%06d", g.next)), nil
}
