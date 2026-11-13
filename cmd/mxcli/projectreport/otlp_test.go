// SPDX-License-Identifier: Apache-2.0

package projectreport

import (
	"bytes"
	"encoding/binary"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// A tiny protobuf writer, enough to build an ExportTraceServiceRequest the way
// the runtime's agent sends it.
type pb struct{ b []byte }

func (p *pb) tag(num, wt int) { p.b = binary.AppendUvarint(p.b, uint64(num<<3|wt)) }
func (p *pb) bytes(num int, v []byte) *pb {
	p.tag(num, 2)
	p.b = binary.AppendUvarint(p.b, uint64(len(v)))
	p.b = append(p.b, v...)
	return p
}
func (p *pb) str(num int, s string) *pb { return p.bytes(num, []byte(s)) }
func (p *pb) msg(num int, m *pb) *pb    { return p.bytes(num, m.b) }
func (p *pb) varint(num int, v uint64) *pb {
	p.tag(num, 0)
	p.b = binary.AppendUvarint(p.b, v)
	return p
}
func (p *pb) fixed64(num int, v uint64) *pb {
	p.tag(num, 1)
	p.b = binary.LittleEndian.AppendUint64(p.b, v)
	return p
}

func kv(key string, val *pb) *pb { return (&pb{}).str(1, key).msg(2, val) }

func otlpRequest() []byte {
	span := (&pb{}).bytes(1, []byte{0xab, 0xcd}).bytes(2, []byte{0x01}).bytes(4, []byte{0x02}).
		str(5, "Microflow Shop.ACT_Order").varint(6, 1).fixed64(7, 1_000_000_000).fixed64(8, 1_250_000_000).
		msg(9, kv("db.system", (&pb{}).str(1, "postgresql"))).
		msg(9, kv("rows", (&pb{}).varint(3, 42))).
		varint(99, 7) // an unknown field must be skipped, not fail
	scope := (&pb{}).msg(1, (&pb{}).str(1, "agent")).msg(2, span)
	resource := (&pb{}).msg(1, kv("service.name", (&pb{}).str(1, "Shop")))
	rs := (&pb{}).msg(1, resource).msg(2, scope)
	return (&pb{}).msg(1, rs).b
}

func TestDecodeOTLPProto(t *testing.T) {
	spans, err := DecodeOTLPProto(otlpRequest())
	if err != nil {
		t.Fatal(err)
	}
	if len(spans) != 1 {
		t.Fatalf("spans = %+v", spans)
	}
	s := spans[0]
	if s.TraceID != "abcd" || s.SpanID != "01" || s.ParentID != "02" || s.Service != "Shop" || s.Kind != 1 {
		t.Errorf("ids/service/kind wrong: %+v", s)
	}
	if s.DurationMs() != 250 || s.Microflow() != "Shop.ACT_Order" || !s.IsDB() || s.Attrs["rows"] != "42" {
		t.Errorf("decoded fields wrong: %+v", s)
	}
}

func TestCollectorReceivesProtobuf(t *testing.T) {
	out := filepath.Join(t.TempDir(), "spans.jsonl")
	c, err := StartCollector("127.0.0.1:0", out)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Post("http://"+c.Addr()+"/v1/traces", "application/x-protobuf", bytes.NewReader(otlpRequest()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 200 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	spans, err := ReadSpans([]string{out})
	if err != nil || len(spans) != 1 || spans[0].Microflow() != "Shop.ACT_Order" {
		t.Fatalf("read back %+v, %v", spans, err)
	}
}

func TestReadSpansOTLPJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "export.json")
	_ = os.WriteFile(p, []byte(`{"resourceSpans":[{"resource":{"attributes":[{"key":"service.name","value":{"stringValue":"Shop"}}]},
	"scopeSpans":[{"spans":[{"traceId":"t","spanId":"s","name":"Microflow Shop.A","startTimeUnixNano":"1000000","endTimeUnixNano":"3000000"}]}]}]}`), 0o644)
	spans, err := ReadSpans([]string{p})
	if err != nil || len(spans) != 1 || spans[0].DurationMs() != 2 || spans[0].Service != "Shop" {
		t.Fatalf("%+v %v", spans, err)
	}
}
