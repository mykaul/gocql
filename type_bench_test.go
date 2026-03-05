// Copyright (c) 2012 The gocql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build all || unit

package gocql

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"math"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// benchOnce ensures pinToSingleCore is called exactly once, and only when a
// benchmark function runs — not during regular unit tests.
var benchOnce sync.Once

// benchInit should be called at the start of every top-level Benchmark function.
func benchInit(b *testing.B) {
	b.Helper()
	benchOnce.Do(pinToSingleCore)
}

// ---------------------------------------------------------------------
// Helper: build UDT wire data (4-byte length prefix per field)
// ---------------------------------------------------------------------

// udtWireData builds the binary wire format for a UDT with nFields int32 fields.
// Each field is encoded as [4-byte big-endian length][4-byte big-endian value].
func udtWireData(nFields int) []byte {
	var buf []byte
	for i := 0; i < nFields; i++ {
		// 4-byte length prefix (value is 4 bytes)
		buf = append(buf, 0, 0, 0, 4)
		// 4-byte big-endian int32 value
		b := [4]byte{}
		binary.BigEndian.PutUint32(b[:], uint32(i))
		buf = append(buf, b[:]...)
	}
	return buf
}

// makeUDTTypeInfo builds a UDTTypeInfo with nFields int fields named "field0", "field1", etc.
func makeUDTTypeInfo(nFields int) UDTTypeInfo {
	elems := make([]UDTField, nFields)
	for i := 0; i < nFields; i++ {
		elems[i] = UDTField{
			Name: "field" + strconv.Itoa(i),
			Type: NewNativeType(protoVersion4, TypeInt),
		}
	}
	return UDTTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeUDT),
		Name:       "bench_udt",
		KeySpace:   "bench_ks",
		Elements:   elems,
	}
}

// makeTupleTypeInfo builds a TupleTypeInfo with nElems int elements.
func makeTupleTypeInfo(nElems int) TupleTypeInfo {
	elems := make([]TypeInfo, nElems)
	for i := 0; i < nElems; i++ {
		elems[i] = NewNativeType(protoVersion4, TypeInt)
	}
	return TupleTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeTuple),
		Elems:      elems,
	}
}

// tupleWireData builds the binary wire format for a tuple with nElems int32 fields.
func tupleWireData(nElems int) []byte {
	var buf []byte
	for i := 0; i < nElems; i++ {
		buf = append(buf, 0, 0, 0, 4)
		b := [4]byte{}
		binary.BigEndian.PutUint32(b[:], uint32(i))
		buf = append(buf, b[:]...)
	}
	return buf
}

// makeListTypeInfo builds a CollectionType for list<int>.
func makeListTypeInfo() CollectionType {
	return CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeList),
		Elem:       NewNativeType(protoVersion4, TypeInt),
	}
}

// listWireData builds the binary wire format for a list<int> with n elements.
// Protocol v4 format: [4-byte count][4-byte len][4-byte value]...
func listWireData(n int) []byte {
	buf := make([]byte, 4, 4+n*8)
	binary.BigEndian.PutUint32(buf[:4], uint32(n))
	for i := 0; i < n; i++ {
		// 4-byte length prefix
		buf = append(buf, 0, 0, 0, 4)
		// 4-byte value
		b := [4]byte{}
		binary.BigEndian.PutUint32(b[:], uint32(i))
		buf = append(buf, b[:]...)
	}
	return buf
}

// makeMapTypeInfo builds a CollectionType for map<text,int>.
func makeMapTypeInfo() CollectionType {
	return CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeMap),
		Key:        NewNativeType(protoVersion4, TypeText),
		Elem:       NewNativeType(protoVersion4, TypeInt),
	}
}

// mapWireData builds the binary wire format for a map<text,int> with n entries.
// Protocol v4 format: [4-byte count][4-byte keylen][key bytes][4-byte vallen][val bytes]...
func mapWireData(n int) []byte {
	buf := make([]byte, 4)
	binary.BigEndian.PutUint32(buf[:4], uint32(n))
	for i := 0; i < n; i++ {
		key := []byte("key" + strconv.Itoa(i))
		// key: [4-byte length][key bytes]
		klen := [4]byte{}
		binary.BigEndian.PutUint32(klen[:], uint32(len(key)))
		buf = append(buf, klen[:]...)
		buf = append(buf, key...)
		// value: [4-byte length][4-byte int value]
		buf = append(buf, 0, 0, 0, 4)
		b := [4]byte{}
		binary.BigEndian.PutUint32(b[:], uint32(i))
		buf = append(buf, b[:]...)
	}
	return buf
}

// =======================================================================
// Section A: Type Resolution
// =======================================================================

// BenchmarkGoType measures the cost of goType() for all CQL type categories.
// This is called per-column per-row via NewWithError() -> RowData() -> MapScan.
func BenchmarkGoType(b *testing.B) {
	benchInit(b)
	types := []struct {
		name string
		info TypeInfo
	}{
		// Scalar types
		{"Varchar", NewNativeType(protoVersion4, TypeVarchar)},
		{"Ascii", NewNativeType(protoVersion4, TypeAscii)},
		{"Text", NewNativeType(protoVersion4, TypeText)},
		{"Inet", NewNativeType(protoVersion4, TypeInet)},
		{"BigInt", NewNativeType(protoVersion4, TypeBigInt)},
		{"Counter", NewNativeType(protoVersion4, TypeCounter)},
		{"Time", NewNativeType(protoVersion4, TypeTime)},
		{"Timestamp", NewNativeType(protoVersion4, TypeTimestamp)},
		{"Blob", NewNativeType(protoVersion4, TypeBlob)},
		{"Boolean", NewNativeType(protoVersion4, TypeBoolean)},
		{"Float", NewNativeType(protoVersion4, TypeFloat)},
		{"Double", NewNativeType(protoVersion4, TypeDouble)},
		{"Int", NewNativeType(protoVersion4, TypeInt)},
		{"SmallInt", NewNativeType(protoVersion4, TypeSmallInt)},
		{"TinyInt", NewNativeType(protoVersion4, TypeTinyInt)},
		{"Decimal", NewNativeType(protoVersion4, TypeDecimal)},
		{"UUID", NewNativeType(protoVersion4, TypeUUID)},
		{"TimeUUID", NewNativeType(protoVersion4, TypeTimeUUID)},
		{"Varint", NewNativeType(protoVersion4, TypeVarint)},
		{"Date", NewNativeType(protoVersion4, TypeDate)},
		{"Duration", NewNativeType(protoVersion4, TypeDuration)},
		// Collection types
		{"List_int", makeListTypeInfo()},
		{"Map_text_int", makeMapTypeInfo()},
		// Composite types
		{"UDT_5", makeUDTTypeInfo(5)},
		{"Tuple_3", makeTupleTypeInfo(3)},
	}

	for _, tc := range types {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			info := tc.info
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := goType(info)
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkNewWithError measures the full NewWithError() path which calls
// goType() and then reflect.New(). This is the per-column cost in RowData().
func BenchmarkNewWithError(b *testing.B) {
	benchInit(b)
	types := []struct {
		name string
		info TypeInfo
	}{
		{"Int", NewNativeType(protoVersion4, TypeInt)},
		{"Text", NewNativeType(protoVersion4, TypeText)},
		{"UUID", NewNativeType(protoVersion4, TypeUUID)},
		{"Timestamp", NewNativeType(protoVersion4, TypeTimestamp)},
		{"Boolean", NewNativeType(protoVersion4, TypeBoolean)},
		{"List_int", makeListTypeInfo()},
		{"Map_text_int", makeMapTypeInfo()},
		{"UDT_5", makeUDTTypeInfo(5)},
		{"Tuple_3", makeTupleTypeInfo(3)},
	}

	for _, tc := range types {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			info := tc.info
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, err := info.NewWithError()
				if err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// =======================================================================
// Section B: Marshal / Unmarshal
// =======================================================================

// --- Scalars ---

func BenchmarkMarshalInt(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeInt)
	val := 42
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalInt(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeInt)
	data, _ := Marshal(info, 42)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v int
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalText(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeText)
	val := "hello world benchmark text value"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalText(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeText)
	data, _ := Marshal(info, "hello world benchmark text value")
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v string
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalUUID(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeUUID)
	val := TimeUUID()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalUUID2(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeUUID)
	data, _ := Marshal(info, TimeUUID())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v UUID
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalBool(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeBoolean)
	val := true
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalBool(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeBoolean)
	data, _ := Marshal(info, true)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v bool
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalTimestamp(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeTimestamp)
	val := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalTimestamp(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := NewNativeType(protoVersion4, TypeTimestamp)
	data, _ := Marshal(info, time.Now())
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v time.Time
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

// --- Collections with scaling ---

func BenchmarkMarshalListInt(b *testing.B) {
	benchInit(b)
	for _, n := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			info := makeListTypeInfo()
			val := make([]int, n)
			for i := range val {
				val[i] = i
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Marshal(info, val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnmarshalListInt(b *testing.B) {
	benchInit(b)
	for _, n := range []int{10, 100, 1000} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			info := makeListTypeInfo()
			data := listWireData(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var v []int
				if err := Unmarshal(info, data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkMarshalMapTextInt(b *testing.B) {
	benchInit(b)
	for _, n := range []int{10, 100} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			info := makeMapTypeInfo()
			val := make(map[string]int, n)
			for i := 0; i < n; i++ {
				val["key"+strconv.Itoa(i)] = i
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Marshal(info, val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnmarshalMapTextInt(b *testing.B) {
	benchInit(b)
	for _, n := range []int{10, 100} {
		b.Run(strconv.Itoa(n), func(b *testing.B) {
			b.ReportAllocs()
			info := makeMapTypeInfo()
			data := mapWireData(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				var v map[string]int
				if err := Unmarshal(info, data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// --- UDT struct path (with cql tags) ---

type benchUDT5 struct {
	Field0 int `cql:"field0"`
	Field1 int `cql:"field1"`
	Field2 int `cql:"field2"`
	Field3 int `cql:"field3"`
	Field4 int `cql:"field4"`
}

type benchUDT20 struct {
	Field0  int `cql:"field0"`
	Field1  int `cql:"field1"`
	Field2  int `cql:"field2"`
	Field3  int `cql:"field3"`
	Field4  int `cql:"field4"`
	Field5  int `cql:"field5"`
	Field6  int `cql:"field6"`
	Field7  int `cql:"field7"`
	Field8  int `cql:"field8"`
	Field9  int `cql:"field9"`
	Field10 int `cql:"field10"`
	Field11 int `cql:"field11"`
	Field12 int `cql:"field12"`
	Field13 int `cql:"field13"`
	Field14 int `cql:"field14"`
	Field15 int `cql:"field15"`
	Field16 int `cql:"field16"`
	Field17 int `cql:"field17"`
	Field18 int `cql:"field18"`
	Field19 int `cql:"field19"`
}

func BenchmarkMarshalUDTStruct(b *testing.B) {
	benchInit(b)
	b.Run("5fields", func(b *testing.B) {
		b.ReportAllocs()
		info := makeUDTTypeInfo(5)
		val := benchUDT5{0, 1, 2, 3, 4}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := Marshal(info, val); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("20fields", func(b *testing.B) {
		b.ReportAllocs()
		info := makeUDTTypeInfo(20)
		val := benchUDT20{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19}
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := Marshal(info, val); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkUnmarshalUDTStruct(b *testing.B) {
	benchInit(b)
	b.Run("5fields", func(b *testing.B) {
		b.ReportAllocs()
		info := makeUDTTypeInfo(5)
		data := udtWireData(5)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var v benchUDT5
			if err := Unmarshal(info, data, &v); err != nil {
				b.Fatal(err)
			}
		}
	})

	b.Run("20fields", func(b *testing.B) {
		b.ReportAllocs()
		info := makeUDTTypeInfo(20)
		data := udtWireData(20)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			var v benchUDT20
			if err := Unmarshal(info, data, &v); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// --- UDT map path ---

func BenchmarkMarshalUDTMap(b *testing.B) {
	benchInit(b)
	for _, n := range []int{5, 20, 50, 100} {
		b.Run(strconv.Itoa(n)+"fields", func(b *testing.B) {
			b.ReportAllocs()
			info := makeUDTTypeInfo(n)
			val := make(map[string]interface{}, n)
			for i := 0; i < n; i++ {
				val["field"+strconv.Itoa(i)] = i
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Marshal(info, val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnmarshalUDTMap(b *testing.B) {
	benchInit(b)
	for _, n := range []int{5, 20, 50, 100} {
		b.Run(strconv.Itoa(n)+"fields", func(b *testing.B) {
			b.ReportAllocs()
			info := makeUDTTypeInfo(n)
			data := udtWireData(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				v := make(map[string]interface{})
				if err := Unmarshal(info, data, &v); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// --- Tuple ---

func BenchmarkMarshalTuple(b *testing.B) {
	benchInit(b)
	for _, n := range []int{3, 10} {
		b.Run(strconv.Itoa(n)+"elems", func(b *testing.B) {
			b.ReportAllocs()
			info := makeTupleTypeInfo(n)
			// Tuple marshal expects []interface{} for >1 elements
			val := make([]interface{}, n)
			for i := 0; i < n; i++ {
				val[i] = i
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				if _, err := Marshal(info, val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkUnmarshalTuple(b *testing.B) {
	benchInit(b)
	for _, n := range []int{3, 10} {
		b.Run(strconv.Itoa(n)+"elems", func(b *testing.B) {
			b.ReportAllocs()
			info := makeTupleTypeInfo(n)
			data := tupleWireData(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				val := make([]interface{}, n)
				if err := Unmarshal(info, data, &val); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// --- Vector float64 (768-dim) ---
// float32 vectors are already benchmarked in vector_bench_test.go.

func makeFloat64VectorType(dim int) VectorType {
	return VectorType{
		NativeType: NativeType{
			proto:  protoVersion4,
			typ:    TypeCustom,
			custom: apacheCassandraTypePrefix + "VectorType(" + apacheCassandraTypePrefix + "DoubleType, " + strconv.Itoa(dim) + ")",
		},
		SubType:    NativeType{proto: protoVersion4, typ: TypeDouble},
		Dimensions: dim,
	}
}

func BenchmarkMarshalVectorFloat64(b *testing.B) {
	benchInit(b)
	dim := 768
	b.Run("dim_768", func(b *testing.B) {
		b.ReportAllocs()
		vec := make([]float64, dim)
		for i := range vec {
			vec[i] = float64(i) * 0.1
		}
		info := makeFloat64VectorType(dim)
		b.SetBytes(int64(dim * 8))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if _, err := marshalVector(info, vec); err != nil {
				b.Fatal(err)
			}
		}
	})
}

func BenchmarkUnmarshalVectorFloat64(b *testing.B) {
	benchInit(b)
	dim := 768
	b.Run("dim_768", func(b *testing.B) {
		b.ReportAllocs()
		data := make([]byte, dim*8)
		for i := 0; i < dim; i++ {
			binary.BigEndian.PutUint64(data[i*8:], math.Float64bits(float64(i)*0.1))
		}
		info := makeFloat64VectorType(dim)
		var result []float64
		b.SetBytes(int64(dim * 8))
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			if err := unmarshalVector(info, data, &result); err != nil {
				b.Fatal(err)
			}
		}
	})
}

// --- Nested: list<UDT(5)> and map<text, list<int(10)>> ---

func BenchmarkMarshalNestedListUDT(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	udtInfo := makeUDTTypeInfo(5)
	info := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeList),
		Elem:       udtInfo,
	}
	val := make([]map[string]interface{}, 10)
	for i := range val {
		m := make(map[string]interface{}, 5)
		for j := 0; j < 5; j++ {
			m["field"+strconv.Itoa(j)] = j + i*5
		}
		val[i] = m
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalNestedListUDT(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	udtInfo := makeUDTTypeInfo(5)
	info := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeList),
		Elem:       udtInfo,
	}
	// Marshal a value to get valid wire data
	val := make([]map[string]interface{}, 10)
	for i := range val {
		m := make(map[string]interface{}, 5)
		for j := 0; j < 5; j++ {
			m["field"+strconv.Itoa(j)] = j + i*5
		}
		val[i] = m
	}
	data, err := Marshal(info, val)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v []map[string]interface{}
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkMarshalNestedMapTextListInt(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeMap),
		Key:        NewNativeType(protoVersion4, TypeText),
		Elem: CollectionType{
			NativeType: NewNativeType(protoVersion4, TypeList),
			Elem:       NewNativeType(protoVersion4, TypeInt),
		},
	}
	val := map[string][]int{
		"alpha": make([]int, 10),
		"beta":  make([]int, 10),
		"gamma": make([]int, 10),
	}
	for k := range val {
		for i := range val[k] {
			val[k][i] = i
		}
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := Marshal(info, val); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkUnmarshalNestedMapTextListInt(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	info := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeMap),
		Key:        NewNativeType(protoVersion4, TypeText),
		Elem: CollectionType{
			NativeType: NewNativeType(protoVersion4, TypeList),
			Elem:       NewNativeType(protoVersion4, TypeInt),
		},
	}
	val := map[string][]int{
		"alpha": make([]int, 10),
		"beta":  make([]int, 10),
		"gamma": make([]int, 10),
	}
	for k := range val {
		for i := range val[k] {
			val[k][i] = i
		}
	}
	data, err := Marshal(info, val)
	if err != nil {
		b.Fatal(err)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var v map[string][]int
		if err := Unmarshal(info, data, &v); err != nil {
			b.Fatal(err)
		}
	}
}

// =======================================================================
// Section C: Type String Parsing
// =======================================================================

const prefix = apacheCassandraTypePrefix

func BenchmarkSplitCompositeTypes(b *testing.B) {
	benchInit(b)
	b.Run("simple_map", func(b *testing.B) {
		b.ReportAllocs()
		name := prefix + "MapType(" + prefix + "UTF8Type," + prefix + "Int32Type)"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			splitJavaCompositeTypes(name, prefix+"MapType")
		}
	})

	b.Run("nested_map_list", func(b *testing.B) {
		b.ReportAllocs()
		name := prefix + "MapType(" + prefix + "UTF8Type," + prefix + "ListType(" + prefix + "Int32Type))"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			splitJavaCompositeTypes(name, prefix+"MapType")
		}
	})

	b.Run("deep_100fields", func(b *testing.B) {
		b.ReportAllocs()
		// Build a UserType string with 100 fields (hex-encoded field names)
		var sb strings.Builder
		sb.WriteString(prefix + "UserType(bench_ks,")
		sb.WriteString(hex.EncodeToString([]byte("bench_udt")))
		for i := 0; i < 100; i++ {
			sb.WriteByte(',')
			sb.WriteString(hex.EncodeToString([]byte("field" + strconv.Itoa(i))))
			sb.WriteByte(':')
			sb.WriteString(prefix + "Int32Type")
		}
		sb.WriteByte(')')
		name := sb.String()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			splitJavaCompositeTypes(name, prefix+"UserType")
		}
	})
}

// BenchmarkGetCassandraLongType measures the full type parsing pipeline.
func BenchmarkGetCassandraLongType(b *testing.B) {
	benchInit(b)
	logger := nopLogger{}

	b.Run("simple_int", func(b *testing.B) {
		b.ReportAllocs()
		name := prefix + "Int32Type"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getCassandraLongType(name, protoVersion4, logger)
		}
	})

	b.Run("udt_5fields", func(b *testing.B) {
		b.ReportAllocs()
		var sb strings.Builder
		sb.WriteString(prefix + "UserType(bench_ks,")
		sb.WriteString(hex.EncodeToString([]byte("bench_udt")))
		for i := 0; i < 5; i++ {
			sb.WriteByte(',')
			sb.WriteString(hex.EncodeToString([]byte("field" + strconv.Itoa(i))))
			sb.WriteByte(':')
			sb.WriteString(prefix + "Int32Type")
		}
		sb.WriteByte(')')
		name := sb.String()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getCassandraLongType(name, protoVersion4, logger)
		}
	})

	b.Run("udt_20fields", func(b *testing.B) {
		b.ReportAllocs()
		var sb strings.Builder
		sb.WriteString(prefix + "UserType(bench_ks,")
		sb.WriteString(hex.EncodeToString([]byte("bench_udt")))
		for i := 0; i < 20; i++ {
			sb.WriteByte(',')
			sb.WriteString(hex.EncodeToString([]byte("field" + strconv.Itoa(i))))
			sb.WriteByte(':')
			sb.WriteString(prefix + "Int32Type")
		}
		sb.WriteByte(')')
		name := sb.String()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getCassandraLongType(name, protoVersion4, logger)
		}
	})

	b.Run("nested_map_list_udt", func(b *testing.B) {
		b.ReportAllocs()
		// MapType(UTF8Type, ListType(UserType(ks, udt, f0:Int32Type, f1:Int32Type, f2:Int32Type)))
		var sb strings.Builder
		sb.WriteString(prefix + "MapType(")
		sb.WriteString(prefix + "UTF8Type,")
		sb.WriteString(prefix + "ListType(")
		sb.WriteString(prefix + "UserType(bench_ks,")
		sb.WriteString(hex.EncodeToString([]byte("bench_udt")))
		for i := 0; i < 3; i++ {
			sb.WriteByte(',')
			sb.WriteString(hex.EncodeToString([]byte("field" + strconv.Itoa(i))))
			sb.WriteByte(':')
			sb.WriteString(prefix + "Int32Type")
		}
		sb.WriteString(")))")
		name := sb.String()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getCassandraLongType(name, protoVersion4, logger)
		}
	})

	b.Run("collection_map", func(b *testing.B) {
		b.ReportAllocs()
		name := prefix + "MapType(" + prefix + "UTF8Type," + prefix + "Int32Type)"
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			getCassandraLongType(name, protoVersion4, logger)
		}
	})
}

// =======================================================================
// Section D: String Formatting
// =======================================================================

func BenchmarkUDTTypeInfoString(b *testing.B) {
	benchInit(b)
	for _, n := range []int{5, 20, 50, 100} {
		b.Run(strconv.Itoa(n)+"fields", func(b *testing.B) {
			b.ReportAllocs()
			info := makeUDTTypeInfo(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = info.String()
			}
		})
	}
}

func BenchmarkTupleTypeInfoString(b *testing.B) {
	benchInit(b)
	for _, n := range []int{3, 10, 50} {
		b.Run(strconv.Itoa(n)+"elems", func(b *testing.B) {
			b.ReportAllocs()
			info := makeTupleTypeInfo(n)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = info.String()
			}
		})
	}
}

func BenchmarkCollectionTypeString(b *testing.B) {
	benchInit(b)
	b.Run("Map", func(b *testing.B) {
		b.ReportAllocs()
		info := makeMapTypeInfo()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = info.String()
		}
	})

	b.Run("List", func(b *testing.B) {
		b.ReportAllocs()
		info := makeListTypeInfo()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = info.String()
		}
	})
}

func BenchmarkTupleColumnName(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	col := "my_column"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = TupleColumnName(col, 42)
	}
}

// BenchmarkDereference measures the cost of dereference() which is called
// per-column per-row in rowMap().
func BenchmarkDereference(b *testing.B) {
	benchInit(b)
	b.Run("int_ptr", func(b *testing.B) {
		b.ReportAllocs()
		v := 42
		p := &v
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = dereference(p)
		}
	})

	b.Run("string_ptr", func(b *testing.B) {
		b.ReportAllocs()
		v := "hello world"
		p := &v
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = dereference(p)
		}
	})

	b.Run("uuid_ptr", func(b *testing.B) {
		b.ReportAllocs()
		v := TimeUUID()
		p := &v
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = dereference(p)
		}
	})
}

// BenchmarkNativeTypeString measures NativeType.String() formatting overhead.
func BenchmarkNativeTypeString(b *testing.B) {
	benchInit(b)
	b.Run("scalar", func(b *testing.B) {
		b.ReportAllocs()
		info := NewNativeType(protoVersion4, TypeInt)
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = info.String()
		}
	})

	b.Run("custom", func(b *testing.B) {
		b.ReportAllocs()
		info := NewCustomType(protoVersion4, TypeCustom, prefix+"VectorType("+prefix+"FloatType, 768)")
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = info.String()
		}
	})
}

// BenchmarkUnwrapCompositeTypeDefinition measures the unwrap helper.
func BenchmarkUnwrapCompositeTypeDefinition(b *testing.B) {
	benchInit(b)
	b.ReportAllocs()
	name := prefix + "SetType(" + prefix + "Int32Type)"
	typeName := prefix + "SetType"
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = unwrapCompositeTypeDefinition(name, typeName, '(')
	}
}

// BenchmarkGetApacheCassandraType measures the type name -> Type lookup.
func BenchmarkGetApacheCassandraType(b *testing.B) {
	benchInit(b)
	cases := []struct {
		name  string
		class string
	}{
		{"known_short", "Int32Type"},
		{"known_prefixed", prefix + "Int32Type"},
		{"unknown", "SomeUnknownType"},
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			class := tc.class
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = getApacheCassandraType(class)
			}
		})
	}
}

// BenchmarkMarshalOverhead measures the entry-point overhead of Marshal()
// and Unmarshal() for a trivial type to isolate dispatch cost.
func BenchmarkMarshalOverhead(b *testing.B) {
	benchInit(b)
	b.Run("marshal_int", func(b *testing.B) {
		b.ReportAllocs()
		info := NewNativeType(protoVersion4, TypeInt)
		val := 1
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = Marshal(info, val)
		}
	})

	b.Run("marshal_int_ptr", func(b *testing.B) {
		b.ReportAllocs()
		info := NewNativeType(protoVersion4, TypeInt)
		val := 1
		p := &val
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_, _ = Marshal(info, p)
		}
	})

	b.Run("unmarshal_int", func(b *testing.B) {
		b.ReportAllocs()
		info := NewNativeType(protoVersion4, TypeInt)
		data := []byte{0, 0, 0, 1}
		var v int
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			_ = Unmarshal(info, data, &v)
		}
	})
}

// Ensure fmt import is used (for error messages in helpers).
var _ = fmt.Sprintf
