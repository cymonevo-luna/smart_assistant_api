// Package validator wraps go-playground/validator and decodes/validates JSON
// request bodies in one step, returning structured application errors.
package validator

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"

	"github.com/cymonevo/go_template/pkg/response"
	"github.com/go-playground/validator/v10"
)

// Validator validates structs using `validate` struct tags.
type Validator struct {
	v *validator.Validate
}

// New constructs a Validator configured to report JSON field names.
func New() *Validator {
	v := validator.New(validator.WithRequiredStructEnabled())
	v.RegisterTagNameFunc(func(fld reflect.StructField) string {
		name := strings.SplitN(fld.Tag.Get("json"), ",", 2)[0]
		if name == "-" {
			return ""
		}
		return name
	})
	return &Validator{v: v}
}

// Struct validates a struct and returns a *response.Error on failure.
func (val *Validator) Struct(s interface{}) error {
	if err := val.v.Struct(s); err != nil {
		return toValidationError(err)
	}
	return nil
}

// BindJSON decodes the request body into dst and validates it.
func (val *Validator) BindJSON(r *http.Request, dst interface{}) error {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return response.NewBadRequest("invalid JSON body").Wrap(err)
	}
	return val.Struct(dst)
}

func toValidationError(err error) error {
	var ve validator.ValidationErrors
	if !asValidationErrors(err, &ve) {
		return response.NewBadRequest("invalid request").Wrap(err)
	}

	fields := make(map[string]string, len(ve))
	summary := make([]string, 0, len(ve))
	for _, fe := range ve {
		key := fieldKey(fe)
		msg := messageFor(fe)
		fields[key] = msg
		summary = append(summary, key+" "+msg)
	}
	// Fold the field errors into a single self-explanatory message so clients
	// that only surface the top-level message still tell the user what to fix.
	message := "validation failed: " + strings.Join(summary, "; ")
	return response.NewValidation(fields).WithMessage(message)
}

func asValidationErrors(err error, target *validator.ValidationErrors) bool {
	if ve, ok := err.(validator.ValidationErrors); ok {
		*target = ve
		return true
	}
	return false
}

// fieldKey returns the JSON path of the offending field, including indices for
// slices (e.g. "repos[0].git_url"), with the root struct name stripped so the
// key matches the request body shape.
func fieldKey(fe validator.FieldError) string {
	ns := fe.Namespace()
	if i := strings.IndexByte(ns, '.'); i >= 0 {
		return ns[i+1:]
	}
	return fe.Field()
}

func messageFor(fe validator.FieldError) string {
	switch fe.Tag() {
	case "required":
		return "is required"
	case "email":
		return "must be a valid email address"
	case "url":
		return "must be a valid URL"
	case "uuid", "uuid4":
		return "must be a valid UUID"
	case "min":
		return "must be at least " + fe.Param() + " characters"
	case "max":
		return "must be at most " + fe.Param() + " characters"
	case "len":
		return "must be exactly " + fe.Param() + " characters"
	case "oneof":
		return "must be one of: " + fe.Param()
	default:
		return "is invalid (failed " + fe.Tag() + " validation)"
	}
}
