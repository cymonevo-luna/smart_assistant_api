package places

import "errors"

// ErrEmptyAddress is returned when geocoding is called with a blank address.
var ErrEmptyAddress = errors.New("places: address is required")

// ErrAddressNotFound is returned when an address cannot be resolved uniquely.
var ErrAddressNotFound = errors.New("places: address not found or ambiguous")

// Unprocessable reports whether err should map to HTTP 422.
func Unprocessable(err error) bool {
	return errors.Is(err, ErrEmptyAddress) || errors.Is(err, ErrAddressNotFound)
}
