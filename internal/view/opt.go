package view

// Opt is a value that may not be there: a temperature a machine has no sensor
// for, a load before its second sample. It replaces a value-and-Has pair, so a
// reading with five quantities is five fields and not ten (spec 044).
type Opt[T any] struct {
	V  T    `json:"v"`
	OK bool `json:"ok"`
}

// Some is v, present.
func Some[T any](v T) Opt[T] { return Opt[T]{V: v, OK: true} }

// Maybe is v where ok, and nothing otherwise: for a source that reports a
// value and whether it has one.
func Maybe[T any](v T, ok bool) Opt[T] {
	if !ok {
		var zero T
		return Opt[T]{V: zero}
	}
	return Opt[T]{V: v, OK: true}
}
