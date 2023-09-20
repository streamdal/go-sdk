package snitch

import (
	"sync"
	"testing"

	"github.com/streamdal/snitch-go-client/logger/loggerfakes"
	"github.com/streamdal/snitch-protos/build/go/protos"
)

func TestSetTailing(t *testing.T) {
	s := &Snitch{
		tailsMtx: &sync.RWMutex{},
		tails:    make(map[string]map[string]*Tail),
		config: &Config{
			Logger: &loggerfakes.FakeLogger{},
		},
	}

	aud := &protos.Audience{
		OperationName: "test-operation",
		ServiceName:   "test-service",
		OperationType: protos.OperationType_OPERATION_TYPE_PRODUCER,
		ComponentName: "test-component",
	}

	tail := &Tail{
		Request: &protos.Command{
			Command: &protos.Command_Tail{
				Tail: &protos.TailCommand{
					Request: &protos.TailRequest{
						Id:       "test-tail",
						Audience: aud,
					},
				},
			},
		},
	}

	s.setTailing(tail)

	if len(s.tails) != 1 {
		t.Errorf("expected tails to have 1 item, got %d", len(s.tails))
	}
}

func TestSendTail(t *testing.T) {
	// Start fake gRPC server
	//
	//serverFake := &serverfakes.FakeIServerClient{}
	//
	//serverFake.GetTailStreamStub = func(context.Context) (protos.Internal_SendTailClient, error) {
	//	return &serverfakes.FakeStream, nil
	//}
	//
	//s := &Snitch{
	//	config: &Config{
	//		Logger: &loggerfakes.FakeLogger{},
	//	},
	//	tailsMtx:     &sync.RWMutex{},
	//	tails:        make(map[string]map[string]*Tail),
	//	serverClient: serverFake,
	//}
	//
	//tailID := uuid.New().String()
	//pipelineID := uuid.New().String()
	//
	//aud := &protos.Audience{
	//	ServiceName:   "test-service",
	//	ComponentName: "test-component",
	//	OperationType: protos.OperationType_OPERATION_TYPE_CONSUMER,
	//	OperationName: "test-op",
	//}
	//
	//tail := &Tail{
	//	outboundCh: make(chan *protos.TailResponse, 1),
	//	Request: &protos.Command{
	//		Command: &protos.Command_Tail{
	//			Tail: &protos.TailCommand{
	//				Request: &protos.TailRequest{
	//					Id:         tailID,
	//					PipelineId: pipelineID,
	//					Audience:   aud,
	//				},
	//			},
	//		},
	//	},
	//}
	//
	//s.tails[audToStr(aud)] = map[string]*Tail{
	//	"test-tail": tail,
	//}
	//
	//_ = tail.startWorkers()
	//
	//s.sendTail(aud, pipelineID, []byte(`old`), []byte(`new`))
	//
	//if len(serverFake.SendCallCount()) != 1 {
	//	t.Errorf("expected SendTail to be called once, got %d", len(serverFake.SendTailCallCount()))
	//}
}
