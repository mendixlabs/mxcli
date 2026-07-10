// SPDX-License-Identifier: Apache-2.0

package pages

import (
	"go.mongodb.org/mongo-driver/v2/bson"

	"github.com/JordtenBulte-OLC/mxcli/modelsdk/codec"
)

// BSONArrayItems 将 raw BSON 字段统一转换为 []any。
//
// MongoDB Go 驱动在解码 BSON 数组时可能返回 []any 或 bson.A，
// 两者底层一致，但 Go 类型不同。本函数让 executor 调用方无需直接
// 依赖 primitive 包即可处理两种形式。
//
// 返回的第二个值表示输入是否为可识别的数组类型。
func BSONArrayItems(v any) ([]any, bool) {
	switch arr := v.(type) {
	case []any:
		return arr, true
	case bson.A:
		return []any(arr), true
	default:
		return nil, false
	}
}

// BSONBinaryData 将 raw BSON 字段中的 bson.Binary 提取为 []byte。
// 如果输入不是 bson.Binary，返回 (nil, false)。
//
// 调用方应优先用类型 switch 处理 string / []byte 这些已是 Go 原生
// 类型的情况，再用本函数处理 BSON 专有的 Binary 包装。
func BSONBinaryData(v any) ([]byte, bool) {
	b, ok := v.(bson.Binary)
	if !ok {
		return nil, false
	}
	return b.Data, true
}

// BSONDocumentMap 将 raw BSON 字段统一转换为 map[string]any。
//
// MongoDB Go 驱动在解码嵌套 BSON 文档时可能返回 map[string]any 或
// bson.M，两者底层一致但类型不同。本函数让 executor 调用方无
// 需直接依赖 primitive 包即可处理两种形式。
//
// 返回的第二个值表示输入是否为可识别的文档类型。
func BSONDocumentMap(v any) (map[string]any, bool) {
	switch m := v.(type) {
	case map[string]any:
		return m, true
	case bson.M:
		return map[string]any(m), true
	case bson.D:
		// v2: nested docs inside bson.M decode as bson.D.
		out := make(map[string]any, len(m))
		for _, e := range m {
			out[e.Key] = e.Value
		}
		return out, true
	default:
		return nil, false
	}
}

// WidgetActionMap returns the "Action" sub-document from a raw widget map
// as a Go map. It exists so executor callers can extract the Action
// property of an ActionButton (and other action-carrying widgets) without
// indexing the raw widget map by string literal — which the
// describe-raw-bson architectural guard forbids in newly added code.
//
// Returns (nil, false) when the widget has no Action property or the
// stored value is neither map[string]any nor bson.M.
func WidgetActionMap(w map[string]any) (map[string]any, bool) {
	if w == nil {
		return nil, false
	}
	return BSONDocumentMap(w["Action"])
}

// DecodeMicroflowQNFromDataSource marshals a raw Forms$MicroflowSource map
// back to BSON, decodes via the gen-type registry, and returns the
// microflow qualified name pulled from MicroflowSettings. Returns "" if
// the map is not a MicroflowSource or has no microflow set.
//
// Lives here so executor describe code can stay off bson/codec while
// the migration away from raw-map widget walking proceeds.
func DecodeMicroflowQNFromDataSource(ds map[string]any) string {
	raw, err := bson.Marshal(ds)
	if err != nil {
		return ""
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if err != nil {
		return ""
	}
	ms, ok := elem.(*MicroflowSource)
	if !ok || ms == nil {
		return ""
	}
	settings, ok := ms.MicroflowSettings().(*MicroflowSettings)
	if !ok || settings == nil {
		return ""
	}
	return settings.MicroflowQualifiedName()
}

// DecodeNanoflowQNFromDataSource marshals a raw Forms$NanoflowSource map
// back to BSON, decodes via the gen-type registry, and returns the
// nanoflow qualified name. Returns "" if the map is not a NanoflowSource
// or has no nanoflow set.
func DecodeNanoflowQNFromDataSource(ds map[string]any) string {
	raw, err := bson.Marshal(ds)
	if err != nil {
		return ""
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if err != nil {
		return ""
	}
	ns, ok := elem.(*NanoflowSource)
	if !ok || ns == nil {
		return ""
	}
	return ns.NanoflowQualifiedName()
}

// DecodeMicroflowClientAction marshals a raw client-action property map
// back to BSON and decodes it via the gen-type registry, returning the
// typed value or nil when the map is not a MicroflowClientAction.
func DecodeMicroflowClientAction(action map[string]any) *MicroflowClientAction {
	raw, err := bson.Marshal(action)
	if err != nil {
		return nil
	}
	elem, err := codec.NewDecoder(codec.DefaultRegistry).Decode(bson.Raw(raw))
	if err != nil {
		return nil
	}
	mca, _ := elem.(*MicroflowClientAction)
	return mca
}
