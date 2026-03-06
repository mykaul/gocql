/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements.  See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership.  The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License.  You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
/*
 * Content before git sha 34fdeebefcbf183ed7f916f931aa0586fdaa1b40
 * Copyright (c) 2012, The Gocql authors,
 * provided under the BSD-3-Clause License.
 * See the NOTICE file distributed with this work for additional information.
 */

package gocql

import (
	"encoding/hex"
	"fmt"
	"math/big"
	"reflect"
	"strconv"
	"strings"
	"time"

	"gopkg.in/inf.v0"
)

type RowData struct {
	Columns []string
	Values  []interface{}
}

// asVectorType attempts to convert a NativeType(custom) which represents a VectorType
// into a concrete VectorType. It also works recursively (nested vectors).
func asVectorType(t TypeInfo) (VectorType, bool) {
	if v, ok := t.(VectorType); ok {
		return v, true
	}
	n, ok := t.(NativeType)
	if !ok || n.Type() != TypeCustom {
		return VectorType{}, false
	}
	const vectorTypePrefix = apacheCassandraTypePrefix + "VectorType"
	spec, ok2 := strings.CutPrefix(n.Custom(), vectorTypePrefix)
	if !ok2 || !strings.HasPrefix(spec, "(") {
		return VectorType{}, false
	}

	spec = strings.Trim(spec, "()")
	// split last comma -> subtype spec , dimensions
	idx := strings.LastIndex(spec, ",")
	if idx <= 0 {
		return VectorType{}, false
	}
	subStr := strings.TrimSpace(spec[:idx])
	dimStr := strings.TrimSpace(spec[idx+1:])
	dim, err := strconv.Atoi(dimStr)
	if err != nil {
		return VectorType{}, false
	}
	subType := getCassandraLongType(subStr, n.Version(), nopLogger{})
	// recurse if subtype itself is still a custom vector
	if innerVec, ok := asVectorType(subType); ok {
		subType = innerVec
	}
	return VectorType{
		NativeType: NewCustomType(n.Version(), TypeCustom, vectorTypePrefix),
		SubType:    subType,
		Dimensions: dim,
	}, true
}

// Cached reflect.Type values for CQL types.
// These are computed once at package initialization to avoid repeated
// reflect.TypeOf calls on every goType() invocation.
var (
	goTypeString      = reflect.TypeOf("")
	goTypeInt64       = reflect.TypeOf(int64(0))
	goTypeDuration    = reflect.TypeOf(time.Duration(0))
	goTypeTime        = reflect.TypeOf(time.Time{})
	goTypeByteSlice   = reflect.TypeOf([]byte(nil))
	goTypeBool        = reflect.TypeOf(false)
	goTypeFloat32     = reflect.TypeOf(float32(0))
	goTypeFloat64     = reflect.TypeOf(float64(0))
	goTypeInt         = reflect.TypeOf(int(0))
	goTypeInt16       = reflect.TypeOf(int16(0))
	goTypeInt8        = reflect.TypeOf(int8(0))
	goTypeInfDec      = reflect.TypeOf((*inf.Dec)(nil))
	goTypeUUID        = reflect.TypeOf(UUID{})
	goTypeBigInt      = reflect.TypeOf((*big.Int)(nil))
	goTypeIfaceSlice  = reflect.TypeOf([]interface{}(nil))
	goTypeIfaceMap    = reflect.TypeOf(map[string]interface{}(nil))
	goTypeCqlDuration = reflect.TypeOf(Duration{})
)

func goType(t TypeInfo) (reflect.Type, error) {
	switch t.Type() {
	case TypeVarchar, TypeAscii, TypeInet, TypeText:
		return goTypeString, nil
	case TypeBigInt, TypeCounter:
		return goTypeInt64, nil
	case TypeTime:
		return goTypeDuration, nil
	case TypeTimestamp:
		return goTypeTime, nil
	case TypeBlob:
		return goTypeByteSlice, nil
	case TypeBoolean:
		return goTypeBool, nil
	case TypeFloat:
		return goTypeFloat32, nil
	case TypeDouble:
		return goTypeFloat64, nil
	case TypeInt:
		return goTypeInt, nil
	case TypeSmallInt:
		return goTypeInt16, nil
	case TypeTinyInt:
		return goTypeInt8, nil
	case TypeDecimal:
		return goTypeInfDec, nil
	case TypeUUID, TypeTimeUUID:
		return goTypeUUID, nil
	case TypeList, TypeSet:
		elemType, err := goType(t.(CollectionType).Elem)
		if err != nil {
			return nil, err
		}
		return reflect.SliceOf(elemType), nil
	case TypeMap:
		keyType, err := goType(t.(CollectionType).Key)
		if err != nil {
			return nil, err
		}
		valueType, err := goType(t.(CollectionType).Elem)
		if err != nil {
			return nil, err
		}
		return reflect.MapOf(keyType, valueType), nil
	case TypeVarint:
		return goTypeBigInt, nil
	case TypeTuple:
		return goTypeIfaceSlice, nil
	case TypeUDT:
		return goTypeIfaceMap, nil
	case TypeDate:
		return goTypeTime, nil
	case TypeDuration:
		return goTypeCqlDuration, nil
	case TypeCustom:
		// Handle VectorType encoded as custom
		if vec, ok := asVectorType(t); ok {
			innerPtr, err := vec.SubType.NewWithError()
			if err != nil {
				return nil, err
			}
			elemType := reflect.TypeOf(innerPtr)
			if elemType.Kind() == reflect.Ptr {
				elemType = elemType.Elem()
			}
			return reflect.SliceOf(elemType), nil
		}
		return nil, fmt.Errorf("cannot create Go type for unknown CQL type %s", t)
	default:
		return nil, fmt.Errorf("cannot create Go type for unknown CQL type %s", t)
	}
}

func dereference(i interface{}) interface{} {
	// Fast path: avoid reflect for the common pointer types returned by
	// NativeType.NewWithError and used in RowData/MapScan.
	switch v := i.(type) {
	case *string:
		return *v
	case *int:
		return *v
	case *int64:
		return *v
	case *int32:
		return *v
	case *int16:
		return *v
	case *int8:
		return *v
	case *float64:
		return *v
	case *float32:
		return *v
	case *bool:
		return *v
	case *[]byte:
		return *v
	case *time.Time:
		return *v
	case *time.Duration:
		return *v
	case *UUID:
		return *v
	case *Duration:
		return *v
	case **inf.Dec:
		return *v
	case **big.Int:
		return *v
	case *[]interface{}:
		return *v
	case *map[string]interface{}:
		return *v
	default:
		return reflect.Indirect(reflect.ValueOf(i)).Interface()
	}
}

// TODO: Cover with unit tests.
// Parses long Java-style type definition to internal data structures.
func getCassandraLongType(name string, protoVer byte, logger StdLogger) TypeInfo {
	const prefix = apacheCassandraTypePrefix
	if strings.HasPrefix(name, prefix+"SetType") {
		return CollectionType{
			NativeType: NewNativeType(protoVer, TypeSet),
			Elem:       getCassandraLongType(unwrapCompositeTypeDefinition(name, prefix+"SetType", '('), protoVer, logger),
		}
	} else if strings.HasPrefix(name, prefix+"ListType") {
		return CollectionType{
			NativeType: NewNativeType(protoVer, TypeList),
			Elem:       getCassandraLongType(unwrapCompositeTypeDefinition(name, prefix+"ListType", '('), protoVer, logger),
		}
	} else if strings.HasPrefix(name, prefix+"MapType") {
		names := splitJavaCompositeTypes(name, prefix+"MapType")
		if len(names) != 2 {
			logger.Printf("gocql: error parsing map type, it has %d subelements, expecting 2\n", len(names))
			return NewNativeType(protoVer, TypeCustom)
		}
		return CollectionType{
			NativeType: NewNativeType(protoVer, TypeMap),
			Key:        getCassandraLongType(names[0], protoVer, logger),
			Elem:       getCassandraLongType(names[1], protoVer, logger),
		}
	} else if strings.HasPrefix(name, prefix+"TupleType") {
		names := splitJavaCompositeTypes(name, prefix+"TupleType")
		types := make([]TypeInfo, len(names))

		for i, name := range names {
			types[i] = getCassandraLongType(name, protoVer, logger)
		}

		return TupleTypeInfo{
			NativeType: NewNativeType(protoVer, TypeTuple),
			Elems:      types,
		}
	} else if strings.HasPrefix(name, prefix+"UserType") {
		names := splitJavaCompositeTypes(name, prefix+"UserType")
		fields := make([]UDTField, len(names)-2)

		for i := 2; i < len(names); i++ {
			spec := strings.Split(names[i], ":")
			fieldName, _ := hex.DecodeString(spec[0])
			fields[i-2] = UDTField{
				Name: string(fieldName),
				Type: getCassandraLongType(spec[1], protoVer, logger),
			}
		}

		udtName, _ := hex.DecodeString(names[1])
		return UDTTypeInfo{
			NativeType: NewNativeType(protoVer, TypeUDT),
			KeySpace:   names[0],
			Name:       string(udtName),
			Elements:   fields,
		}
	} else if strings.HasPrefix(name, prefix+"VectorType") {
		names := splitJavaCompositeTypes(name, prefix+"VectorType")
		subType := getCassandraLongType(strings.TrimSpace(names[0]), protoVer, logger)
		dim, err := strconv.Atoi(strings.TrimSpace(names[1]))
		if err != nil {
			logger.Printf("gocql: error parsing vector dimensions: %v\n", err)
			return NewNativeType(protoVer, TypeCustom)
		}

		return VectorType{
			NativeType: NewCustomType(protoVer, TypeCustom, prefix+"VectorType"),
			SubType:    subType,
			Dimensions: dim,
		}
	} else if strings.HasPrefix(name, prefix+"FrozenType") {
		names := splitJavaCompositeTypes(name, prefix+"FrozenType")
		return getCassandraLongType(strings.TrimSpace(names[0]), protoVer, logger)
	} else {
		// basic type
		return NativeType{
			proto: protoVer,
			typ:   getApacheCassandraType(name),
		}
	}
}

func splitJavaCompositeTypes(name string, typeName string) []string {
	return splitCompositeTypes(name, typeName, '(', ')')
}

func unwrapCompositeTypeDefinition(name string, typeName string, typeOpen int32) string {
	// Strip trailing close delimiter and the "typeName(" prefix.
	// Note: typeOpen is assumed to be a single-byte ASCII character (e.g. '('),
	// which is true for all current callers. Multi-byte runes would require
	// utf8.RuneLen(typeOpen) instead of the +1 below.
	inner := name[:len(name)-1]
	prefixLen := len(typeName) + 1 // +1 for typeOpen (single-byte ASCII)
	if len(inner) >= prefixLen {
		inner = inner[prefixLen:]
	}
	return inner
}

func splitCompositeTypes(name string, typeName string, typeOpen int32, typeClose int32) []string {
	def := unwrapCompositeTypeDefinition(name, typeName, typeOpen)
	if !strings.ContainsRune(def, typeOpen) {
		parts := strings.Split(def, ",")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		return parts
	}
	var parts []string
	lessCount := 0
	start := 0
	for i, char := range def {
		if char == ',' && lessCount == 0 {
			if i > start {
				parts = append(parts, strings.TrimSpace(def[start:i]))
			}
			start = i + 1 // skip the comma (1 byte)
			continue
		}
		if char == typeOpen {
			lessCount++
		} else if char == typeClose {
			lessCount--
		}
	}
	if start < len(def) {
		parts = append(parts, strings.TrimSpace(def[start:]))
	}
	return parts
}

func getApacheCassandraType(class string) Type {
	switch strings.TrimPrefix(class, apacheCassandraTypePrefix) {
	case "AsciiType":
		return TypeAscii
	case "LongType":
		return TypeBigInt
	case "BytesType":
		return TypeBlob
	case "BooleanType":
		return TypeBoolean
	case "CounterColumnType":
		return TypeCounter
	case "DecimalType":
		return TypeDecimal
	case "DoubleType":
		return TypeDouble
	case "FloatType":
		return TypeFloat
	case "Int32Type":
		return TypeInt
	case "ShortType":
		return TypeSmallInt
	case "ByteType":
		return TypeTinyInt
	case "TimeType":
		return TypeTime
	case "DateType", "TimestampType":
		return TypeTimestamp
	case "UUIDType", "LexicalUUIDType":
		return TypeUUID
	case "UTF8Type":
		return TypeVarchar
	case "IntegerType":
		return TypeVarint
	case "TimeUUIDType":
		return TypeTimeUUID
	case "InetAddressType":
		return TypeInet
	case "MapType":
		return TypeMap
	case "ListType":
		return TypeList
	case "SetType":
		return TypeSet
	case "TupleType":
		return TypeTuple
	case "DurationType":
		return TypeDuration
	case "SimpleDateType":
		return TypeDate
	case "UserType":
		return TypeUDT
	default:
		return TypeCustom
	}
}

func (r *RowData) rowMap(m map[string]interface{}) {
	for i, column := range r.Columns {
		val := dereference(r.Values[i])
		if valVal := reflect.ValueOf(val); valVal.Kind() == reflect.Slice && !valVal.IsNil() {
			valCopy := reflect.MakeSlice(valVal.Type(), valVal.Len(), valVal.Cap())
			reflect.Copy(valCopy, valVal)
			m[column] = valCopy.Interface()
		} else {
			m[column] = val
		}
	}
}

// TupeColumnName will return the column name of a tuple value in a column named
// c at index n. It should be used if a specific element within a tuple is needed
// to be extracted from a map returned from SliceMap or MapScan.
func TupleColumnName(c string, n int) string {
	return c + "[" + strconv.Itoa(n) + "]"
}

// RowData returns the RowData for the iterator.
func (iter *Iter) RowData() (RowData, error) {
	if iter.err != nil {
		return RowData{}, iter.err
	}

	columns, err := iter.getScanColumns()
	if err != nil {
		return RowData{}, err
	}

	values, err := iter.newScanValues()
	if err != nil {
		return RowData{}, err
	}

	return RowData{
		Columns: columns,
		Values:  values,
	}, nil
}

// getScanColumns returns the cached column names for this iterator,
// computing them on the first call. Column names don't change between
// rows, so they are computed once and reused.
//
// The returned slice is shared across all callers and must not be mutated.
func (iter *Iter) getScanColumns() ([]string, error) {
	if iter.scanColumns != nil {
		return iter.scanColumns, nil
	}

	columns := make([]string, 0, len(iter.Columns()))
	for _, column := range iter.Columns() {
		if c, ok := column.TypeInfo.(TupleTypeInfo); !ok {
			columns = append(columns, column.Name)
		} else {
			for i := range c.Elems {
				columns = append(columns, TupleColumnName(column.Name, i))
			}
		}
	}

	iter.scanColumns = columns
	return columns, nil
}

// newScanValues allocates fresh zero-value pointers for each column,
// suitable for passing to Scan.
func (iter *Iter) newScanValues() ([]interface{}, error) {
	values := make([]interface{}, 0, len(iter.Columns()))
	for _, column := range iter.Columns() {
		if c, ok := column.TypeInfo.(TupleTypeInfo); !ok {
			val, err := column.TypeInfo.NewWithError()
			if err != nil {
				iter.err = err
				return nil, err
			}
			values = append(values, val)
		} else {
			for _, elem := range c.Elems {
				val, err := elem.NewWithError()
				if err != nil {
					iter.err = err
					return nil, err
				}
				values = append(values, val)
			}
		}
	}
	return values, nil
}

// TODO(zariel): is it worth exporting this?
func (iter *Iter) rowMap() (map[string]interface{}, error) {
	if iter.err != nil {
		return nil, iter.err
	}

	rowData, err := iter.RowData()
	if err != nil {
		return nil, err
	}
	iter.Scan(rowData.Values...)
	m := make(map[string]interface{}, len(rowData.Columns))
	rowData.rowMap(m)
	return m, nil
}

// SliceMap is a helper function to make the API easier to use
// returns the data from the query in the form of []map[string]interface{}
func (iter *Iter) SliceMap() ([]map[string]interface{}, error) {
	if iter.err != nil {
		return nil, iter.err
	}

	// Not checking for the error because we just did
	rowData, err := iter.RowData()
	if err != nil {
		return nil, err
	}
	dataToReturn := make([]map[string]interface{}, 0)
	for iter.Scan(rowData.Values...) {
		m := make(map[string]interface{}, len(rowData.Columns))
		rowData.rowMap(m)
		dataToReturn = append(dataToReturn, m)
	}
	if iter.err != nil {
		return nil, iter.err
	}
	return dataToReturn, nil
}

// MapScan takes a map[string]interface{} and populates it with a row
// that is returned from cassandra.
//
// Each call to MapScan() must be called with a new map object.
// During the call to MapScan() any pointers in the existing map
// are replaced with non pointer types before the call returns
//
//	iter := session.Query(`SELECT * FROM mytable`).Iter()
//	for {
//		// New map each iteration
//		row := make(map[string]interface{})
//		if !iter.MapScan(row) {
//			break
//		}
//		// Do things with row
//		if fullname, ok := row["fullname"]; ok {
//			fmt.Printf("Full Name: %s\n", fullname)
//		}
//	}
//
// You can also pass pointers in the map before each call
//
//	var fullName FullName // Implements gocql.Unmarshaler and gocql.Marshaler interfaces
//	var address net.IP
//	var age int
//	iter := session.Query(`SELECT * FROM scan_map_table`).Iter()
//	for {
//		// New map each iteration
//		row := map[string]interface{}{
//			"fullname": &fullName,
//			"age":      &age,
//			"address":  &address,
//		}
//		if !iter.MapScan(row) {
//			break
//		}
//		fmt.Printf("First: %s Age: %d Address: %q\n", fullName.FirstName, age, address)
//	}
func (iter *Iter) MapScan(m map[string]interface{}) bool {
	if iter.err != nil {
		return false
	}

	rowData, err := iter.RowData()
	if err != nil {
		return false
	}

	for i, col := range rowData.Columns {
		if dest, ok := m[col]; ok {
			rowData.Values[i] = dest
		}
	}

	if iter.Scan(rowData.Values...) {
		rowData.rowMap(m)
		return true
	}
	return false
}

func copyBytes(p []byte) []byte {
	b := make([]byte, len(p))
	copy(b, p)
	return b
}
