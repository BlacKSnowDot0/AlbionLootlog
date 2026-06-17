// Package loot implements Albion Online loot tracking on top of decoded Photon
// events. Handler logic mirrors the upstream C# project
// Triky313/AlbionOnline-StatisticsAnalysis (GPL-3.0).
package loot

import "fmt"

// toInt coerces a Protocol18 value to int. Photon delivers numerics as varying
// concrete types (byte, int16, int32, int64, float32/64) depending on the
// game's wire encoding, so handlers must coerce rather than type-assert.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case byte:
		return int(n)
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	case float32:
		return int(n)
	case float64:
		return int(n)
	case bool:
		if n {
			return 1
		}
		return 0
	default:
		return 0
	}
}

// toInt64 coerces a Protocol18 value to int64 (for silver amounts, which can
// exceed int32 on some servers).
func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case byte:
		return int64(n)
	case int16:
		return int64(n)
	case int32:
		return int64(n)
	case int64:
		return n
	case uint16:
		return int64(n)
	case uint32:
		return int64(n)
	case uint64:
		return int64(n)
	case float32:
		return int64(n)
	case float64:
		return int64(n)
	default:
		return 0
	}
}

// toBool coerces a Protocol18 value to bool.
func toBool(v interface{}) bool {
	switch b := v.(type) {
	case bool:
		return b
	case byte:
		return b != 0
	case int16:
		return b != 0
	case int32:
		return b != 0
	case int64:
		return b != 0
	default:
		return false
	}
}

// toString coerces a Protocol18 value to its string form.
func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprintf("%v", v)
}
