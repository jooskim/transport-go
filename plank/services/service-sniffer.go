package services

import (
	"errors"
	"github.com/vmware/transport-go/bus"
	"github.com/vmware/transport-go/model"
	"github.com/vmware/transport-go/service"
	"net/http"
	"sync"
	"sync/atomic"
)

const ServiceSnifferChannel = "service-sniffer"

func NewServiceSniffer() *ServiceSniffer {
	return &ServiceSniffer{
		watchChannelsMap: make(map[string]*ServiceActivityTracker),
		activeMsgHandlers: make(map[string][]bus.MessageHandler),
	}
}

type ServiceSniffer struct {
	core service.FabricServiceCore
	watchChannelsMap map[string]*ServiceActivityTracker
	activeMsgHandlers map[string][]bus.MessageHandler
	mu sync.Mutex
}

type ServiceActivityTracker struct {
	bus bus.EventBus
	channel string
	requestCount int64
	responseCount int64
}

func (at *ServiceActivityTracker) IncrementRequest() {
	newCount := atomic.AddInt64(&at.requestCount, 1)
	at.BroadcastUpdate("request", newCount)
}

func (at *ServiceActivityTracker) IncrementResponse() {
	newCount := atomic.AddInt64(&at.responseCount, 1)
	at.BroadcastUpdate("response", newCount)
}

func (at *ServiceActivityTracker) BroadcastUpdate(typ string, count int64) {
	_ = at.bus.SendResponseMessage(ServiceSnifferChannel, map[string]interface{}{
		typ: count,
		"channel": at.channel,
	}, nil)
}

func (s *ServiceSniffer) Init(core service.FabricServiceCore) error {
	s.core = core
	return nil
}

func (s *ServiceSniffer) HandleServiceRequest(request *model.Request, core service.FabricServiceCore) {
	switch request.Request {
	case "watch":
		chansToWatch := request.Payload.([]interface{})
		for _, channel := range chansToWatch {
			err := s.watchServiceActivity(channel.(string))
			if err != nil {
				core.SendErrorResponse(request, 400, err.Error())
			}
		}
		break
	case "stop_watch":
		chansToStopWatch := request.Payload.([]interface{})
		for _, channel := range chansToStopWatch {
			err := s.stopWatchingServiceActivity(channel.(string))
			if err != nil {
				core.SendErrorResponse(request, 400, err.Error())
			}
		}
		break
	default:
		core.SendErrorResponse(request, http.StatusBadRequest, "unknown request")
	}
}

func (s *ServiceSniffer) watchServiceActivity(serviceChan string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, exists := s.watchChannelsMap[serviceChan]
	if exists {
		return errors.New(serviceChan + " already being watched")
	}

	// set up ListenRequestStream for serviceChan
	reqHandler, err := s.core.Bus().ListenRequestStream(serviceChan)
	if err != nil {
		return err
	}

	// set up ListenStream for serviceChan
	respHandler, err := s.core.Bus().ListenStream(serviceChan)
	if err != nil {
		return err
	}

	// register the handler to be called when unwatch command is requested
	_, exists = s.activeMsgHandlers[serviceChan]
	if exists {
		return errors.New(serviceChan + " already has active handlers registered. this is a bug!")
	}

	// register a new ServiceActivityTracker
	activityTracker := &ServiceActivityTracker{channel: serviceChan, bus: s.core.Bus()}
	s.watchChannelsMap[serviceChan] = activityTracker

	handlersSlice := make([]bus.MessageHandler, 0)

	reqHandler.Handle(func(msg *model.Message) {
		activityTracker.IncrementRequest()
	}, func(err error) {})

	respHandler.Handle(func(msg *model.Message) {
		activityTracker.IncrementResponse()
	}, func(err error) {})

	handlersSlice = append(handlersSlice, reqHandler, respHandler)
	s.activeMsgHandlers[serviceChan] = handlersSlice

	return nil
}

func (s *ServiceSniffer) stopWatchingServiceActivity(serviceChan string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// look up handlers and close them
	handlers, exists := s.activeMsgHandlers[serviceChan]
	if !exists {
		return errors.New("no active handler found for channel " + serviceChan)
	}

	for _, handler := range handlers {
		handler.Close()
	}

	delete(s.activeMsgHandlers, serviceChan)
	delete(s.watchChannelsMap, serviceChan)
	return nil
}