package memory

import (
	"reflect"
	"sync"
)

// clone returns a deep copy of v when v is a composite type, or v itself
// when v is a primitive (string, []byte as bytes, scalar, interface
// wrapping a primitive). The returned value shares no mutable state with
// v: slices get fresh backing arrays, maps get fresh buckets, pointers
// are followed, and structs are copied field-by-field. nil pointers and
// nil maps / slices stay nil. unexported struct fields are skipped —
// reflect cannot set them anyway, and silently skipping matches Go's
// own assignment semantics.
//
// Cyclic structures are tolerated by tracking visited pointer pairs;
// they clone once and re-use the clone to break the cycle.
//
// The implementation is intentionally minimal: anything reflect cannot
// handle (channels, funcs, unsafe.Pointer) is returned unchanged so a
// caller that stores such a value gets back the same reference — they
// own the semantics for those types.
//
// Clone is safe for concurrent use; the visited-pointer map is per-call
// via the recursionStack argument so two goroutines never share state.
func clone(v reflect.Value) reflect.Value {
	return cloneInto(v, &sync.Map{})
}

func cloneInto(v reflect.Value, seen *sync.Map) reflect.Value {
	if !v.IsValid() {
		return v
	}
	switch v.Kind() {
	case reflect.Ptr:
		if v.IsNil() {
			return v
		}
		// Break cycles: track (original.Pointer, clone) pairs so a
		// back-edge reuses the partial clone instead of recursing.
		if cached, ok := seen.Load(v.Pointer()); ok {
			return cached.(reflect.Value)
		}
		placeholder := reflect.New(v.Elem().Type())
		seen.Store(v.Pointer(), placeholder)
		placeholder.Elem().Set(cloneInto(v.Elem(), seen))
		return placeholder
	case reflect.Interface:
		if v.IsNil() {
			return v
		}
		return cloneInto(v.Elem(), seen)
	case reflect.Slice:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeSlice(v.Type(), v.Len(), v.Cap())
		reflect.Copy(out, v)
		return out
	case reflect.Map:
		if v.IsNil() {
			return v
		}
		out := reflect.MakeMapWithSize(v.Type(), v.Len())
		iter := v.MapRange()
		for iter.Next() {
			out.SetMapIndex(cloneInto(iter.Key(), seen), cloneInto(iter.Value(), seen))
		}
		return out
	case reflect.Array:
		out := reflect.New(v.Type()).Elem()
		for i := 0; i < v.Len(); i++ {
			out.Index(i).Set(cloneInto(v.Index(i), seen))
		}
		return out
	case reflect.Struct:
		out := reflect.New(v.Type()).Elem()
		t := v.Type()
		for i := 0; i < t.NumField(); i++ {
			f := t.Field(i)
			if !f.IsExported() {
				continue
			}
			out.Field(i).Set(cloneInto(v.Field(i), seen))
		}
		return out
	default:
		return v
	}
}
