package translator

// Format identifies a request/response schema used inside the proxy.
type Format = string

// FromString converts an arbitrary identifier to a translator format.
func FromString(v string) Format {
	return v
}
