package photon

import (
	"bytes"
	"encoding/binary"
	"sync"
	"sync/atomic"
)

// Message kinds emitted by the parser.
const (
	msgOperationRequest  byte = 2
	msgOperationResponse byte = 3
	msgEvent             byte = 4
	msgOperationRespAlt  byte = 7
	msgEncrypted         byte = 131
)

// Photon command types within a packet.
const (
	cmdAcknowledge     byte = 1
	cmdConnect         byte = 2
	cmdVerifyConnect   byte = 3
	cmdDisconnect      byte = 4
	cmdPing            byte = 5
	cmdSendReliable    byte = 6
	cmdSendUnreliable  byte = 7
	cmdSendFragment    byte = 8
	cmdSendUnsequenced byte = 9
)

// Event is a decoded Photon event: a numeric code plus its parameter table.
// Parameter 252 conventionally carries the event code itself.
type Event struct {
	Code       int16
	Parameters map[byte]interface{}
}

// OperationRequest / OperationResponse are decoded the same way; loot logging
// only needs Events, but requests/responses are exposed for completeness.
type OperationRequest struct {
	Code       int16
	Parameters map[byte]interface{}
}

type OperationResponse struct {
	Code         int16
	ReturnCode   int16
	DebugMessage interface{}
	Parameters   map[byte]interface{}
}

// Handler receives decoded messages. Implementations must be safe for calls
// from the single capture goroutine.
type Handler interface {
	OnEvent(Event)
	OnRequest(OperationRequest)
	OnResponse(OperationResponse)
}

// fragmentBuffer accumulates the pieces of a fragmented reliable message.
type fragmentBuffer struct {
	totalLength       int
	bytesWritten      int
	data              []byte
	fragmentsReceived int
	fragmentCount     int
}

// Parser is a stateful Photon packet parser. It reassembles fragmented
// messages across packets, so a single Parser instance must be used for one
// logical connection/capture stream. It is not safe for concurrent use; the
// capture layer calls Parse from a single goroutine.
type Parser struct {
	mu        sync.Mutex
	handler   Handler
	fragments map[int32]*fragmentBuffer // keyed by startSequenceNumber

	packets   atomic.Uint64
	messages  atomic.Uint64
	events    atomic.Uint64
	requests  atomic.Uint64
	responses atomic.Uint64
}

// Stats is a snapshot of parser activity.
type Stats struct {
	Packets   uint64
	Messages  uint64
	Events    uint64
	Requests  uint64
	Responses uint64
}

// NewParser returns a Parser dispatching decoded messages to h.
func NewParser(h Handler) *Parser {
	return &Parser{
		handler:   h,
		fragments: make(map[int32]*fragmentBuffer),
	}
}

// Stats returns parser activity counters for diagnostics.
func (p *Parser) Stats() Stats {
	return Stats{
		Packets:   p.packets.Load(),
		Messages:  p.messages.Load(),
		Events:    p.events.Load(),
		Requests:  p.requests.Load(),
		Responses: p.responses.Load(),
	}
}

// Parse decodes one UDP payload (a Photon packet). Malformed or non-Photon
// payloads are ignored without error; the capture layer pre-filters most of
// these via port + first-byte heuristics.
func (p *Parser) Parse(payload []byte) {
	p.packets.Add(1)
	p.mu.Lock()
	defer p.mu.Unlock()
	defer func() {
		// Packet sniffing sees retransmits, fragments, and sometimes traffic that
		// only looks like Photon. Malformed payloads must never terminate the
		// logger; drop the current packet and keep capturing.
		_ = recover()
	}()

	r := &reader{b: payload}

	// Photon header: peerId (2), flags (1), commandCount (1), timestamp (4),
	// challenge (4) = 12 bytes.
	if r.remaining() < 12 {
		return
	}
	if _, err := r.readUint16(); err != nil { // peerId
		return
	}
	flags, err := r.readByte() // crcEnabled flag
	if err != nil {
		return
	}
	commandCount, err := r.readByte()
	if err != nil {
		return
	}
	if _, err := r.readUint32(); err != nil { // timestamp
		return
	}
	if _, err := r.readInt32(); err != nil { // challenge
		return
	}
	_ = flags // CRC validation is not required for read-only sniffing.

	for i := 0; i < int(commandCount); i++ {
		if !p.parseCommand(r) {
			return
		}
	}
}

// parseCommand parses one command header + body. Returns false to abort the
// remaining commands in this packet (e.g. on a short buffer).
func (p *Parser) parseCommand(r *reader) bool {
	// Command header: type (1), channelId (1), flags (1), reserved (1),
	// length (4), reliableSequenceNumber (4) = 12 bytes.
	cmdType, err := r.readByte()
	if err != nil {
		return false
	}
	if _, err := r.readByte(); err != nil { // channelId
		return false
	}
	if _, err := r.readByte(); err != nil { // flags
		return false
	}
	if _, err := r.readByte(); err != nil { // reserved byte
		return false
	}
	length, err := r.readInt32()
	if err != nil {
		return false
	}
	seqNumber, err := r.readInt32()
	if err != nil {
		return false
	}

	// Command body length excludes the 12-byte command header.
	bodyLen := int(length) - 12
	if bodyLen < 0 || bodyLen > r.remaining() {
		return false
	}

	switch cmdType {
	case cmdSendReliable, cmdSendUnreliable:
		body, err := readCommandBody(r, cmdType, bodyLen)
		if err != nil {
			return false
		}
		p.handleMessage(body)
	case cmdSendFragment:
		if !p.handleFragment(r, bodyLen) {
			return false
		}
	default:
		// Acknowledge / Connect / Ping / Disconnect carry no Photon message
		// payload we care about; skip their bodies.
		if _, err := r.readBytes(bodyLen); err != nil {
			return false
		}
	}
	_ = seqNumber
	return true
}

// readCommandBody reads the message payload of a reliable/unreliable command.
// Unreliable commands have an extra 4-byte unreliable sequence number before
// the message.
func readCommandBody(r *reader, cmdType byte, bodyLen int) ([]byte, error) {
	if cmdType == cmdSendUnreliable {
		// Skip the 4-byte unreliable sequence number; it is part of bodyLen.
		if _, err := r.readInt32(); err != nil {
			return nil, err
		}
		bodyLen -= 4
	}
	if bodyLen < 0 {
		return nil, ErrShortBuffer
	}
	return r.readBytes(bodyLen)
}

// handleFragment reassembles SendFragment commands. The fragment header is:
// startSequenceNumber (4), fragmentCount (4), fragmentNumber (4),
// totalLength (4), fragmentOffset (4) = 20 bytes, then the fragment payload.
func (p *Parser) handleFragment(r *reader, bodyLen int) bool {
	if bodyLen < 20 {
		_, _ = r.readBytes(bodyLen)
		return true
	}
	startSeq, err := r.readInt32()
	if err != nil {
		return false
	}
	fragCount, err := r.readInt32()
	if err != nil {
		return false
	}
	if _, err := r.readInt32(); err != nil { // fragmentNumber
		return false
	}
	totalLen, err := r.readInt32()
	if err != nil {
		return false
	}
	fragOffset, err := r.readInt32()
	if err != nil {
		return false
	}
	payloadLen := bodyLen - 20
	frag, err := r.readBytes(payloadLen)
	if err != nil {
		return false
	}

	if totalLen < 0 || fragOffset < 0 || int(fragOffset)+payloadLen > int(totalLen) {
		return true // discard nonsensical fragment, keep parsing
	}

	buf, ok := p.fragments[startSeq]
	if !ok {
		buf = &fragmentBuffer{
			totalLength:   int(totalLen),
			data:          make([]byte, int(totalLen)),
			fragmentCount: int(fragCount),
		}
		p.fragments[startSeq] = buf
	}
	copy(buf.data[fragOffset:], frag)
	buf.bytesWritten += payloadLen
	buf.fragmentsReceived++

	if buf.fragmentsReceived >= buf.fragmentCount || buf.bytesWritten >= buf.totalLength {
		delete(p.fragments, startSeq)
		p.handleMessage(buf.data)
	}
	return true
}

// handleMessage decodes a reassembled Photon message and dispatches it.
func (p *Parser) handleMessage(body []byte) {
	if len(body) < 2 {
		return
	}
	mr := &reader{b: body}

	// Signature byte (0xF3 historically) then message type.
	if _, err := mr.readByte(); err != nil { // signature
		return
	}
	msgType, err := mr.readByte()
	if err != nil {
		return
	}

	switch msgType {
	case msgEvent:
		p.messages.Add(1)
		p.decodeEvent(body[mr.pos:])
	case msgOperationRequest:
		p.messages.Add(1)
		p.decodeRequest(body[mr.pos:])
	case msgOperationResponse, msgOperationRespAlt:
		p.messages.Add(1)
		p.decodeResponse(body[mr.pos:])
	case msgEncrypted:
		return
	}
}

func (p *Parser) decodeEvent(data []byte) {
	if len(data) < 1 {
		return
	}
	code := int16(data[0])
	params := deserializeParameterTable(data[1:])
	if realCode, ok := resolveCode(params[252]); ok {
		code = int16(realCode)
	}
	p.events.Add(1)
	p.handler.OnEvent(Event{Code: int16(code), Parameters: params})
}

func (p *Parser) decodeRequest(data []byte) {
	if len(data) < 1 {
		return
	}
	code := int16(data[0])
	params := deserializeParameterTable(data[1:])
	if realCode, ok := resolveCode(params[253]); ok {
		code = int16(realCode)
	}
	p.requests.Add(1)
	p.handler.OnRequest(OperationRequest{Code: int16(code), Parameters: params})
}

func (p *Parser) decodeResponse(data []byte) {
	if len(data) < 3 {
		return
	}
	code := int16(data[0])
	returnCode := int16(binary.LittleEndian.Uint16(data[1:3]))
	buf := bytes.NewBuffer(data[3:])
	var dbg interface{}
	if buf.Len() > 0 {
		dbgType, _ := buf.ReadByte()
		dbg = deserialize(buf, dbgType)
	}
	params := readParameterTable(buf)
	if realCode, ok := resolveCode(params[253]); ok {
		code = int16(realCode)
	}
	p.responses.Add(1)
	p.handler.OnResponse(OperationResponse{
		Code:         int16(code),
		ReturnCode:   returnCode,
		DebugMessage: dbg,
		Parameters:   params,
	})
}

func resolveCode(v interface{}) (uint16, bool) {
	switch n := v.(type) {
	case byte:
		return uint16(n), true
	case int16:
		return uint16(n), true
	case int32:
		return uint16(n), true
	case int64:
		return uint16(n), true
	case uint16:
		return n, true
	case uint32:
		return uint16(n), true
	case uint64:
		return uint16(n), true
	default:
		return 0, false
	}
}
