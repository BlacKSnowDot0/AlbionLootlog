// Protocol18 deserializer for Albion Online's current Photon payloads.
//
// This follows the maintained Go implementation in ao-data/albiondata-client,
// adapted to this package with defensive allocation bounds. The important
// difference from the earlier Protocol16 attempt is that parameter table counts
// and many primitive integers are compressed varints, and the real Albion event
// / operation ids are carried in params[252] / params[253].
package photon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"reflect"
)

var ErrShortBuffer = errors.New("photon: short buffer")

const maxCollectionElements = 1 << 20

// reader is a big-endian cursor used only for the Photon command envelope.
// The inner Protocol18 payload uses the bytes.Buffer helpers below.
type reader struct {
	b   []byte
	pos int
}

func (r *reader) remaining() int { return len(r.b) - r.pos }

func (r *reader) readByte() (byte, error) {
	if r.remaining() < 1 {
		return 0, ErrShortBuffer
	}
	v := r.b[r.pos]
	r.pos++
	return v, nil
}

func (r *reader) readInt32() (int32, error) {
	if r.remaining() < 4 {
		return 0, ErrShortBuffer
	}
	v := int32(binary.BigEndian.Uint32(r.b[r.pos:]))
	r.pos += 4
	return v, nil
}

func (r *reader) readUint16() (uint16, error) {
	if r.remaining() < 2 {
		return 0, ErrShortBuffer
	}
	v := binary.BigEndian.Uint16(r.b[r.pos:])
	r.pos += 2
	return v, nil
}

func (r *reader) readUint32() (uint32, error) {
	if r.remaining() < 4 {
		return 0, ErrShortBuffer
	}
	v := binary.BigEndian.Uint32(r.b[r.pos:])
	r.pos += 4
	return v, nil
}

func (r *reader) readBytes(n int) ([]byte, error) {
	if n < 0 || r.remaining() < n {
		return nil, ErrShortBuffer
	}
	out := r.b[r.pos : r.pos+n]
	r.pos += n
	return out, nil
}

// Protocol18 type codes.
const (
	typeUnknown          = byte(0)
	typeBoolean          = byte(2)
	typeByte             = byte(3)
	typeShort            = byte(4)
	typeFloat            = byte(5)
	typeDouble           = byte(6)
	typeString           = byte(7)
	typeNull             = byte(8)
	typeCompressedInt    = byte(9)
	typeCompressedLong   = byte(10)
	typeInt1             = byte(11)
	typeInt1Neg          = byte(12)
	typeInt2             = byte(13)
	typeInt2Neg          = byte(14)
	typeLong1            = byte(15)
	typeLong1Neg         = byte(16)
	typeLong2            = byte(17)
	typeLong2Neg         = byte(18)
	typeCustom           = byte(19)
	typeDictionary       = byte(20)
	typeHashtable        = byte(21)
	typeObjectArray      = byte(23)
	typeOperationRequest = byte(24)
	typeOperationResp    = byte(25)
	typeEventData        = byte(26)
	typeBoolFalse        = byte(27)
	typeBoolTrue         = byte(28)
	typeShortZero        = byte(29)
	typeIntZero          = byte(30)
	typeLongZero         = byte(31)
	typeFloatZero        = byte(32)
	typeDoubleZero       = byte(33)
	typeByteZero         = byte(34)
	typeArray            = byte(0x40)
	customTypeSlimBase   = byte(0x80)
)

func deserializeParameterTable(data []byte) map[byte]interface{} {
	return readParameterTable(bytes.NewBuffer(data))
}

func readParameterTable(buf *bytes.Buffer) map[byte]interface{} {
	count := boundedCount(readCount(buf), buf.Len()+1)
	params := make(map[byte]interface{}, count)
	for i := 0; i < count && buf.Len() > 0; i++ {
		key, err := buf.ReadByte()
		if err != nil {
			break
		}
		tc, err := buf.ReadByte()
		if err != nil {
			break
		}
		params[key] = deserialize(buf, tc)
	}
	return params
}

func deserialize(buf *bytes.Buffer, tc byte) interface{} {
	if tc >= customTypeSlimBase {
		return deserializeCustom(buf, tc)
	}
	switch tc {
	case typeUnknown, typeNull:
		return nil
	case typeBoolean:
		b, _ := buf.ReadByte()
		return b != 0
	case typeByte:
		b, _ := buf.ReadByte()
		return b
	case typeShort:
		return readInt16(buf)
	case typeFloat:
		return readFloat32(buf)
	case typeDouble:
		return readFloat64(buf)
	case typeString:
		return readString(buf)
	case typeCompressedInt:
		return readCompressedInt32(buf)
	case typeCompressedLong:
		return readCompressedInt64(buf)
	case typeInt1:
		b, _ := buf.ReadByte()
		return int32(b)
	case typeInt1Neg:
		b, _ := buf.ReadByte()
		return -int32(b)
	case typeInt2:
		return int32(readUint16(buf))
	case typeInt2Neg:
		return -int32(readUint16(buf))
	case typeLong1:
		b, _ := buf.ReadByte()
		return int64(b)
	case typeLong1Neg:
		b, _ := buf.ReadByte()
		return -int64(b)
	case typeLong2:
		return int64(readUint16(buf))
	case typeLong2Neg:
		return -int64(readUint16(buf))
	case typeCustom:
		return deserializeCustom(buf, 0)
	case typeDictionary:
		return deserializeDictionary(buf)
	case typeHashtable:
		return deserializeDictionary(buf)
	case typeObjectArray:
		return deserializeObjectArray(buf)
	case typeOperationRequest:
		return deserializeOperationRequestInner(buf)
	case typeOperationResp:
		return deserializeOperationResponseInner(buf)
	case typeEventData:
		return deserializeEventDataInner(buf)
	case typeBoolFalse:
		return false
	case typeBoolTrue:
		return true
	case typeShortZero:
		return int16(0)
	case typeIntZero:
		return int32(0)
	case typeLongZero:
		return int64(0)
	case typeFloatZero:
		return float32(0)
	case typeDoubleZero:
		return float64(0)
	case typeByteZero:
		return byte(0)
	case typeArray:
		return deserializeNestedArray(buf)
	default:
		if tc&typeArray == typeArray {
			return deserializeTypedArray(buf, tc&^typeArray)
		}
		return fmt.Sprintf("ERROR - unknown type 0x%02X", tc)
	}
}

func deserializeTypedArray(buf *bytes.Buffer, elemType byte) interface{} {
	size := boundedCount(readCount(buf), maxCollectionElements)
	switch elemType {
	case typeBoolean:
		result := make([]bool, size)
		packedBytes := (size + 7) / 8
		packed := make([]byte, min(packedBytes, buf.Len()))
		_, _ = buf.Read(packed)
		for i := 0; i < size && i/8 < len(packed); i++ {
			result[i] = (packed[i/8] & (1 << uint(i%8))) != 0
		}
		return result
	case typeByte:
		data := make([]byte, min(size, buf.Len()))
		_, _ = buf.Read(data)
		return data
	case typeShort:
		result := make([]int16, boundedCount(uint32(size), buf.Len()/2))
		for i := range result {
			result[i] = readInt16(buf)
		}
		return result
	case typeFloat:
		result := make([]float32, boundedCount(uint32(size), buf.Len()/4))
		for i := range result {
			result[i] = readFloat32(buf)
		}
		return result
	case typeDouble:
		result := make([]float64, boundedCount(uint32(size), buf.Len()/8))
		for i := range result {
			result[i] = readFloat64(buf)
		}
		return result
	case typeString:
		result := make([]string, size)
		for i := range result {
			result[i] = readString(buf)
		}
		return result
	case typeCompressedInt:
		result := make([]int32, size)
		for i := range result {
			result[i] = readCompressedInt32(buf)
		}
		return result
	case typeCompressedLong:
		result := make([]int64, size)
		for i := range result {
			result[i] = readCompressedInt64(buf)
		}
		return result
	default:
		result := make([]interface{}, size)
		for i := range result {
			result[i] = deserialize(buf, elemType)
		}
		return result
	}
}

func deserializeNestedArray(buf *bytes.Buffer) interface{} {
	size := boundedCount(readCount(buf), maxCollectionElements)
	tc, err := buf.ReadByte()
	if err != nil {
		return nil
	}
	result := make([]interface{}, size)
	for i := range result {
		result[i] = deserialize(buf, tc)
	}
	return result
}

func deserializeObjectArray(buf *bytes.Buffer) interface{} {
	size := boundedCount(readCount(buf), maxCollectionElements)
	result := make([]interface{}, size)
	for i := range result {
		tc, err := buf.ReadByte()
		if err != nil {
			break
		}
		result[i] = deserialize(buf, tc)
	}
	return result
}

func deserializeDictionary(buf *bytes.Buffer) map[interface{}]interface{} {
	keyTC, _ := buf.ReadByte()
	valTC, _ := buf.ReadByte()
	count := boundedCount(readCount(buf), maxCollectionElements)
	out := make(map[interface{}]interface{}, count)
	for i := 0; i < count && buf.Len() > 0; i++ {
		kt := keyTC
		if keyTC == 0 {
			kt, _ = buf.ReadByte()
		}
		vt := valTC
		if valTC == 0 {
			vt, _ = buf.ReadByte()
		}
		key := deserialize(buf, kt)
		val := deserialize(buf, vt)
		if isComparable(key) {
			out[key] = val
		} else {
			out[fmt.Sprintf("UNHASHABLE_%d_%T", i, key)] = val
		}
	}
	return out
}

func deserializeCustom(buf *bytes.Buffer, gpType byte) interface{} {
	var customID byte
	isSlim := gpType >= customTypeSlimBase
	if isSlim {
		customID = gpType & 0x7F
	} else {
		customID, _ = buf.ReadByte()
	}
	return deserializeCustomPayload(buf, customID, isSlim)
}

func deserializeCustomPayload(buf *bytes.Buffer, customID byte, isSlim bool) interface{} {
	size := int(readCount(buf))
	if size < 0 || size > buf.Len() {
		if isSlim {
			data := make([]byte, buf.Len())
			_, _ = buf.Read(data)
			return map[string]interface{}{"type": customID, "data": data}
		}
		return nil
	}
	data := make([]byte, size)
	_, _ = buf.Read(data)
	return map[string]interface{}{"type": customID, "data": data}
}

func deserializeOperationRequestInner(buf *bytes.Buffer) interface{} {
	opCode, _ := buf.ReadByte()
	params := readParameterTable(buf)
	return map[string]interface{}{"operationCode": opCode, "parameters": params}
}

func deserializeOperationResponseInner(buf *bytes.Buffer) interface{} {
	if buf.Len() < 3 {
		return nil
	}
	opCode, _ := buf.ReadByte()
	returnCode := readInt16(buf)
	debugMsg := ""
	if buf.Len() > 0 {
		tc, _ := buf.ReadByte()
		if v, ok := deserialize(buf, tc).(string); ok {
			debugMsg = v
		}
	}
	params := readParameterTable(buf)
	return map[string]interface{}{"operationCode": opCode, "returnCode": returnCode, "debugMessage": debugMsg, "parameters": params}
}

func deserializeEventDataInner(buf *bytes.Buffer) interface{} {
	code, _ := buf.ReadByte()
	params := readParameterTable(buf)
	return map[string]interface{}{"code": code, "parameters": params}
}

func readInt16(buf *bytes.Buffer) int16 {
	var v int16
	_ = binary.Read(buf, binary.LittleEndian, &v)
	return v
}

func readUint16(buf *bytes.Buffer) uint16 {
	var v uint16
	_ = binary.Read(buf, binary.LittleEndian, &v)
	return v
}

func readFloat32(buf *bytes.Buffer) float32 {
	var bits uint32
	_ = binary.Read(buf, binary.LittleEndian, &bits)
	return math.Float32frombits(bits)
}

func readFloat64(buf *bytes.Buffer) float64 {
	var bits uint64
	_ = binary.Read(buf, binary.LittleEndian, &bits)
	return math.Float64frombits(bits)
}

func readString(buf *bytes.Buffer) string {
	length := int(readCompressedUint32(buf))
	if length <= 0 || length > buf.Len() {
		return ""
	}
	b := make([]byte, length)
	_, _ = buf.Read(b)
	return string(b)
}

func readCount(buf *bytes.Buffer) uint32 { return readCompressedUint32(buf) }

func readCompressedUint32(buf *bytes.Buffer) uint32 {
	var value uint32
	shift := uint(0)
	for {
		b, err := buf.ReadByte()
		if err != nil {
			return 0
		}
		value |= uint32(b&0x7F) << shift
		if b&0x80 == 0 {
			return value
		}
		shift += 7
		if shift >= 35 {
			return 0
		}
	}
}

func readCompressedUint64(buf *bytes.Buffer) uint64 {
	var value uint64
	shift := uint(0)
	for {
		b, err := buf.ReadByte()
		if err != nil {
			return 0
		}
		value |= uint64(b&0x7F) << shift
		if b&0x80 == 0 {
			return value
		}
		shift += 7
		if shift >= 70 {
			return 0
		}
	}
}

func readCompressedInt32(buf *bytes.Buffer) int32 {
	v := readCompressedUint32(buf)
	return int32((v >> 1) ^ uint32(-(int32(v & 1))))
}

func readCompressedInt64(buf *bytes.Buffer) int64 {
	v := readCompressedUint64(buf)
	return int64((v >> 1) ^ uint64(-(int64(v & 1))))
}

func boundedCount(n uint32, max int) int {
	if max < 0 || max > maxCollectionElements {
		max = maxCollectionElements
	}
	if uint64(n) > uint64(max) {
		return max
	}
	return int(n)
}

func isComparable(v interface{}) bool {
	if v == nil {
		return true
	}
	return reflect.TypeOf(v).Comparable()
}
