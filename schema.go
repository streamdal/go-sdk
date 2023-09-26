package snitch

import (
	"context"

	"github.com/streamdal/snitch-protos/build/go/protos"
)

func (s *Snitch) getSchema(_ context.Context, aud *protos.Audience) []byte {
	s.schemasMtx.RLock()
	defer s.schemasMtx.RUnlock()

	schema, ok := s.schemas[audToStr(aud)]
	if !ok {
		return []byte(``)
	}

	return schema.JsonSchema
}

func (s *Snitch) setSchema(_ context.Context, aud *protos.Audience, schema []byte) {
	s.schemasMtx.Lock()
	defer s.schemasMtx.Unlock()

	s.schemas[audToStr(aud)] = &protos.Schema{
		JsonSchema: schema,
	}
}

// handleSchema will handle the schema step in the pipeline, if necessary
func (s *Snitch) handleSchema(ctx context.Context, aud *protos.Audience, step *protos.PipelineStep, resp *protos.WASMResponse) {
	inferSchema := step.GetInferSchema()

	if inferSchema == nil {
		// nothing to do
		return
	}
	if resp.ExitCode != protos.WASMExitCode_WASM_EXIT_CODE_SUCCESS {
		return
	}
	// Get existing schema for audience
	existingSchema := s.getSchema(ctx, aud)

	if string(resp.OutputStep) == string(existingSchema) {
		// Schema matches what we have in memory, noting to do
		return
	}

	// Schema is new or modified, update in memory and send to snitch server
	s.setSchema(ctx, aud, resp.OutputStep)

	go func() {
		err := s.serverClient.SendSchema(ctx, aud, resp.OutputStep)
		if err != nil {
			s.config.Logger.Errorf("failed to send schema: %s", err)
		}

		s.config.Logger.Debugf("published schema for audience '%s'", audToStr(aud))
	}()
}