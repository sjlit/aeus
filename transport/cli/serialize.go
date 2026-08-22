package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/mattn/go-runewidth"
	"github.com/sjlit/aeus/pkg/bytepool"
)

func isNormalKind(kind reflect.Kind) bool {
	return slices.Contains([]reflect.Kind{
		reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64, reflect.Int,
		reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uint,
		reflect.Float32, reflect.Float64,
		reflect.String,
	}, kind)
}

func serializeMap(val map[any]any) ([]byte, error) {
	var (
		canFormat bool
		width     int
		maxWidth  int
	)
	canFormat = true
	for k, v := range val {
		if !isNormalKind(reflect.Indirect(reflect.ValueOf(k)).Kind()) || !isNormalKind(reflect.Indirect(reflect.ValueOf(v)).Kind()) {
			canFormat = false
			break
		}
	}
	if !canFormat {
		return json.MarshalIndent(val, "", "\t")
	}
	ms := make(map[string]string)
	for k, v := range val {
		sk := fmt.Sprint(k)
		ms[sk] = fmt.Sprint(v)
		width = runewidth.StringWidth(sk)
		if width > maxWidth {
			maxWidth = width
		}
	}
	buffer := bytepool.GetBuffer()
	defer bytepool.PutBuffer(buffer)
	for k, v := range ms {
		fmt.Fprintf(buffer, "%-"+strconv.Itoa(maxWidth+4)+"s %s\n", k, v)
	}
	return pooledBytes(buffer), nil
}

// pooledBytes returns a copy of b's contents that survives b being returned
// to the bytepool — the pool retains the backing array, so buffer.Bytes()
// would alias it and be overwritten by the next caller.
func pooledBytes(b *bytes.Buffer) []byte {
	return bytes.Clone(b.Bytes())
}

func printBorder(w *bytes.Buffer, ws []int) {
	for _, l := range ws {
		w.WriteString("+")
		w.WriteString(strings.Repeat("-", l+2))
	}
	w.WriteString("+\n")
}

func toString(v any) string {
	switch t := v.(type) {
	case float32, float64:
		return fmt.Sprintf("%.2f", t)
	case time.Time:
		return t.Format("2006-01-02 15:04:05")
	default:
		return fmt.Sprint(v)
	}
}

func printArray(vals [][]any) (buf []byte) {
	var (
		cell      string
		str       string
		widths    []int
		maxLength int
		width     int
		rows      [][]string
	)
	rows = make([][]string, 0, len(vals))
	for _, value := range vals {
		if len(value) > maxLength {
			maxLength = len(value)
		}
	}
	widths = make([]int, maxLength)
	for _, vs := range vals {
		rl := len(vs)
		row := make([]string, rl)
		for i, val := range vs {
			str = toString(val)
			if rl > 1 {
				width = runewidth.StringWidth(str)
				if width > widths[i] {
					widths[i] = width
				}
			}
			row[i] = str
		}
		rows = append(rows, row)
	}
	buffer := bytepool.GetBuffer()
	defer bytepool.PutBuffer(buffer)
	printBorder(buffer, widths)
	for index, row := range rows {
		size := len(row)
		for i, w := range widths {
			cell = ""
			buffer.WriteString("|")
			if size > i {
				cell = row[i]
			}
			buffer.WriteString(" ")
			buffer.WriteString(cell)
			cl := runewidth.StringWidth(cell)
			if w > cl {
				buffer.WriteString(strings.Repeat(" ", w-cl))
			}
			buffer.WriteString(" ")
		}
		buffer.WriteString("|\n")
		if index == 0 {
			printBorder(buffer, widths)
		}
	}
	printBorder(buffer, widths)
	return pooledBytes(buffer)
}

func serializeArray(val []any) (buf []byte, err error) {
	if !canTabulate(val) {
		return json.MarshalIndent(val, "", "\t")
	}
	rows, ok := buildRows(val)
	if !ok {
		return json.MarshalIndent(val, "", "\t")
	}
	return printArray(rows), nil
}

// canTabulate reports whether any element of val can be printed in
// tabular form. Empty input is considered tabular (an empty table).
func canTabulate(val []any) bool {
	for _, row := range val {
		kind := reflect.Indirect(reflect.ValueOf(row)).Kind()
		if !isNormalKind(kind) {
			return false
		}
	}
	return true
}

// buildRows converts val into a 2-D any slice suitable for printArray.
// The first row is the header (column names derived from struct tags or
// field names). Subsequent rows hold the field values in column order.
//
// Returns ok=false if the input mixes kinds or contains fields that
// cannot be formatted; callers should fall back to JSON.
func buildRows(val []any) (rows [][]any, ok bool) {
	if len(val) == 0 {
		return [][]any{}, true
	}
	// All elements must be the same kind. We support []any-of-slice and
	// []any-of-struct today; anything else falls back to JSON.
	first := reflect.Indirect(reflect.ValueOf(val[0])).Kind()
	switch first {
	case reflect.Array, reflect.Slice:
		return buildRowsFromSlices(val)
	case reflect.Struct:
		return buildRowsFromStructs(val)
	default:
		return nil, false
	}
}

// buildRowsFromSlices converts a []any of slice/array elements into row
// data for printArray. Returns ok=false on the first non-slice element or
// non-formatable field; callers should fall back to JSON.
func buildRowsFromSlices(val []any) (rows [][]any, ok bool) {
	rows = make([][]any, 0, len(val))
	for _, v := range val {
		rv := reflect.Indirect(reflect.ValueOf(v))
		if rv.Kind() != reflect.Array && rv.Kind() != reflect.Slice {
			return nil, false
		}
		row := make([]any, 0, rv.Len())
		for i := 0; i < rv.Len(); i++ {
			elem := rv.Index(i)
			if !isNormalKind(elem.Kind()) && elem.Interface() != nil {
				return nil, false
			}
			row = append(row, elem.Interface())
		}
		rows = append(rows, row)
	}
	return rows, true
}

// buildRowsFromStructs converts a []any of struct elements into row data
// for printArray. The header row is built from struct tags (key "kos",
// "-" skips the field) and falls back to upper-cased field names. Returns
// ok=false on the first non-struct element.
func buildRowsFromStructs(val []any) (rows [][]any, ok bool) {
	rows = make([][]any, 0, len(val)+1)
	var columnIndexes []int
	for i, v := range val {
		rv := reflect.Indirect(reflect.ValueOf(v))
		if rv.Kind() != reflect.Struct {
			return nil, false
		}
		if i == 0 {
			header := make([]any, 0, rv.Type().NumField())
			for j := 0; j < rv.Type().NumField(); j++ {
				fieldType := rv.Type().Field(j)
				if !fieldType.IsExported() {
					continue
				}
				name, hasTag := fieldType.Tag.Lookup("kos")
				if hasTag {
					if name == "-" {
						continue
					}
				} else {
					name = strings.ToUpper(fieldType.Name)
				}
				columnIndexes = append(columnIndexes, j)
				header = append(header, name)
			}
			rows = append(rows, header)
		}
		row := make([]any, 0, rv.Type().NumField())
		for j := 0; j < rv.Type().NumField(); j++ {
			if slices.Index(columnIndexes, j) > -1 {
				row = append(row, rv.Field(j).Interface())
			}
		}
		rows = append(rows, row)
	}
	return rows, true
}

func serialize(val any) (buf []byte, err error) {
	var (
		refVal reflect.Value
	)
	refVal = reflect.Indirect(reflect.ValueOf(val))
	switch refVal.Kind() {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		buf = []byte(strconv.FormatInt(refVal.Int(), 10))
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		buf = []byte(strconv.FormatUint(refVal.Uint(), 10))
	case reflect.Float32, reflect.Float64:
		buf = []byte(strconv.FormatFloat(refVal.Float(), 'f', -1, 64))
	case reflect.String:
		buf = []byte(refVal.String())
	case reflect.Slice, reflect.Array:
		if refVal.Type().Elem().Kind() == reflect.Uint8 {
			buf = refVal.Bytes()
		} else {
			as := make([]any, 0, refVal.Len())
			for i := 0; i < refVal.Len(); i++ {
				as = append(as, refVal.Index(i).Interface())
			}
			buf, err = serializeArray(as)
		}
	case reflect.Map:
		ms := make(map[any]any)
		keys := refVal.MapKeys()
		for _, key := range keys {
			ms[key.Interface()] = refVal.MapIndex(key).Interface()
		}
		buf, err = serializeMap(ms)
	default:
		buf, err = json.MarshalIndent(refVal.Interface(), "", "\t")
	}
	return
}
