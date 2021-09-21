package server

import (
	"encoding/json"
	"fmt"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/vmware/transport-go/bus"
	"github.com/vmware/transport-go/model"
	"github.com/vmware/transport-go/service"
	"net/http"
	"testing"
	"time"
)

func TestBuildEndpointHandler_Timeout(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")
	assert.HTTPBodyContains(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		return model.Request{
			Payload: nil,
			Request: "test-request",
		}
	}, 5*time.Millisecond), "GET", "http://localhost", nil, "request timed out")
}

func TestBuildEndpointHandler_ChanResponseErr(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")
	assert.HTTPErrorf(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		uId := &uuid.UUID{}
		_ = b.SendErrorMessage("test-chan", fmt.Errorf("test error"), uId)
		return model.Request{
			Id:      uId,
			Payload: nil,
			Request: "test-request",
		}
	}, 5*time.Second), "GET", "http://localhost", nil, "test error")
}

func TestBuildEndpointHandler_SuccessResponse(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")
	assert.HTTPBodyContains(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		uId := &uuid.UUID{}
		_ = b.SendResponseMessage("test-chan", &model.Response{
			Id:      uId,
			Payload: []byte("{\"error\": false}"),
		}, uId)
		return model.Request{
			Id:      uId,
			Payload: nil,
			Request: "test-request",
		}
	}, 5*time.Second), "GET", "http://localhost", nil, "{\"error\": false}")
}

func TestBuildEndpointHandler_ErrorResponse(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")

	uId := &uuid.UUID{}
	rsp := &model.Response{
		Id:        uId,
		Payload:   "{\"error\": true}",
		ErrorCode: 500,
		Error:     true,
	}
	expected, _ := json.Marshal(rsp.Payload)

	assert.HTTPBodyContains(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		_ = b.SendResponseMessage("test-chan", rsp, uId)
		return model.Request{
			Id:      uId,
			Payload: nil,
			Request: "test-request",
		}

	}, 5*time.Second), "GET", "http://localhost", nil, string(expected))
}

func TestBuildEndpointHandler_ErrorResponseAlternative(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")

	uId := &uuid.UUID{}
	rsp := &model.Response{
		Id:        uId,
		ErrorCode: 418,
		Error:     true,
	}

	assert.HTTPBodyContains(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		_ = b.SendResponseMessage("test-chan", rsp, uId)
		return model.Request{
			Id:      uId,
			Payload: nil,
			Request: "test-request",
		}

	}, 5*time.Second), "GET", "http://localhost", nil, "418")
}

func TestBuildEndpointHandler_CatchPanic(t *testing.T) {
	b := bus.ResetBus()
	service.ResetServiceRegistry()
	_ = b.GetChannelManager().CreateChannel("test-chan")
	assert.HTTPBodyContains(t, buildEndpointHandler("test-chan", func(w http.ResponseWriter, r *http.Request) model.Request {
		panic("peekaboo")
		return model.Request{
			Payload: nil,
			Request: "test-request",
		}
	}, 5*time.Second), "GET", "http://localhost", nil, "Internal Server Error")
}
