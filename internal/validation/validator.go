package validation

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"

	ut "github.com/go-playground/universal-translator"
	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
)

type violation struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// PayloadError represents struct with failed checks
type PayloadError struct {
	violations []violation
}

// Error returns error string
func (e *PayloadError) Error() string {
	buff := bytes.NewBufferString("")

	for _, err := range e.violations {
		buff.WriteString(err.Message)
		buff.WriteString("\n")
	}

	return buff.String()
}

// Violation adds new violation
func (e *PayloadError) Violation(v violation) {
	e.violations = append(e.violations, v)
}

// MarshalJSON defines json marshaling
func (e *PayloadError) MarshalJSON() ([]byte, error) {
	return json.Marshal(&struct {
		Errors []violation `json:"errors"`
	}{
		Errors: e.violations,
	})
}

// EchoValidator represents echo error handler
type EchoValidator struct {
	validator  *validator.Validate
	translator ut.Translator
}

// Echo builds validator for echo
func Echo(v *validator.Validate, trans ut.Translator) *EchoValidator {
	return &EchoValidator{
		validator:  v,
		translator: trans,
	}
}

// Validate runs validation against provided struct
func (v *EchoValidator) Validate(i any) error {
	err := v.validator.Struct(i)
	if err == nil {
		return nil
	}

	var ve validator.ValidationErrors
	if errors.As(err, &ve) {
		return v.payloadError(ve)
	}

	return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
}

func (v *EchoValidator) payloadError(ve validator.ValidationErrors) error {
	pldErr := &PayloadError{violations: make([]violation, 0)}
	for _, e := range ve {
		pldErr.Violation(violation{
			Field:   e.Field(),
			Message: e.Translate(v.translator),
		})
	}
	return pldErr
}
