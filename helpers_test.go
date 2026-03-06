// Copyright (c) 2012 The gocql Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

//go:build all || unit

package gocql

import (
	"math/big"
	"reflect"
	"testing"
	"time"

	"gopkg.in/inf.v0"
)

// =======================================================================
// goType() — verify reflect.Type for every CQL type
// =======================================================================

func TestGoType(t *testing.T) {
	tests := []struct {
		name     string
		info     TypeInfo
		wantType reflect.Type
	}{
		{"Varchar", NewNativeType(protoVersion4, TypeVarchar), reflect.TypeOf("")},
		{"Ascii", NewNativeType(protoVersion4, TypeAscii), reflect.TypeOf("")},
		{"Inet", NewNativeType(protoVersion4, TypeInet), reflect.TypeOf("")},
		{"Text", NewNativeType(protoVersion4, TypeText), reflect.TypeOf("")},
		{"BigInt", NewNativeType(protoVersion4, TypeBigInt), reflect.TypeOf(int64(0))},
		{"Counter", NewNativeType(protoVersion4, TypeCounter), reflect.TypeOf(int64(0))},
		{"Time", NewNativeType(protoVersion4, TypeTime), reflect.TypeOf(time.Duration(0))},
		{"Timestamp", NewNativeType(protoVersion4, TypeTimestamp), reflect.TypeOf(time.Time{})},
		{"Date", NewNativeType(protoVersion4, TypeDate), reflect.TypeOf(time.Time{})},
		{"Blob", NewNativeType(protoVersion4, TypeBlob), reflect.TypeOf([]byte(nil))},
		{"Boolean", NewNativeType(protoVersion4, TypeBoolean), reflect.TypeOf(false)},
		{"Float", NewNativeType(protoVersion4, TypeFloat), reflect.TypeOf(float32(0))},
		{"Double", NewNativeType(protoVersion4, TypeDouble), reflect.TypeOf(float64(0))},
		{"Int", NewNativeType(protoVersion4, TypeInt), reflect.TypeOf(int(0))},
		{"SmallInt", NewNativeType(protoVersion4, TypeSmallInt), reflect.TypeOf(int16(0))},
		{"TinyInt", NewNativeType(protoVersion4, TypeTinyInt), reflect.TypeOf(int8(0))},
		// Decimal and Varint return *pointer* types
		{"Decimal", NewNativeType(protoVersion4, TypeDecimal), reflect.TypeOf((*inf.Dec)(nil))},
		{"Varint", NewNativeType(protoVersion4, TypeVarint), reflect.TypeOf((*big.Int)(nil))},
		{"UUID", NewNativeType(protoVersion4, TypeUUID), reflect.TypeOf(UUID{})},
		{"TimeUUID", NewNativeType(protoVersion4, TypeTimeUUID), reflect.TypeOf(UUID{})},
		{"Duration", NewNativeType(protoVersion4, TypeDuration), reflect.TypeOf(Duration{})},
		{"Tuple", makeTupleTypeInfo(3), reflect.TypeOf([]interface{}(nil))},
		{"UDT", makeUDTTypeInfo(3), reflect.TypeOf(map[string]interface{}(nil))},
		// Collection types
		{"List_int", makeListTypeInfo(), reflect.SliceOf(reflect.TypeOf(int(0)))},
		{"Map_text_int", makeMapTypeInfo(), reflect.MapOf(reflect.TypeOf(""), reflect.TypeOf(int(0)))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := goType(tc.info)
			if err != nil {
				t.Fatalf("goType(%s) returned error: %v", tc.name, err)
			}
			if got != tc.wantType {
				t.Errorf("goType(%s) = %v, want %v", tc.name, got, tc.wantType)
			}
		})
	}
}

func TestGoTypeUnknown(t *testing.T) {
	// TypeCustom without vector encoding should return an error.
	info := NewNativeType(protoVersion4, TypeCustom)
	_, err := goType(info)
	if err == nil {
		t.Error("goType(TypeCustom) should return error for unknown custom type")
	}
}

// =======================================================================
// NativeType.NewWithError() — verify concrete types, especially pointer levels
// =======================================================================

func TestNewWithErrorConcreteTypes(t *testing.T) {
	tests := []struct {
		name     string
		typ      Type
		wantType reflect.Type
	}{
		{"Varchar", TypeVarchar, reflect.TypeOf(new(string))},
		{"Text", TypeText, reflect.TypeOf(new(string))},
		{"Ascii", TypeAscii, reflect.TypeOf(new(string))},
		{"Inet", TypeInet, reflect.TypeOf(new(string))},
		{"BigInt", TypeBigInt, reflect.TypeOf(new(int64))},
		{"Counter", TypeCounter, reflect.TypeOf(new(int64))},
		{"Int", TypeInt, reflect.TypeOf(new(int))},
		{"SmallInt", TypeSmallInt, reflect.TypeOf(new(int16))},
		{"TinyInt", TypeTinyInt, reflect.TypeOf(new(int8))},
		{"Float", TypeFloat, reflect.TypeOf(new(float32))},
		{"Double", TypeDouble, reflect.TypeOf(new(float64))},
		{"Boolean", TypeBoolean, reflect.TypeOf(new(bool))},
		{"Blob", TypeBlob, reflect.TypeOf(new([]byte))},
		{"Time", TypeTime, reflect.TypeOf(new(time.Duration))},
		{"Timestamp", TypeTimestamp, reflect.TypeOf(new(time.Time))},
		{"Date", TypeDate, reflect.TypeOf(new(time.Time))},
		{"UUID", TypeUUID, reflect.TypeOf(new(UUID))},
		{"TimeUUID", TypeTimeUUID, reflect.TypeOf(new(UUID))},
		{"Duration", TypeDuration, reflect.TypeOf(new(Duration))},
		// Decimal and Varint must be double-pointer (**inf.Dec, **big.Int)
		// because goType returns a pointer type for these.
		{"Decimal", TypeDecimal, reflect.TypeOf(new(*inf.Dec))},
		{"Varint", TypeVarint, reflect.TypeOf(new(*big.Int))},
		{"Tuple", TypeTuple, reflect.TypeOf(new([]interface{}))},
		{"UDT", TypeUDT, reflect.TypeOf(new(map[string]interface{}))},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := NewNativeType(protoVersion4, tc.typ)
			val, err := info.NewWithError()
			if err != nil {
				t.Fatalf("NewWithError() returned error: %v", err)
			}
			gotType := reflect.TypeOf(val)
			if gotType != tc.wantType {
				t.Errorf("NewWithError() type = %v, want %v", gotType, tc.wantType)
			}
		})
	}
}

// TestNewWithErrorDecimalPointerLevel is a focused regression test for the
// TypeDecimal double-pointer issue: NewWithError must return **inf.Dec,
// not *inf.Dec. Dereferencing it must yield *inf.Dec (a nil pointer).
func TestNewWithErrorDecimalPointerLevel(t *testing.T) {
	info := NewNativeType(protoVersion4, TypeDecimal)
	val, err := info.NewWithError()
	if err != nil {
		t.Fatal(err)
	}
	// Must be **inf.Dec
	pp, ok := val.(**inf.Dec)
	if !ok {
		t.Fatalf("expected **inf.Dec, got %T", val)
	}
	// The inner pointer should be nil (zero value of *inf.Dec)
	if *pp != nil {
		t.Errorf("expected inner pointer to be nil, got %v", *pp)
	}
}

// TestNewWithErrorVarintPointerLevel is a focused regression test for the
// TypeVarint double-pointer issue: NewWithError must return **big.Int,
// not *big.Int. Dereferencing it must yield *big.Int (a nil pointer).
func TestNewWithErrorVarintPointerLevel(t *testing.T) {
	info := NewNativeType(protoVersion4, TypeVarint)
	val, err := info.NewWithError()
	if err != nil {
		t.Fatal(err)
	}
	// Must be **big.Int
	pp, ok := val.(**big.Int)
	if !ok {
		t.Fatalf("expected **big.Int, got %T", val)
	}
	if *pp != nil {
		t.Errorf("expected inner pointer to be nil, got %v", *pp)
	}
}

// =======================================================================
// dereference() — verify correct output types for all common pointer types
// =======================================================================

func TestDereference(t *testing.T) {
	// String types
	s := "hello"
	if got := dereference(&s); got != "hello" {
		t.Errorf("dereference(*string) = %v (%T), want \"hello\"", got, got)
	}

	// Numeric types
	i := 42
	if got := dereference(&i); got != 42 {
		t.Errorf("dereference(*int) = %v (%T), want 42", got, got)
	}
	i64 := int64(100)
	if got := dereference(&i64); got != int64(100) {
		t.Errorf("dereference(*int64) = %v (%T), want int64(100)", got, got)
	}
	i32 := int32(50)
	if got := dereference(&i32); got != int32(50) {
		t.Errorf("dereference(*int32) = %v (%T), want int32(50)", got, got)
	}
	i16 := int16(10)
	if got := dereference(&i16); got != int16(10) {
		t.Errorf("dereference(*int16) = %v (%T), want int16(10)", got, got)
	}
	i8 := int8(5)
	if got := dereference(&i8); got != int8(5) {
		t.Errorf("dereference(*int8) = %v (%T), want int8(5)", got, got)
	}
	f32 := float32(3.14)
	if got := dereference(&f32); got != float32(3.14) {
		t.Errorf("dereference(*float32) = %v (%T), want float32(3.14)", got, got)
	}
	f64 := 2.718
	if got := dereference(&f64); got != 2.718 {
		t.Errorf("dereference(*float64) = %v (%T), want 2.718", got, got)
	}
	b := true
	if got := dereference(&b); got != true {
		t.Errorf("dereference(*bool) = %v (%T), want true", got, got)
	}

	// Time types
	now := time.Now()
	if got := dereference(&now); got != now {
		t.Errorf("dereference(*time.Time) mismatch")
	}
	dur := time.Second * 5
	if got := dereference(&dur); got != dur {
		t.Errorf("dereference(*time.Duration) mismatch")
	}

	// UUID
	uuid := TimeUUID()
	if got := dereference(&uuid); got != uuid {
		t.Errorf("dereference(*UUID) mismatch")
	}

	// Duration (CQL)
	cqlDur := Duration{Months: 1, Days: 2, Nanoseconds: 3}
	if got := dereference(&cqlDur); got != cqlDur {
		t.Errorf("dereference(*Duration) mismatch")
	}

	// Byte slice
	bs := []byte{1, 2, 3}
	if got, ok := dereference(&bs).([]byte); !ok || len(got) != 3 {
		t.Errorf("dereference(*[]byte) = %v (%T), want []byte{1,2,3}", got, got)
	}

	// Interface slice (tuple)
	iSlice := []interface{}{"a", 1}
	if got, ok := dereference(&iSlice).([]interface{}); !ok || len(got) != 2 {
		t.Errorf("dereference(*[]interface{}) mismatch")
	}

	// Map (UDT)
	m := map[string]interface{}{"a": 1}
	if got, ok := dereference(&m).(map[string]interface{}); !ok || len(got) != 1 {
		t.Errorf("dereference(*map[string]interface{}) mismatch")
	}
}

// TestDereferenceDecimalVarint verifies the double-pointer types that
// NewWithError produces for TypeDecimal and TypeVarint.
func TestDereferenceDecimalVarint(t *testing.T) {
	// TypeDecimal: NewWithError returns **inf.Dec
	// dereference(**inf.Dec) should yield *inf.Dec
	var decPtr *inf.Dec
	val := dereference(&decPtr)
	if _, ok := val.(*inf.Dec); !ok {
		t.Errorf("dereference(**inf.Dec) = %T, want *inf.Dec", val)
	}
	if val.(*inf.Dec) != nil {
		t.Errorf("dereference(**inf.Dec) with nil inner should be nil, got %v", val)
	}

	// With a non-nil inner pointer
	dec := inf.NewDec(42, 0)
	decPtr = dec
	val = dereference(&decPtr)
	if got, ok := val.(*inf.Dec); !ok {
		t.Errorf("dereference(**inf.Dec) = %T, want *inf.Dec", val)
	} else if got.UnscaledBig().Int64() != 42 {
		t.Errorf("dereference(**inf.Dec) value mismatch: got %v", got)
	}

	// TypeVarint: NewWithError returns **big.Int
	// dereference(**big.Int) should yield *big.Int
	var bigPtr *big.Int
	val = dereference(&bigPtr)
	if _, ok := val.(*big.Int); !ok {
		t.Errorf("dereference(**big.Int) = %T, want *big.Int", val)
	}
	if val.(*big.Int) != nil {
		t.Errorf("dereference(**big.Int) with nil inner should be nil, got %v", val)
	}

	// With a non-nil inner pointer
	bigVal := big.NewInt(99)
	bigPtr = bigVal
	val = dereference(&bigPtr)
	if got, ok := val.(*big.Int); !ok {
		t.Errorf("dereference(**big.Int) = %T, want *big.Int", val)
	} else if got.Int64() != 99 {
		t.Errorf("dereference(**big.Int) value mismatch: got %v", got)
	}
}

// TestDereferenceReflectFallback verifies the reflect fallback path for
// types not in the fast-path switch.
func TestDereferenceReflectFallback(t *testing.T) {
	// A custom type not in the switch
	type myStruct struct{ X int }
	v := myStruct{X: 42}
	got := dereference(&v)
	if s, ok := got.(myStruct); !ok || s.X != 42 {
		t.Errorf("dereference(*myStruct) = %v (%T), want myStruct{X:42}", got, got)
	}
}

// TestDereferenceEndToEnd tests the full chain: NewWithError → dereference,
// which is what rowMap() does. This catches pointer-level mismatches.
func TestDereferenceEndToEnd(t *testing.T) {
	tests := []struct {
		name    string
		typ     Type
		checkFn func(t *testing.T, val interface{})
	}{
		{"Int", TypeInt, func(t *testing.T, val interface{}) {
			if _, ok := val.(int); !ok {
				t.Errorf("got %T, want int", val)
			}
		}},
		{"Text", TypeText, func(t *testing.T, val interface{}) {
			if _, ok := val.(string); !ok {
				t.Errorf("got %T, want string", val)
			}
		}},
		{"Boolean", TypeBoolean, func(t *testing.T, val interface{}) {
			if _, ok := val.(bool); !ok {
				t.Errorf("got %T, want bool", val)
			}
		}},
		{"UUID", TypeUUID, func(t *testing.T, val interface{}) {
			if _, ok := val.(UUID); !ok {
				t.Errorf("got %T, want UUID", val)
			}
		}},
		{"Decimal", TypeDecimal, func(t *testing.T, val interface{}) {
			// Must be *inf.Dec (nil), not inf.Dec zero value
			decPtr, ok := val.(*inf.Dec)
			if !ok {
				t.Errorf("got %T, want *inf.Dec", val)
			}
			if decPtr != nil {
				t.Errorf("expected nil *inf.Dec, got %v", decPtr)
			}
		}},
		{"Varint", TypeVarint, func(t *testing.T, val interface{}) {
			// Must be *big.Int (nil), not big.Int zero value
			bigPtr, ok := val.(*big.Int)
			if !ok {
				t.Errorf("got %T, want *big.Int", val)
			}
			if bigPtr != nil {
				t.Errorf("expected nil *big.Int, got %v", bigPtr)
			}
		}},
		{"Timestamp", TypeTimestamp, func(t *testing.T, val interface{}) {
			if _, ok := val.(time.Time); !ok {
				t.Errorf("got %T, want time.Time", val)
			}
		}},
		{"Duration", TypeDuration, func(t *testing.T, val interface{}) {
			if _, ok := val.(Duration); !ok {
				t.Errorf("got %T, want Duration", val)
			}
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			info := NewNativeType(protoVersion4, tc.typ)
			ptr, err := info.NewWithError()
			if err != nil {
				t.Fatal(err)
			}
			val := dereference(ptr)
			tc.checkFn(t, val)
		})
	}
}

// =======================================================================
// udtFields() — verify field mapping with tags, embedded structs, unexported
// =======================================================================

func TestUdtFieldsBasic(t *testing.T) {
	type Simple struct {
		Name string
		Age  int32
	}

	fields := udtFields(reflect.TypeOf(Simple{}))
	if idx, ok := fields["Name"]; !ok {
		t.Error("expected 'Name' in udtFields")
	} else if len(idx) != 1 || idx[0] != 0 {
		t.Errorf("expected Name index [0], got %v", idx)
	}
	if idx, ok := fields["Age"]; !ok {
		t.Error("expected 'Age' in udtFields")
	} else if len(idx) != 1 || idx[0] != 1 {
		t.Errorf("expected Age index [1], got %v", idx)
	}
}

func TestUdtFieldsWithTags(t *testing.T) {
	type Tagged struct {
		GoName string `cql:"cql_name"`
		Other  int32  `cql:"other_col"`
	}

	fields := udtFields(reflect.TypeOf(Tagged{}))
	// Tag should override Go name
	if _, ok := fields["cql_name"]; !ok {
		t.Error("expected 'cql_name' in udtFields (from cql tag)")
	}
	if _, ok := fields["other_col"]; !ok {
		t.Error("expected 'other_col' in udtFields (from cql tag)")
	}
	// Go name should still be present (untagged behavior adds it first,
	// then tag overrides with a different key)
	if _, ok := fields["GoName"]; !ok {
		t.Error("expected 'GoName' in udtFields (Go field name)")
	}
}

func TestUdtFieldsTagOverridesGoName(t *testing.T) {
	// If the cql tag equals another field's Go name, the tag takes priority
	type Conflict struct {
		A string `cql:"B"`
		B string
	}

	fields := udtFields(reflect.TypeOf(Conflict{}))
	// "B" should point to field A (index 0) because A's tag is "B"
	if idx, ok := fields["B"]; !ok {
		t.Error("expected 'B' in udtFields")
	} else if len(idx) != 1 || idx[0] != 0 {
		t.Errorf("expected 'B' to point to field A (index 0), got %v", idx)
	}
}

func TestUdtFieldsUnexported(t *testing.T) {
	type WithUnexported struct {
		Exported   string
		unexported string //nolint:unused
	}

	fields := udtFields(reflect.TypeOf(WithUnexported{}))
	if _, ok := fields["Exported"]; !ok {
		t.Error("expected 'Exported' in udtFields")
	}
	if _, ok := fields["unexported"]; ok {
		t.Error("unexported field should NOT be in udtFields")
	}
}

func TestUdtFieldsEmbeddedStruct(t *testing.T) {
	type Base struct {
		BaseField string
	}
	type Derived struct {
		Base
		DerivedField int32
	}

	fields := udtFields(reflect.TypeOf(Derived{}))
	if _, ok := fields["BaseField"]; !ok {
		t.Error("expected promoted 'BaseField' in udtFields for embedded struct")
	}
	if _, ok := fields["DerivedField"]; !ok {
		t.Error("expected 'DerivedField' in udtFields")
	}
	// The embedded struct itself (Base) should NOT appear as a field
	if _, ok := fields["Base"]; ok {
		t.Error("anonymous embedded struct 'Base' should NOT be in udtFields")
	}
}

func TestUdtFieldsEmbeddedWithTag(t *testing.T) {
	type Base struct {
		BaseField string `cql:"base_col"`
	}
	type Derived struct {
		Base
		Top string
	}

	fields := udtFields(reflect.TypeOf(Derived{}))
	if _, ok := fields["base_col"]; !ok {
		t.Error("expected 'base_col' from embedded struct tag")
	}
	if _, ok := fields["Top"]; !ok {
		t.Error("expected 'Top' in udtFields")
	}
}

func TestUdtFieldsCaching(t *testing.T) {
	type CacheTest struct {
		X string
	}
	typ := reflect.TypeOf(CacheTest{})
	fields1 := udtFields(typ)
	fields2 := udtFields(typ)
	// Should return the same map instance (cached)
	if fields1 == nil || fields2 == nil {
		t.Fatal("unexpected nil")
	}
	// Verify cached result has same contents and length
	if len(fields1) != len(fields2) {
		t.Error("cached map should have same length")
	}
	for k, v1 := range fields1 {
		v2, ok := fields2[k]
		if !ok {
			t.Errorf("key %q missing from cached result", k)
		}
		if !reflect.DeepEqual(v1, v2) {
			t.Errorf("key %q: got %v, want %v", k, v2, v1)
		}
	}
}

func TestUdtFieldsUnexportedWithTag(t *testing.T) {
	type HasUnexportedTag struct {
		Exported   string
		unexported string `cql:"hidden"` //nolint:unused
	}
	fields := udtFields(reflect.TypeOf(HasUnexportedTag{}))
	if _, ok := fields["Exported"]; !ok {
		t.Error("expected 'Exported' in udtFields")
	}
	if _, ok := fields["hidden"]; ok {
		t.Error("unexported field with cql tag should NOT appear in udtFields")
	}
}

func TestUdtFieldsAmbiguousPromoted(t *testing.T) {
	type Embed1 struct {
		Shared string
	}
	type Embed2 struct {
		Shared string
	}
	type Ambiguous struct {
		Embed1
		Embed2
	}
	// With reflect.VisibleFields, ambiguous promoted fields at the same
	// depth are NOT returned as leaf fields — only the anonymous (embedded)
	// struct fields themselves are listed, and those are filtered out by
	// the sf.Anonymous check in udtFields. This matches FieldByName
	// behavior (which returns ok=false for ambiguous fields).
	// We verify the function doesn't panic and returns a non-nil map.
	fields := udtFields(reflect.TypeOf(Ambiguous{}))
	if fields == nil {
		t.Fatal("udtFields returned nil for ambiguous promoted fields")
	}
}

// =======================================================================
// marshalUDT / unmarshalUDT — embedded struct round-trip
// =======================================================================

func TestMarshalUnmarshalUDTEmbeddedStruct(t *testing.T) {
	type Base struct {
		Name string `cql:"name"`
	}
	type Derived struct {
		Base
		Age int32 `cql:"age"`
	}

	udt := UDTTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeUDT),
		KeySpace:   "test",
		Name:       "person",
		Elements: []UDTField{
			{Name: "name", Type: NewNativeType(protoVersion4, TypeText)},
			{Name: "age", Type: NewNativeType(protoVersion4, TypeInt)},
		},
	}

	input := Derived{
		Base: Base{Name: "Alice"},
		Age:  30,
	}

	data, err := Marshal(udt, &input)
	if err != nil {
		t.Fatalf("Marshal embedded UDT: %v", err)
	}

	var output Derived
	if err := Unmarshal(udt, data, &output); err != nil {
		t.Fatalf("Unmarshal embedded UDT: %v", err)
	}

	if output.Name != "Alice" {
		t.Errorf("Name = %q, want \"Alice\"", output.Name)
	}
	if output.Age != 30 {
		t.Errorf("Age = %d, want 30", output.Age)
	}
}

func TestUnmarshalUDTStructMissingField(t *testing.T) {
	// UDT has a field that doesn't exist in the struct — should be skipped
	type Partial struct {
		Name string `cql:"name"`
	}

	udt := UDTTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeUDT),
		KeySpace:   "test",
		Name:       "person",
		Elements: []UDTField{
			{Name: "name", Type: NewNativeType(protoVersion4, TypeText)},
			{Name: "age", Type: NewNativeType(protoVersion4, TypeInt)},
		},
	}

	input := struct {
		Name string `cql:"name"`
		Age  int32  `cql:"age"`
	}{Name: "Bob", Age: 25}

	data, err := Marshal(udt, &input)
	if err != nil {
		t.Fatal(err)
	}

	var output Partial
	if err := Unmarshal(udt, data, &output); err != nil {
		t.Fatalf("Unmarshal partial UDT: %v", err)
	}
	if output.Name != "Bob" {
		t.Errorf("Name = %q, want \"Bob\"", output.Name)
	}
}

// =======================================================================
// splitCompositeTypes / unwrapCompositeTypeDefinition — correctness
// =======================================================================

func TestUnwrapCompositeTypeDefinition(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		typeName string
		typeOpen int32
		want     string
	}{
		{
			"SetType",
			"org.apache.cassandra.db.marshal.SetType(org.apache.cassandra.db.marshal.UTF8Type)",
			"org.apache.cassandra.db.marshal.SetType",
			'(',
			"org.apache.cassandra.db.marshal.UTF8Type",
		},
		{
			"ListType",
			"org.apache.cassandra.db.marshal.ListType(org.apache.cassandra.db.marshal.Int32Type)",
			"org.apache.cassandra.db.marshal.ListType",
			'(',
			"org.apache.cassandra.db.marshal.Int32Type",
		},
		{
			"ShortInput",
			"X(",
			"LongTypeName",
			'(',
			"X",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := unwrapCompositeTypeDefinition(tc.input, tc.typeName, tc.typeOpen)
			if got != tc.want {
				t.Errorf("unwrapCompositeTypeDefinition() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestSplitCompositeTypes(t *testing.T) {
	const prefix = "org.apache.cassandra.db.marshal."

	tests := []struct {
		name     string
		input    string
		typeName string
		want     []string
	}{
		{
			"simple_two_types",
			prefix + "MapType(" + prefix + "UTF8Type," + prefix + "Int32Type)",
			prefix + "MapType",
			[]string{prefix + "UTF8Type", prefix + "Int32Type"},
		},
		{
			"nested_map",
			prefix + "MapType(" + prefix + "UTF8Type," + prefix + "MapType(" + prefix + "UTF8Type," + prefix + "Int32Type))",
			prefix + "MapType",
			[]string{prefix + "UTF8Type", prefix + "MapType(" + prefix + "UTF8Type," + prefix + "Int32Type)"},
		},
		{
			"single_type",
			prefix + "SetType(" + prefix + "UTF8Type)",
			prefix + "SetType",
			[]string{prefix + "UTF8Type"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := splitCompositeTypes(tc.input, tc.typeName, '(', ')')
			if len(got) != len(tc.want) {
				t.Fatalf("splitCompositeTypes() returned %d parts, want %d: %v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("part[%d] = %q, want %q", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// =======================================================================
// TupleColumnName — correctness
// =======================================================================

func TestTupleColumnName(t *testing.T) {
	tests := []struct {
		col  string
		idx  int
		want string
	}{
		{"col", 0, "col[0]"},
		{"col", 1, "col[1]"},
		{"col", 99, "col[99]"},
		{"", 0, "[0]"},
	}

	for _, tc := range tests {
		got := TupleColumnName(tc.col, tc.idx)
		if got != tc.want {
			t.Errorf("TupleColumnName(%q, %d) = %q, want %q", tc.col, tc.idx, got, tc.want)
		}
	}
}

// =======================================================================
// TypeInfo String() methods — correctness
// =======================================================================

func TestNativeTypeString(t *testing.T) {
	info := NewNativeType(protoVersion4, TypeInt)
	if s := info.String(); s != "int" {
		t.Errorf("NativeType(TypeInt).String() = %q, want \"int\"", s)
	}

	custom := NewCustomType(protoVersion4, TypeCustom, "my.custom.Type")
	if s := custom.String(); s != "custom(my.custom.Type)" {
		t.Errorf("NativeType(TypeCustom).String() = %q, want \"custom(my.custom.Type)\"", s)
	}
}

func TestCollectionTypeString(t *testing.T) {
	mapType := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeMap),
		Key:        NewNativeType(protoVersion4, TypeText),
		Elem:       NewNativeType(protoVersion4, TypeInt),
	}
	if s := mapType.String(); s != "map(text, int)" {
		t.Errorf("CollectionType(Map).String() = %q, want \"map(text, int)\"", s)
	}

	listType := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeList),
		Elem:       NewNativeType(protoVersion4, TypeInt),
	}
	if s := listType.String(); s != "list(int)" {
		t.Errorf("CollectionType(List).String() = %q, want \"list(int)\"", s)
	}

	setType := CollectionType{
		NativeType: NewNativeType(protoVersion4, TypeSet),
		Elem:       NewNativeType(protoVersion4, TypeInt),
	}
	if s := setType.String(); s != "set(int)" {
		t.Errorf("CollectionType(Set).String() = %q, want \"set(int)\"", s)
	}
}

func TestTupleTypeInfoString(t *testing.T) {
	tuple := TupleTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeTuple),
		Elems: []TypeInfo{
			NewNativeType(protoVersion4, TypeInt),
			NewNativeType(protoVersion4, TypeText),
		},
	}
	if s := tuple.String(); s != "tuple(int, text)" {
		t.Errorf("TupleTypeInfo.String() = %q, want \"tuple(int, text)\"", s)
	}

	// Empty tuple
	emptyTuple := TupleTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeTuple),
		Elems:      nil,
	}
	if s := emptyTuple.String(); s != "tuple()" {
		t.Errorf("empty TupleTypeInfo.String() = %q, want \"tuple()\"", s)
	}
}

func TestUDTTypeInfoString(t *testing.T) {
	udt := UDTTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeUDT),
		KeySpace:   "ks",
		Name:       "my_type",
		Elements: []UDTField{
			{Name: "a", Type: NewNativeType(protoVersion4, TypeInt)},
			{Name: "b", Type: NewNativeType(protoVersion4, TypeText)},
		},
	}
	if s := udt.String(); s != "ks.my_type{a=int,b=text}" {
		t.Errorf("UDTTypeInfo.String() = %q, want \"ks.my_type{a=int,b=text}\"", s)
	}

	// Empty UDT
	emptyUdt := UDTTypeInfo{
		NativeType: NewNativeType(protoVersion4, TypeUDT),
		KeySpace:   "ks",
		Name:       "empty",
	}
	if s := emptyUdt.String(); s != "ks.empty{}" {
		t.Errorf("empty UDTTypeInfo.String() = %q, want \"ks.empty{}\"", s)
	}
}
