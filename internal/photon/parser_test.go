package photon

import (
	"bytes"
	"encoding/binary"
	"testing"
)

type captureHandler struct {
	events    []Event
	requests  []OperationRequest
	responses []OperationResponse
}

func (h *captureHandler) OnEvent(e Event)                { h.events = append(h.events, e) }
func (h *captureHandler) OnRequest(r OperationRequest)   { h.requests = append(h.requests, r) }
func (h *captureHandler) OnResponse(r OperationResponse) { h.responses = append(h.responses, r) }

func TestProtocol18DeserializePrimitives(t *testing.T) {
	tests := []struct {
		name string
		tc   byte
		data []byte
		want interface{}
	}{
		{"true literal", typeBoolTrue, nil, true},
		{"false literal", typeBoolFalse, nil, false},
		{"byte", typeByte, []byte{7}, byte(7)},
		{"int1", typeInt1, []byte{42}, int32(42)},
		{"int2", typeInt2, []byte{0x34, 0x12}, int32(0x1234)},
		{"compressed int", typeCompressedInt, encodeVarint(2468 << 1), int32(2468)},
		{"string", typeString, append([]byte{5}, []byte("Hello")...), "Hello"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := deserialize(bytes.NewBuffer(tt.data), tt.tc)
			if got != tt.want {
				t.Fatalf("got %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestReadParameterTableProtocol18(t *testing.T) {
	var buf bytes.Buffer
	buf.Write(encodeVarint(2))
	buf.WriteByte(1)
	buf.WriteByte(typeByte)
	buf.WriteByte(7)
	buf.WriteByte(252)
	buf.WriteByte(typeCompressedInt)
	buf.Write(encodeVarint(277 << 1))

	params := readParameterTable(&buf)
	if params[1].(byte) != 7 {
		t.Fatalf("param 1: got %v", params[1])
	}
	if params[252].(int32) != 277 {
		t.Fatalf("param 252: got %v", params[252])
	}
}

func TestParseEventPacketUsesParam252Code(t *testing.T) {
	h := &captureHandler{}
	p := NewParser(h)

	params := map[byte]testValue{
		1:   {tc: typeString, data: append([]byte{7}, []byte("MobBody")...)},
		2:   {tc: typeString, data: append([]byte{9}, []byte("PlayerOne")...)},
		3:   {tc: typeBoolFalse},
		4:   {tc: typeCompressedInt, data: encodeVarint(1841 << 1)},
		5:   {tc: typeCompressedInt, data: encodeVarint(3 << 1)},
		252: {tc: typeCompressedInt, data: encodeVarint(277 << 1)},
	}
	p.Parse(buildEventPacket(0, params))

	if len(h.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(h.events))
	}
	ev := h.events[0]
	if ev.Code != 277 {
		t.Fatalf("event code: got %d, want 277", ev.Code)
	}
	if ev.Parameters[2].(string) != "PlayerOne" {
		t.Fatalf("looter param: got %v", ev.Parameters[2])
	}
	if ev.Parameters[4].(int32) != 1841 {
		t.Fatalf("item param: got %v", ev.Parameters[4])
	}
}

func TestParseGarbageIsSafe(t *testing.T) {
	h := &captureHandler{}
	p := NewParser(h)
	for _, b := range [][]byte{
		nil,
		{0x00},
		{0xff, 0xff, 0xff},
		bytes.Repeat([]byte{0xab}, 50),
	} {
		p.Parse(b)
	}
	if len(h.events) != 0 {
		t.Fatalf("garbage produced %d events", len(h.events))
	}
}

func TestParseMalformedEventArrayDoesNotPanic(t *testing.T) {
	h := &captureHandler{}
	p := NewParser(h)

	var payload bytes.Buffer
	payload.WriteByte(0) // compact event code, ignored when params[252] exists
	payload.Write(encodeVarint(1))
	payload.WriteByte(1)
	payload.WriteByte(typeArray)
	// Huge compressed count previously risked massive allocation / panic.
	payload.Write([]byte{0xff, 0xff, 0xff, 0xff, 0x0f})
	payload.WriteByte(typeByte)

	p.Parse(wrapReliableMessage(msgEvent, payload.Bytes()))
	if len(h.events) != 1 {
		// The malformed array is clamped and decoded as best-effort; the important
		// assertion is no panic. One event is acceptable here.
		return
	}
}

type testValue struct {
	tc   byte
	data []byte
}

func buildEventPacket(compactCode byte, params map[byte]testValue) []byte {
	var data bytes.Buffer
	data.WriteByte(compactCode)
	data.Write(encodeVarint(uint32(len(params))))
	for k, v := range params {
		data.WriteByte(k)
		data.WriteByte(v.tc)
		data.Write(v.data)
	}
	return wrapReliableMessage(msgEvent, data.Bytes())
}

func wrapReliableMessage(msgType byte, data []byte) []byte {
	var msg bytes.Buffer
	msg.WriteByte(0xF3)
	msg.WriteByte(msgType)
	msg.Write(data)
	body := msg.Bytes()

	var cmd bytes.Buffer
	cmd.WriteByte(cmdSendReliable)
	cmd.WriteByte(0)
	cmd.WriteByte(0)
	cmd.WriteByte(0)
	binary.Write(&cmd, binary.BigEndian, int32(12+len(body)))
	binary.Write(&cmd, binary.BigEndian, int32(1))
	cmd.Write(body)

	var pkt bytes.Buffer
	binary.Write(&pkt, binary.BigEndian, uint16(0))
	pkt.WriteByte(0)
	pkt.WriteByte(1)
	binary.Write(&pkt, binary.BigEndian, uint32(0))
	binary.Write(&pkt, binary.BigEndian, int32(0))
	pkt.Write(cmd.Bytes())
	return pkt.Bytes()
}

func encodeVarint(v uint32) []byte {
	var out []byte
	for v >= 0x80 {
		out = append(out, byte(v)|0x80)
		v >>= 7
	}
	out = append(out, byte(v))
	return out
}
