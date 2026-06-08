package testutil

import (
	"context"
	"sync/atomic"

	"charm.land/fantasy"
)

// StubLanguageModel implements fantasy.LanguageModel for use in tests.
// Set the function fields to override specific behavior, or set Response
// for a simple canned text response.
type StubLanguageModel struct {
	GenerateFunc    func(context.Context, fantasy.Call) (*fantasy.Response, error)
	StreamFunc      func(context.Context, fantasy.Call) (fantasy.StreamResponse, error)
	GenerateObjFunc func(context.Context, fantasy.ObjectCall) (*fantasy.ObjectResponse, error)
	StreamObjFunc   func(context.Context, fantasy.ObjectCall) (fantasy.ObjectStreamResponse, error)
	ProviderVal     string
	ModelVal        string
	// Response is used as the default Generate text when GenerateFunc is nil.
	Response string
	// TrackGenerate counts how many times Generate is called.
	TrackGenerate bool
	callCount     atomic.Int64
}

// Generate calls GenerateFunc if set; otherwise returns Response as text content.
func (s *StubLanguageModel) Generate(
	ctx context.Context, call fantasy.Call,
) (*fantasy.Response, error) {
	s.callCount.Add(1)
	if s.GenerateFunc != nil {
		return s.GenerateFunc(ctx, call)
	}
	return &fantasy.Response{
		Content: fantasy.ResponseContent{
			fantasy.TextContent{Text: s.Response},
		},
	}, nil
}

// Stream calls StreamFunc if set; otherwise returns nil, nil.
func (s *StubLanguageModel) Stream(
	ctx context.Context, call fantasy.Call,
) (fantasy.StreamResponse, error) {
	if s.StreamFunc != nil {
		return s.StreamFunc(ctx, call)
	}
	return nil, nil
}

// GenerateObject calls GenerateObjFunc if set; otherwise returns an empty response.
func (s *StubLanguageModel) GenerateObject(
	ctx context.Context, call fantasy.ObjectCall,
) (*fantasy.ObjectResponse, error) {
	if s.GenerateObjFunc != nil {
		return s.GenerateObjFunc(ctx, call)
	}
	return &fantasy.ObjectResponse{}, nil
}

// StreamObject calls StreamObjFunc if set; otherwise returns nil, nil.
func (s *StubLanguageModel) StreamObject(
	ctx context.Context, call fantasy.ObjectCall,
) (fantasy.ObjectStreamResponse, error) {
	if s.StreamObjFunc != nil {
		return s.StreamObjFunc(ctx, call)
	}
	return nil, nil
}

// Provider returns the configured provider string.
func (s *StubLanguageModel) Provider() string { return s.ProviderVal }

// Model returns the configured model string.
func (s *StubLanguageModel) Model() string { return s.ModelVal }

// CallCount returns the number of times Generate has been called.
func (s *StubLanguageModel) CallCount() int64 { return s.callCount.Load() }

// StubOption is a functional option for NewStubLM.
type StubOption func(*StubLanguageModel)

// WithGenerateFunc sets a custom Generate implementation.
func WithGenerateFunc(
	fn func(context.Context, fantasy.Call) (*fantasy.Response, error),
) StubOption {
	return func(s *StubLanguageModel) { s.GenerateFunc = fn }
}

// WithStreamFunc sets a custom Stream implementation.
func WithStreamFunc(
	fn func(context.Context, fantasy.Call) (fantasy.StreamResponse, error),
) StubOption {
	return func(s *StubLanguageModel) { s.StreamFunc = fn }
}

// WithProvider sets the provider string.
func WithProvider(p string) StubOption {
	return func(s *StubLanguageModel) { s.ProviderVal = p }
}

// WithModel sets the model string.
func WithModel(m string) StubOption {
	return func(s *StubLanguageModel) { s.ModelVal = m }
}

// WithResponse sets a canned text response for Generate.
func WithResponse(r string) StubOption {
	return func(s *StubLanguageModel) { s.Response = r }
}

// NewStubLM creates a StubLanguageModel with the given options.
func NewStubLM(opts ...StubOption) *StubLanguageModel {
	s := &StubLanguageModel{}
	for _, o := range opts {
		o(s)
	}
	return s
}
