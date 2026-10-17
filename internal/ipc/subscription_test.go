package ipc

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/TIANLI0/THRM/internal/appmeta"
	"github.com/TIANLI0/THRM/internal/types"
)

// TestAppendEventFrameMatchesJSONMarshal 保证手工拼接的事件帧与此前
// json.Marshal(Event{...}) 的输出逐字节一致，线上协议不因此发生任何变化。
func TestAppendEventFrameMatchesJSONMarshal(t *testing.T) {
	payloads := []any{
		nil,
		"plain",
		"needs <escaping> & \"quotes\" \\ 中文  ",
		int64(1712345678901),
		map[string]any{"connected": true, "model": "BS2PRO", "deviceSettings": map[string]any{"a": 1.5}},
		types.FanData{CurrentRPM: 2400, TargetRPM: 2500, WorkMode: "实时转速模式"},
		types.TemperatureData{CPUTemp: 61, CpuSensors: []types.TemperatureSensor{{Key: "cpu/0", Name: "Core <0>", Value: 61}}, OtherSensors: []types.TemperatureSensor{}},
		json.RawMessage(`{"pre":"encoded"}`),
	}
	eventTypes := []string{EventFanDataUpdate, EventTemperatureUpdate, "show-window", "odd\"type<>"}
	for _, eventType := range eventTypes {
		for _, payload := range payloads {
			data, err := json.Marshal(payload)
			if err != nil {
				t.Fatalf("marshal payload: %v", err)
			}
			id := newMessageID("evt")
			ts := time.Now().UnixMilli()
			want, err := json.Marshal(Event{
				SchemaVersion: appmeta.ProtocolVersion,
				EventID:       id,
				Timestamp:     ts,
				Source:        "core",
				IsEvent:       true,
				Type:          eventType,
				Data:          data,
			})
			if err != nil {
				t.Fatalf("marshal event: %v", err)
			}
			want = append(want, '\n')
			got := appendEventFrame(nil, id, ts, eventType, data)
			if !bytes.Equal(got, want) {
				t.Fatalf("event frame mismatch for type %q payload %T\n got: %s\nwant: %s", eventType, payload, got, want)
			}
		}
	}
}

func TestClientDecodesResponsesAndEventsInOnePass(t *testing.T) {
	var msg inboundMessage
	line := []byte(`{"protocolVersion":"3.0","requestId":"req-1","timestamp":5,"isResponse":true,"success":false,"errorCode":"x","error":"boom","data":{"k":1}}`)
	if err := json.Unmarshal(line, &msg); err != nil {
		t.Fatal(err)
	}
	var want Response
	if err := json.Unmarshal(line, &want); err != nil {
		t.Fatal(err)
	}
	got := msg.response()
	if got.ProtocolVersion != want.ProtocolVersion || got.RequestID != want.RequestID || got.Timestamp != want.Timestamp ||
		got.IsResponse != want.IsResponse || got.Success != want.Success || got.ErrorCode != want.ErrorCode ||
		got.Error != want.Error || !bytes.Equal(got.Data, want.Data) {
		t.Fatalf("response mismatch: got %+v want %+v", got, want)
	}

	msg = inboundMessage{}
	line = appendEventFrame(nil, "evt-1", 7, EventConfigUpdate, []byte(`{"autoControl":true}`))
	if err := json.Unmarshal(line, &msg); err != nil {
		t.Fatal(err)
	}
	var wantEvent Event
	if err := json.Unmarshal(line, &wantEvent); err != nil {
		t.Fatal(err)
	}
	gotEvent := msg.event()
	if gotEvent.SchemaVersion != wantEvent.SchemaVersion || gotEvent.EventID != wantEvent.EventID || gotEvent.Timestamp != wantEvent.Timestamp ||
		gotEvent.Source != wantEvent.Source || gotEvent.IsEvent != wantEvent.IsEvent || gotEvent.Type != wantEvent.Type ||
		!bytes.Equal(gotEvent.Data, wantEvent.Data) {
		t.Fatalf("event mismatch: got %+v want %+v", gotEvent, wantEvent)
	}
}

func waitFor(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// TestUnsubscribedClientSkipsHighFrequencyEvents 覆盖窗口隐藏的 GUI：退订后核心不再
// 把它算作前台客户端，也不再给它发风扇/温度遥测，但状态类事件照常送达。
func TestUnsubscribedClientSkipsHighFrequencyEvents(t *testing.T) {
	s := NewServer(func(req Request) Response {
		return Response{Success: false, Error: "handler must not see subscription requests"}
	}, testLogger{})
	if err := s.Start(); err != nil {
		t.Fatalf("Start error: %v", err)
	}
	defer s.Stop()

	c := NewClient(testLogger{})
	if err := c.Connect(); err != nil {
		t.Fatalf("Connect error: %v", err)
	}
	defer c.Close()

	received := make(chan Event, 16)
	c.SetEventHandler(func(ev Event) { received <- ev })

	waitFor(t, "client registration", s.HasClients)
	// 新连接默认订阅，兼容从不发订阅请求的旧 GUI。
	if !s.HasActiveClients() {
		t.Fatal("new connections must start subscribed")
	}
	select {
	case <-s.ActiveClientsChanged():
	case <-time.After(time.Second):
		t.Fatal("a new connection must signal ActiveClientsChanged")
	}

	resp, err := c.SendRequest(ReqUnsubscribeEvents, nil)
	if err != nil || !resp.Success {
		t.Fatalf("unsubscribe failed: resp=%+v err=%v", resp, err)
	}
	if s.HasActiveClients() {
		t.Fatal("unsubscribed client must not count as active")
	}
	if !s.HasClients() {
		t.Fatal("unsubscribed client is still connected")
	}

	s.BroadcastEvent(EventFanDataUpdate, types.FanData{CurrentRPM: 1})
	s.BroadcastEvent(EventConfigUpdate, map[string]bool{"autoControl": true})
	select {
	case ev := <-received:
		if ev.Type != EventConfigUpdate {
			t.Fatalf("unsubscribed client received %q, want only %q", ev.Type, EventConfigUpdate)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("low-frequency event was not delivered to an unsubscribed client")
	}

	resp, err = c.SendRequest(ReqSubscribeEvents, SubscribeEventsParams{Features: []string{FeatureTemperatureSensorValues, "unknown-future-feature"}})
	if err != nil || !resp.Success {
		t.Fatalf("subscribe failed: resp=%+v err=%v", resp, err)
	}
	if !s.HasActiveClients() {
		t.Fatal("resubscribed client must count as active")
	}
	select {
	case <-s.ActiveClientsChanged():
	default:
		t.Fatal("resubscribing must signal ActiveClientsChanged")
	}
	if !s.ActiveClientsSupport(FeatureTemperatureSensorValues) {
		t.Fatal("declared feature not recorded")
	}

	s.BroadcastEvent(EventTemperatureUpdate, types.TemperatureData{CPUTemp: 50})
	select {
	case ev := <-received:
		if ev.Type != EventTemperatureUpdate {
			t.Fatalf("got %q, want %q", ev.Type, EventTemperatureUpdate)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("high-frequency event not delivered after resubscribing")
	}

	// 再连上一个不声明能力的旧客户端：共用编码必须退回旧格式。
	legacy := NewClient(testLogger{})
	if err := legacy.Connect(); err != nil {
		t.Fatalf("legacy Connect error: %v", err)
	}
	defer legacy.Close()
	waitFor(t, "legacy client registration", func() bool { return !s.ActiveClientsSupport(FeatureTemperatureSensorValues) })
	// 旧客户端退订后（例如它也被隐藏），剩下的前台客户端都支持新格式。
	if resp, err := legacy.SendRequest(ReqUnsubscribeEvents, nil); err != nil || !resp.Success {
		t.Fatalf("legacy unsubscribe failed: resp=%+v err=%v", resp, err)
	}
	if !s.ActiveClientsSupport(FeatureTemperatureSensorValues) {
		t.Fatal("only subscribed clients should decide the encoding")
	}
}

func TestActiveClientsSupportWithoutClients(t *testing.T) {
	s := NewServer(func(Request) Response { return Response{Success: true} }, testLogger{})
	if s.HasActiveClients() {
		t.Fatal("no clients yet")
	}
	if s.ActiveClientsSupport(FeatureTemperatureSensorValues) {
		t.Fatal("no active client means nobody to encode for")
	}
	if s.ActiveClientsSupport("unknown") {
		t.Fatal("unknown features are never supported")
	}
}
