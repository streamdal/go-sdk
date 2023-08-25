package snitch

import (
	"context"
	"errors"
	"io"
	"runtime"
	"strings"
	"time"

	"github.com/relistan/go-director"

	"github.com/streamdal/snitch-protos/build/go/protos"
)

func (s *Snitch) register(looper director.Looper) {
	req := &protos.RegisterRequest{
		ServiceName: s.config.ServiceName,
		SessionId:   s.sessionID,
		ClientInfo: &protos.ClientInfo{
			ClientType:     protos.ClientType(s.config.ClientType),
			LibraryName:    "snitch-go-client",
			LibraryVersion: "0.0.1", // TODO: inject via build tag
			Language:       "go",
			Arch:           runtime.GOARCH,
			Os:             runtime.GOOS,
		},
		Audiences: make([]*protos.Audience, 0),
		DryRun:    s.config.DryRun,
	}

	for _, aud := range s.config.Audiences {
		req.Audiences = append(req.Audiences, aud.ToProto())
	}

	var stream protos.Internal_RegisterClient
	var err error
	var quit bool

	srv, err := s.serverClient.Register(s.config.ShutdownCtx, req)
	if err != nil && !strings.Contains(err.Error(), context.Canceled.Error()) {
		panic("Failed to register with snitch server: " + err.Error())
	}
	stream = srv

	looper.Loop(func() error {
		if quit {
			time.Sleep(time.Millisecond * 100)
			return nil
		}

		if stream == nil {
			newStream, err := s.serverClient.Register(s.config.ShutdownCtx, req)
			if err != nil {
				if strings.Contains(err.Error(), context.Canceled.Error()) {
					s.config.Logger.Debug("context cancelled during connect")
					quit = true
					looper.Quit()
					return nil
				}

				s.config.Logger.Errorf("Failed to reconnect with snitch server: %s, retrying in '%s'", err, ReconnectSleep.String())
				time.Sleep(ReconnectSleep)
				return nil
			}

			stream = newStream
		}

		// Blocks until something is received
		cmd, err := stream.Recv()
		if err != nil {
			if err.Error() == "rpc error: code = Canceled desc = context canceled" {
				s.config.Logger.Errorf("context cancelled during recv: %s", err)
				quit = true
				looper.Quit()
				return nil
			}

			if errors.Is(err, io.EOF) {
				// Nicer reconnect messages
				stream = nil
				s.config.Logger.Warnf("snitch server is unavailable, retrying in %s...", ReconnectSleep.String())
				time.Sleep(ReconnectSleep)
			} else {
				s.config.Logger.Warnf("Error receiving message, retrying in %s: %s", ReconnectSleep.String(), err)
				time.Sleep(ReconnectSleep)
			}

			return nil

		}

		if cmd.GetKeepAlive() != nil {
			s.config.Logger.Debug("Received keep alive")
			return nil
		}

		if cmd.Audience.ServiceName != s.config.ServiceName {
			s.config.Logger.Debugf("Received command for different service name: %s, ignoring command", cmd.Audience.ServiceName)
			return nil
		}

		if attach := cmd.GetAttachPipeline(); attach != nil {
			s.config.Logger.Debugf("Received attach pipeline command: %s", attach.Pipeline.Id)
			if err := s.attachPipeline(context.Background(), cmd); err != nil {
				s.config.Logger.Errorf("Failed to attach pipeline: %s", err)
				return nil
			}
		} else if detach := cmd.GetDetachPipeline(); detach != nil {
			s.config.Logger.Debugf("Received detach pipeline command: %s", detach.PipelineId)
			if err := s.detachPipeline(context.Background(), cmd); err != nil {
				s.config.Logger.Errorf("Failed to detach pipeline: %s", err)
				return nil
			}
		} else if pause := cmd.GetPausePipeline(); pause != nil {
			s.config.Logger.Debugf("Received pause pipeline command: %s", pause.PipelineId)
			if err := s.pausePipeline(context.Background(), cmd); err != nil {
				s.config.Logger.Errorf("Failed to pause pipeline: %s", err)
				return nil
			}
		} else if resume := cmd.GetResumePipeline(); resume != nil {
			s.config.Logger.Debugf("Received resume pipeline command: %s", resume.PipelineId)
			if err := s.resumePipeline(context.Background(), cmd); err != nil {
				s.config.Logger.Errorf("Failed to resume pipeline: %s", err)
				return nil
			}
		}

		return nil
	})
}

func (s *Snitch) attachPipeline(_ context.Context, cmd *protos.Command) error {
	if cmd == nil {
		return ErrEmptyCommand
	}

	s.pipelinesMtx.Lock()
	defer s.pipelinesMtx.Unlock()

	if _, ok := s.pipelines[audToStr(cmd.Audience)]; !ok {
		s.pipelines[audToStr(cmd.Audience)] = make(map[string]*protos.Command)
	}

	s.pipelines[audToStr(cmd.Audience)][cmd.GetAttachPipeline().Pipeline.Id] = cmd

	s.config.Logger.Debugf("Attached pipeline %s", cmd.GetAttachPipeline().Pipeline.Id)

	return nil
}

func (s *Snitch) detachPipeline(_ context.Context, cmd *protos.Command) error {
	if cmd == nil {
		return ErrEmptyCommand
	}

	s.pipelinesMtx.Lock()
	defer s.pipelinesMtx.Unlock()

	if _, ok := s.pipelines[audToStr(cmd.Audience)]; !ok {
		return nil
	}

	delete(s.pipelines[audToStr(cmd.Audience)], cmd.GetDetachPipeline().PipelineId)

	s.config.Logger.Debugf("Detached pipeline %s", cmd.GetDetachPipeline().PipelineId)

	return nil
}

func (s *Snitch) pausePipeline(_ context.Context, cmd *protos.Command) error {
	if cmd == nil {
		return ErrEmptyCommand
	}

	s.pipelinesMtx.Lock()
	defer s.pipelinesMtx.Unlock()
	s.pipelinesPausedMtx.Lock()
	defer s.pipelinesPausedMtx.Unlock()

	if _, ok := s.pipelines[audToStr(cmd.Audience)]; !ok {
		return errors.New("pipeline not active or does not exist")
	}

	pipeline, ok := s.pipelines[audToStr(cmd.Audience)][cmd.GetPausePipeline().PipelineId]
	if !ok {
		return errors.New("pipeline not active or does not exist")
	}

	if _, ok := s.pipelinesPaused[audToStr(cmd.Audience)]; !ok {
		s.pipelinesPaused[audToStr(cmd.Audience)] = make(map[string]*protos.Command)
	}

	s.pipelinesPaused[audToStr(cmd.Audience)][cmd.GetPausePipeline().PipelineId] = pipeline

	delete(s.pipelines[audToStr(cmd.Audience)], cmd.GetPausePipeline().PipelineId)

	return nil
}

func (s *Snitch) resumePipeline(_ context.Context, cmd *protos.Command) error {
	if cmd == nil {
		return ErrEmptyCommand
	}

	s.pipelinesMtx.Lock()
	defer s.pipelinesMtx.Unlock()
	s.pipelinesPausedMtx.Lock()
	defer s.pipelinesPausedMtx.Unlock()

	if _, ok := s.pipelinesPaused[audToStr(cmd.Audience)]; !ok {
		return errors.New("pipeline not paused")
	}

	pipeline, ok := s.pipelinesPaused[audToStr(cmd.Audience)][cmd.GetResumePipeline().PipelineId]
	if !ok {
		return errors.New("pipeline not paused")
	}

	if _, ok := s.pipelines[audToStr(cmd.Audience)]; !ok {
		s.pipelines[audToStr(cmd.Audience)] = make(map[string]*protos.Command)
	}

	s.pipelines[audToStr(cmd.Audience)][cmd.GetResumePipeline().PipelineId] = pipeline

	delete(s.pipelinesPaused[audToStr(cmd.Audience)], cmd.GetResumePipeline().PipelineId)

	return nil
}