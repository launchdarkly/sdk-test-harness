package opt

import (
	"encoding/json"
	"fmt"
)

// Maybe is a simple implementation of an optional value type.
type Maybe[V any] struct {
	defined bool
	value   V
}

// Some returns a Maybe that has a defined value.
func Some[V any](value V) Maybe[V] {
	return Maybe[V]{defined: true, value: value}
}

// None returns a Maybe with no value.
func None[V any]() Maybe[V] { return Maybe[V]{} }

// FromPtr returns a Maybe that has a defined value of *ptr if ptr is non-nil, or
// no value if ptr is nil.
func FromPtr[V any](ptr *V) Maybe[V] {
	if ptr != nil {
		return Some[V](*ptr)
	}
	return None[V]()
}

// IsDefined returns true if the Maybe has a value.
func (m Maybe[V]) IsDefined() bool { return m.defined }

// Value returns the value if a value is defined, or the zero value for the type otherwise.
func (m Maybe[V]) Value() V { return m.value }

// AsPtr returns a pointer to the value if the value is defined, or nil otherwise.
func (m Maybe[V]) AsPtr() *V {
	if m.defined {
		return &m.value
	}
	return nil
}

// OrElse returns the value of the Maybe if any, or the valueIfUndefined otherwise.
func (m Maybe[V]) OrElse(valueIfUndefined V) V {
	if m.defined {
		return m.value
	}
	return valueIfUndefined
}

// String returns a string representation of the value, or "[none]" if undefined. The string
// representation of a value is either its own String() if it has such a method, or else the
// result of fmt.Sprintf with "%v".
func (m Maybe[V]) String() string {
	if m.defined {
		var v interface{}
		v = m.value
		if s, ok := v.(fmt.Stringer); ok {
			return s.String()
		}
		return fmt.Sprintf("%v", m.value)
	}
	return "[none]"
}

// MarshalJSON produces whatever JSON representation would normally be produced for the value if
// a value is defined, or a JSON null otherwise.
func (m Maybe[V]) MarshalJSON() ([]byte, error) {
	if m.defined {
		return json.Marshal(m.value)
	}
	return []byte("null"), nil
}

// UnmarshalJSON sets the Maybe to None[V] if the data is a JSON null, or else unmarshals a value
// of type V as usual and sets the Maybe to Some(value).
func (m *Maybe[V]) UnmarshalJSON(data []byte) error {
	var temp interface{}
	if err := json.Unmarshal(data, &temp); err != nil {
		return err
	}
	if temp == nil {
		*m = None[V]()
		return nil
	}
	var value V
	if err := json.Unmarshal(data, &value); err != nil {
		return err
	}
	*m = Some(value)
	return nil
}
